package clientset

import (
	rg "github.com/szuecs/routegroup-client/client/clientset/versioned"
	rgv1 "github.com/szuecs/routegroup-client/client/clientset/versioned/typed/zalando.org/v1"
	stackset "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned"
	zalandov1 "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/typed/zalando.org/v1"
	"k8s.io/client-go/kubernetes"
	rest "k8s.io/client-go/rest"
)

type Interface interface {
	kubernetes.Interface
	ZalandoV1() zalandov1.ZalandoV1Interface
	RouteGroupV1() rgv1.ZalandoV1Interface
}

type Clientset struct {
	kubernetes.Interface
	stackset   stackset.Interface
	RouteGroup rg.Interface
}

func NewClientset(kubernetes kubernetes.Interface, stackset stackset.Interface, routegroup rg.Interface) *Clientset {
	return &Clientset{
		kubernetes,
		stackset,
		routegroup,
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

	rgClient, err := rg.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	return NewClientset(kubeClient, stacksetClient, rgClient), nil
}

func (c *Clientset) ZalandoV1() zalandov1.ZalandoV1Interface {
	return c.stackset.ZalandoV1()
}

func (c *Clientset) RouteGroupV1() rgv1.ZalandoV1Interface {
	return c.RouteGroup.ZalandoV1()
}
