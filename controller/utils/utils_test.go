package utils_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando-incubator/stackset-controller/controller/utils"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestGetServicePorts(tt *testing.T) {
	backendPort := intstr.FromInt(int(8080))
	backendPort2 := intstr.FromInt(int(8081))
	namedBackendPort := intstr.FromString("ingress")

	for _, ti := range []struct {
		msg           string
		stack         zv1.Stack
		backendPort   *intstr.IntOrString
		expectedPorts []v1.ServicePort
		err           error
	}{
		{
			msg: "test using ports from pod spec",
			stack: zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: zv1.StackSpec{
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
			stack: zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: zv1.StackSpec{
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
			stack: zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: zv1.StackSpec{
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
			stack: zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: zv1.StackSpec{
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
			stack: zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: zv1.StackSpec{
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
			ports, err := utils.GetServicePorts(ti.backendPort, ti.stack)
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

	expectedTemplate := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
	}

	newTemplate := utils.TemplateInjectLabels(template, labels)
	assert.Equal(t, expectedTemplate, newTemplate)
}

func TestLimitLabels(t *testing.T) {
	labels := map[string]string{
		"foo": "bar",
		"foz": "baz",
	}

	validKeys := map[string]struct{}{
		"foo": struct{}{},
	}

	assert.Len(t, utils.LimitLabels(labels, validKeys), 1)
}
