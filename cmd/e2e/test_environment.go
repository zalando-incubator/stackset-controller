package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	rg "github.com/szuecs/routegroup-client/client/clientset/versioned"
	rgv1client "github.com/szuecs/routegroup-client/client/clientset/versioned/typed/zalando.org/v1"
	zv1client "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/typed/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"k8s.io/client-go/kubernetes"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	autoscalingv2 "k8s.io/client-go/kubernetes/typed/autoscaling/v2beta2"
	corev1typed "k8s.io/client-go/kubernetes/typed/core/v1"
	networking "k8s.io/client-go/kubernetes/typed/networking/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	kubernetesClient, stacksetClient, routegroupClient = createClients()
	namespace                                          = requiredEnvar("E2E_NAMESPACE")
	clusterDomain                                      = requiredEnvar("CLUSTER_DOMAIN")
	clusterDomainInternal                              = requiredEnvar("CLUSTER_DOMAIN_INTERNAL")
	clusterDomains                                     = []string{clusterDomain, clusterDomainInternal}
	controllerId                                       = os.Getenv("CONTROLLER_ID")
	waitTimeout                                        = *flag.Duration("wait-timeout", 60*time.Second, "Waiting interval before getting the resource")
	trafficSwitchWaitTimeout                           = *flag.Duration("traffic-switch-wait-timeout", 150*time.Second, "Waiting interval before getting the checking stackset new traffic")
)

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{ForceColors: true})
}

func createClients() (kubernetes.Interface, clientset.Interface, rg.Interface) {
	kubeconfig := os.Getenv("KUBECONFIG")

	var cfg *rest.Config
	var err error
	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		panic(err)
	}

	cfg.QPS = 100
	cfg.Burst = 100

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	stacksetClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	routegroupClient, err := rg.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	return kubeClient, stacksetClient, routegroupClient
}

func stacksetInterface() zv1client.StackSetInterface {
	return stacksetClient.ZalandoV1().StackSets(namespace)
}

func stackInterface() zv1client.StackInterface {
	return stacksetClient.ZalandoV1().Stacks(namespace)
}

func deploymentInterface() appsv1.DeploymentInterface {
	return kubernetesClient.AppsV1().Deployments(namespace)
}

func serviceInterface() corev1typed.ServiceInterface {
	return kubernetesClient.CoreV1().Services(namespace)
}

func hpaInterface() autoscalingv2.HorizontalPodAutoscalerInterface {
	return kubernetesClient.AutoscalingV2beta2().HorizontalPodAutoscalers(namespace)
}

func ingressInterface() networking.IngressInterface {
	return kubernetesClient.NetworkingV1().Ingresses(namespace)
}

func routegroupInterface() rgv1client.RouteGroupInterface {
	return routegroupClient.ZalandoV1().RouteGroups(namespace)
}

func requiredEnvar(envar string) string {
	namespace := os.Getenv(envar)
	if namespace == "" {
		panic(fmt.Sprintf("%s not set", envar))
	}
	return namespace
}

func hostnames(stacksetName string) []string {
	names := make([]string, 0, len(clusterDomains))
	for _, domain := range clusterDomains {
		names = append(names, fmt.Sprintf("%s-%s.%s", namespace, stacksetName, domain))
	}
	return names
}
