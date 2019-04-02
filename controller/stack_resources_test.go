package controller

import (
	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestNewDeploymentFromStack(t *testing.T) {
	stack := zv1.Stack{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"application":        "all-fun",
				stackVersionLabelKey: "v2",
			},
		},
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
	deployment := NewDeploymentFromStack(stack)
	require.Equal(t, stack.Name, deployment.Name,
		"NewDeploymentFromStack should copy name")
	require.Equal(t, stack.Labels, deployment.Labels,
		"NewDeploymentFromStack should copy top-level labels")
	require.Equal(t, stack.Labels, deployment.Spec.Template.Labels,
		"NewDeploymentFromStack should copy pod template labels")
	require.Equal(t,
		map[string]string{stackVersionLabelKey: "v2"}, deployment.Spec.Selector.MatchLabels,
		"NewDeploymentFromStack should copy selector labels in MatchLabels")
}
