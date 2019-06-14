package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/core"
	apps "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestGetOwnerUID(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		OwnerReferences: []metav1.OwnerReference{
			{
				UID: types.UID("x"),
			},
		},
	}

	uid, ok := getOwnerUID(objectMeta)
	assert.Equal(t, types.UID("x"), uid)
	assert.True(t, ok)

	uid, ok = getOwnerUID(metav1.ObjectMeta{})
	assert.Equal(t, types.UID(""), uid)
	assert.False(t, ok)
}

func TestCollectResources(t *testing.T) {
	testStacksetA := testStackset("foo", "default", "123")
	testStacksetB := testStackset("bar", "namespace", "999")

	testStackA1 := testStack("foo-v1", testStacksetA.Namespace, "abc1", testStacksetA)
	testStackA2 := testStack("foo-v2", testStacksetA.Namespace, "abc2", testStacksetA)
	testStackB1 := testStack("bar-v1", testStacksetB.Namespace, "def3", testStacksetB)

	testPrescalingStackset := testStackset("baz", "namespace", "456")
	testPrescalingStackset.Annotations = map[string]string{PrescaleStacksAnnotationKey: ""}

	testOrphanMeta := stackOwned(testStack("nonexistent", "default", "xxx", zv1.StackSet{}))
	testUnownedA1Meta := metav1.ObjectMeta{Name: testStackA1.Name, Namespace: testStackA1.Namespace}
	testUnownedBMeta := metav1.ObjectMeta{Name: testStacksetB.Name, Namespace: testStacksetB.Namespace}

	testPrescalingCustomStackset := testStackset("foobaz", "namespace", "789")
	testPrescalingCustomStackset.Annotations = map[string]string{PrescaleStacksAnnotationKey: "", ResetHPAMinReplicasDelayAnnotationKey: "30s"}

	for _, tc := range []struct {
		name        string
		stacksets   []zv1.StackSet
		stacks      []zv1.Stack
		deployments []apps.Deployment
		ingresses   []extensions.Ingress
		services    []v1.Service
		hpas        []autoscaling.HorizontalPodAutoscaler
		expected    map[types.UID]*core.StackSetContainer
	}{
		{
			name: "works correctly without any resources",
			stacksets: []zv1.StackSet{
				testStacksetA,
				testPrescalingStackset,
				testPrescalingCustomStackset,
			},
			expected: map[types.UID]*core.StackSetContainer{
				testStacksetA.UID: {
					StackSet:          &testStacksetA,
					StackContainers:   map[types.UID]*core.StackContainer{},
					TrafficReconciler: &core.SimpleTrafficReconciler{},
				},
				testPrescalingStackset.UID: {
					StackSet:        &testPrescalingStackset,
					StackContainers: map[types.UID]*core.StackContainer{},
					TrafficReconciler: &core.PrescalingTrafficReconciler{
						ResetHPAMinReplicasTimeout: defaultResetMinReplicasDelay,
					},
				},
				testPrescalingCustomStackset.UID: {
					StackSet:        &testPrescalingCustomStackset,
					StackContainers: map[types.UID]*core.StackContainer{},
					TrafficReconciler: &core.PrescalingTrafficReconciler{
						ResetHPAMinReplicasTimeout: 30 * time.Second,
					},
				},
			},
		},
		{
			name:      "stacks are collected even without resources",
			stacksets: []zv1.StackSet{testStacksetA, testStacksetB},
			stacks:    []zv1.Stack{testStackA1, testStackA2, testStackB1},
			expected: map[types.UID]*core.StackSetContainer{
				testStacksetA.UID: {
					StackSet: &testStacksetA,
					StackContainers: map[types.UID]*core.StackContainer{
						testStackA1.UID: {
							Stack: &testStackA1,
						},
						testStackA2.UID: {
							Stack: &testStackA2,
						},
					},
					TrafficReconciler: &core.SimpleTrafficReconciler{},
				},
				testStacksetB.UID: {
					StackSet: &testStacksetB,
					StackContainers: map[types.UID]*core.StackContainer{
						testStackB1.UID: {
							Stack: &testStackB1,
						},
					},
					TrafficReconciler: &core.SimpleTrafficReconciler{},
				},
			},
		},
		{
			name:      "all resources are collected",
			stacksets: []zv1.StackSet{testStacksetA, testStacksetB},
			stacks:    []zv1.Stack{testStackA1, testStackA2, testStackB1},
			deployments: []apps.Deployment{
				{ObjectMeta: stackOwned(testStackA2)}, // stack owned
				{ObjectMeta: testOrphanMeta},          // owned by unknown stack
				{ObjectMeta: testUnownedA1Meta},       // same name, but not owned by a stack
			},
			ingresses: []extensions.Ingress{
				{ObjectMeta: stackOwned(testStackA2)},      // stack owned
				{ObjectMeta: testOrphanMeta},               // owned by unknown stack
				{ObjectMeta: testUnownedA1Meta},            // same name, but not owned by a stack
				{ObjectMeta: stacksetOwned(testStacksetA)}, // owned by stackset
				{ObjectMeta: testUnownedBMeta},             // same name, but not owned by a stackset
			},
			services: []v1.Service{
				{ObjectMeta: stackOwned(testStackA2)}, // stack owned
				{ObjectMeta: testOrphanMeta},          // owned by unknown stack
				{ObjectMeta: testUnownedA1Meta},       // same name, but not owned by a stack
			},
			hpas: []autoscaling.HorizontalPodAutoscaler{
				{ObjectMeta: stackOwned(testStackA2)}, // stack owned
				{ObjectMeta: testOrphanMeta},          // owned by unknown stack
				{ObjectMeta: testUnownedA1Meta},       // same name, but not owned by a stack
			},
			expected: map[types.UID]*core.StackSetContainer{
				testStacksetA.UID: {
					StackSet: &testStacksetA,
					StackContainers: map[types.UID]*core.StackContainer{
						testStackA1.UID: {
							Stack: &testStackA1,
						},
						testStackA2.UID: {
							Stack: &testStackA2,
							Resources: core.StackResources{
								Deployment: &apps.Deployment{ObjectMeta: stackOwned(testStackA2)},
								HPA:        &autoscaling.HorizontalPodAutoscaler{ObjectMeta: stackOwned(testStackA2)},
								Service:    &v1.Service{ObjectMeta: stackOwned(testStackA2)},
								Ingress:    &extensions.Ingress{ObjectMeta: stackOwned(testStackA2)},
							},
						},
					},
					Ingress:           &extensions.Ingress{ObjectMeta: stacksetOwned(testStacksetA)},
					TrafficReconciler: &core.SimpleTrafficReconciler{},
				},
				testStacksetB.UID: {
					StackSet: &testStacksetB,
					StackContainers: map[types.UID]*core.StackContainer{
						testStackB1.UID: {
							Stack: &testStackB1,
						},
					},
					TrafficReconciler: &core.SimpleTrafficReconciler{},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnvironment()

			err := env.CreateStacksets(tc.stacksets)
			require.NoError(t, err)

			err = env.CreateStacks(tc.stacks)
			require.NoError(t, err)

			err = env.CreateDeployments(tc.deployments)
			require.NoError(t, err)

			err = env.CreateIngresses(tc.ingresses)
			require.NoError(t, err)

			err = env.CreateServices(tc.services)
			require.NoError(t, err)

			err = env.CreateHpas(tc.hpas)
			require.NoError(t, err)

			resources, err := env.controller.collectResources()
			require.NoError(t, err)
			require.Equal(t, tc.expected, resources)
		})
	}
}

func TestCreateCurrentStack(t *testing.T) {
	env := NewTestEnvironment()

	replicas := int32(1)

	stackset := testStackset("foo", "default", "123")
	stackset.Spec.StackTemplate.Spec = zv1.StackSpecTemplate{
		Version: "v1",
		StackSpec: zv1.StackSpec{
			Replicas: &replicas,
			PodTemplate: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "nginx",
						},
					},
				},
			},
		},
	}

	err := env.CreateStacksets([]zv1.StackSet{stackset})
	require.NoError(t, err)

	_, err = env.client.ZalandoV1().Stacks(stackset.Namespace).Get("foo-v1", metav1.GetOptions{})
	require.True(t, errors.IsNotFound(err))

	container := core.StackSetContainer{
		StackSet:          &stackset,
		StackContainers:   map[types.UID]*core.StackContainer{},
		TrafficReconciler: &core.SimpleTrafficReconciler{},
	}

	// Check that the stack is created and the container is updated afterwards
	err = env.controller.CreateCurrentStack(container)
	require.NoError(t, err)

	stack, err := env.client.ZalandoV1().Stacks(stackset.Namespace).Get("foo-v1", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, stackset.Spec.StackTemplate.Spec.StackSpec, stack.Spec)
	require.Equal(t, map[types.UID]*core.StackContainer{
		stack.UID: {
			Stack: stack,
		},
	}, container.StackContainers)
	require.Equal(t, "v1", stackset.Status.ObservedStackVersion)

	// Check that we don't create the stack if not needed
	stackset.Status.ObservedStackVersion = "v2"
	stackset.Spec.StackTemplate.Spec.Version = "v2"

	err = env.controller.CreateCurrentStack(container)
	require.NoError(t, err)

	_, err = env.client.ZalandoV1().Stacks(stackset.Namespace).Get("foo-v2", metav1.GetOptions{})
	require.True(t, errors.IsNotFound(err))
}
