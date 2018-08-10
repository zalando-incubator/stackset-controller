package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	clientset "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned"
	"github.com/zalando-incubator/stackset-controller/pkg/traffic"
	"github.com/alecthomas/kingpin"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultNamespace = "default"
)

var (
	config struct {
		Application string
		Stack       string
		Traffic     float64
		Namespace   string
	}
)

func main() {
	kingpin.Arg("application", "help").Required().StringVar(&config.Application)
	kingpin.Arg("stack", "help").StringVar(&config.Stack)
	kingpin.Arg("traffic", "help").Default("-1").Float64Var(&config.Traffic)
	kingpin.Flag("namespace", "Namespace of the application resource.").Default(defaultNamespace).StringVar(&config.Namespace)
	kingpin.Parse()

	kubeconfig, err := newKubeConfig()
	if err != nil {
		log.Fatalf("Failed to setup Kubernetes client: %v.", err)
	}

	client, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("Failed to setup Kubernetes client: %v", err)
	}

	appClient, err := clientset.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("Failed to setup Kubernetes CRD client: %v", err)
	}

	trafficSwitcher := traffic.NewSwitcher(client, appClient)

	if config.Stack != "" && config.Traffic != -1 {
		weight := config.Traffic
		if weight < 0 || weight > 100 {
			log.Fatalf("Traffic weight must be between 0 and 100.")
		}

		stacks, err := trafficSwitcher.Switch(config.Application, config.Stack, config.Namespace, weight)
		if err != nil {
			log.Fatal(err)
		}
		printTrafficTable(stacks)
		return
	}

	stacks, err := trafficSwitcher.TrafficWeights(config.Application, config.Namespace)
	if err != nil {
		log.Fatal(err)
	}
	printTrafficTable(stacks)
}

func printTrafficTable(stacks []traffic.StackTrafficWeight) {
	var w *tabwriter.Writer

	w = tabwriter.NewWriter(os.Stdout, 8, 8, 4, ' ', 0)
	fmt.Fprintf(w, "%s\t%s\t%s\n", "STACK", "DESIRED TRAFFIC", "ACTUAL TRAFFIC")

	for _, stack := range stacks {
		fmt.Fprintf(w,
			"%s\t%s\t%s\n",
			stack.Name,
			fmt.Sprintf("%.1f%%", stack.Weight),
			fmt.Sprintf("%.1f%%", stack.ActualWeight),
		)
	}

	w.Flush()
}

func newKubeConfig() (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	return kubeConfig.ClientConfig()
}
