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
				APIVersion: apiVersion,
				Kind:       stackKind,
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
			Hosts:       []string{"example.org", "example.com"},
			BackendPort: intstr.FromInt(80),
			Path:        "example",
		},
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
					Host: "foo-v1.org",
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
							},
						},
					},
				},
				{
					Host: "foo-v1.com",
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
		expectedReplicas   *int32
	}{
		{
			name:               "stack scaled down to zero, deployment still running",
			stackReplicas:      0,
			deploymentReplicas: 3,
			expectedReplicas:   wrapReplicas(0),
		},
		{
			name:               "stack scaled down to zero, deployment already scaled down",
			stackReplicas:      0,
			deploymentReplicas: 0,
			expectedReplicas:   nil,
		},
		{
			name:               "stack scaled down because it doesn't have traffic, deployment still running",
			stackReplicas:      3,
			deploymentReplicas: 3,
			noTrafficSince:     time.Now().Add(-time.Hour),
			expectedReplicas:   wrapReplicas(0),
		},
		{
			name:               "stack scaled down because it doesn't have traffic, deployment already scaled down",
			stackReplicas:      3,
			deploymentReplicas: 0,
			noTrafficSince:     time.Now().Add(-time.Hour),
			expectedReplicas:   nil,
		},
		{
			name:               "stack scaled down to zero, deployment already scaled down",
			stackReplicas:      0,
			deploymentReplicas: 0,
			expectedReplicas:   nil,
		},
		{
			name:               "stack running, deployment has zero replicas",
			stackReplicas:      3,
			deploymentReplicas: 0,
			expectedReplicas:   wrapReplicas(3),
		},
		{
			name:               "stack running, deployment has zero replicas, hpa enabled",
			hpaEnabled:         true,
			stackReplicas:      3,
			deploymentReplicas: 0,
			expectedReplicas:   wrapReplicas(3),
		},
		{
			name:               "stack running, deployment has the same amount replicas",
			stackReplicas:      3,
			deploymentReplicas: 3,
			expectedReplicas:   nil,
		},
		{
			name:               "stack running, deployment has a different amount of replicas",
			stackReplicas:      3,
			deploymentReplicas: 5,
			expectedReplicas:   wrapReplicas(3),
		},
		{
			name:               "stack running, deployment has a different amount of replicas, hpa enabled",
			hpaEnabled:         true,
			stackReplicas:      3,
			deploymentReplicas: 5,
			expectedReplicas:   nil,
		},
		{
			name:               "stack running, deployment has zero replicas, prescaling enabled",
			stackReplicas:      3,
			prescalingActive:   true,
			prescalingReplicas: 7,
			deploymentReplicas: 0,
			expectedReplicas:   wrapReplicas(7),
		},
		{
			name:               "stack running, deployment has zero replicas, hpa enabled, prescaling enabled",
			hpaEnabled:         true,
			prescalingActive:   true,
			prescalingReplicas: 7,
			stackReplicas:      3,
			deploymentReplicas: 0,
			expectedReplicas:   wrapReplicas(7),
		},
		{
			name:               "stack running, deployment has the same amount replicas, prescaling enabled",
			stackReplicas:      3,
			prescalingActive:   true,
			prescalingReplicas: 7,
			deploymentReplicas: 7,
			expectedReplicas:   nil,
		},
		{
			name:               "stack running, deployment has a different amount of replicas, prescaling enabled",
			stackReplicas:      3,
			prescalingActive:   true,
			prescalingReplicas: 7,
			deploymentReplicas: 5,
			expectedReplicas:   wrapReplicas(7),
		},
		{
			name:               "stack running, deployment has a different amount of replicas, hpa enabled, prescaling enabled",
			hpaEnabled:         true,
			prescalingActive:   true,
			prescalingReplicas: 7,
			stackReplicas:      3,
			deploymentReplicas: 5,
			expectedReplicas:   nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: testStackMeta,
					Spec: zv1.StackSpec{
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
					Replicas: tc.expectedReplicas,
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
			require.Equal(t, expected, deployment)
		})
	}
}

func TestGenerateStackStatus(t *testing.T) {
	hourAgo := time.Now().Add(-time.Hour)

	for _, tc := range []struct {
		name                           string
		actualTrafficWeight            float64
		desiredTrafficWeight           float64
		noTrafficSince                 time.Time
		prescalingActive               bool
		prescalingReplicas             int32
		prescalingDesiredTrafficWeight float64
		prescalingLastTrafficIncrease  time.Time
	}{
		{
			name:                 "with traffic",
			actualTrafficWeight:  0.25,
			desiredTrafficWeight: 0.75,
		},
		{
			name:           "without traffic",
			noTrafficSince: hourAgo,
		},
		{
			name:                           "prescaled",
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
				actualTrafficWeight:            tc.actualTrafficWeight,
				desiredTrafficWeight:           tc.desiredTrafficWeight,
				createdReplicas:                3,
				readyReplicas:                  2,
				updatedReplicas:                1,
				desiredReplicas:                4,
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
