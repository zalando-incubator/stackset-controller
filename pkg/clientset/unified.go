package clientset

import (
	stackset "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned"
	zalandov1 "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/typed/zalando.org/v1"
	"k8s.io/client-go/kubernetes"
	rest "k8s.io/client-go/rest"
)

type Interface interface {
	kubernetes.Interface
	ZalandoV1() zalandov1.ZalandoV1Interface
}

type Clientset struct {
	kubernetes.Interface
	stackset stackset.Interface
}

func NewClientset(kubernetes kubernetes.Interface, stackset stackset.Interface) *Clientset {
	return &Clientset{
		kubernetes,
		stackset,
	}
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

	return &Clientset{kubeClient, stacksetClient}, nil
}

func (c *Clientset) ZalandoV1() zalandov1.ZalandoV1Interface {
	return c.stackset.ZalandoV1()
}
