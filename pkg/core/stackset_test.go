package core

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/traffic"
	apps "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta2"
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestExpiredStacks(t *testing.T) {
	now := time.Now()

	for _, tc := range []struct {
		name         string
		limit        int32
		scaledownTTL time.Duration
		ingress      bool
		stacks       []*StackContainer
		expected     map[string]bool
	}{
		{
			name:    "test GC oldest stack",
			limit:   1,
			ingress: true,
			stacks: []*StackContainer{
				testStack("stack1").createdAt(now.Add(-1 * time.Hour)).noTrafficSince(now.Add(-1 * time.Hour)).stack(),
				testStack("stack2").createdAt(now.Add(-2 * time.Hour)).noTrafficSince(now.Add(-1 * time.Hour)).stack(),
			},
			expected: map[string]bool{"stack2": true},
		},
		{
			name:    "test GC oldest stack (without ingress defined)",
			limit:   1,
			ingress: false,
			stacks: []*StackContainer{
				testStack("stack1").createdAt(now.Add(-1 * time.Hour)).stack(),
				testStack("stack2").createdAt(now.Add(-2 * time.Hour)).stack(),
			},
			expected: map[string]bool{"stack2": true},
		},
		{
			name:    "test don't GC stacks when all are getting traffic",
			limit:   1,
			ingress: true,
			stacks: []*StackContainer{
				testStack("stack1").createdAt(now.Add(-1*time.Hour)).noTrafficSince(now.Add(-1*time.Hour)).traffic(1, 0).stack(),
				testStack("stack2").createdAt(now.Add(-2*time.Hour)).noTrafficSince(now.Add(-2*time.Hour)).traffic(0, 1).stack(),
			},
			expected: nil,
		},
		{
			name:    "test don't GC stacks when there are less than limit",
			limit:   3,
			ingress: true,
			stacks: []*StackContainer{
				testStack("stack1").createdAt(now.Add(-1 * time.Hour)).noTrafficSince(now.Add(-1 * time.Hour)).stack(),
				testStack("stack2").createdAt(now.Add(-2 * time.Hour)).noTrafficSince(now.Add(-2 * time.Hour)).stack(),
			},
			expected: nil,
		},
		{
			name:    "test stacks with traffic don't count against limit",
			limit:   2,
			ingress: true,
			stacks: []*StackContainer{
				testStack("stack1").createdAt(now.Add(-1*time.Hour)).noTrafficSince(now.Add(-1*time.Hour)).traffic(1, 1).stack(),
				testStack("stack2").createdAt(now.Add(-2 * time.Hour)).noTrafficSince(now.Add(-2 * time.Hour)).stack(),
				testStack("stack3").createdAt(now.Add(-3 * time.Hour)).noTrafficSince(now.Add(-3 * time.Hour)).stack(),
			},
			expected: nil,
		},
		{
			name:         "not GC'ing a stack with no-traffic-since less than ScaledownTTLSeconds",
			limit:        1,
			ingress:      true,
			scaledownTTL: 300,
			stacks: []*StackContainer{
				testStack("stack1").createdAt(now.Add(-1 * time.Hour)).noTrafficSince(now.Add(-200 * time.Second)).stack(),
				testStack("stack2").createdAt(now.Add(-2 * time.Hour)).noTrafficSince(now.Add(-250 * time.Second)).stack(),
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
				StackContainers:             map[types.UID]*StackContainer{},
				backendWeightsAnnotationKey: traffic.DefaultBackendWeightsAnnotationKey,
			}
			c.StackSet.Spec.StackLifecycle.Limit = &tc.limit
			for _, stack := range tc.stacks {
				if tc.scaledownTTL == 0 {
					stack.scaledownTTL = defaultScaledownTTL
				} else {
					stack.scaledownTTL = time.Second * tc.scaledownTTL
				}
				if tc.ingress {
					stack.ingressSpec = &zv1.StackSetIngressSpec{}
				}
				c.StackContainers[types.UID(stack.Name())] = stack
			}

			c.MarkExpiredStacks()
			for _, stack := range tc.stacks {
				require.Equal(t, tc.expected[stack.Name()], stack.PendingRemoval, "stack %s", stack.Stack.Name)
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
				"foo": testStack("foo-v1").stack(),
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
					APIVersion: APIVersion,
					Kind:       KindStackSet,
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
								APIVersion: APIVersion,
								Kind:       KindStackSet,
								Name:       "foo",
								UID:        "1234-abc-2134",
							},
						},
					},
				},
			},
			expectedStackName: "v1",
		},
		{
			name: "stack needs to have the same update strategy",
			stackset: &zv1.StackSet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: APIVersion,
					Kind:       KindStackSet,
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
							StackSpec: zv1.StackSpec{
								Strategy: &apps.DeploymentStrategy{
									Type: apps.RollingUpdateDeploymentStrategyType,
									RollingUpdate: &apps.RollingUpdateDeployment{
										MaxUnavailable: intstrptr("10%"),
										MaxSurge:       intstrptr("100%"),
									},
								},
							},
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
								APIVersion: APIVersion,
								Kind:       KindStackSet,
								Name:       "foo",
								UID:        "1234-abc-2134",
							},
						},
					},
					Spec: zv1.StackSpec{
						Strategy: &apps.DeploymentStrategy{
							Type: apps.RollingUpdateDeploymentStrategyType,
							RollingUpdate: &apps.RollingUpdateDeployment{
								MaxUnavailable: intstrptr("10%"),
								MaxSurge:       intstrptr("100%"),
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
				StackSet:                    tc.stackset,
				StackContainers:             tc.stacks,
				backendWeightsAnnotationKey: traffic.DefaultBackendWeightsAnnotationKey,
			}
			newStack, newStackName := stackset.NewStack()
			require.EqualValues(t, tc.expectedStack, newStack)
			require.EqualValues(t, tc.expectedStackName, newStackName)
		})
	}
}

func intstrptr(value string) *intstr.IntOrString {
	v := intstr.FromString(value)
	return &v
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
		backendWeightsAnnotationKey: traffic.DefaultBackendWeightsAnnotationKey,
	}
}

func TestStackSetUpdateFromResourcesPopulatesIngress(t *testing.T) {
	for _, tc := range []struct {
		name            string
		ingress         *zv1.StackSetIngressSpec
		expectedIngress *zv1.StackSetIngressSpec
	}{
		{
			name:            "no ingress",
			expectedIngress: nil,
		},
		{
			name: "has one ingress",
			ingress: &zv1.StackSetIngressSpec{
				Hosts: []string{"foo"},
			},
			expectedIngress: &zv1.StackSetIngressSpec{
				Hosts: []string{"foo"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := dummyStacksetContainer()
			c.StackSet.Spec.Ingress = tc.ingress
			err := c.UpdateFromResources()
			require.NoError(t, err)

			for _, sc := range c.StackContainers {
				require.Equal(t, c.StackSet.Name, sc.stacksetName)
				require.EqualValues(t, c.StackSet.Spec.Ingress, sc.ingressSpec)
			}
		})
	}
}

func TestStackSetUpdateFromResourcesPopulatesBackendPort(t *testing.T) {
	beport := intstr.FromInt(8080)
	for _, tc := range []struct {
		name     string
		spec     zv1.StackSetSpec
		expected *intstr.IntOrString
	}{
		{
			name:     "no backendport",
			expected: nil,
		},
		{
			name: "has ingress",
			spec: zv1.StackSetSpec{
				Ingress: &zv1.StackSetIngressSpec{
					BackendPort: beport,
				},
			},
			expected: &beport,
		},
		{
			name: "has external ingress",
			spec: zv1.StackSetSpec{
				ExternalIngress: &zv1.StackSetExternalIngressSpec{
					BackendPort: beport,
				},
			},
			expected: &beport,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := dummyStacksetContainer()
			c.StackSet.Spec = tc.spec
			err := c.UpdateFromResources()
			require.NoError(t, err)

			for _, sc := range c.StackContainers {
				require.EqualValues(t, tc.expected, sc.backendPort)
			}
		})
	}
}

func TestStackSetUpdateFromResourcesScaleDown(t *testing.T) {
	minute := int64(60)

	for _, tc := range []struct {
		name                 string
		scaledownTTL         *int64
		ingress              *zv1.StackSetIngressSpec
		externalIngress      *zv1.StackSetExternalIngressSpec
		expectedScaledownTTL time.Duration
	}{
		{
			name:                 "no ingress, default scaledown TTL",
			expectedScaledownTTL: defaultScaledownTTL,
		},
		{
			name: "no ingress, an externalIngress default scaledown TTL",
			externalIngress: &zv1.StackSetExternalIngressSpec{
				BackendPort: intstr.FromInt(80),
			},
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
			if tc.externalIngress != nil {
				c.StackSet.Spec.ExternalIngress = tc.externalIngress
			}
			if tc.ingress != nil {
				c.StackSet.Spec.Ingress = tc.ingress
			}

			err := c.UpdateFromResources()
			require.NoError(t, err)

			for _, sc := range c.StackContainers {
				require.Equal(t, c.StackSet.Name, sc.stacksetName)

				require.EqualValues(t, c.StackSet.Spec.Ingress, sc.ingressSpec)
				if tc.externalIngress != nil || tc.ingress != nil {
					require.NotNil(t, sc.backendPort, "stack container backendport should not be nil")
				}
				if tc.externalIngress != nil {
					require.EqualValues(t, c.StackSet.Spec.ExternalIngress.BackendPort, *sc.backendPort)
				}
				require.Equal(t, tc.expectedScaledownTTL, sc.scaledownTTL)
			}
		})
	}
}

func TestStackSetUpdateFromResourcesClusterDomain(t *testing.T) {
	c := dummyStacksetContainer()
	c.clusterDomains = []string{"foo.example.org"}

	err := c.UpdateFromResources()
	require.NoError(t, err)

	for _, sc := range c.StackContainers {
		require.Equal(t, c.clusterDomains, sc.clusterDomains)
	}
}

func TestStackUpdateFromResources(t *testing.T) {
	runTest := func(name string, testFn func(t *testing.T, container *StackContainer)) {
		t.Run(name, func(t *testing.T) {
			testFn(t, testStack("foo-v1").stack())
		})
	}

	deployment := func(stackGeneration int64, generation int64, observedGeneration int64) *apps.Deployment {
		return &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Generation: generation,
				Annotations: map[string]string{
					stackGenerationAnnotationKey: strconv.FormatInt(stackGeneration, 10),
				},
			},
			Status: apps.DeploymentStatus{
				ObservedGeneration: observedGeneration,
			},
		}
	}
	service := func(stackGeneration int64) *v1.Service {
		return &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					stackGenerationAnnotationKey: strconv.FormatInt(stackGeneration, 10),
				},
			},
		}
	}
	ingress := func(stackGeneration int64) *networking.Ingress {
		return &networking.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					stackGenerationAnnotationKey: strconv.FormatInt(stackGeneration, 10),
				},
			},
		}
	}
	hpa := func(stackGeneration int64) *autoscaling.HorizontalPodAutoscaler {
		return &autoscaling.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					stackGenerationAnnotationKey: strconv.FormatInt(stackGeneration, 10),
				},
			},
		}
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

	runTest("noTrafficSince can be unset", func(t *testing.T, container *StackContainer) {
		container.updateFromResources()
		require.EqualValues(t, time.Time{}, container.noTrafficSince)
	})
	runTest("noTrafficSince is parsed from the status", func(t *testing.T, container *StackContainer) {
		container.Stack.Status.NoTrafficSince = &metav1.Time{Time: hourAgo}
		container.updateFromResources()
		require.EqualValues(t, hourAgo, container.noTrafficSince)
	})

	runTest("missing resources are handled fine", func(t *testing.T, container *StackContainer) {
		container.updateFromResources()
		require.EqualValues(t, false, container.resourcesUpdated)
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
		container.Resources.Deployment = deployment(10, 5, 5)
		container.Resources.Service = service(11)
		container.updateFromResources()
		require.EqualValues(t, false, container.resourcesUpdated)
	})
	runTest("deployment isn't considered updated if observedGeneration is different", func(t *testing.T, container *StackContainer) {
		container.Stack.Generation = 11
		container.Resources.Deployment = deployment(11, 5, 4)
		container.Resources.Service = service(11)
		container.updateFromResources()
		require.EqualValues(t, false, container.resourcesUpdated)
	})

	runTest("service isn't considered updated if the generation is different", func(t *testing.T, container *StackContainer) {
		container.Stack.Generation = 11
		container.Resources.Deployment = deployment(11, 5, 5)
		container.Resources.Service = service(10)
		container.updateFromResources()
		require.EqualValues(t, false, container.resourcesUpdated)
	})

	runTest("ingress isn't considered updated if the generation is different", func(t *testing.T, container *StackContainer) {
		container.Stack.Generation = 11
		container.ingressSpec = &zv1.StackSetIngressSpec{}
		container.Resources.Deployment = deployment(11, 5, 5)
		container.Resources.Service = service(11)
		container.Resources.Ingress = ingress(10)
		container.updateFromResources()
		require.EqualValues(t, false, container.resourcesUpdated)
	})
	runTest("ingress isn't considered updated if it should be gone", func(t *testing.T, container *StackContainer) {
		container.Stack.Generation = 11
		container.Resources.Deployment = deployment(11, 5, 5)
		container.Resources.Service = service(11)
		container.Resources.Ingress = ingress(11)
		container.updateFromResources()
		require.EqualValues(t, false, container.resourcesUpdated)
	})

	runTest("hpa isn't considered updated if the generation is different", func(t *testing.T, container *StackContainer) {
		container.Stack.Generation = 11
		container.Stack.Spec.Autoscaler = &zv1.Autoscaler{}
		container.Resources.Deployment = deployment(11, 5, 5)
		container.Resources.Service = service(11)
		container.Resources.HPA = hpa(10)
		container.updateFromResources()
		require.EqualValues(t, false, container.resourcesUpdated)
	})
	runTest("hpa isn't considered updated if it should be gone", func(t *testing.T, container *StackContainer) {
		container.Stack.Generation = 11
		container.Resources.Deployment = deployment(11, 5, 5)
		container.Resources.Service = service(11)
		container.Resources.HPA = hpa(11)
		container.updateFromResources()
		require.EqualValues(t, false, container.resourcesUpdated)
	})

	runTest("resources are recognised as updated correctly (deployment and service)", func(t *testing.T, container *StackContainer) {
		container.Stack.Generation = 11
		container.Resources.Deployment = deployment(11, 5, 5)
		container.Resources.Service = service(11)
		container.updateFromResources()
		require.EqualValues(t, true, container.resourcesUpdated)
	})
	runTest("resources are recognised as updated correctly (all resources)", func(t *testing.T, container *StackContainer) {
		container.Stack.Generation = 11
		container.ingressSpec = &zv1.StackSetIngressSpec{}
		container.Stack.Spec.Autoscaler = &zv1.Autoscaler{}
		container.Resources.Deployment = deployment(11, 5, 5)
		container.Resources.Service = service(11)
		container.Resources.Ingress = ingress(11)
		container.Resources.HPA = hpa(11)
		container.updateFromResources()
		require.EqualValues(t, true, container.resourcesUpdated)
	})

	runTest("prescaling information is parsed from the status", func(t *testing.T, container *StackContainer) {
		container.Stack.Status.Prescaling = zv1.PrescalingStatus{
			Active:               true,
			Replicas:             11,
			DesiredTrafficWeight: 23.5,
			LastTrafficIncrease:  &metav1.Time{Time: hourAgo},
		}
		container.updateFromResources()
		require.EqualValues(t, 1, container.stackReplicas)
		require.EqualValues(t, true, container.prescalingActive)
		require.EqualValues(t, 11, container.prescalingReplicas)
		require.EqualValues(t, 23.5, container.prescalingDesiredTrafficWeight)
		require.EqualValues(t, hourAgo, container.prescalingLastTrafficIncrease)
	})
}

func TestUpdateTrafficFromStackSet(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		desiredTraffic         []*zv1.DesiredTraffic
		actualTraffic          []*zv1.ActualTraffic
		expectedDesiredWeights map[string]float64
		expectedActualWeights  map[string]float64
	}{
		{
			name: "desired and actual weights are parsed correctly",
			desiredTraffic: []*zv1.DesiredTraffic{
				{
					StackName: "foo-v1",
					Weight:    float64(25),
				},
				{
					StackName: "foo-v2",
					Weight:    float64(50),
				},
				{
					StackName: "foo-v3",
					Weight:    float64(25),
				},
			},
			actualTraffic: []*zv1.ActualTraffic{
				{
					ServiceName: "foo-v1",
					Weight:      float64(62.5),
				},
				{
					ServiceName: "foo-v2",
					Weight:      float64(12.5),
				},
				{
					ServiceName: "foo-v3",
					Weight:      float64(25),
				},
			},
			expectedDesiredWeights: map[string]float64{"foo-v1": 25, "foo-v2": 50, "foo-v3": 25},
			expectedActualWeights:  map[string]float64{"foo-v1": 62.5, "foo-v2": 12.5, "foo-v3": 25},
		},
		{
			name: "unknown stacks are removed, remaining weights are renormalised",
			desiredTraffic: []*zv1.DesiredTraffic{
				{
					StackName: "foo-v4",
					Weight:    float64(50),
				},
				{
					StackName: "foo-v2",
					Weight:    float64(25),
				},
				{
					StackName: "foo-v3",
					Weight:    float64(25),
				},
			},
			actualTraffic: []*zv1.ActualTraffic{
				{
					ServiceName: "foo-v4",
					Weight:      float64(50),
				},
				{
					ServiceName: "foo-v2",
					Weight:      float64(12.5),
				},
				{
					ServiceName: "foo-v3",
					Weight:      float64(37.5),
				},
			},
			expectedDesiredWeights: map[string]float64{"foo-v2": 50, "foo-v3": 50},
			expectedActualWeights:  map[string]float64{"foo-v2": 25, "foo-v3": 75},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stack1 := testStack("foo-v1").stack()
			stack2 := testStack("foo-v2").stack()
			stack3 := testStack("foo-v3").stack()
			// need a service definition
			for _, s := range []*StackContainer{stack1, stack2, stack3} {
				s.Resources.Service = &v1.Service{
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{
							{
								Port: int32(testPort),
							},
						},
					},
				}
			}

			ssc := &StackSetContainer{
				StackSet: &zv1.StackSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{},
						Traffic: tc.desiredTraffic,
					},
					Status: zv1.StackSetStatus{
						Traffic: tc.actualTraffic,
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					"v1": stack1,
					"v2": stack2,
					"v3": stack3,
				},
				backendWeightsAnnotationKey: traffic.DefaultBackendWeightsAnnotationKey,
			}

			err := ssc.UpdateFromResources()
			require.NoError(t, err)

			for _, sc := range ssc.StackContainers {
				require.Equal(t, tc.expectedDesiredWeights[sc.Name()], sc.desiredTrafficWeight, "desired stack %s", sc.Stack.Name)
				require.Equal(t, tc.expectedActualWeights[sc.Name()], sc.actualTrafficWeight, "actual stack %s", sc.Stack.Name)
				require.Equal(t, tc.expectedActualWeights[sc.Name()], sc.currentActualTrafficWeight, "current stack %s", sc.Stack.Name)
			}
		})
	}
}

func TestGenerateStackSetStatus(t *testing.T) {
	c := &StackSetContainer{
		StackSet: &zv1.StackSet{
			Status: zv1.StackSetStatus{
				ObservedStackVersion: "v1",
			},
		},
		StackContainers: map[types.UID]*StackContainer{
			"v1": testStack("v1").pendingRemoval().ready(3).stack(),
			"v2": testStack("v2").ready(3).traffic(80, 90).stack(),
			"v3": testStack("v3").ready(3).stack(),
			"v4": testStack("v4").stack(),
			"v5": testStack("v5").ready(3).traffic(20, 10).stack(),
		},
		backendWeightsAnnotationKey: traffic.DefaultBackendWeightsAnnotationKey,
	}

	expected := &zv1.StackSetStatus{
		Stacks:               4,
		ReadyStacks:          3,
		StacksWithTraffic:    2,
		ObservedStackVersion: "v1",
		Traffic: []*zv1.ActualTraffic{
			{
				StackName:   "v2",
				ServiceName: "v2",
				ServicePort: intstr.FromInt(testPort),
				Weight:      90,
			},
			{
				StackName:   "v3",
				ServiceName: "v3",
				ServicePort: intstr.FromInt(testPort),
				Weight:      0,
			},

			{
				StackName:   "v4",
				ServiceName: "v4",
				ServicePort: intstr.FromInt(testPort),
				Weight:      0,
			},
			{
				StackName:   "v5",
				ServiceName: "v5",
				ServicePort: intstr.FromInt(testPort),
				Weight:      10,
			},
		},
	}
	status := c.GenerateStackSetStatus()
	require.Equal(t, expected.Stacks, status.Stacks)
	require.Equal(t, expected.ReadyStacks, status.ReadyStacks)
	require.Equal(t, expected.StacksWithTraffic, status.StacksWithTraffic)
	require.Equal(t, expected.ObservedStackVersion, status.ObservedStackVersion)
	require.Equal(t, expected.Traffic, status.Traffic)
}

func TestGenerateStackSetTraffic(t *testing.T) {
	c := &StackSetContainer{
		StackSet: &zv1.StackSet{
			Status: zv1.StackSetStatus{
				ObservedStackVersion: "v1",
			},
		},
		StackContainers: map[types.UID]*StackContainer{
			"v1": testStack("v1").pendingRemoval().ready(3).stack(),
			"v2": testStack("v2").ready(3).traffic(80, 90).stack(),
			"v3": testStack("v3").ready(3).stack(),
			"v4": testStack("v4").stack(),
			"v5": testStack("v5").ready(3).traffic(20, 10).stack(),
		},
		backendWeightsAnnotationKey: traffic.DefaultBackendWeightsAnnotationKey,
	}

	expected := []*zv1.DesiredTraffic{
		{
			StackName: "v2",
			Weight:    80,
		},
		{
			StackName: "v5",
			Weight:    20,
		},
	}
	require.Equal(t, expected, c.GenerateStackSetTraffic())
}

func TestStackSetGenerateIngress(t *testing.T) {
	c := &StackSetContainer{
		StackSet: &zv1.StackSet{
			TypeMeta: metav1.TypeMeta{
				APIVersion: APIVersion,
				Kind:       KindStackSet,
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
					EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
						Annotations: map[string]string{"ingress": "annotation"},
					},
					Hosts:       []string{"example.org", "example.com"},
					BackendPort: intstr.FromInt(testPort),
					Path:        "example",
				},
			},
		},
		StackContainers: map[types.UID]*StackContainer{
			"v1": testStack("foo-v1").traffic(0.125, 0.25).stack(),
			"v2": testStack("foo-v2").traffic(0.5, 0.125).stack(),
			"v3": testStack("foo-v3").traffic(0.625, 0.625).stack(),
			"v4": testStack("foo-v4").traffic(0, 0).stack(),
		},
		backendWeightsAnnotationKey: traffic.DefaultBackendWeightsAnnotationKey,
	}
	ingress, err := c.GenerateIngress()
	require.NoError(t, err)

	annotations := map[string]string{
		"ingress":                           "annotation",
		"zalando.org/backend-weights":       `{"foo-v1":0.25,"foo-v2":0.125,"foo-v3":0.625}`,
		"zalando.org/traffic-authoritative": "false",
	}

	expected := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
			Labels: map[string]string{
				"stackset":       "foo",
				"stackset-label": "foobar",
			},
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: APIVersion,
					Kind:       KindStackSet,
					Name:       "foo",
					UID:        "abc-123",
				},
			},
		},
		Spec: networking.IngressSpec{
			Rules: []networking.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path: "example",
									Backend: networking.IngressBackend{
										ServiceName: "foo-v1",
										ServicePort: intstr.FromInt(testPort),
									},
								},
								{
									Path: "example",
									Backend: networking.IngressBackend{
										ServiceName: "foo-v2",
										ServicePort: intstr.FromInt(testPort),
									},
								},
								{
									Path: "example",
									Backend: networking.IngressBackend{
										ServiceName: "foo-v3",
										ServicePort: intstr.FromInt(testPort),
									},
								},
							},
						},
					},
				},
				{
					Host: "example.org",
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path: "example",
									Backend: networking.IngressBackend{
										ServiceName: "foo-v1",
										ServicePort: intstr.FromInt(testPort),
									},
								},
								{
									Path: "example",
									Backend: networking.IngressBackend{
										ServiceName: "foo-v2",
										ServicePort: intstr.FromInt(testPort),
									},
								},
								{
									Path: "example",
									Backend: networking.IngressBackend{
										ServiceName: "foo-v3",
										ServicePort: intstr.FromInt(testPort),
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
		StackSet:                    &zv1.StackSet{},
		backendWeightsAnnotationKey: traffic.DefaultBackendWeightsAnnotationKey,
	}
	ingress, err := c.GenerateIngress()
	require.NoError(t, err)
	require.Nil(t, ingress)
}

func TestStackSetGenerateRouteGroup(t *testing.T) {
	c := &StackSetContainer{
		StackSet: &zv1.StackSet{
			TypeMeta: metav1.TypeMeta{
				APIVersion: APIVersion,
				Kind:       KindStackSet,
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
				RouteGroup: &zv1.RouteGroupSpec{
					Hosts: []string{"example.org", "example.com"},
					AdditionalBackends: []rgv1.RouteGroupBackend{
						{
							Name: "shunt",
							Type: rgv1.ShuntRouteGroupBackend,
						},
					},
					Routes: []rgv1.RouteGroupRouteSpec{
						{
							PathSubtree: "/example",
						},
						{
							Path: "/ok",
							Backends: []rgv1.RouteGroupBackendReference{
								{
									BackendName: "shunt",
								},
							},
						},
					},
					BackendPort: testPort,
				},
			},
		},
		StackContainers: map[types.UID]*StackContainer{
			"v1": testStack("foo-v1").traffic(12.5, 25).stack(),
			"v2": testStack("foo-v2").traffic(50, 13).stack(),
			"v3": testStack("foo-v3").traffic(62.5, 62).stack(),
			"v4": testStack("foo-v4").traffic(0, 0).stack(),
		},
	}
	routegroup, err := c.GenerateRouteGroup()
	require.NoError(t, err)

	expected := &rgv1.RouteGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
			Labels: map[string]string{
				"stackset":       "foo",
				"stackset-label": "foobar",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: APIVersion,
					Kind:       KindStackSet,
					Name:       "foo",
					UID:        "abc-123",
				},
			},
		},
		Spec: rgv1.RouteGroupSpec{
			Hosts: []string{"example.org", "example.com"},
			Backends: []rgv1.RouteGroupBackend{
				{
					Name:        "foo-v1",
					Type:        rgv1.ServiceRouteGroupBackend,
					ServiceName: "foo-v1",
					ServicePort: testPort,
				},
				{
					Name:        "foo-v2",
					Type:        rgv1.ServiceRouteGroupBackend,
					ServiceName: "foo-v2",
					ServicePort: testPort,
				},
				{
					Name:        "foo-v3",
					Type:        rgv1.ServiceRouteGroupBackend,
					ServiceName: "foo-v3",
					ServicePort: testPort,
				},
				{
					Name:        "foo-v4",
					Type:        rgv1.ServiceRouteGroupBackend,
					ServiceName: "foo-v4",
					ServicePort: testPort,
				},
				{
					Name: "shunt",
					Type: rgv1.ShuntRouteGroupBackend,
				},
			},
			DefaultBackends: []rgv1.RouteGroupBackendReference{
				{
					BackendName: "foo-v1",
					Weight:      25,
				},
				{
					BackendName: "foo-v2",
					Weight:      13,
				},
				{
					BackendName: "foo-v3",
					Weight:      62,
				},
			},
			Routes: []rgv1.RouteGroupRouteSpec{
				{
					PathSubtree: "/example",
				},
				{
					Path: "/ok",
					Backends: []rgv1.RouteGroupBackendReference{
						{
							BackendName: "shunt",
						},
					},
				},
			},
		},
	}
	require.Equal(t, expected, routegroup)
}
