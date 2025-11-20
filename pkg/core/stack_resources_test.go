package core

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	apps "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	testStackMeta = metav1.ObjectMeta{
		Name:       "foo-v1",
		Namespace:  "bar",
		UID:        "abc-123",
		Generation: 11,
		Labels: map[string]string{
			StacksetHeritageLabelKey: "foo",
			StackVersionLabelKey:     "v1",
			"stack-label":            "foobar",
		},
	}
	testResourceMeta = metav1.ObjectMeta{
		Name:      "foo-v1",
		Namespace: "bar",
		Labels: map[string]string{
			StacksetHeritageLabelKey: "foo",
			StackVersionLabelKey:     "v1",
			"stack-label":            "foobar",
		},
		Annotations: map[string]string{
			stackGenerationAnnotationKey: "11",
		},
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: APIVersion,
				Kind:       KindStack,
				Name:       "foo-v1",
				UID:        "abc-123",
			},
		},
	}
)

func TestGetServicePorts(tt *testing.T) {
	backendPort := intstr.FromInt(int(8080))
	backendPort2 := intstr.FromInt(int(8081))
	namedBackendPort := intstr.FromString("ingress")

	for _, ti := range []struct {
		msg           string
		stackSpec     zv1.StackSpecInternal
		backendPort   *intstr.IntOrString
		expectedPorts []v1.ServicePort
		err           error
	}{
		{
			msg: "test using ports from pod spec",
			stackSpec: zv1.StackSpecInternal{
				StackSpec: zv1.StackSpec{
					Service: nil,
					PodTemplate: zv1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Ports: []v1.ContainerPort{
										{
											ContainerPort: 8080,
										},
									},
								},
								{
									Ports: []v1.ContainerPort{
										{
											ContainerPort: 8081,
										},
									},
								},
							},
						},
					},
				},
			},
			expectedPorts: []v1.ServicePort{
				{
					Name:       "port-0-0",
					Protocol:   v1.ProtocolTCP,
					Port:       8080,
					TargetPort: backendPort,
				},
				{
					Name:       "port-1-0",
					Protocol:   v1.ProtocolTCP,
					Port:       8081,
					TargetPort: backendPort2,
				},
			},
			backendPort: nil,
		},
		{
			msg: "test using ports from pod spec with ingress",
			stackSpec: zv1.StackSpecInternal{
				StackSpec: zv1.StackSpec{
					Service: nil,
					PodTemplate: zv1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Ports: []v1.ContainerPort{
										{
											ContainerPort: 8080,
										},
									},
								},
							},
						},
					},
				},
			},
			expectedPorts: []v1.ServicePort{
				{
					Name:       "port-0-0",
					Protocol:   v1.ProtocolTCP,
					Port:       8080,
					TargetPort: backendPort,
				},
			},
			backendPort: &backendPort,
		},
		{
			msg: "test using ports from pod spec with named ingress port",
			stackSpec: zv1.StackSpecInternal{
				StackSpec: zv1.StackSpec{
					Service: nil,
					PodTemplate: zv1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Ports: []v1.ContainerPort{
										{
											Name:          "ingress",
											ContainerPort: 8080,
										},
									},
								},
							},
						},
					},
				},
			},
			expectedPorts: []v1.ServicePort{
				{
					Name:       "ingress",
					Protocol:   v1.ProtocolTCP,
					Port:       8080,
					TargetPort: backendPort,
				},
			},
			backendPort: &namedBackendPort,
		},
		{
			msg: "test using ports from pod spec with invalid named ingress port",
			stackSpec: zv1.StackSpecInternal{
				StackSpec: zv1.StackSpec{
					Service: nil,
					PodTemplate: zv1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Ports: []v1.ContainerPort{
										{
											Name:          "ingress-invalid",
											ContainerPort: 8080,
										},
									},
								},
							},
						},
					},
				},
			},
			expectedPorts: []v1.ServicePort{
				{
					Name:       "ingress",
					Protocol:   v1.ProtocolTCP,
					Port:       8080,
					TargetPort: backendPort,
				},
			},
			backendPort: &namedBackendPort,
			err:         errors.New("error"),
		},
		{
			msg: "test using ports from service definition",
			stackSpec: zv1.StackSpecInternal{
				StackSpec: zv1.StackSpec{
					Service: &zv1.StackServiceSpec{
						Ports: []v1.ServicePort{
							{
								Name:       "ingress",
								Protocol:   v1.ProtocolTCP,
								Port:       8080,
								TargetPort: backendPort,
							},
						},
					},
					PodTemplate: zv1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Ports: []v1.ContainerPort{
										{
											Name:          "ingress-invalid",
											ContainerPort: 8080,
										},
									},
								},
							},
						},
					},
				},
			},
			expectedPorts: []v1.ServicePort{
				{
					Name:       "ingress",
					Protocol:   v1.ProtocolTCP,
					Port:       8080,
					TargetPort: backendPort,
				},
			},
			backendPort: &namedBackendPort,
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			ports, err := getServicePorts(ti.stackSpec, ti.backendPort)
			if ti.err != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, ports, ti.expectedPorts)
			}
		})
	}
}

func TestObjectMetaInjectLabels(t *testing.T) {
	labels := map[string]string{"foo": "bar"}

	expectedObjectMeta := metav1.ObjectMeta{
		Labels: labels,
	}

	newObjectMeta := objectMetaInjectLabels(metav1.ObjectMeta{}, labels)
	require.Equal(t, expectedObjectMeta, newObjectMeta)
}

func TestLimitLabels(t *testing.T) {
	labels := map[string]string{
		"foo": "bar",
		"foz": "baz",
	}

	validKeys := map[string]struct{}{
		"foo": {},
	}

	require.Len(t, limitLabels(labels, validKeys), 1)
}

func TestStackGenerateIngress(t *testing.T) {
	for _, tc := range []struct {
		name             string
		ingressSpec      *zv1.StackSetIngressSpec
		stackAnnotations map[string]string

		expectDisabled      bool
		expectError         bool
		expectedAnnotations map[string]string
		expectedHosts       []string
	}{
		{
			name:           "no ingress spec",
			ingressSpec:    nil,
			expectDisabled: true,
		},
		{
			name: "basic",
			ingressSpec: &zv1.StackSetIngressSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"ingress": "annotation"},
				},
				Hosts: []string{"foo.example.org", "foo.example.com"},
				Path:  "example",
			},
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey: "11",
				"ingress":                    "annotation",
			},
			expectedHosts: []string{"foo-v1.example.org"},
		},
		{
			name: "cluster migration",
			ingressSpec: &zv1.StackSetIngressSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"ingress": "annotation"},
				},
				Hosts: []string{"foo.example.org", "foo.example.com"},
				Path:  "example",
			},
			stackAnnotations: map[string]string{
				forwardBackendAnnotation: "fwd-ingress",
			},
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey:  "11",
				"ingress":                     "annotation",
				"zalando.org/skipper-backend": "forward",
			},
			expectedHosts: []string{"foo-v1.example.org"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backendPort := int32(80)
			intStrBackendPort := intstr.FromInt(int(backendPort))
			c := &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: testStackMeta,
				},
				stacksetName:   "foo",
				ingressSpec:    tc.ingressSpec,
				backendPort:    &intStrBackendPort,
				clusterDomains: []string{"example.org"},
			}
			if tc.stackAnnotations != nil {
				c.Stack.Annotations = tc.stackAnnotations
			}

			ingress, err := c.GenerateIngress()

			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tc.expectDisabled {
				require.Nil(t, ingress)
				return
			}

			expectedMeta := testResourceMeta.DeepCopy()
			expectedMeta.Annotations = tc.expectedAnnotations

			expected := &networking.Ingress{
				ObjectMeta: *expectedMeta,
				Spec: networking.IngressSpec{
					Rules: []networking.IngressRule{},
				},
			}
			for _, host := range tc.expectedHosts {
				expected.Spec.Rules = append(expected.Spec.Rules, networking.IngressRule{
					Host: host,
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path:     "example",
									PathType: &PathTypeImplementationSpecific,
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "foo-v1",
											Port: networking.ServiceBackendPort{
												Number: backendPort,
											},
										},
									},
								},
							},
						},
					},
				})
			}
			require.Equal(t, expected, ingress)
		})
	}
}

func TestStackGenerateIngressSegment(t *testing.T) {
	for _, tc := range []struct {
		ingressSpec       *zv1.StackSetIngressSpec
		lowerLimit        float64
		upperLimit        float64
		expectNil         bool
		expectError       bool
		expectedPredicate string
		expectedHosts     []string
	}{
		{
			ingressSpec: nil,
			expectNil:   true,
		},
		{
			ingressSpec: &zv1.StackSetIngressSpec{
				Hosts: []string{},
			},
			expectNil: true,
		},
		{
			ingressSpec: &zv1.StackSetIngressSpec{
				Hosts: []string{"example.teapot.zalan.do"},
			},
			expectNil:         false,
			expectError:       false,
			expectedPredicate: "TrafficSegment(0.00, 0.00)",
			expectedHosts:     []string{"example.teapot.zalan.do"},
		},
		{
			ingressSpec: &zv1.StackSetIngressSpec{
				Hosts: []string{
					"example.teapot.zalan.do",
					"sample.teapot.zalan.do",
				},
			},
			lowerLimit:        0.1,
			upperLimit:        0.3,
			expectNil:         false,
			expectError:       false,
			expectedPredicate: "TrafficSegment(0.10, 0.30)",
			expectedHosts: []string{
				"example.teapot.zalan.do",
				"sample.teapot.zalan.do",
			},
		},
		{
			ingressSpec: &zv1.StackSetIngressSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{
						IngressPredicateKey: "Method(\"GET\")",
					},
				},
				Hosts: []string{"example.teapot.zalan.do"},
			},
			lowerLimit:        0.1,
			upperLimit:        0.3,
			expectNil:         false,
			expectError:       false,
			expectedPredicate: "TrafficSegment(0.10, 0.30) && Method(\"GET\")",
			expectedHosts:     []string{"example.teapot.zalan.do"},
		},
		{
			ingressSpec: &zv1.StackSetIngressSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{
						IngressPredicateKey: "",
					},
				},
				Hosts: []string{"example.teapot.zalan.do"},
			},
			lowerLimit:        0.1,
			upperLimit:        0.3,
			expectNil:         false,
			expectError:       false,
			expectedPredicate: "TrafficSegment(0.10, 0.30)",
			expectedHosts:     []string{"example.teapot.zalan.do"},
		},
	} {
		backendPort := intstr.FromInt(int(80))
		c := &StackContainer{
			Stack: &zv1.Stack{
				ObjectMeta: testStackMeta,
			},
			ingressSpec:       tc.ingressSpec,
			segmentLowerLimit: tc.lowerLimit,
			segmentUpperLimit: tc.upperLimit,
			backendPort:       &backendPort,
		}
		ingress, err := c.GenerateIngressSegment()

		if (err != nil) != tc.expectError {
			t.Errorf("expected error: %t , got %v", tc.expectError, err)
			continue
		}

		if (ingress == nil) != tc.expectNil {
			t.Errorf("expected nil: %t, got %v", tc.expectNil, ingress)
			continue
		}

		if ingress == nil {
			continue
		}

		if ingress.Annotations[IngressPredicateKey] != tc.expectedPredicate {
			t.Errorf("expected predicate %q, got %q",
				tc.expectedPredicate,
				ingress.Annotations[IngressPredicateKey],
			)
			continue
		}

		ingressHosts := []string{}
		for _, rules := range ingress.Spec.Rules {
			ingressHosts = append(ingressHosts, rules.Host)
		}
		slices.Sort(ingressHosts)
		slices.Sort(tc.expectedHosts)
		if !slices.Equal(ingressHosts, tc.expectedHosts) {
			t.Errorf(
				"expected hosts %v, got %v",
				tc.expectedHosts,
				ingressHosts,
			)
		}
	}
}

func TestGenerateIngressSegmentWithSyncAnnotations(t *testing.T) {
	for _, tc := range []struct {
		ingressSpec               *zv1.StackSetIngressSpec
		ingresssAnnotationsToSync []string
		syncAnnotationsInIngress  map[string]string
		expected                  map[string]string
	}{
		{
			ingressSpec: &zv1.StackSetIngressSpec{
				Hosts: []string{"example.teapot.zalan.do"},
			},
			ingresssAnnotationsToSync: []string{},
			syncAnnotationsInIngress:  map[string]string{},
			expected:                  map[string]string{},
		},
		{
			ingressSpec: &zv1.StackSetIngressSpec{
				Hosts: []string{"example.teapot.zalan.do"},
			},
			ingresssAnnotationsToSync: []string{"aSync"},
			syncAnnotationsInIngress:  map[string]string{"aSync": "1Sync"},
			expected:                  map[string]string{"aSync": "1Sync"},
		},
		{
			ingressSpec: &zv1.StackSetIngressSpec{
				Hosts: []string{"example.teapot.zalan.do"},
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{
						"a": "1",
					},
				},
			},
			ingresssAnnotationsToSync: []string{"aSync"},
			syncAnnotationsInIngress:  map[string]string{"aSync": "1Sync"},
			expected:                  map[string]string{"a": "1", "aSync": "1Sync"},
		},
		{
			ingressSpec: &zv1.StackSetIngressSpec{
				Hosts: []string{"example.teapot.zalan.do"},
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"aSync": "1Sync"},
				},
			},
			ingresssAnnotationsToSync: []string{"aSync"},
			syncAnnotationsInIngress:  map[string]string{},
			expected:                  map[string]string{},
		},
		{
			ingressSpec: &zv1.StackSetIngressSpec{
				Hosts: []string{"example.teapot.zalan.do"},
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"aSync": "1Sync", "a": "1"},
				},
			},
			ingresssAnnotationsToSync: []string{"aSync"},
			syncAnnotationsInIngress:  map[string]string{},
			expected:                  map[string]string{"a": "1"},
		},
	} {
		backendPort := intstr.FromInt(int(80))
		c := &StackContainer{
			Stack: &zv1.Stack{
				ObjectMeta: testStackMeta,
			},
			ingressSpec:              tc.ingressSpec,
			backendPort:              &backendPort,
			ingressAnnotationsToSync: tc.ingresssAnnotationsToSync,
			syncAnnotationsInIngress: tc.syncAnnotationsInIngress,
		}
		res, _ := c.GenerateIngressSegment()

		delete(
			res.Annotations,
			"stackset-controller.zalando.org/stack-generation",
		)
		delete(
			res.Annotations,
			IngressPredicateKey,
		)

		if !reflect.DeepEqual(tc.expected, res.Annotations) {
			t.Errorf("expected %v, got %v", tc.expected, res.Annotations)
		}
	}
}

func TestStackGenerateRouteGroup(t *testing.T) {
	for _, tc := range []struct {
		name                string
		routeGroupSpec      *zv1.RouteGroupSpec
		stackAnnotations    map[string]string
		expectDisabled      bool
		expectError         bool
		expectedAnnotations map[string]string
		expectedHosts       []string
		expectedRouteGroup  *rgv1.RouteGroup
	}{
		{
			name:           "no route group spec",
			routeGroupSpec: nil,
			expectDisabled: true,
		},
		{
			name: "basic",
			routeGroupSpec: &zv1.RouteGroupSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"routegroup": "annotation"},
				},
				Hosts: []string{"foo.example.org", "foo.example.com"},
				Routes: []rgv1.RouteGroupRouteSpec{
					{
						PathSubtree: "/example",
					},
				},
			},
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey: "11",
				"routegroup":                 "annotation",
			},
			expectedHosts: []string{"foo-v1.example.org"},
		},
		{
			name: "custom load balancer",
			routeGroupSpec: &zv1.RouteGroupSpec{
				LBAlgorithm:                       rgv1.ConsistentHashBackendAlgorithm,
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{},
				Hosts:                             []string{"foo.example.org"},
				Routes: []rgv1.RouteGroupRouteSpec{
					{
						PathSubtree: "/example",
					},
				},
			},
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey: "11",
			},
			expectedHosts: []string{"foo-v1.example.org"},
		},
		{
			name: "cluster migration",
			routeGroupSpec: &zv1.RouteGroupSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"routegroup": "annotation"},
				},
				Hosts: []string{"foo.example.org", "foo.example.com"},
				Routes: []rgv1.RouteGroupRouteSpec{
					{
						PathSubtree: "/example",
					},
				},
			},
			stackAnnotations: map[string]string{
				forwardBackendAnnotation: "fwd-routegroup",
			},
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey: "11",
				"routegroup":                 "annotation",
			},
			expectedHosts: []string{"foo-v1.example.org"},
			expectedRouteGroup: &rgv1.RouteGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-v1",
					Namespace: "bar",
					Annotations: map[string]string{
						stackGenerationAnnotationKey: "11",
						"routegroup":                 "annotation",
					},
					Labels: map[string]string{
						StacksetHeritageLabelKey: "foo",
						StackVersionLabelKey:     "v1",
						"stack-label":            "foobar",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: APIVersion,
							Kind:       KindStack,
							Name:       "foo-v1",
							UID:        "abc-123",
						},
					},
				},
				Spec: rgv1.RouteGroupSpec{
					Hosts: []string{"foo-v1.example.org"},
					Backends: []rgv1.RouteGroupBackend{
						{
							Name: "fwd",
							Type: rgv1.ForwardRouteGroupBackend,
						},
					},
					DefaultBackends: []rgv1.RouteGroupBackendReference{
						{
							BackendName: "fwd",
						},
					},
					Routes: []rgv1.RouteGroupRouteSpec{
						{
							Backends: []rgv1.RouteGroupBackendReference{
								{
									BackendName: "fwd",
								},
							},
							PathSubtree: "/example",
						},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backendPort := int32(80)
			intStrBackendPort := intstr.FromInt(int(backendPort))
			c := &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: testStackMeta,
				},
				stacksetName:   "foo",
				routeGroupSpec: tc.routeGroupSpec,
				backendPort:    &intStrBackendPort,
				clusterDomains: []string{"example.org"},
			}
			if tc.stackAnnotations != nil {
				if c.Stack.Annotations != nil {
					maps.Copy(c.Stack.Annotations, tc.stackAnnotations)
				} else {
					c.Stack.Annotations = tc.stackAnnotations
				}
			}
			rg, err := c.GenerateRouteGroup()
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tc.expectDisabled {
				require.Nil(t, rg)
				return
			}

			// Annotations are copied from the routegroup as well
			expectedMeta := testResourceMeta.DeepCopy()
			expectedMeta.Annotations = tc.expectedAnnotations

			expected := &rgv1.RouteGroup{
				ObjectMeta: *expectedMeta,
				Spec: rgv1.RouteGroupSpec{
					Hosts: tc.expectedHosts,
					Backends: []rgv1.RouteGroupBackend{
						{
							Name:        "foo-v1",
							Type:        rgv1.ServiceRouteGroupBackend,
							ServiceName: "foo-v1",
							ServicePort: int(backendPort),
							Algorithm:   tc.routeGroupSpec.LBAlgorithm,
						},
					},
					DefaultBackends: []rgv1.RouteGroupBackendReference{
						{
							BackendName: "foo-v1",
							Weight:      100,
						},
					},
					Routes: []rgv1.RouteGroupRouteSpec{
						{
							PathSubtree: "/example",
						},
					},
				},
			}

			if tc.expectedRouteGroup != nil {
				expected = tc.expectedRouteGroup
			}

			require.Equal(t, expected, rg)
		})
	}
}

func TestStackGenerateRouteGroupSegment(t *testing.T) {
	for _, tc := range []struct {
		rgSpec            *zv1.RouteGroupSpec
		stackAnnotations  map[string]string
		lowerLimit        float64
		upperLimit        float64
		expectNil         bool
		expectError       bool
		expectedPredicate string
		expectedHosts     []string
	}{
		{
			rgSpec:    nil,
			expectNil: true,
		},
		{
			rgSpec: &zv1.RouteGroupSpec{
				Hosts: []string{},
			},
			expectNil: true,
		},
		{
			rgSpec: &zv1.RouteGroupSpec{
				Hosts:  []string{"example.teapot.zalan.do"},
				Routes: []rgv1.RouteGroupRouteSpec{{}},
			},
			expectNil:         false,
			expectError:       false,
			expectedPredicate: "TrafficSegment(0.00, 0.00)",
			expectedHosts:     []string{"example.teapot.zalan.do"},
		},
		{
			rgSpec: &zv1.RouteGroupSpec{
				Hosts: []string{
					"example.teapot.zalan.do",
					"sample.teapot.zalan.do",
				},
				Routes: []rgv1.RouteGroupRouteSpec{{}},
			},
			expectNil:         false,
			expectError:       false,
			expectedPredicate: "TrafficSegment(0.00, 0.00)",
			expectedHosts: []string{
				"example.teapot.zalan.do",
				"sample.teapot.zalan.do",
			},
		},
		{
			rgSpec: &zv1.RouteGroupSpec{
				Hosts:  []string{"example.teapot.zalan.do"},
				Routes: []rgv1.RouteGroupRouteSpec{{}},
			},
			lowerLimit:        0.1,
			upperLimit:        0.3,
			expectNil:         false,
			expectError:       false,
			expectedPredicate: "TrafficSegment(0.10, 0.30)",
			expectedHosts: []string{
				"example.teapot.zalan.do",
			},
		},
		{
			rgSpec: &zv1.RouteGroupSpec{
				Hosts: []string{"example.teapot.zalan.do"},
				Routes: []rgv1.RouteGroupRouteSpec{
					{
						Predicates: []string{`Method("GET")`},
					},
				},
			},
			lowerLimit:        0.1,
			upperLimit:        0.3,
			expectNil:         false,
			expectError:       false,
			expectedPredicate: "TrafficSegment(0.10, 0.30)",
			expectedHosts: []string{
				"example.teapot.zalan.do",
			},
		},
		{
			rgSpec: &zv1.RouteGroupSpec{
				Hosts: []string{"example.teapot.zalan.do"},
				Routes: []rgv1.RouteGroupRouteSpec{
					{Predicates: []string{`Method("GET")`}},
					{Predicates: []string{`Method("PUT")`}},
				},
			},
			lowerLimit:        0.1,
			upperLimit:        0.3,
			expectNil:         false,
			expectError:       false,
			expectedPredicate: "TrafficSegment(0.10, 0.30)",
			expectedHosts: []string{
				"example.teapot.zalan.do",
			},
		},
		{
			rgSpec: &zv1.RouteGroupSpec{
				Hosts:  []string{"example.teapot.zalan.do"},
				Routes: []rgv1.RouteGroupRouteSpec{{}},
			},
			stackAnnotations: map[string]string{
				forwardBackendAnnotation: forwardBackendName,
			},
			lowerLimit:        0.1,
			upperLimit:        0.3,
			expectNil:         false,
			expectError:       false,
			expectedPredicate: "TrafficSegment(0.10, 0.30)",
			expectedHosts: []string{
				"example.teapot.zalan.do",
			},
		},
	} {
		backendPort := intstr.FromInt(int(80))
		c := &StackContainer{
			Stack: &zv1.Stack{
				ObjectMeta: testStackMeta,
			},
			routeGroupSpec:    tc.rgSpec,
			segmentLowerLimit: tc.lowerLimit,
			segmentUpperLimit: tc.upperLimit,
			backendPort:       &backendPort,
		}
		if tc.stackAnnotations != nil {
			if c.Stack.Annotations != nil {
				maps.Copy(c.Stack.Annotations, tc.stackAnnotations)
			} else {
				c.Stack.Annotations = tc.stackAnnotations
			}
		}
		rg, err := c.GenerateRouteGroupSegment()

		if (err != nil) != tc.expectError {
			t.Errorf("expected error: %t , got %v", tc.expectError, err)
			continue
		}

		if (rg == nil) != tc.expectNil {
			t.Errorf("expected nil: %t, got %v", tc.expectNil, rg)
			continue
		}

		if rg == nil {
			continue
		}

		slices.Sort(rg.Spec.Hosts)
		slices.Sort(tc.expectedHosts)
		if !slices.Equal(rg.Spec.Hosts, tc.expectedHosts) {
			t.Errorf(
				"expected hosts %v, got %v",
				tc.expectedHosts,
				rg.Spec.Hosts,
			)
		}

		for _, r := range rg.Spec.Routes {
			if !slices.Contains(r.Predicates, tc.expectedPredicate) {
				t.Errorf("predicate %q not found in route %v",
					tc.expectedPredicate,
					r,
				)
				break
			}
		}
	}
}
func TestGenerateRouteGroupSegmentWithSyncAnnotations(t *testing.T) {
	for _, tc := range []struct {
		rgSpec                      *zv1.RouteGroupSpec
		ingresssAnnotationsToSync   []string
		syncAnnotationsInRouteGroup map[string]string
		expected                    map[string]string
	}{
		{
			rgSpec: &zv1.RouteGroupSpec{
				Hosts:  []string{"example.teapot.zalan.do"},
				Routes: []rgv1.RouteGroupRouteSpec{{}},
			},
			ingresssAnnotationsToSync:   []string{},
			syncAnnotationsInRouteGroup: map[string]string{},
			expected:                    map[string]string{},
		},
		{
			rgSpec: &zv1.RouteGroupSpec{
				Hosts:  []string{"example.teapot.zalan.do"},
				Routes: []rgv1.RouteGroupRouteSpec{{}},
			},
			ingresssAnnotationsToSync:   []string{"aSync"},
			syncAnnotationsInRouteGroup: map[string]string{"aSync": "1Sync"},
			expected:                    map[string]string{"aSync": "1Sync"},
		},
		{
			rgSpec: &zv1.RouteGroupSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"a": "1"},
				},
				Hosts:  []string{"example.teapot.zalan.do"},
				Routes: []rgv1.RouteGroupRouteSpec{{}},
			},
			ingresssAnnotationsToSync:   []string{"aSync"},
			syncAnnotationsInRouteGroup: map[string]string{"aSync": "1Sync"},
			expected:                    map[string]string{"a": "1", "aSync": "1Sync"},
		},
		{
			rgSpec: &zv1.RouteGroupSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"aSync": "1Sync"},
				},
				Hosts:  []string{"example.teapot.zalan.do"},
				Routes: []rgv1.RouteGroupRouteSpec{{}},
			},
			ingresssAnnotationsToSync:   []string{"aSync"},
			syncAnnotationsInRouteGroup: map[string]string{},
			expected:                    map[string]string{},
		},
		{
			rgSpec: &zv1.RouteGroupSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"aSync": "1Sync", "a": "1"},
				},
				Hosts:  []string{"example.teapot.zalan.do"},
				Routes: []rgv1.RouteGroupRouteSpec{{}},
			},
			ingresssAnnotationsToSync:   []string{"aSync"},
			syncAnnotationsInRouteGroup: map[string]string{},
			expected:                    map[string]string{"a": "1"},
		},
	} {
		backendPort := intstr.FromInt(int(80))
		c := &StackContainer{
			Stack: &zv1.Stack{
				ObjectMeta: testStackMeta,
			},
			routeGroupSpec:              tc.rgSpec,
			backendPort:                 &backendPort,
			ingressAnnotationsToSync:    tc.ingresssAnnotationsToSync,
			syncAnnotationsInRouteGroup: tc.syncAnnotationsInRouteGroup,
		}
		res, _ := c.GenerateRouteGroupSegment()

		delete(
			res.Annotations,
			"stackset-controller.zalando.org/stack-generation",
		)

		if !reflect.DeepEqual(tc.expected, res.Annotations) {
			t.Errorf("expected %v, got %v", tc.expected, res.Annotations)
		}
	}
}

func TestStackGenerateService(t *testing.T) {
	svcAnnotations := map[string]string{
		"zalando.org/api-usage-monitoring-tag": "beta",
	}
	backendPort := intstr.FromInt(int(8080))
	for _, ti := range []struct {
		msg        string
		sc         *StackContainer
		expService *v1.Service
		err        error
	}{
		{
			msg: "test with annotations and labels on service",
			sc: &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: testStackMeta,
					Spec: zv1.StackSpecInternal{
						StackSpec: zv1.StackSpec{
							Service: &zv1.StackServiceSpec{
								EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
									Annotations: svcAnnotations,
								},
								Ports: []v1.ServicePort{
									{
										Port:       80,
										TargetPort: intstr.FromInt(8080),
									},
								},
							},
						},
					},
				},
				stacksetName: "foo",
				ingressSpec: &zv1.StackSetIngressSpec{
					BackendPort: intstr.FromInt(80),
				},
			},
			expService: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:            testResourceMeta.Name,
					Annotations:     mergeLabels(testResourceMeta.Annotations, svcAnnotations),
					Labels:          testResourceMeta.Labels,
					OwnerReferences: testResourceMeta.OwnerReferences,
					Namespace:       testResourceMeta.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						},
					},
					Selector: map[string]string{
						StacksetHeritageLabelKey: "foo",
						StackVersionLabelKey:     "v1",
					},
					Type: v1.ServiceTypeClusterIP,
				},
			},
		},
		{
			msg: "test without annotations and labels on service",
			sc: &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: testStackMeta,
					Spec: zv1.StackSpecInternal{
						StackSpec: zv1.StackSpec{
							Service: &zv1.StackServiceSpec{
								Ports: []v1.ServicePort{
									{
										Port:       80,
										TargetPort: intstr.FromInt(8080),
									},
								},
							},
						},
					},
				},
				stacksetName: "foo",
				ingressSpec: &zv1.StackSetIngressSpec{
					BackendPort: intstr.FromInt(80),
				},
			},
			expService: &v1.Service{
				ObjectMeta: testResourceMeta,
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Port:       80,
							TargetPort: intstr.FromInt(8080),
						},
					},
					Selector: map[string]string{
						StacksetHeritageLabelKey: "foo",
						StackVersionLabelKey:     "v1",
					},
					Type: v1.ServiceTypeClusterIP,
				},
			},
		},
		{
			msg: "test with empty/nil service spec",
			sc: &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: testStackMeta,
					Spec: zv1.StackSpecInternal{
						StackSpec: zv1.StackSpec{
							Service: nil,
							PodTemplate: zv1.PodTemplateSpec{
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Ports: []v1.ContainerPort{
												{
													ContainerPort: 8080,
												},
											},
										},
									},
								},
							},
						},
					},
				},
				stacksetName: "foo",
				ingressSpec: &zv1.StackSetIngressSpec{
					BackendPort: intstr.FromInt(80),
				},
			},
			expService: &v1.Service{
				ObjectMeta: testResourceMeta,
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       "port-0-0",
							Protocol:   v1.ProtocolTCP,
							Port:       8080,
							TargetPort: backendPort,
						},
					},
					Selector: map[string]string{
						StacksetHeritageLabelKey: "foo",
						StackVersionLabelKey:     "v1",
					},
					Type: v1.ServiceTypeClusterIP,
				},
			},
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			service, err := ti.sc.GenerateService()
			require.NoError(t, err)
			require.Equal(t, ti.expService, service)
		})
	}
}

func TestStackGenerateDeployment(t *testing.T) {
	for _, tc := range []struct {
		name               string
		hpaEnabled         bool
		stackReplicas      int32
		minReadySeconds    int32
		prescalingActive   bool
		prescalingReplicas int32
		deploymentReplicas int32
		noTrafficSince     time.Time
		expectedReplicas   int32
		maxUnavailable     int
		maxSurge           int
		stackAnnotations   map[string]string
	}{
		{
			name:               "stack scaled down to zero, deployment still running",
			stackReplicas:      0,
			deploymentReplicas: 3,
			expectedReplicas:   0,
		},
		{
			name:               "stack scaled down to zero, deployment already scaled down",
			stackReplicas:      0,
			deploymentReplicas: 0,
			expectedReplicas:   0,
		},
		{
			name:               "stack scaled down because it doesn't have traffic, deployment still running",
			stackReplicas:      3,
			deploymentReplicas: 3,
			noTrafficSince:     time.Now().Add(-time.Hour),
			expectedReplicas:   0,
		},
		{
			name:               "stack scaled down because it doesn't have traffic, deployment already scaled down",
			stackReplicas:      3,
			deploymentReplicas: 0,
			noTrafficSince:     time.Now().Add(-time.Hour),
			expectedReplicas:   0,
		},
		{
			name:               "stack scaled down to zero, deployment already scaled down",
			stackReplicas:      0,
			deploymentReplicas: 0,
			expectedReplicas:   0,
		},
		{
			name:               "stack running, deployment has zero replicas",
			stackReplicas:      3,
			deploymentReplicas: 0,
			expectedReplicas:   3,
		},
		{
			name:               "stack running, deployment has zero replicas, hpa enabled",
			hpaEnabled:         true,
			stackReplicas:      3,
			deploymentReplicas: 0,
			expectedReplicas:   3,
		},
		{
			name:               "stack running, deployment has the same amount replicas",
			stackReplicas:      3,
			deploymentReplicas: 3,
			expectedReplicas:   3,
		},
		{
			name:               "stack running, deployment has a different amount of replicas",
			stackReplicas:      3,
			deploymentReplicas: 5,
			expectedReplicas:   3,
		},
		{
			name:               "stack running, deployment has a different amount of replicas, hpa enabled",
			hpaEnabled:         true,
			stackReplicas:      3,
			deploymentReplicas: 5,
			expectedReplicas:   5,
		},
		{
			name:               "stack running, deployment has zero replicas, prescaling enabled",
			stackReplicas:      3,
			prescalingActive:   true,
			prescalingReplicas: 7,
			deploymentReplicas: 0,
			expectedReplicas:   7,
		},
		{
			name:               "stack running, deployment has zero replicas, hpa enabled, prescaling enabled",
			hpaEnabled:         true,
			prescalingActive:   true,
			prescalingReplicas: 7,
			stackReplicas:      3,
			deploymentReplicas: 0,
			expectedReplicas:   7,
		},
		{
			name:               "stack running, deployment has the same amount replicas, prescaling enabled",
			stackReplicas:      3,
			prescalingActive:   true,
			prescalingReplicas: 7,
			deploymentReplicas: 7,
			expectedReplicas:   7,
		},
		{
			name:               "stack running, deployment has a different amount of replicas, prescaling enabled",
			stackReplicas:      3,
			prescalingActive:   true,
			prescalingReplicas: 7,
			deploymentReplicas: 5,
			expectedReplicas:   7,
		},
		{
			name:               "stack running, deployment has a different amount of replicas, hpa enabled, prescaling enabled",
			hpaEnabled:         true,
			prescalingActive:   true,
			prescalingReplicas: 7,
			stackReplicas:      3,
			deploymentReplicas: 5,
			expectedReplicas:   5,
		},
		{
			name:               "max surge is specified",
			stackReplicas:      3,
			deploymentReplicas: 3,
			expectedReplicas:   3,
			maxSurge:           10,
		},
		{
			name:               "max unavailable is specified",
			stackReplicas:      3,
			deploymentReplicas: 3,
			expectedReplicas:   3,
			maxUnavailable:     10,
		},
		{
			name:               "max surge and max unavailable are specified",
			stackReplicas:      3,
			deploymentReplicas: 3,
			expectedReplicas:   3,
			maxSurge:           1,
			maxUnavailable:     10,
		},
		{
			name:            "minReadySeconds should be set",
			minReadySeconds: 5,
		},
		{
			name:               "cluster migration should scale down deployment to 1",
			stackReplicas:      3,
			deploymentReplicas: 3,
			stackAnnotations: map[string]string{
				forwardBackendAnnotation: "fwd-deployment",
			},
			expectedReplicas: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var strategy *apps.DeploymentStrategy
			if tc.maxUnavailable != 0 || tc.maxSurge != 0 {
				strategy = &apps.DeploymentStrategy{
					Type:          apps.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &apps.RollingUpdateDeployment{},
				}
				if tc.maxUnavailable != 0 {
					value := intstr.FromInt(tc.maxUnavailable)
					strategy.RollingUpdate.MaxUnavailable = &value
				}
				if tc.maxSurge != 0 {
					value := intstr.FromInt(tc.maxSurge)
					strategy.RollingUpdate.MaxSurge = &value
				}
			}
			c := &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: testStackMeta,
					Spec: zv1.StackSpecInternal{
						StackSpec: zv1.StackSpec{
							MinReadySeconds: tc.minReadySeconds,
							Strategy:        strategy,
							PodTemplate: zv1.PodTemplateSpec{
								EmbeddedObjectMeta: zv1.EmbeddedObjectMeta{
									Labels: map[string]string{
										"pod-label": "pod-foo",
									},
								},
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Name:  "foo",
											Image: "ghcr.io/zalando/skipper:latest",
										},
									},
								},
							},
						},
					},
				},
				stackReplicas:      tc.stackReplicas,
				prescalingActive:   tc.prescalingActive,
				prescalingReplicas: tc.prescalingReplicas,
				deploymentReplicas: tc.deploymentReplicas,
				noTrafficSince:     tc.noTrafficSince,
				scaledownTTL:       time.Minute,
			}
			if tc.hpaEnabled {
				c.Stack.Spec.StackSpec.Autoscaler = &zv1.Autoscaler{}
			}
			if tc.stackAnnotations != nil {
				if c.Stack.Annotations != nil {
					maps.Copy(c.Stack.Annotations, tc.stackAnnotations)
				} else {
					c.Stack.Annotations = tc.stackAnnotations
				}
			}
			deployment := c.GenerateDeployment()
			expected := &apps.Deployment{
				ObjectMeta: testResourceMeta,
				Spec: apps.DeploymentSpec{
					Replicas:        wrapReplicas(tc.expectedReplicas),
					MinReadySeconds: c.Stack.Spec.StackSpec.MinReadySeconds,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							StacksetHeritageLabelKey: "foo",
							StackVersionLabelKey:     "v1",
						},
					},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								StacksetHeritageLabelKey: "foo",
								StackVersionLabelKey:     "v1",
								"stack-label":            "foobar",
								"pod-label":              "pod-foo",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "foo",
									Image: "ghcr.io/zalando/skipper:latest",
								},
							},
						},
					},
				},
			}
			if strategy != nil {
				expected.Spec.Strategy = *strategy
			}
			require.Equal(t, expected, deployment)
		})
	}
}

func TestGenerateHPA(t *testing.T) {
	min := int32(1)
	max := int32(2)
	utilization := int32(50)

	exampleBehavior := &autoscaling.HorizontalPodAutoscalerBehavior{
		ScaleDown: &autoscaling.HPAScalingRules{
			Policies: []autoscaling.HPAScalingPolicy{
				{
					Type:          autoscaling.PercentScalingPolicy,
					Value:         10,
					PeriodSeconds: 60,
				},
			},
		},
	}
	for _, tc := range []struct {
		name                string
		autoscaler          *zv1.Autoscaler
		stackAnnotations    map[string]string
		expectedMinReplicas *int32
		expectedMaxReplicas int32
		noTrafficSince      time.Time
		expectedMetrics     []autoscaling.MetricSpec
		expectedBehavior    *autoscaling.HorizontalPodAutoscalerBehavior
	}{
		{
			name: "HPA with behavior and default stabilizationWindowSeconds",
			autoscaler: &zv1.Autoscaler{
				MinReplicas: &min,
				MaxReplicas: max,

				Metrics: []zv1.AutoscalerMetrics{
					{
						Type:               zv1.CPUAutoscalerMetric,
						AverageUtilization: &utilization,
					},
				},
				Behavior: exampleBehavior,
			},
			expectedMinReplicas: &min,
			expectedMaxReplicas: max,
			expectedMetrics: []autoscaling.MetricSpec{
				{
					Type: autoscaling.ResourceMetricSourceType,
					Resource: &autoscaling.ResourceMetricSource{
						Name: v1.ResourceCPU,
						Target: autoscaling.MetricTarget{
							Type:               autoscaling.UtilizationMetricType,
							AverageUtilization: &utilization,
						},
					},
				},
			},
			expectedBehavior: exampleBehavior,
		},
		{
			name: "HPA when stack scaled down",
			autoscaler: &zv1.Autoscaler{
				MinReplicas: &min,
				MaxReplicas: max,

				Metrics: []zv1.AutoscalerMetrics{
					{
						Type:               zv1.CPUAutoscalerMetric,
						AverageUtilization: &utilization,
					},
				},
				Behavior: exampleBehavior,
			},
			noTrafficSince:   time.Now().Add(-time.Hour),
			expectedMetrics:  nil,
			expectedBehavior: nil,
		},
		{
			name: "HPA when cluster migration should be scaled down",
			autoscaler: &zv1.Autoscaler{
				MinReplicas: &min,
				MaxReplicas: max,

				Metrics: []zv1.AutoscalerMetrics{
					{
						Type:               zv1.CPUAutoscalerMetric,
						AverageUtilization: &utilization,
					},
				},
				Behavior: exampleBehavior,
			},
			stackAnnotations: map[string]string{
				forwardBackendAnnotation: "fwd-hpa",
			},
			expectedMetrics: []autoscaling.MetricSpec{
				{
					Type: autoscaling.ResourceMetricSourceType,
					Resource: &autoscaling.ResourceMetricSource{
						Name: v1.ResourceCPU,
						Target: autoscaling.MetricTarget{
							Type:               autoscaling.UtilizationMetricType,
							AverageUtilization: &utilization,
						},
					},
				},
			},
			expectedBehavior: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			podTemplate := zv1.PodTemplateSpec{
				EmbeddedObjectMeta: zv1.EmbeddedObjectMeta{
					Labels: map[string]string{
						"pod-label": "pod-foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "foo",
							Image: "ghcr.io/zalando/skipper:latest",
						},
					},
				},
			}
			autoscalerContainer := &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: testStackMeta,
					Spec: zv1.StackSpecInternal{
						StackSpec: zv1.StackSpec{
							PodTemplate: podTemplate,
							Autoscaler:  tc.autoscaler,
						},
					},
				},
				noTrafficSince: tc.noTrafficSince,
				scaledownTTL:   time.Minute,
			}
			if tc.stackAnnotations != nil {
				if autoscalerContainer.Stack.Annotations != nil {
					maps.Copy(autoscalerContainer.Stack.Annotations, tc.stackAnnotations)
				} else {
					autoscalerContainer.Stack.Annotations = tc.stackAnnotations
				}
			}

			hpa, err := autoscalerContainer.GenerateHPA()
			require.NoError(t, err)
			if tc.expectedBehavior == nil {
				require.Nil(t, hpa)
			} else {
				require.Equal(t, tc.expectedMinReplicas, hpa.Spec.MinReplicas)
				require.Equal(t, tc.expectedMaxReplicas, hpa.Spec.MaxReplicas)
				require.Equal(t, tc.expectedMetrics, hpa.Spec.Metrics)
				require.Equal(t, tc.expectedBehavior, hpa.Spec.Behavior)
			}
		})
	}
}

func TestGenerateHPAToSegment(t *testing.T) {
	for _, tc := range []struct {
		name        string
		metricType  zv1.AutoscalerMetricType
		expectedRef string
	}{
		{
			name:        "HPA metric points to ingress segment",
			metricType:  zv1.IngressAutoscalerMetric,
			expectedRef: "foo-v1-traffic-segment",
		},
		{
			name:        "HPA metric points to routeGroup segment",
			metricType:  zv1.RouteGroupAutoscalerMetric,
			expectedRef: "foo-v1-traffic-segment",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metricValue := resource.NewQuantity(20, resource.DecimalSI)
			var minReplicas int32 = 1

			autoScalerContainer := &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: testStackMeta,
					Spec: zv1.StackSpecInternal{
						StackSpec: zv1.StackSpec{
							PodTemplate: zv1.PodTemplateSpec{
								EmbeddedObjectMeta: zv1.EmbeddedObjectMeta{
									Labels: map[string]string{
										"pod-label": "pod-foo",
									},
								},
								Spec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Name:  "foo",
											Image: "ghcr.io/zalando/skipper:latest",
										},
									},
								},
							},
							Autoscaler: &zv1.Autoscaler{
								MinReplicas: &minReplicas,
								MaxReplicas: 2,
								Metrics: []zv1.AutoscalerMetrics{
									{
										Type:    tc.metricType,
										Average: metricValue,
									},
								},
							},
						},
					},
				},
			}

			hpa, err := autoScalerContainer.GenerateHPA()
			require.NoError(t, err)
			require.NotEmpty(t, hpa.Spec.Metrics)
			require.Equal(
				t,
				tc.expectedRef,
				hpa.Spec.Metrics[0].Object.DescribedObject.Name,
			)
		})
	}
}

func TestGenerateStackStatus(t *testing.T) {
	hourAgo := time.Now().Add(-time.Hour)

	for _, tc := range []struct {
		name                           string
		labels                         map[string]string
		expectedLabelSelector          string
		actualTrafficWeight            float64
		desiredTrafficWeight           float64
		noTrafficSince                 time.Time
		prescalingActive               bool
		prescalingReplicas             int32
		prescalingDesiredTrafficWeight float64
		prescalingLastTrafficIncrease  time.Time
	}{
		{
			name:                  "with traffic",
			labels:                map[string]string{},
			expectedLabelSelector: "",
			actualTrafficWeight:   0.25,
			desiredTrafficWeight:  0.75,
		},
		{
			name: "without traffic",
			labels: map[string]string{
				StacksetHeritageLabelKey: "stackset-x",
			},
			expectedLabelSelector: "stackset=stackset-x",
			noTrafficSince:        hourAgo,
		},
		{
			name:                           "prescaled",
			labels:                         map[string]string{},
			expectedLabelSelector:          "",
			actualTrafficWeight:            0.25,
			desiredTrafficWeight:           0.25,
			prescalingActive:               true,
			prescalingReplicas:             3,
			prescalingDesiredTrafficWeight: 22.75,
			prescalingLastTrafficIncrease:  hourAgo,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: metav1.ObjectMeta{
						Labels: tc.labels,
					},
				},
				actualTrafficWeight:            tc.actualTrafficWeight,
				desiredTrafficWeight:           tc.desiredTrafficWeight,
				createdReplicas:                3,
				readyReplicas:                  2,
				updatedReplicas:                1,
				deploymentReplicas:             4,
				noTrafficSince:                 tc.noTrafficSince,
				prescalingActive:               tc.prescalingActive,
				prescalingReplicas:             tc.prescalingReplicas,
				prescalingDesiredTrafficWeight: tc.prescalingDesiredTrafficWeight,
				prescalingLastTrafficIncrease:  tc.prescalingLastTrafficIncrease,
			}
			status := c.GenerateStackStatus()
			expected := &zv1.StackStatus{
				ActualTrafficWeight:  tc.actualTrafficWeight,
				DesiredTrafficWeight: tc.desiredTrafficWeight,
				Replicas:             3,
				ReadyReplicas:        2,
				UpdatedReplicas:      1,
				DesiredReplicas:      4,
				NoTrafficSince:       wrapTime(tc.noTrafficSince),
				LabelSelector:        tc.expectedLabelSelector,
				Prescaling: zv1.PrescalingStatus{
					Active:               tc.prescalingActive,
					Replicas:             tc.prescalingReplicas,
					DesiredTrafficWeight: tc.prescalingDesiredTrafficWeight,
					LastTrafficIncrease:  wrapTime(tc.prescalingLastTrafficIncrease),
				},
			}
			require.Equal(t, expected, status)
		})
	}
}

func TestOwnConfigMap(t *testing.T) {
	c := &StackContainer{
		Stack: &zv1.Stack{
			ObjectMeta: testStackMeta,
		},
		stacksetName: "foo",
	}
	c.Stack.Annotations = map[string]string{
		"stack-annotation": "stack-foo",
	}

	cmLabels := map[string]string{
		"configmap-label": "config-lbl",
	}
	cmAnnotations := map[string]string{
		"configmap-annotations": "config-ann",
	}

	updatedLabels := c.Stack.Labels
	updatedLabels["configmap-label"] = cmLabels["configmap-label"]

	updatedAnnotations := map[string]string{
		stackGenerationAnnotationKey: strconv.FormatInt(c.Stack.Generation, 10),
		"configmap-annotations":      cmAnnotations["configmap-annotations"],
	}

	for _, tc := range []struct {
		name     string
		template *v1.ConfigMap
		result   *v1.ConfigMap
	}{
		{
			name: "ConfigMap is updated with Stack data",
			template: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "foo-v1-configmap",
					Namespace:   testStackMeta.Namespace,
					Labels:      cmLabels,
					Annotations: cmAnnotations,
				},
				Data: map[string]string{
					"testK": "testV",
				},
			},
			result: &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "foo-v1-configmap",
					Namespace:   testStackMeta.Namespace,
					Labels:      updatedLabels,
					Annotations: updatedAnnotations,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: APIVersion,
							Kind:       KindStack,
							Name:       c.Name(),
							UID:        c.Stack.UID,
						},
					},
				},
				Data: map[string]string{
					"testK": "testV",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objMeta := c.UpdateObjectMeta(&tc.template.ObjectMeta)
			require.Equal(t, objMeta.Name, tc.result.Name)
			require.Equal(t, objMeta.Labels, tc.result.Labels)
			require.Equal(t, objMeta.Annotations, tc.result.Annotations)
			require.Equal(t, objMeta.OwnerReferences, tc.result.OwnerReferences)
			require.Equal(t, tc.template.Data, tc.result.Data)
		})
	}
}

func TestOwnSecret(t *testing.T) {
	c := &StackContainer{
		Stack: &zv1.Stack{
			ObjectMeta: testStackMeta,
		},
		stacksetName: "foo",
	}
	c.Stack.Annotations = map[string]string{
		"stack-annotation": "stack-foo",
	}

	sctLabels := map[string]string{
		"secret-label": "sct-lbl",
	}
	sctAnnotations := map[string]string{
		"secret-annotations": "sct-ann",
	}

	updatedLabels := c.Stack.Labels
	updatedLabels["secret-label"] = sctLabels["secret-label"]

	updatedAnnotations := map[string]string{
		stackGenerationAnnotationKey: strconv.FormatInt(c.Stack.Generation, 10),
		"secret-annotations":         sctAnnotations["secret-annotations"],
	}

	for _, tc := range []struct {
		name     string
		template *v1.Secret
		result   *v1.Secret
	}{
		{
			name: "Secret is updated with Stack data",
			template: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "foo-v1-secret",
					Namespace:   testStackMeta.Namespace,
					Labels:      sctLabels,
					Annotations: sctAnnotations,
				},
				Data: map[string][]byte{
					"testK": {0},
				},
			},
			result: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "foo-v1-secret",
					Namespace:   testStackMeta.Namespace,
					Labels:      updatedLabels,
					Annotations: updatedAnnotations,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: APIVersion,
							Kind:       KindStack,
							Name:       c.Name(),
							UID:        c.Stack.UID,
						},
					},
				},
				Data: map[string][]byte{
					"testK": {0},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objMeta := c.UpdateObjectMeta(&tc.template.ObjectMeta)
			require.Equal(t, objMeta.Name, tc.result.Name)
			require.Equal(t, objMeta.Labels, tc.result.Labels)
			require.Equal(t, objMeta.Annotations, tc.result.Annotations)
			require.Equal(t, objMeta.OwnerReferences, tc.result.OwnerReferences)
			require.Equal(t, tc.template.Data, tc.result.Data)
		})
	}
}

func TestGeneratePCS(t *testing.T) {
	sc := &StackContainer{
		Stack: &zv1.Stack{
			ObjectMeta: testStackMeta,
			Spec: zv1.StackSpecInternal{
				StackSpec: zv1.StackSpec{
					ConfigurationResources: []zv1.ConfigurationResourcesSpec{
						{
							PlatformCredentialsSet: &zv1.PCS{
								Name: "foo-v1-pcs-test",
								Tokens: map[string]zv1.Token{
									"token01": {
										Privileges: []string{
											"read-write",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	sc.Stack.Labels["application"] = "pcs-app"
	expectedAnn := map[string]string{
		stackGenerationAnnotationKey: strconv.FormatInt(sc.Stack.Generation, 10),
	}

	for _, tc := range []struct {
		name     string
		template *zv1.PCS
		result   *zv1.PlatformCredentialsSet
	}{
		{
			name:     "platformCredentialsSet is created from Stack data",
			template: sc.Stack.Spec.ConfigurationResources[0].PlatformCredentialsSet,
			result: &zv1.PlatformCredentialsSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "foo-v1-pcs-test",
					Namespace:   testStackMeta.Namespace,
					Labels:      sc.Stack.Labels,
					Annotations: expectedAnn,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: APIVersion,
							Kind:       KindStack,
							Name:       sc.Name(),
							UID:        sc.Stack.UID,
						},
					},
				},
				Spec: zv1.PlatformCredentialsSpec{
					Application:  sc.Stack.Labels["application"],
					TokenVersion: "v2",
					Tokens: map[string]zv1.Token{
						"token01": {
							Privileges: []string{
								"read-write",
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pcs, _ := sc.GeneratePlatformCredentialsSet(tc.template)
			require.Equal(t, tc.result.Name, pcs.Name)
			require.Equal(t, tc.result.Labels, pcs.Labels)
			require.Equal(t, tc.result.Annotations, pcs.Annotations)
			require.Equal(t, tc.result.OwnerReferences, pcs.OwnerReferences)
			require.Equal(t, tc.result.Spec, pcs.Spec)
		})
	}
}
