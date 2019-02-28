package main

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	zv1client "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/typed/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"k8s.io/client-go/kubernetes"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	autoscalingv2 "k8s.io/client-go/kubernetes/typed/autoscaling/v2beta1"
	corev1typed "k8s.io/client-go/kubernetes/typed/core/v1"
	extensionsv1beta1 "k8s.io/client-go/kubernetes/typed/extensions/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	kubernetesClient, stacksetClient = createClients()
	namespace                        = requiredEnvar("E2E_NAMESPACE")
	clusterDomain                    = requiredEnvar("CLUSTER_DOMAIN")
	controllerId                     = os.Getenv("CONTROLLER_ID")
)

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{ForceColors: true})
}

func createClients() (kubernetes.Interface, clientset.Interface) {
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
	return kubeClient, stacksetClient
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
	return kubernetesClient.AutoscalingV2beta1().HorizontalPodAutoscalers(namespace)
}

func ingressInterface() extensionsv1beta1.IngressInterface {
	return kubernetesClient.ExtensionsV1beta1().Ingresses(namespace)
}

func requiredEnvar(envar string) string {
	namespace := os.Getenv(envar)
	if namespace == "" {
		panic(fmt.Sprintf("%s not set", envar))
	}
	return namespace
}

func hostname(stacksetName string) string {
	return fmt.Sprintf("%s-%s.%s", namespace, stacksetName, clusterDomain)
}
