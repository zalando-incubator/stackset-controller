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
	deployment := newDeploymentFromStack(stack)
	require.Equal(t, stack.Name, deployment.Name,
		"newDeploymentFromStack should copy name")
	require.Equal(t, stack.Labels, deployment.Labels,
		"newDeploymentFromStack should copy top-level labels")
	require.Equal(t, stack.Labels, deployment.Spec.Template.Labels,
		"newDeploymentFromStack should copy pod template labels")
	require.Equal(t,
		map[string]string{stackVersionLabelKey: "v2"}, deployment.Spec.Selector.MatchLabels,
		"newDeploymentFromStack should copy selector labels in MatchLabels")
}

func TestAssignResourceOwnershipToStack(t *testing.T) {
	t.Run("add annotation to an object with nil Annotations", func(t *testing.T) {
		// FIXME
		//service := v1.Service{
		//	ObjectMeta: metav1.ObjectMeta{
		//		Annotations: nil,
		//	},
		//}
	})
	t.Run("add annotation to an object with empty Annotations", func(t *testing.T) {
		// FIXME
		//service := v1.Service{
		//	ObjectMeta: metav1.ObjectMeta{
		//		Annotations: make(map[string]string, 0),
		//	},
		//}
	})
	t.Run("add annotation to an object owned by another stack", func(t *testing.T) {
		// FIXME
		//service := v1.Service{
		//	ObjectMeta: metav1.ObjectMeta{
		//		Annotations: make(map[string]string, 0),
		//	},
		//}
	})
	t.Run("not add annotation to an object already owned by the stack", func(t *testing.T) {
		// FIXME
		//annotations := make(map[string]string, 0)
		//service := v1.Service{
		//	ObjectMeta: metav1.ObjectMeta{
		//		Annotations: annotations,
		//	},
		//}
	})
	t.Run("unrelated annotations are not changed", func(t *testing.T) {
		// TODO
	})
	t.Run("works on deployments too", func(t *testing.T) {
		// TODO
	})
}

func TestUpdateServiceSpecFromStack(t *testing.T) {
	t.Run("error when called with a nil service", func(t *testing.T) {
		// TODO
	})
	t.Run("error when backend port doesn't match service ports", func(t *testing.T) {
		// TODO
	})
	t.Run("labels and ports are updated", func(t *testing.T) {
		// TODO
	})
}

func TestGetStackGeneration(t *testing.T) {
	t.Run("returns 0 without generation annotation", func(t *testing.T) {
		// TODO
	})
	t.Run("returns 0 with a non-integer generation", func(t *testing.T) {
		// TODO
	})
	t.Run("returns the decoded generation", func(t *testing.T) {
		// TODO
	})
}
