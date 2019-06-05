package core

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
			ObjectMeta: metav1.ObjectMeta{
				Name:       "foo-v1",
				Namespace:  "bar",
				UID:        "abc-123",
				Generation: 11,
				Labels:     map[string]string{"stack-label": "foobar"},
			},
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
	expected := &extensions.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-v1",
			Namespace: "bar",
			Labels: map[string]string{
				"stack-label": "foobar",
			},
			Annotations: map[string]string{
				"ingress":                    "annotation",
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
		},
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
			ObjectMeta: metav1.ObjectMeta{
				Name:       "foo-v1",
				Namespace:  "bar",
				UID:        "abc-123",
				Generation: 11,
				Labels: map[string]string{
					StacksetHeritageLabelKey: "foo",
					StackVersionLabelKey:     "v1",
					"stack-label":            "foobar",
				},
			},
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
		ObjectMeta: metav1.ObjectMeta{
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
	}
	require.Equal(t, expected, service)
}
