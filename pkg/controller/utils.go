package controller

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	api_v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/retry"
)

var kubeconfig *string

// LoadK8SClientConfigFile configures and initializes the k8s API clientset object.
// If run inside the cluster is uses the pods service account to access the API.
// Otherwise it uses either the configuration of ~/.kube/config or the config
// provided by the 'kubeconfig' flag.
func LoadK8SClientConfigFile() (*kubernetes.Clientset, error) {
	if kubeconfig == nil {
		// Parse "kubeconfig" argument if provided
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}
		flag.Parse()
	}

	// Check & Load config file
	var conf string
	if s, err := os.Stat(*kubeconfig); err == nil && !s.IsDir() {
		slog.Info(fmt.Sprintf("Using %s file to configure k8s API connection", *kubeconfig))
		conf = *kubeconfig
	} else {
		slog.Info(fmt.Sprintf("%s file not found", *kubeconfig))
		conf = ""
	}
	config, err := clientcmd.BuildConfigFromFlags("", conf)
	if err != nil {
		return nil, err
	}

	// Create API client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, err
}

// ToggleDeployment "disables" or "enables" a deployment by changing
// the configured replicas number. The function will retry the change if
// the initial resource update fails.
func ToggleDeployment(clientset kubernetes.Interface, namespace, deployment string, targetState DeploymentState) error {
	deploymentsClient := clientset.AppsV1().Deployments(namespace)
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		deploymentObj, getErr := deploymentsClient.Get(context.Background(), deployment, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("Failed to get latest version of Deployment: %v", getErr)
		}

		// Memorize current replicas number
		if *deploymentObj.Spec.Replicas != 0 {
			deploymentObj.ObjectMeta.Annotations[REPLICAS_MEMORY_ANNOTATION] = strconv.Itoa(int(*deploymentObj.Spec.Replicas))
		}

		// Set the new replicas number
		if targetState == DISABLED {
			if *deploymentObj.Spec.Replicas == 0 {
				return nil
			}
			slog.Info(fmt.Sprintf("Scaling down deployment '%s.%s'\n", namespace, deployment))
			deploymentObj.Spec.Replicas = int32Ptr(0)
		} else {
			if *deploymentObj.Spec.Replicas != 0 {
				return nil
			}
			slog.Info(fmt.Sprintf("Scaling up deployment '%s.%s'\n", namespace, deployment))
			if value, exists := deploymentObj.ObjectMeta.Annotations[REPLICAS_MEMORY_ANNOTATION]; exists {
				i, err := strconv.Atoi(value)
				if err != nil {
					return err
				}
				deploymentObj.Spec.Replicas = int32Ptr(int32(i))
				delete(deploymentObj.ObjectMeta.Annotations, REPLICAS_MEMORY_ANNOTATION)
			}
		}

		// Make the update call to k8s API
		_, updateErr := deploymentsClient.Update(context.Background(), deploymentObj, metav1.UpdateOptions{})
		return updateErr
	})
	if retryErr != nil {
		return fmt.Errorf("Update failed: %v", retryErr)
	}

	return nil
}

// AttemptToggleDeployment "disables" or "enables" a deployment by changing
// the configured replicas number. The function will not retry the change in
// case of a failure during the initial resource update. This function is meant
// to be a bit more efficient than ToggleDeployment but in endge cases it
// might fail to apply the change.
func AttemptToggleDeployment(clientset kubernetes.Interface, deployment *api_v1.Deployment, targetState DeploymentState) error {
	namespace := deployment.Namespace
	deploymentName := deployment.Name

	// Memorize current replicas number
	if *deployment.Spec.Replicas != 0 {
		deployment.ObjectMeta.Annotations[REPLICAS_MEMORY_ANNOTATION] = strconv.Itoa(int(*deployment.Spec.Replicas))
	}

	// Set the new replicas number
	if targetState == DISABLED {
		if *deployment.Spec.Replicas == 0 {
			return nil
		}
		slog.Info(fmt.Sprintf("Scaling down deployment '%s.%s'\n", namespace, deploymentName))
		deployment.Spec.Replicas = int32Ptr(0)
	} else {
		if *deployment.Spec.Replicas != 0 {
			return nil
		}
		slog.Info(fmt.Sprintf("Scaling up deployment '%s.%s'\n", namespace, deploymentName))
		if value, exists := deployment.ObjectMeta.Annotations[REPLICAS_MEMORY_ANNOTATION]; exists {
			i, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			deployment.Spec.Replicas = int32Ptr(int32(i))
			delete(deployment.ObjectMeta.Annotations, REPLICAS_MEMORY_ANNOTATION)
		}
	}

	// Make the update call to k8s API
	_, updateErr := clientset.AppsV1().Deployments(namespace).Update(context.Background(), deployment, metav1.UpdateOptions{})
	return updateErr
}

func int32Ptr(i int32) *int32 { return &i }
