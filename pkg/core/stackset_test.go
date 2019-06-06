package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	apps "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestExpiredStacks(t *testing.T) {
	now := time.Now()

	stackContainer := func(name string, creationTime time.Time, noTrafficSince time.Time, desiredTraffic, actualTraffic float64) *StackContainer {
		return &StackContainer{
			Stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name:              name,
					CreationTimestamp: metav1.NewTime(creationTime),
				},
			},
			desiredTrafficWeight: desiredTraffic,
			actualTrafficWeight:  actualTraffic,
			noTrafficSince:       noTrafficSince,
		}
	}

	for _, tc := range []struct {
		name                string
		limit               int32
		scaledownTTLSeconds time.Duration
		ingress             bool
		stacks              []*StackContainer
		expected            map[string]bool
	}{
		{
			name:    "test GC oldest stack",
			limit:   1,
			ingress: true,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), now.Add(-1*time.Hour), 0, 0),
				stackContainer("stack2", now.Add(-2*time.Hour), now.Add(-1*time.Hour), 0, 0),
			},
			expected: map[string]bool{"stack2": true},
		},
		{
			name:    "test GC oldest stack (without ingress defined)",
			limit:   1,
			ingress: false,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), time.Time{}, 0, 0),
				stackContainer("stack2", now.Add(-2*time.Hour), time.Time{}, 0, 0),
			},
			expected: map[string]bool{"stack2": true},
		},
		{
			name:    "test don't GC stacks when all are getting traffic",
			limit:   1,
			ingress: true,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), now.Add(-1*time.Hour), 1, 0),
				stackContainer("stack2", now.Add(-2*time.Hour), now.Add(-2*time.Hour), 0, 1),
			},
			expected: nil,
		},
		{
			name:    "test don't GC stacks when there are less than limit",
			limit:   3,
			ingress: true,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), now.Add(-1*time.Hour), 0, 0),
				stackContainer("stack2", now.Add(-2*time.Hour), now.Add(-2*time.Hour), 0, 0),
			},
			expected: nil,
		},
		{
			name:    "test stacks with traffic don't count against limit",
			limit:   2,
			ingress: true,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), now.Add(-1*time.Hour), 1, 1),
				stackContainer("stack2", now.Add(-2*time.Hour), now.Add(-2*time.Hour), 0, 0),
				stackContainer("stack3", now.Add(-3*time.Hour), now.Add(-3*time.Hour), 0, 0),
			},
			expected: nil,
		},
		{
			name:                "not GC'ing a stack with no-traffic-since less than ScaledownTTLSeconds",
			limit:               1,
			ingress:             true,
			scaledownTTLSeconds: 300,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), now.Add(-200*time.Second), 0, 0),
				stackContainer("stack2", now.Add(-2*time.Hour), now.Add(-250*time.Second), 0, 0),
			},
			expected: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{
						StackLifecycle: zv1.StackLifecycle{
							ScaledownTTLSeconds: nil,
							Limit:               nil,
						},
					},
				},
				StackContainers: map[types.UID]*StackContainer{},
			}
			c.StackSet.Spec.StackLifecycle.Limit = &tc.limit
			for _, stack := range tc.stacks {
				if tc.scaledownTTLSeconds == 0 {
					stack.scaledownTTL = defaultScaledownTTL
				} else {
					stack.scaledownTTL = time.Second * tc.scaledownTTLSeconds
				}
				if tc.ingress {
					stack.ingressSpec = &zv1.StackSetIngressSpec{}
				}
				c.StackContainers[types.UID(stack.Stack.Name)] = stack
			}

			err := c.MarkExpiredStacks()
			require.NoError(t, err)
			for _, stack := range tc.stacks {
				require.Equal(t, tc.expected[stack.Stack.Name], stack.PendingRemoval, "stack %s", stack.Stack.Name)
			}
		})
	}
}

func TestSanitizeServicePorts(t *testing.T) {
	service := &zv1.StackServiceSpec{
		Ports: []v1.ServicePort{
			{
				Protocol: "",
			},
		},
	}

	service = sanitizeServicePorts(service)
	require.Len(t, service.Ports, 1)
	require.Equal(t, v1.ProtocolTCP, service.Ports[0].Protocol)
}

func TestStackSetNewStack(t *testing.T) {
	for _, tc := range []struct {
		name              string
		stackset          *zv1.StackSet
		stacks            map[types.UID]*StackContainer
		expectedStack     *StackContainer
		expectedStackName string
	}{
		{
			name: "stack already exists",
			stackset: &zv1.StackSet{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: zv1.StackSetSpec{
					StackTemplate: zv1.StackTemplate{
						Spec: zv1.StackSpecTemplate{
							Version: "v1",
						},
					},
				},
			},
			stacks: map[types.UID]*StackContainer{
				"foo": {
					Stack: &zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{Name: "foo-v1"},
					},
				},
			},
			expectedStack:     nil,
			expectedStackName: "",
		},
		{
			name: "stack already created",
			stackset: &zv1.StackSet{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: zv1.StackSetSpec{
					StackTemplate: zv1.StackTemplate{
						Spec: zv1.StackSpecTemplate{
							Version: "v1",
						},
					},
				},
				Status: zv1.StackSetStatus{
					ObservedStackVersion: "v1",
				},
			},
			stacks:            map[types.UID]*StackContainer{},
			expectedStack:     nil,
			expectedStackName: "",
		},
		{
			name: "stack needs to be created",
			stackset: &zv1.StackSet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: apiVersion,
					Kind:       stackSetKind,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					UID:       "1234-abc-2134",
					Labels:    map[string]string{"custom": "label"},
				},
				Spec: zv1.StackSetSpec{
					StackTemplate: zv1.StackTemplate{
						Spec: zv1.StackSpecTemplate{
							Version: "v1",
						},
					},
				},
			},
			stacks: map[types.UID]*StackContainer{},
			expectedStack: &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1",
						Namespace: "bar",
						Labels: map[string]string{
							StacksetHeritageLabelKey: "foo",
							"custom":                 "label",
							StackVersionLabelKey:     "v1",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: apiVersion,
								Kind:       stackSetKind,
								Name:       "foo",
								UID:        "1234-abc-2134",
							},
						},
					},
				},
			},
			expectedStackName: "v1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stackset := &StackSetContainer{
				StackSet:        tc.stackset,
				StackContainers: tc.stacks,
			}
			newStack, newStackName := stackset.NewStack()
			require.EqualValues(t, tc.expectedStack, newStack)
			require.EqualValues(t, tc.expectedStackName, newStackName)
		})
	}
}

func dummyStacksetContainer() *StackSetContainer {
	return &StackSetContainer{
		StackSet: &zv1.StackSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
		},
		StackContainers: map[types.UID]*StackContainer{
			"foo-1": {
				Stack: &zv1.Stack{},
			},
			"foo-2": {
				Stack: &zv1.Stack{},
			},
		},
	}
}

func TestStackSetUpdateFromResources(t *testing.T) {
	minute := int64(60)

	for _, tc := range []struct {
		name                 string
		scaledownTTL         *int64
		ingress              *zv1.StackSetIngressSpec
		expectedScaledownTTL time.Duration
	}{
		{
			name:                 "no ingress, default scaledown TTL",
			expectedScaledownTTL: defaultScaledownTTL,
		},
		{
			name:                 "explicit scaledown TTL",
			scaledownTTL:         &minute,
			expectedScaledownTTL: 60 * time.Second,
		},
		{
			name: "ingress",
			ingress: &zv1.StackSetIngressSpec{
				Hosts: []string{"foo"},
			},
			expectedScaledownTTL: defaultScaledownTTL,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := dummyStacksetContainer()
			c.StackSet.Spec.StackLifecycle.ScaledownTTLSeconds = tc.scaledownTTL
			err := c.UpdateFromResources()
			require.NoError(t, err)

			for _, sc := range c.StackContainers {
				require.Equal(t, c.StackSet.Name, sc.stacksetName)

				require.EqualValues(t, c.StackSet.Spec.Ingress, sc.ingressSpec)

				require.Equal(t, tc.expectedScaledownTTL, sc.scaledownTTL)
			}
		})
	}
}

func TestStackUpdateFromResources(t *testing.T) {
	runTest := func(name string, testFn func(t *testing.T, container *StackContainer)) {
		container := &StackContainer{
			Stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-v1",
				},
			},
		}
		t.Run(name, func(t *testing.T) {
			testFn(t, container)
		})
	}

	hourAgo := time.Now().Add(-time.Hour)

	runTest("stackset replicas default to 1", func(t *testing.T, container *StackContainer) {
		container.Stack.Spec.Replicas = nil
		container.updateFromResources()
		require.EqualValues(t, 1, container.stackReplicas)
	})
	runTest("stackset replicas are parsed from the spec", func(t *testing.T, container *StackContainer) {
		container.Stack.Spec.Replicas = wrapReplicas(3)
		container.updateFromResources()
		require.EqualValues(t, 3, container.stackReplicas)
	})

	runTest("desired replicas are set to 0 if there's no HPA", func(t *testing.T, container *StackContainer) {
		container.updateFromResources()
		require.EqualValues(t, 0, container.desiredReplicas)
	})
	runTest("desired replicas are parsed from the HPA", func(t *testing.T, container *StackContainer) {
		container.Resources.HPA = &autoscaling.HorizontalPodAutoscaler{
			Status: autoscaling.HorizontalPodAutoscalerStatus{
				DesiredReplicas: 7,
			},
		}
		container.updateFromResources()
		require.EqualValues(t, 7, container.desiredReplicas)
	})

	runTest("noTrafficSince can be unset", func(t *testing.T, container *StackContainer) {
		container.updateFromResources()
		require.EqualValues(t, time.Time{}, container.noTrafficSince)
	})
	runTest("noTrafficSince is parsed from the status", func(t *testing.T, container *StackContainer) {
		container.Stack.Status.NoTrafficSince = &metav1.Time{Time: hourAgo}
		container.updateFromResources()
		require.EqualValues(t, hourAgo, container.noTrafficSince)
	})

	runTest("missing deployment is handled fine", func(t *testing.T, container *StackContainer) {
		container.updateFromResources()
		require.EqualValues(t, false, container.deploymentUpdated)
		require.EqualValues(t, 0, container.createdReplicas)
		require.EqualValues(t, 0, container.readyReplicas)
		require.EqualValues(t, 0, container.updatedReplicas)
	})
	runTest("replica information is parsed from the deployment", func(t *testing.T, container *StackContainer) {
		container.Resources.Deployment = &apps.Deployment{
			Spec: apps.DeploymentSpec{
				Replicas: wrapReplicas(3),
			},
			Status: apps.DeploymentStatus{
				Replicas:        11,
				UpdatedReplicas: 7,
				ReadyReplicas:   5,
			},
		}
		container.updateFromResources()
		require.EqualValues(t, 3, container.deploymentReplicas)
		require.EqualValues(t, 11, container.createdReplicas)
		require.EqualValues(t, 5, container.readyReplicas)
		require.EqualValues(t, 7, container.updatedReplicas)
	})
	runTest("missing deployment replicas default to 1", func(t *testing.T, container *StackContainer) {
		container.Resources.Deployment = &apps.Deployment{
			Spec: apps.DeploymentSpec{
				Replicas: nil,
			},
		}
		container.updateFromResources()
		require.EqualValues(t, 1, container.deploymentReplicas)
	})
	runTest("deployment isn't considered updated if the generation is different", func(t *testing.T, container *StackContainer) {
		container.Stack.Generation = 11
		container.Resources.Deployment = &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 5,
				Annotations: map[string]string{
					stackGenerationAnnotationKey: "10",
				},
			},
			Status: apps.DeploymentStatus{
				ObservedGeneration: 5,
			},
		}
		container.updateFromResources()
		require.EqualValues(t, false, container.deploymentUpdated)
	})
	runTest("deployment isn't considered updated if observedGeneration is different", func(t *testing.T, container *StackContainer) {
		container.Stack.Generation = 11
		container.Resources.Deployment = &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 5,
				Annotations: map[string]string{
					stackGenerationAnnotationKey: "11",
				},
			},
			Status: apps.DeploymentStatus{
				ObservedGeneration: 4,
			},
		}
		container.updateFromResources()
		require.EqualValues(t, false, container.deploymentUpdated)
	})
	runTest("deployment is considered updated if observedGeneration is the same", func(t *testing.T, container *StackContainer) {
		container.Stack.Generation = 11
		container.Resources.Deployment = &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 5,
				Annotations: map[string]string{
					stackGenerationAnnotationKey: "11",
				},
			},
			Status: apps.DeploymentStatus{
				ObservedGeneration: 5,
			},
		}
		container.updateFromResources()
		require.EqualValues(t, true, container.deploymentUpdated)
	})

	runTest("prescaling information is parsed from the status", func(t *testing.T, container *StackContainer) {
		container.Stack.Status.Prescaling = zv1.PrescalingStatus{
			Active:              true,
			Replicas:            11,
			LastTrafficIncrease: &metav1.Time{Time: hourAgo},
		}
		container.updateFromResources()
		require.EqualValues(t, 1, container.stackReplicas)
		require.EqualValues(t, true, container.prescalingActive)
		require.EqualValues(t, 11, container.prescalingReplicas)
		require.EqualValues(t, hourAgo, container.prescalingLastTrafficIncrease)
	})
}

func TestUpdateTrafficFromIngress(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		desiredWeights         string
		actualWeights          string
		expectedDesiredWeights map[string]float64
		expectedActualWeights  map[string]float64
	}{
		{
			name: "no weights are present",
		},
		{
			name:                   "desired and actual weights are parsed correctly",
			desiredWeights:         `{"foo-v1": 25, "foo-v2": 50, "foo-v3": 25}`,
			actualWeights:          `{"foo-v1": 62.5, "foo-v2": 12.5, "foo-v3": 25}`,
			expectedDesiredWeights: map[string]float64{"foo-v1": 25, "foo-v2": 50, "foo-v3": 25},
			expectedActualWeights:  map[string]float64{"foo-v1": 62.5, "foo-v2": 12.5, "foo-v3": 25},
		},
		{
			name:                   "unknown stacks are removed, remaining weights are renormalised",
			desiredWeights:         `{"foo-v4": 50, "foo-v2": 25, "foo-v3": 25}`,
			actualWeights:          `{"foo-v4": 50, "foo-v2": 12.5, "foo-v3": 37.5}`,
			expectedDesiredWeights: map[string]float64{"foo-v2": 50, "foo-v3": 50},
			expectedActualWeights:  map[string]float64{"foo-v2": 25, "foo-v3": 75},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stack := func(name string) *StackContainer {
				return &StackContainer{
					Stack: &zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{Name: name},
					},
				}
			}

			stack1 := stack("foo-v1")
			stack2 := stack("foo-v2")
			stack3 := stack("foo-v3")

			ssc := &StackSetContainer{
				StackSet: &zv1.StackSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{},
					},
				},
				Ingress: &extensions.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "foo",
						Annotations: map[string]string{},
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					"v1": stack1,
					"v2": stack2,
					"v3": stack3,
				},
			}

			if tc.desiredWeights != "" {
				ssc.Ingress.Annotations[stackTrafficWeightsAnnotationKey] = tc.desiredWeights
			}
			if tc.actualWeights != "" {
				ssc.Ingress.Annotations[backendWeightsAnnotationKey] = tc.actualWeights
			}

			err := ssc.UpdateFromResources()
			require.NoError(t, err)
			for _, sc := range ssc.StackContainers {
				require.Equal(t, tc.expectedDesiredWeights[sc.Stack.Name], sc.desiredTrafficWeight, "stack %s", sc.Stack.Name)
				require.Equal(t, tc.expectedActualWeights[sc.Stack.Name], sc.actualTrafficWeight, "stack %s", sc.Stack.Name)
			}
		})
	}
}

func TestGenerateStackSetStatus(t *testing.T) {
	stackContainer := func(pendingRemoval, ready bool, hasTraffic bool) *StackContainer {
		result := &StackContainer{
			Stack:          &zv1.Stack{},
			PendingRemoval: pendingRemoval,
		}
		if ready {
			result.deploymentUpdated = true
			result.deploymentReplicas = 3
			result.readyReplicas = 3
			result.updatedReplicas = 3
		}
		if hasTraffic {
			result.desiredTrafficWeight = 0.3
			result.actualTrafficWeight = 0.3
		}
		return result
	}

	c := &StackSetContainer{
		StackSet: &zv1.StackSet{
			Status: zv1.StackSetStatus{
				Stacks:               1,
				ReadyStacks:          2,
				StacksWithTraffic:    3,
				ObservedStackVersion: "v1",
			},
		},
		StackContainers: map[types.UID]*StackContainer{
			"v1": stackContainer(true, true, false),
			"v2": stackContainer(false, true, true),
			"v3": stackContainer(false, true, false),
			"v4": stackContainer(false, false, false),
		},
	}

	expected := &zv1.StackSetStatus{
		Stacks:               3,
		ReadyStacks:          2,
		StacksWithTraffic:    1,
		ObservedStackVersion: "v1",
	}
	require.Equal(t, expected, c.GenerateStackSetStatus())
}

func TestStackSetGenerateIngress(t *testing.T) {
	stackContainer := func(name string, desiredTrafficWeight, actualTrafficWeight float64) *StackContainer {
		return &StackContainer{
			Stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
			},
			desiredTrafficWeight: desiredTrafficWeight,
			actualTrafficWeight:  actualTrafficWeight,
		}
	}

	c := &StackSetContainer{
		StackSet: &zv1.StackSet{
			TypeMeta: metav1.TypeMeta{
				APIVersion: apiVersion,
				Kind:       stackSetKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				Labels: map[string]string{
					"stackset-label": "foobar",
				},
				UID: "abc-123",
			},
			Spec: zv1.StackSetSpec{
				Ingress: &zv1.StackSetIngressSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      map[string]string{"ignored": "label"},
						Annotations: map[string]string{"ingress": "annotation"},
					},
					Hosts:       []string{"example.org", "example.com"},
					BackendPort: intstr.FromInt(80),
					Path:        "example",
				},
			},
		},
		StackContainers: map[types.UID]*StackContainer{
			"v1": stackContainer("foo-v1", 0.125, 0.25),
			"v2": stackContainer("foo-v2", 0.5, 0.125),
			"v3": stackContainer("foo-v3", 0.625, 0.625),
			"v4": stackContainer("foo-v4", 0, 0),
		},
	}
	ingress, err := c.GenerateIngress()
	require.NoError(t, err)
	expected := &extensions.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
			Labels: map[string]string{
				"stackset":       "foo",
				"stackset-label": "foobar",
			},
			Annotations: map[string]string{
				"ingress":                           "annotation",
				"zalando.org/stack-traffic-weights": `{"foo-v1":0.125,"foo-v2":0.5,"foo-v3":0.625}`,
				"zalando.org/backend-weights":       `{"foo-v1":0.25,"foo-v2":0.125,"foo-v3":0.625}`,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: apiVersion,
					Kind:       stackSetKind,
					Name:       "foo",
					UID:        "abc-123",
				},
			},
		},
		Spec: extensions.IngressSpec{
			Rules: []extensions.IngressRule{
				{
					Host: "example.org",
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								{
									Path: "example",
									Backend: extensions.IngressBackend{
										ServiceName: "foo-v1",
										ServicePort: intstr.FromInt(80),
									},
								},
								{
									Path: "example",
									Backend: extensions.IngressBackend{
										ServiceName: "foo-v2",
										ServicePort: intstr.FromInt(80),
									},
								},
								{
									Path: "example",
									Backend: extensions.IngressBackend{
										ServiceName: "foo-v3",
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
				{
					Host: "example.com",
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								{
									Path: "example",
									Backend: extensions.IngressBackend{
										ServiceName: "foo-v1",
										ServicePort: intstr.FromInt(80),
									},
								},
								{
									Path: "example",
									Backend: extensions.IngressBackend{
										ServiceName: "foo-v2",
										ServicePort: intstr.FromInt(80),
									},
								},
								{
									Path: "example",
									Backend: extensions.IngressBackend{
										ServiceName: "foo-v3",
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	require.Equal(t, expected, ingress)
}

func TestStackSetGenerateIngressNone(t *testing.T) {
	c := &StackSetContainer{
		StackSet: &zv1.StackSet{},
	}
	ingress, err := c.GenerateIngress()
	require.NoError(t, err)
	require.Nil(t, ingress)
}
