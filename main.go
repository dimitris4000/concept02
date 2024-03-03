package main

import (
	"fmt"
	"time"

	"github.com/dimitris4000/concept02/pkg/controller"
	"github.com/dimitris4000/concept02/pkg/service"
)

var (
	Version = "0.1.0"
)

func main() {
	fmt.Printf("Version: %s\n", Version)
	fmt.Printf("Current Time: %s\n", time.Now())

	// Start the K8S controller of the scheduler
	controllerCh, err := controller.Start()
	if err != nil {
		panic(err)
	}
	defer close(controllerCh)

	// Start the HTTP service of the scheduler
	schedulerConfig := service.NewDefaultSchedulerServiceConfig()
	schedulerConfig.Version = Version
	schedulerConfig.ShutdownWaitDuration = 5 * time.Second
	scheduler := service.NewSchedulerService(schedulerConfig)
	scheduler.RunForever()
}
