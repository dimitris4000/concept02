package controller

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	apps_v1 "k8s.io/api/apps/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/cache"
)

const (
	REPLICAS_MEMORY_ANNOTATION = "scheduler.replicas-memory"
	SCHEDULE_ANNOTATION        = "scheduler.off-schedule"
	ENABLED_ANNOTATION         = "scheduler.enabled"
)

// DeploymentState is used across the controller package to designate whether
// a deployment is, or must be, scalled down or up by the controller.
type DeploymentState bool

const (
	ENABLED  DeploymentState = true
	DISABLED DeploymentState = false
)

const postRestartBackoffPeriod = 7200

// TimeRange represents a time range taking only into account hour and
// minute component of Time value.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// InRangeNow checks if the current time (i.e. time.Now()) is between the
// Sart and End times configured in the TimeRange object. The function
// ignores the Year, Month, Day and Second components of the time values.
// If the Start time is after the End time, the function will assume that
// the range crosses to the midnight time an respond accordingly.
func (t TimeRange) InRangeNow() bool {
	now, _ := time.Parse("15:04", time.Now().Format("15:04"))
	var result bool
	if t.End.Before(t.Start) {
		result = now.After(t.Start) || now.Before(t.End)
	} else {
		result = now.After(t.Start) && now.Before(t.End)
	}
	return result
}

// Controller holds the components of the schedule controller
type Controller struct {
	clientset          kubernetes.Interface
	deploymentInformer cache.SharedIndexInformer
}

// NewResourceController can be used to initialize a Controller object in an
// easy way.
func NewResourceController(client kubernetes.Interface, deploymentInformer cache.SharedIndexInformer) *Controller {
	return &Controller{
		clientset:          client,
		deploymentInformer: deploymentInformer,
	}
}

// Run is the main loop of the controller where the business logic lives.
// This methods is supposed to be run as a goroutine. The loop will keep
// running until the stopCh is closed.
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	slog.Info("Starting scheduler controller")

	go c.deploymentInformer.Run(stopCh)

	// Waiting for client-go to load the cache
	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	slog.Info("Scheduler controller synced and ready")

	// Run the controller's logic every 5sec
	wait.Until(c.loopIteration, 5*time.Second, stopCh)
}

// HasSynced is required for the cache.Controller interface.
func (c *Controller) HasSynced() bool {
	return c.deploymentInformer.HasSynced()
}

// LastSyncResourceVersion is required for the cache.Controller interface.
func (c *Controller) LastSyncResourceVersion() string {
	return c.deploymentInformer.LastSyncResourceVersion()
}

// loopIteration contains the logic of the controller that needs to be run in every
// loop. It is supposed to be called from within the controllers loop only.
func (c *Controller) loopIteration() {
	// Check deployments with scheduler.enabled:"true" annotation
	for _, deploymentName := range c.deploymentInformer.GetIndexer().ListKeys() {
		deployment, exists, err := c.deploymentInformer.GetIndexer().GetByKey(deploymentName)
		if err != nil {
			slog.Error(fmt.Sprintf("Error while checking deployment %s. Moving to the next one", deploymentName))
			continue
		}
		if !exists {
			continue
		}

		// Using the informer's object
		switch object := deployment.(type) {
		case *apps_v1.Deployment:
			// Check deployment's annotation
			annotations := object.GetAnnotations()
			value, exists := annotations[ENABLED_ANNOTATION]
			if !exists || strings.ToLower(value) != "true" {
				continue
			}

			// Check deployment
			slog.Info(fmt.Sprintf("Checking deployment %s", deploymentName))
			schedule, err := c.parseScheduleAnnotation(annotations)
			if err != nil {
				slog.Error(fmt.Sprintf("%s", err))
				continue
			}
			if schedule.InRangeNow() {
				err := ToggleDeployment(c.clientset, object.Namespace, object.Name, DISABLED)
				if err != nil {
					slog.Error(fmt.Sprintf("%s", err))
					continue
				}
			} else {
				err := ToggleDeployment(c.clientset, object.Namespace, object.Name, ENABLED)
				if err != nil {
					slog.Error(fmt.Sprintf("%s", err))
					continue
				}
			}
		}
	}
}

// parseScheduleAnnotation parse annotation that contains the shutdown schedule
func (c *Controller) parseScheduleAnnotation(annotations map[string]string) (TimeRange, error) {
	scheduleText, exists := annotations[SCHEDULE_ANNOTATION]
	if !exists {
		return TimeRange{}, fmt.Errorf("could not find %s annotation", SCHEDULE_ANNOTATION)
	}
	tokens := strings.Split(scheduleText, "-")

	start, err := time.Parse("15:04", strings.Trim(tokens[0], " "))
	if err != nil {
		return TimeRange{}, err
	}

	end, err := time.Parse("15:04", strings.Trim(tokens[1], " "))
	if err != nil {
		return TimeRange{}, err
	}

	return TimeRange{start, end}, nil
}

// Boostraps and start the deployment resource watcher and the controller
// Returns a channel which will close the watcher when closed.
func Start() (chan struct{}, error) {
	kubeClient, err := LoadK8SClientConfigFile()
	if err != nil {
		return nil, err
	}

	// Watch Deployments
	deploymentInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.AppsV1().Deployments("").List(context.Background(), options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.AppsV1().Deployments("").Watch(context.Background(), options)
			},
		},
		&apps_v1.Deployment{},
		5*time.Minute,
		cache.Indexers{},
	)

	c := NewResourceController(
		kubeClient,
		deploymentInformer,
	)

	stopCh := make(chan struct{}) // Closing this will terminate the controller
	go c.Run(stopCh)

	return stopCh, nil
}
