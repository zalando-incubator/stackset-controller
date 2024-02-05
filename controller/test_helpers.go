package controller

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	rginterface "github.com/szuecs/routegroup-client/client/clientset/versioned"
	rgfake "github.com/szuecs/routegroup-client/client/clientset/versioned/fake"
	rgi "github.com/szuecs/routegroup-client/client/clientset/versioned/typed/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	ssinterface "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned"
	ssfake "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/fake"
	zi "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/typed/zalando.org/v1"
	ssunified "github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"github.com/zalando-incubator/stackset-controller/pkg/core"
	apps "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	timeNow = time.Now().Format(time.RFC3339)
	// ttl for the test environment is time.Minute, here
	// timeOldEnough is set to twice this value.
	timeOldEnough = time.Now().Add(-2 * time.Minute).Format(time.RFC3339)
)

type testClient struct {
	kubernetes.Interface
	ssClient ssinterface.Interface
	rgClient rginterface.Interface
}

func (c *testClient) ZalandoV1() zi.ZalandoV1Interface {
	return c.ssClient.ZalandoV1()
}

func (c *testClient) RouteGroupV1() rgi.ZalandoV1Interface {
	return c.rgClient.ZalandoV1()
}

type testEnvironment struct {
	client     ssunified.Interface
	controller *StackSetController
}

func NewTestEnvironment() *testEnvironment {
	client := &testClient{
		Interface: fake.NewSimpleClientset(),
		ssClient:  ssfake.NewSimpleClientset(),
		rgClient:  rgfake.NewSimpleClientset(),
	}

	controller, err := NewStackSetController(
		client,
		v1.NamespaceAll,
		"",
		10,
		"",
		nil,
		prometheus.NewPedanticRegistry(),
		time.Minute,
		true,
		true,
		true,
		[]string{},
		true,
		true,
		time.Minute,
	)

	if err != nil {
		panic(err)
	}

	controller.now = func() string {
		return timeNow
	}

	return &testEnvironment{
		client:     client,
		controller: controller,
	}
}

func (f *testEnvironment) CreateStacksets(ctx context.Context, stacksets []zv1.StackSet) error {
	for _, stackset := range stacksets {
		_, err := f.client.ZalandoV1().StackSets(stackset.Namespace).Create(ctx, &stackset, metav1.CreateOptions{})
		if err != nil {
			return err
		}

		// Controller can't be tested properly due to https://github.com/kubernetes/client-go/issues/352
		f.controller.stacksetStore[stackset.UID] = stackset
	}
	return nil
}

func (f *testEnvironment) CreateStacks(ctx context.Context, stacks []zv1.Stack) error {
	for _, stack := range stacks {
		_, err := f.client.ZalandoV1().Stacks(stack.Namespace).Create(ctx, &stack, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *testEnvironment) CreateDeployments(ctx context.Context, deployments []apps.Deployment) error {
	for _, deployment := range deployments {
		_, err := f.client.AppsV1().Deployments(deployment.Namespace).Create(ctx, &deployment, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *testEnvironment) CreateIngresses(ctx context.Context, ingresses []networking.Ingress) error {
	for _, ingress := range ingresses {
		_, err := f.client.NetworkingV1().Ingresses(ingress.Namespace).Create(ctx, &ingress, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *testEnvironment) CreateRouteGroups(ctx context.Context, routegroups []rgv1.RouteGroup) error {
	for _, routegroup := range routegroups {
		_, err := f.client.RouteGroupV1().RouteGroups(routegroup.Namespace).Create(ctx, &routegroup, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *testEnvironment) CreateServices(ctx context.Context, services []v1.Service) error {
	for _, service := range services {
		_, err := f.client.CoreV1().Services(service.Namespace).Create(ctx, &service, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *testEnvironment) CreateHPAs(ctx context.Context, hpas []autoscaling.HorizontalPodAutoscaler) error {
	for _, hpa := range hpas {
		_, err := f.client.AutoscalingV2().HorizontalPodAutoscalers(hpa.Namespace).Create(ctx, &hpa, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *testEnvironment) CreateConfigMaps(ctx context.Context, configMaps []v1.ConfigMap) error {
	for _, configMap := range configMaps {
		_, err := f.client.CoreV1().ConfigMaps(configMap.Namespace).Create(ctx, &configMap, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *testEnvironment) CreateSecrets(ctx context.Context, secrets []v1.Secret) error {
	for _, secret := range secrets {
		_, err := f.client.CoreV1().Secrets(secret.Namespace).Create(ctx, &secret, metav1.CreateOptions{})
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

func testStacksetAnnotatedSegment(name, namespace string, uid types.UID) zv1.StackSet {
	res := testStackset(name, namespace, uid)
	res.Annotations = map[string]string{TrafficSegmentsAnnotationKey: "true"}
	return res
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

func segmentStackOwned(owner zv1.Stack) metav1.ObjectMeta {
	meta := stackOwned(owner)
	meta.Name += core.SegmentSuffix

	return meta
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
