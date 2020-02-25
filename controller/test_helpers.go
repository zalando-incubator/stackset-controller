package controller

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	ssinterface "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned"
	ssfake "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/fake"
	zi "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/typed/zalando.org/v1"
	ssunified "github.com/zalando-incubator/stackset-controller/pkg/clientset"
	apps "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type testClient struct {
	kubernetes.Interface
	ssClient ssinterface.Interface
}

func (c *testClient) ZalandoV1() zi.ZalandoV1Interface {
	return c.ssClient.ZalandoV1()
}

type testEnvironment struct {
	client     ssunified.Interface
	controller *StackSetController
}

func NewTestEnvironment() *testEnvironment {
	client := &testClient{
		Interface: fake.NewSimpleClientset(),
		ssClient:  ssfake.NewSimpleClientset(),
	}

	controller, err := NewStackSetController(client, "", "", "", prometheus.NewPedanticRegistry(), time.Minute)
	if err != nil {
		panic(err)
	}

	return &testEnvironment{
		client:     client,
		controller: controller,
	}
}

func (f *testEnvironment) CreateStacksets(stacksets []zv1.StackSet) error {
	for _, stackset := range stacksets {
		_, err := f.client.ZalandoV1().StackSets(stackset.Namespace).Create(&stackset)
		if err != nil {
			return err
		}

		// Controller can't be tested properly due to https://github.com/kubernetes/client-go/issues/352
		f.controller.stacksetStore[stackset.UID] = stackset
	}
	return nil
}

func (f *testEnvironment) CreateStacks(stacks []zv1.Stack) error {
	for _, stack := range stacks {
		_, err := f.client.ZalandoV1().Stacks(stack.Namespace).Create(&stack)
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *testEnvironment) CreateDeployments(deployments []apps.Deployment) error {
	for _, deployment := range deployments {
		_, err := f.client.AppsV1().Deployments(deployment.Namespace).Create(&deployment)
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *testEnvironment) CreateIngresses(ingresses []extensions.Ingress) error {
	for _, ingresse := range ingresses {
		_, err := f.client.ExtensionsV1beta1().Ingresses(ingresse.Namespace).Create(&ingresse)
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *testEnvironment) CreateServices(services []v1.Service) error {
	for _, service := range services {
		_, err := f.client.CoreV1().Services(service.Namespace).Create(&service)
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *testEnvironment) CreateHPAs(hpas []autoscaling.HorizontalPodAutoscaler) error {
	for _, hpa := range hpas {
		_, err := f.client.AutoscalingV2beta1().HorizontalPodAutoscalers(hpa.Namespace).Create(&hpa)
		if err != nil {
			return err
		}
	}
	return nil
}

func testStackset(name, namespace string, uid types.UID) zv1.StackSet {
	return zv1.StackSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       uid,
		},
	}
}

func testStack(name, namespace string, uid types.UID, ownerStack zv1.StackSet) zv1.Stack {
	return zv1.Stack{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "zalando.org/v1",
			Kind:       "Stack",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       uid,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "zalando.org/v1",
					Kind:       "StackSet",
					Name:       ownerStack.Name,
					UID:        ownerStack.UID,
				},
			},
		},
	}
}

func stackOwned(owner zv1.Stack) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      owner.Name,
		Namespace: owner.Namespace,
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: "zalando.org/v1",
				Kind:       "Stack",
				Name:       owner.Name,
				UID:        owner.UID,
			},
		},
	}
}

func deploymentOwned(owner apps.Deployment) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      owner.Name,
		Namespace: owner.Namespace,
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       owner.Name,
				UID:        owner.UID,
			},
		},
	}
}

func stacksetOwned(owner zv1.StackSet) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      owner.Name,
		Namespace: owner.Namespace,
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: "zalando.org/v1",
				Kind:       "StackSet",
				Name:       owner.Name,
				UID:        owner.UID,
			},
		},
	}
}
