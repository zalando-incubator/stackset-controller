package main

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/zalando-incubator/stackset-controller/controller"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"github.com/zalando-incubator/stackset-controller/pkg/traffic"
	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
)

const (
	defaultInterval               = "10s"
	defaultIngressSourceSwitchTTL = "5m"
	defaultMetricsAddress         = ":7979"
	defaultClientGOTimeout        = 30 * time.Second
)

var (
	config struct {
		Debug                       bool
		Interval                    time.Duration
		APIServer                   *url.URL
		MetricsAddress              string
		ClusterDomains              []string
		NoTrafficScaledownTTL       time.Duration
		ControllerID                string
		BackendWeightsAnnotationKey string
		RouteGroupSupportEnabled    bool
		IngressSourceSwitchTTL      time.Duration
	}
)

func main() {
	kingpin.Flag("debug", "Enable debug logging.").BoolVar(&config.Debug)
	kingpin.Flag("interval", "Interval between syncing stacksets.").
		Default(defaultInterval).DurationVar(&config.Interval)
	kingpin.Flag("apiserver", "API server url.").URLVar(&config.APIServer)
	kingpin.Flag("metrics-address", "defines where to serve metrics").Default(defaultMetricsAddress).StringVar(&config.MetricsAddress)
	kingpin.Flag("controller-id", "ID of the controller used to determine ownership of StackSet resources").StringVar(&config.ControllerID)
	kingpin.Flag("backend-weights-key", "Backend weights annotation key the controller will use to set current traffic values").Default(traffic.DefaultBackendWeightsAnnotationKey).StringVar(&config.BackendWeightsAnnotationKey)
	kingpin.Flag("cluster-domain", "Main domains of the cluster, used for generating Stack Ingress hostnames").Envar("CLUSTER_DOMAIN").Required().StringsVar(&config.ClusterDomains)
	kingpin.Flag("enable-routegroup-support", "Enable support for RouteGroups on StackSets.").Default("false").BoolVar(&config.RouteGroupSupportEnabled)
	kingpin.Flag("ingress-source-switch-ttl", "The ttl before an ingress source is deleted when replaced with another one e.g. switching from RouteGroup to Ingress or vice versa.").
		Default(defaultIngressSourceSwitchTTL).DurationVar(&config.IngressSourceSwitchTTL)
	kingpin.Parse()

	if config.Debug {
		log.SetLevel(log.DebugLevel)
	}

	ctx, cancel := context.WithCancel(context.Background())
	kubeConfig, err := configureKubeConfig(config.APIServer, defaultClientGOTimeout, ctx.Done())
	if err != nil {
		log.Fatalf("Failed to setup Kubernetes config: %v", err)
	}

	client, err := clientset.NewForConfig(kubeConfig)
	if err != nil {
		log.Fatalf("Failed to initialize Kubernetes client: %v", err)
	}

	controller, err := controller.NewStackSetController(
		client,
		config.ControllerID,
		config.BackendWeightsAnnotationKey,
		config.ClusterDomains,
		prometheus.DefaultRegisterer,
		config.Interval,
		config.RouteGroupSupportEnabled,
		config.IngressSourceSwitchTTL,
	)
	if err != nil {
		log.Fatalf("Failed to create Stackset controller: %v", err)
	}

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
func configureKubeConfig(apiServerURL *url.URL, timeout time.Duration, stopCh <-chan struct{}) (*rest.Config, error) {
	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
			DualStack: false, // K8s do not work well with IPv6
		}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       20 * time.Second,
	}

	// We need this to reliably fade on DNS change, which is right
	// now not fixed with IdleConnTimeout in the http.Transport.
	// https://github.com/golang/go/issues/23427
	go func(d time.Duration) {
		for {
			select {
			case <-time.After(d):
				tr.CloseIdleConnections()
			case <-stopCh:
				return
			}
		}
	}(20 * time.Second)

	if apiServerURL != nil {
		return &rest.Config{
			Host:      apiServerURL.String(),
			Timeout:   timeout,
			Transport: tr,
			QPS:       100.0,
			Burst:     500,
		}, nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	// patch TLS config
	restTransportConfig, err := config.TransportConfig()
	if err != nil {
		return nil, err
	}
	restTLSConfig, err := transport.TLSConfigFor(restTransportConfig)
	if err != nil {
		return nil, err
	}
	tr.TLSClientConfig = restTLSConfig

	config.Timeout = timeout
	config.Transport = tr
	config.QPS = 100.0
	config.Burst = 500
	// disable TLSClientConfig to make the custom Transport work
	config.TLSClientConfig = rest.TLSClientConfig{}
	return config, nil
}

// gather go metrics
func serveMetrics(address string) {
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(address, nil))
}
