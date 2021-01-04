package main

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	rg "github.com/szuecs/routegroup-client/client/clientset/versioned"
	rgv1client "github.com/szuecs/routegroup-client/client/clientset/versioned/typed/zalando.org/v1"
	zv1client "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/typed/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"k8s.io/client-go/kubernetes"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	autoscalingv2 "k8s.io/client-go/kubernetes/typed/autoscaling/v2beta2"
	corev1typed "k8s.io/client-go/kubernetes/typed/core/v1"
	networking "k8s.io/client-go/kubernetes/typed/networking/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	kubernetesClient, stacksetClient, routegroupClient = createClients()
	namespace                                          = requiredEnvar("E2E_NAMESPACE")
	clusterDomains                                     = []string{requiredEnvar("CLUSTER_DOMAIN"), requiredEnvar("CLUSTER_DOMAIN_INTERNAL")}
	controllerId                                       = os.Getenv("CONTROLLER_ID")
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
	return kubernetesClient.NetworkingV1beta1().Ingresses(namespace)
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
