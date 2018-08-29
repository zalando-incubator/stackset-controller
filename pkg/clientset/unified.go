package clientset

import (
	stackset "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned"
	zalandov1 "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/typed/zalando/v1"
	"k8s.io/client-go/kubernetes"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	autoscalingv2beta1 "k8s.io/client-go/kubernetes/typed/autoscaling/v2beta1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	extensionsv1beta1 "k8s.io/client-go/kubernetes/typed/extensions/v1beta1"
	rest "k8s.io/client-go/rest"
)

type Interface interface {
	AppsV1() appsv1.AppsV1Interface
	AutoscalingV2beta1() autoscalingv2beta1.AutoscalingV2beta1Interface
	CoreV1() corev1.CoreV1Interface
	ExtensionsV1beta1() extensionsv1beta1.ExtensionsV1beta1Interface
	ZalandoV1() zalandov1.ZalandoV1Interface
}

type Clientset struct {
	kubernetes kubernetes.Interface
	stackset   stackset.Interface
}

func NewForConfig(kubeconfig *rest.Config) (*Clientset, error) {
	kubeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	stacksetClient, err := stackset.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	return &Clientset{kubernetes: kubeClient, stackset: stacksetClient}, nil
}

func (c *Clientset) AppsV1() appsv1.AppsV1Interface {
	return c.kubernetes.AppsV1()
}

func (c *Clientset) AutoscalingV2beta1() autoscalingv2beta1.AutoscalingV2beta1Interface {
	return c.kubernetes.AutoscalingV2beta1()
}

func (c *Clientset) CoreV1() corev1.CoreV1Interface {
	return c.kubernetes.CoreV1()
}

func (c *Clientset) ExtensionsV1beta1() extensionsv1beta1.ExtensionsV1beta1Interface {
	return c.kubernetes.ExtensionsV1beta1()
}

func (c *Clientset) ZalandoV1() zalandov1.ZalandoV1Interface {
	return c.stackset.ZalandoV1()
}
