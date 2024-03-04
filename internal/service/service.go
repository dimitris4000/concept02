// service package contains all the logic and mechanics
// that are responsible for the http-server functionality
// of the service.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dimitris4000/concept02/internal/controller"
)

// SchedulerServiceConfig is holding all the configuration
// of the http service of the scheduler
type SchedulerServiceConfig struct {
	Version              string
	ShutdownWaitDuration time.Duration
}

// NewDefaultSchedulerServiceConfig is used to create an initial
// SchedulerServiceConfig instance with sane defaults
func NewDefaultSchedulerServiceConfig() SchedulerServiceConfig {
	return SchedulerServiceConfig{
		Version:              "0.0.0",
		ShutdownWaitDuration: 15 * time.Second,
	}
}

// SchedulerService is the core struct of the http service
// portion of the scheduler service
type SchedulerService struct {
	Http               *http.Server
	Config             SchedulerServiceConfig
	serverReady        bool
	terminationChannel chan os.Signal
}

// NewSchedulerService initializes the http server of the scheduler service
func NewSchedulerService(config SchedulerServiceConfig) *SchedulerService {
	mux := http.NewServeMux()
	newService := &SchedulerService{
		Http: &http.Server{
			Addr:    ":8081", // This can be remapped in k8s resources
			Handler: mux,
		},
		Config:             config,
		serverReady:        true,
		terminationChannel: make(chan os.Signal, 1),
	}
	newService.configureHandlers()

	return newService
}

// configureHandlers functions is meant to contain all the configuration of
// the URL paths of the Scheduler service
func (h *SchedulerService) configureHandlers() {
	mux := h.Http.Handler.(*http.ServeMux)
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, h.Config.Version)
	})

	mux.HandleFunc("/liveness", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	readinessHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if r.URL.Path == "/readiness/ready" {
				h.serverReady = true
			} else if r.URL.Path == "/readiness/notready" {
				h.serverReady = false
			}
		}

		if h.serverReady {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "OK")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "NOT OK")
		}
	}
	mux.HandleFunc("/readiness", readinessHandler)
	mux.HandleFunc("/readiness/", readinessHandler)

	mux.HandleFunc("/scaleDown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotImplemented)
		}

		var d JsonResourceSpecifier
		if r.Body == nil {
			http.Error(w, "Please send a request body", http.StatusBadRequest)
			return
		}
		err := json.NewDecoder(r.Body).Decode(&d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		k8s, err := controller.LoadK8SClientConfigFile()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			slog.Warn(fmt.Sprintf("%s", err))
			return
		}
		err = controller.ToggleDeployment(k8s, d.Namespace, d.Name, controller.DISABLED)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			slog.Warn(fmt.Sprintf("%s", err))
			return
		}

		fmt.Fprintln(w, "Request received")
	})

	mux.HandleFunc("/scaleUp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotImplemented)
		}

		var d JsonResourceSpecifier
		if r.Body == nil {
			http.Error(w, "Please send a request body", http.StatusBadRequest)
			return
		}
		err := json.NewDecoder(r.Body).Decode(&d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		k8s, err := controller.LoadK8SClientConfigFile()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			slog.Warn(fmt.Sprintf("%s", err))
			return
		}
		err = controller.ToggleDeployment(k8s, d.Namespace, d.Name, controller.ENABLED)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			slog.Warn(fmt.Sprintf("%s", err))
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Request received")
	})

}

// RunForever blocking function that is starting the http server and the listening
// process. It is meant to be run only in the main function of the scheduler, for
// other cases feel free to copy the code and adapt to your needs (i.e. Not efficient
// to run as gofunc).
func (h *SchedulerService) RunForever() {
	slog.Info(fmt.Sprintf("SchedulerService is listening on '%s'", h.Http.Addr))
	go func() {
		h.Http.ListenAndServe()
	}()

	//Block until an unterrupt signal is received.
	signal.Notify(h.terminationChannel, syscall.SIGTERM, syscall.SIGINT)
	<-h.terminationChannel

	slog.Info(fmt.Sprintf("Server will shut down in %d seconds...", h.Config.ShutdownWaitDuration/time.Second))
	h.serverReady = false
	time.Sleep(h.Config.ShutdownWaitDuration)

	h.Http.Shutdown(context.Background())
	slog.Info("BYE")
}
