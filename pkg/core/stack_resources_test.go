package core

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
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
				PodTemplate: v1.PodTemplateSpec{
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
				PodTemplate: v1.PodTemplateSpec{
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
				PodTemplate: v1.PodTemplateSpec{
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
				PodTemplate: v1.PodTemplateSpec{
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
				PodTemplate: v1.PodTemplateSpec{
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

func TestTemplateInjectLabels(t *testing.T) {
	template := v1.PodTemplateSpec{}
	labels := map[string]string{"foo": "bar"}

	expectedTemplate := &v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
	}

	newTemplate := templateInjectLabels(&template, labels)
	require.Equal(t, expectedTemplate, newTemplate)
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
	backendPort := intstr.FromInt(80)
	c := &StackContainer{
		Stack: &zv1.Stack{
			ObjectMeta: testStackMeta,
		},
		stacksetName: "foo",
		ingressSpec: &zv1.StackSetIngressSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      map[string]string{"ignored": "label"},
				Annotations: map[string]string{"ingress": "annotation"},
			},
			Hosts: []string{"foo.example.org", "foo.example.com"},
			Path:  "example",
		},
		backendPort:   &backendPort,
		clusterDomain: "example.org",
	}
	ingress, err := c.GenerateIngress()
	require.NoError(t, err)

	// Annotations are copied from the ingress as well
	expectedMeta := testResourceMeta.DeepCopy()
	expectedMeta.Annotations["ingress"] = "annotation"

	expected := &extensions.Ingress{
		ObjectMeta: *expectedMeta,
		Spec: extensions.IngressSpec{
			Rules: []extensions.IngressRule{
				{
					Host: "foo-v1.example.org",
					IngressRuleValue: extensions.IngressRuleValue{
						HTTP: &extensions.HTTPIngressRuleValue{
							Paths: []extensions.HTTPIngressPath{
								{
									Path: "example",
									Backend: extensions.IngressBackend{
										ServiceName: "foo-v1",
										ServicePort: backendPort,
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

func TestStackGenerateIngressNone(t *testing.T) {
	c := &StackContainer{}
	ingress, err := c.GenerateIngress()
	require.NoError(t, err)
	require.Nil(t, ingress)
}

func TestStackGenerateService(t *testing.T) {
	c := &StackContainer{
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
	}
	service, err := c.GenerateService()
	require.NoError(t, err)
	expected := &v1.Service{
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
	}
	require.Equal(t, expected, service)
}

func TestStackGenerateDeployment(t *testing.T) {
	for _, tc := range []struct {
		name               string
		hpaEnabled         bool
		stackReplicas      int32
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
						Strategy: strategy,
						PodTemplate: v1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"pod-label": "pod-foo",
								},
							},
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
					Replicas: wrapReplicas(tc.expectedReplicas),
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
									Image: "nginx",
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
