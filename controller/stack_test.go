package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTemplateInjectLabels(t *testing.T) {
	template := v1.PodTemplateSpec{}
	labels := map[string]string{"foo": "bar"}

	expectedTemplate := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
	}

	newTemplate := templateInjectLabels(template, labels)
	assert.Equal(t, expectedTemplate, newTemplate)
}

func TestApplyPodTemplateSpecDefaults(t *testing.T) {
	template := v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			ServiceAccountName: "foo",
		},
	}

	gracePeriod := int64(v1.DefaultTerminationGracePeriodSeconds)

	expectedTemplate := v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			RestartPolicy:                 v1.RestartPolicyAlways,
			TerminationGracePeriodSeconds: &gracePeriod,
			DNSPolicy:                     v1.DNSClusterFirst,
			SecurityContext:               &v1.PodSecurityContext{},
			SchedulerName:                 v1.DefaultSchedulerName,
			ServiceAccountName:            "foo",
			DeprecatedServiceAccount:      "foo",
		},
	}

	newTemplate := applyPodTemplateSpecDefaults(template)
	assert.Equal(t, expectedTemplate, newTemplate)
}

func TestApplyContainersDefaults(t *testing.T) {
	containers := []v1.Container{
		{
			Ports: []v1.ContainerPort{
				{},
			},
			Env: []v1.EnvVar{
				{
					ValueFrom: &v1.EnvVarSource{
						FieldRef: &v1.ObjectFieldSelector{},
					},
				},
			},
			ReadinessProbe: &v1.Probe{
				Handler: v1.Handler{
					HTTPGet: &v1.HTTPGetAction{},
				},
			},
			LivenessProbe: &v1.Probe{
				Handler: v1.Handler{
					HTTPGet: &v1.HTTPGetAction{},
				},
			},
		},
	}

	expectedContainers := []v1.Container{
		{
			Ports: []v1.ContainerPort{
				{
					Protocol: v1.ProtocolTCP,
				},
			},
			Env: []v1.EnvVar{
				{
					ValueFrom: &v1.EnvVarSource{
						FieldRef: &v1.ObjectFieldSelector{
							APIVersion: "v1",
						},
					},
				},
			},
			TerminationMessagePath:   v1.TerminationMessagePathDefault,
			TerminationMessagePolicy: v1.TerminationMessageReadFile,
			ImagePullPolicy:          v1.PullIfNotPresent,
			ReadinessProbe: &v1.Probe{
				Handler: v1.Handler{
					HTTPGet: &v1.HTTPGetAction{
						Scheme: v1.URISchemeHTTP,
					},
				},
				TimeoutSeconds:   1,
				PeriodSeconds:    10,
				SuccessThreshold: 1,
				FailureThreshold: 3,
			},
			LivenessProbe: &v1.Probe{
				Handler: v1.Handler{
					HTTPGet: &v1.HTTPGetAction{
						Scheme: v1.URISchemeHTTP,
					},
				},
				TimeoutSeconds:   1,
				PeriodSeconds:    10,
				SuccessThreshold: 1,
				FailureThreshold: 3,
			},
		},
	}

	applyContainersDefaults(containers)
	assert.Equal(t, expectedContainers, containers)
}
