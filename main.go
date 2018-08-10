package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zalando-incubator/stackset-controller/controller"
	clientset "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultInterval                = "10s"
	defaultMetricsAddress          = ":7979"
	defaultStackMinGCAge           = "24h"
	defaultNoTrafficScaledownTTL   = 1 * time.Hour
	defaultNoTrafficTerminationTTL = 1 * time.Minute
	defaultClientGOTimeout         = 30 * time.Second
)

var (
	config struct {
		Debug                   bool
		Interval                time.Duration
		APIServer               *url.URL
		MetricsAddress          string
		StackMinGCAge           time.Duration
		NoTrafficScaledownTTL   time.Duration
		NoTrafficTerminationTTL time.Duration
	}
)

func main() {
	kingpin.Flag("debug", "Enable debug logging.").BoolVar(&config.Debug)
	kingpin.Flag("interval", "Interval between syncing ingresses.").
		Default(defaultInterval).DurationVar(&config.Interval)
	kingpin.Flag("apiserver", "API server url.").URLVar(&config.APIServer)
	kingpin.Flag("stackset-stack-min-gc-age", "Minimum age for stackset stacks before they are considered for garbage collection.").Default(defaultStackMinGCAge).DurationVar(&config.StackMinGCAge)
	kingpin.Flag("no-traffic-scaledown-ttl", "Default TTL for scaling down deployments not getting any traffic.").Default(defaultNoTrafficScaledownTTL.String()).DurationVar(&config.NoTrafficScaledownTTL)
	kingpin.Flag("no-traffic-termination-ttl", "Default TTL for terminating deployments after they are not getting any traffic.").Default(defaultNoTrafficTerminationTTL.String()).DurationVar(&config.NoTrafficTerminationTTL)
	kingpin.Flag("metrics-address", "defines where to serve metrics").Default(defaultMetricsAddress).StringVar(&config.MetricsAddress)
	kingpin.Parse()

	if config.Debug {
		log.SetLevel(log.DebugLevel)
	}

	kubeConfig, err := configureKubeConfig(config.APIServer, defaultClientGOTimeout)
	if err != nil {
		log.Fatalf("Failed to setup Kubernetes config: %v", err)
	}

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatalf("Failed to setup Kubernetes client: %v", err)
	}

	stacksetClient, err := clientset.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatalf("Failed to setup Kubernetes CRD client: %v", err)
	}

	controller := controller.NewStackSetController(
		client,
		stacksetClient,
		config.StackMinGCAge,
		config.NoTrafficScaledownTTL,
		config.NoTrafficTerminationTTL,
		config.Interval,
	)

	ctx, cancel := context.WithCancel(context.Background())
	go handleSigterm(cancel)
	go serveMetrics(config.MetricsAddress)
	controller.Run(ctx)
}

// handleSigterm handles SIGTERM signal sent to the process.
func handleSigterm(cancelFunc func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	<-signals
	log.Info("Received Term signal. Terminating...")
	cancelFunc()
}

// configureKubeConfig configures a kubeconfig.
func configureKubeConfig(apiServerURL *url.URL, timeout time.Duration) (*rest.Config, error) {
	if apiServerURL != nil {
		return &rest.Config{
			Host:    apiServerURL.String(),
			Timeout: timeout,
		}, nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	config.Timeout = timeout
	return config, nil
}

// gather go metrics
func serveMetrics(address string) {
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(address, nil))
}
