package core

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	apps "k8s.io/api/apps/v1"
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	autoscaling "k8s.io/api/autoscaling/v2beta2"
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
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
		stackSpec     zv1.StackSpec
		backendPort   *intstr.IntOrString
		expectedPorts []v1.ServicePort
		err           error
	}{
		{
			msg: "test using ports from pod spec",
			stackSpec: zv1.StackSpec{
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
			stackSpec: zv1.StackSpec{
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
			stackSpec: zv1.StackSpec{
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
			stackSpec: zv1.StackSpec{
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
			stackSpec: zv1.StackSpec{
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
	boolFalse := false

	for _, tc := range []struct {
		name             string
		ingressSpec      *zv1.StackSetIngressSpec
		ingressOverrides *zv1.StackIngressRouteGroupOverrides

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
			name: "basic, no overrides",
			ingressSpec: &zv1.StackSetIngressSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"ingress": "annotation"},
				},
				Hosts: []string{"foo.example.org", "foo.example.com"},
				Path:  "example",
			},
			ingressOverrides: nil,
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey: "11",
				"ingress":                    "annotation",
			},
			expectedHosts: []string{"foo-v1.example.org"},
		},
		{
			name: "disabled by overrides",
			ingressSpec: &zv1.StackSetIngressSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"ingress": "annotation"},
				},
				Hosts: []string{"foo.example.org", "foo.example.com"},
				Path:  "example",
			},
			ingressOverrides: &zv1.StackIngressRouteGroupOverrides{
				Enabled: &boolFalse,
			},
			expectDisabled: true,
		},
		{
			name: "custom domains via overrides",
			ingressSpec: &zv1.StackSetIngressSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"ingress": "annotation"},
				},
				Hosts: []string{"foo.example.org", "foo.example.com"},
				Path:  "example",
			},
			ingressOverrides: &zv1.StackIngressRouteGroupOverrides{
				Hosts: []string{
					"test-$(STACK_NAME).internal.foobar",
					"$(STACK_NAME).internal.test",
				},
			},
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey: "11",
				"ingress":                    "annotation",
			},
			expectedHosts: []string{"foo-v1.internal.test", "test-foo-v1.internal.foobar"},
		},
		{
			name: "custom domains in the overrides must include the stack_name token",
			ingressSpec: &zv1.StackSetIngressSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"ingress": "annotation"},
				},
				Hosts: []string{"foo.example.org", "foo.example.com"},
				Path:  "example",
			},
			ingressOverrides: &zv1.StackIngressRouteGroupOverrides{
				Hosts: []string{
					"test.internal.foobar",
					"$(STACK_NAME).internal.test",
				},
			},
			expectError: true,
		},
		{
			name: "custom annotations via overrides",
			ingressSpec: &zv1.StackSetIngressSpec{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"ingress": "annotation"},
				},
				Hosts: []string{"foo.example.org", "foo.example.com"},
				Path:  "example",
			},
			ingressOverrides: &zv1.StackIngressRouteGroupOverrides{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"custom": "override"},
				},
			},
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey: "11",
				"custom":                     "override",
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
					Spec: zv1.StackSpec{
						IngressOverrides: tc.ingressOverrides,
					},
				},
				stacksetName:   "foo",
				ingressSpec:    tc.ingressSpec,
				backendPort:    &intStrBackendPort,
				clusterDomains: []string{"example.org"},
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

func TestStackGenerateRouteGroup(t *testing.T) {
	boolFalse := false

	for _, tc := range []struct {
		name                string
		routeGroupSpec      *zv1.RouteGroupSpec
		routeGroupOverrides *zv1.StackIngressRouteGroupOverrides

		expectDisabled      bool
		expectError         bool
		expectedAnnotations map[string]string
		expectedHosts       []string
	}{
		{
			name:           "no route group spec",
			routeGroupSpec: nil,
			expectDisabled: true,
		},
		{
			name: "basic, no overrides",
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
			routeGroupOverrides: nil,
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey: "11",
				"routegroup":                 "annotation",
			},
			expectedHosts: []string{"foo-v1.example.org"},
		},
		{
			name: "disabled by overrides",
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
			routeGroupOverrides: &zv1.StackIngressRouteGroupOverrides{
				Enabled: &boolFalse,
			},
			expectDisabled: true,
		},
		{
			name: "custom domains via overrides",
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
			routeGroupOverrides: &zv1.StackIngressRouteGroupOverrides{
				Hosts: []string{
					"test-$(STACK_NAME).internal.foobar",
					"$(STACK_NAME).internal.test",
				},
			},
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey: "11",
				"routegroup":                 "annotation",
			},
			expectedHosts: []string{"foo-v1.internal.test", "test-foo-v1.internal.foobar"},
		},
		{
			name: "custom domains in the overrides must include the stack_name token",
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
			routeGroupOverrides: &zv1.StackIngressRouteGroupOverrides{
				Hosts: []string{
					"test.internal.foobar",
					"$(STACK_NAME).internal.test",
				},
			},
			expectError: true,
		},
		{
			name: "custom annotations via overrides",
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
			routeGroupOverrides: &zv1.StackIngressRouteGroupOverrides{
				EmbeddedObjectMetaWithAnnotations: zv1.EmbeddedObjectMetaWithAnnotations{
					Annotations: map[string]string{"custom": "override"},
				},
			},
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey: "11",
				"custom":                     "override",
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
			routeGroupOverrides: nil,
			expectedAnnotations: map[string]string{
				stackGenerationAnnotationKey: "11",
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
					Spec: zv1.StackSpec{
						RouteGroupOverrides: tc.routeGroupOverrides,
					},
				},
				stacksetName:   "foo",
				routeGroupSpec: tc.routeGroupSpec,
				backendPort:    &intStrBackendPort,
				clusterDomains: []string{"example.org"},
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
			require.Equal(t, expected, rg)
		})
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
					Spec: zv1.StackSpec{
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
					Spec: zv1.StackSpec{
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
					Spec: zv1.StackSpec{
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
					Spec: zv1.StackSpec{
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
										Image: "registry.opensource.zalan.do/teapot/skipper:latest",
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
				c.Stack.Spec.HorizontalPodAutoscaler = &zv1.HorizontalPodAutoscaler{}
			}
			deployment := c.GenerateDeployment()
			expected := &apps.Deployment{
				ObjectMeta: testResourceMeta,
				Spec: apps.DeploymentSpec{
					Replicas:        wrapReplicas(tc.expectedReplicas),
					MinReadySeconds: c.Stack.Spec.MinReadySeconds,
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
									Image: "registry.opensource.zalan.do/teapot/skipper:latest",
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
		hpa                 *zv1.HorizontalPodAutoscaler
		expectedMinReplicas *int32
		expectedMaxReplicas int32
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
			hpa: &zv1.HorizontalPodAutoscaler{
				MinReplicas: &min,
				MaxReplicas: max,
				Metrics: []autoscalingv2beta1.MetricSpec{
					{
						Type: autoscalingv2beta1.ResourceMetricSourceType,
						Resource: &autoscalingv2beta1.ResourceMetricSource{
							Name:                     v1.ResourceCPU,
							TargetAverageUtilization: &utilization,
						},
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
							Image: "registry.opensource.zalan.do/teapot/skipper:latest",
						},
					},
				},
			}
			autoscalerContainer := &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: testStackMeta,
					Spec: zv1.StackSpec{
						PodTemplate: podTemplate,
						Autoscaler:  tc.autoscaler,
					},
				},
			}

			hpa, err := autoscalerContainer.GenerateHPA()
			require.NoError(t, err)
			require.Equal(t, tc.expectedMinReplicas, hpa.Spec.MinReplicas)
			require.Equal(t, tc.expectedMaxReplicas, hpa.Spec.MaxReplicas)
			require.Equal(t, tc.expectedMetrics, hpa.Spec.Metrics)
			require.Equal(t, tc.expectedBehavior, hpa.Spec.Behavior)

			hpaContainer := &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: testStackMeta,
					Spec: zv1.StackSpec{
						PodTemplate:             podTemplate,
						HorizontalPodAutoscaler: tc.hpa,
					},
				},
			}

			hpa, err = hpaContainer.GenerateHPA()
			require.NoError(t, err)
			require.Equal(t, tc.expectedMinReplicas, hpa.Spec.MinReplicas)
			require.Equal(t, tc.expectedMaxReplicas, hpa.Spec.MaxReplicas)
			require.Equal(t, tc.expectedMetrics, hpa.Spec.Metrics)
			require.Equal(t, tc.expectedBehavior, hpa.Spec.Behavior)
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
