package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/alecthomas/kingpin"
	log "github.com/sirupsen/logrus"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"github.com/zalando-incubator/stackset-controller/pkg/traffic"
	rest "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultNamespace = "default"
)

var (
	config struct {
		Stackset                    string
		Stack                       string
		Traffic                     float64
		Namespace                   string
		BackendWeightsAnnotationKey string
	}
)

func main() {
	kingpin.Arg("stackset", "help").Required().StringVar(&config.Stackset)
	kingpin.Arg("stack", "help").StringVar(&config.Stack)
	kingpin.Arg("traffic", "help").Default("-1").Float64Var(&config.Traffic)
	kingpin.Flag("namespace", "Namespace of the stackset resource.").Default(defaultNamespace).StringVar(&config.Namespace)
	kingpin.Flag("backend-weights-key", "Backend weights annotation key the controller will use to set current traffic values").Default(traffic.DefaultBackendWeightsAnnotationKey).StringVar(&config.BackendWeightsAnnotationKey)
	kingpin.Parse()

	kubeconfig, err := newKubeConfig()
	if err != nil {
		log.Fatalf("Failed to setup Kubernetes client: %v.", err)
	}

	client, err := clientset.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("Failed to initialize Kubernetes client: %v.", err)
	}

	trafficSwitcher := traffic.NewSwitcher(client, config.BackendWeightsAnnotationKey)

	if config.Stack != "" && config.Traffic != -1 {
		weight := config.Traffic
		if weight < 0 || weight > 100 {
			log.Fatalf("Traffic weight must be between 0 and 100.")
		}

		stacks, err := trafficSwitcher.Switch(config.Stackset, config.Stack, config.Namespace, weight)
		if err != nil {
			log.Fatal(err)
		}
		printTrafficTable(stacks)
		return
	}

	stacks, err := trafficSwitcher.TrafficWeights(config.Stackset, config.Namespace)
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
