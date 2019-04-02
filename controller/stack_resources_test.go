package controller

import (
	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"testing"
)

func TestNewDeploymentFromStack(t *testing.T) {
	stack := zv1.Stack{
		Spec: zv1.StackSpec{
			Replicas: int32Ptr(7),
			PodTemplate: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers:    nil,
					RestartPolicy: "hello",
				},
			},
		},
	}
	expectedDeployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(7),
			Selector: nil,
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers:    nil,
					RestartPolicy: "hello",
				},
			},
		},
	}
	actualDeployment := NewDeploymentFromStack(stack)
	require.Equal(t, expectedDeployment, actualDeployment,
		"deployment generated by NewDeploymentFromStack differs from expected deployment")
}
