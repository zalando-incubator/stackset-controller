package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zalando-incubator/stackset-controller/controller/entities"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"k8s.io/api/apps/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestNewDeploymentFromStack(t *testing.T) {
	stack := zv1.Stack{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"application":                 "all-fun",
				entities.StackVersionLabelKey: "v2",
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
		map[string]string{entities.StackVersionLabelKey: "v2"}, deployment.Spec.Selector.MatchLabels,
		"newDeploymentFromStack should copy selector labels in MatchLabels")
}

func newDummyServiceWithAnnotations(annotations map[string]string) v1.Service {
	return v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: annotations,
		},
	}
}

func newDummyStackWithGeneration(generation int64) zv1.Stack {
	return zv1.Stack{
		ObjectMeta: metav1.ObjectMeta{
			Generation: generation,
		},
	}
}

func newDummyGenerationAnnotations(generation string) map[string]string {
	return map[string]string{entities.StackGenerationAnnotationKey: generation}
}

func TestAssignResourceOwnershipToStack(t *testing.T) {
	t.Run("add annotation to an object with nil Annotations", func(t *testing.T) {
		service := newDummyServiceWithAnnotations(nil)
		setStackGenerationOnResource(newDummyStackWithGeneration(7), &service)
		require.Equal(t, newDummyGenerationAnnotations("7"), service.Annotations)
	})
	t.Run("add annotation to an object with empty Annotations", func(t *testing.T) {
		service := newDummyServiceWithAnnotations(make(map[string]string))
		setStackGenerationOnResource(newDummyStackWithGeneration(7), &service)
		require.Equal(t, newDummyGenerationAnnotations("7"), service.Annotations)
	})
	t.Run("overwrite existing stack generation annotation on an object", func(t *testing.T) {
		service := newDummyServiceWithAnnotations(newDummyGenerationAnnotations("1"))
		setStackGenerationOnResource(newDummyStackWithGeneration(2), &service)
		require.Equal(t, newDummyGenerationAnnotations("2"), service.Annotations)
	})
	t.Run("unrelated annotations are not changed", func(t *testing.T) {
		actualAnnotations := newDummyGenerationAnnotations("1")
		actualAnnotations["other"] = "unchanged"
		service := newDummyServiceWithAnnotations(actualAnnotations)

		setStackGenerationOnResource(newDummyStackWithGeneration(2), &service)

		expectedAnnotations := newDummyGenerationAnnotations("2")
		expectedAnnotations["other"] = "unchanged"
		require.Equal(t, expectedAnnotations, service.Annotations)
	})
	t.Run("works on deployments too", func(t *testing.T) {
		deployment := v1beta1.Deployment{}
		setStackGenerationOnResource(newDummyStackWithGeneration(3), &deployment)
		require.Equal(t, newDummyGenerationAnnotations("3"), deployment.Annotations)
	})
}

func TestGetStackGeneration(t *testing.T) {
	t.Run("returns 0 without generation annotation", func(t *testing.T) {
		service := newDummyServiceWithAnnotations(nil)
		require.Equal(t, int64(0), getStackGeneration(service.ObjectMeta))
	})
	t.Run("returns 0 with a non-integer generation", func(t *testing.T) {
		service := newDummyServiceWithAnnotations(newDummyGenerationAnnotations("hi"))
		require.Equal(t, int64(0), getStackGeneration(service.ObjectMeta))
	})
	t.Run("returns the decoded generation", func(t *testing.T) {
		service := newDummyServiceWithAnnotations(newDummyGenerationAnnotations("192"))
		require.Equal(t, int64(192), getStackGeneration(service.ObjectMeta))
	})
}

func TestUpdateServiceSpecFromStack(t *testing.T) {
	t.Run("error when called with a nil service", func(t *testing.T) {
		require.Error(t, updateServiceSpecFromStack(nil, zv1.Stack{}, nil))
	})
	t.Run("error when backend port doesn't match service ports", func(t *testing.T) {
		service := v1.Service{
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{{Port: 123}},
			},
		}
		backendPort := intstr.FromInt(987)
		require.Error(t, updateServiceSpecFromStack(
			&service,
			newDummyStackWithGeneration(0),
			&backendPort,
		))
	})
	t.Run("labels and ports are updated", func(t *testing.T) {
		service := v1.Service{}
		stack := zv1.Stack{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"a": "b", "cd": "ef"},
			},
			Spec: zv1.StackSpec{
				Service: &zv1.StackServiceSpec{
					Ports: []v1.ServicePort{{Port: 123}, {Name: "web"}},
				},
			},
		}

		err := updateServiceSpecFromStack(&service, stack, nil)

		require.Nilf(t, err, "updateServiceSpecFromStack should not return an error")
		require.Equal(t, map[string]string{"a": "b", "cd": "ef"}, service.Labels)
		require.Equal(t, []v1.ServicePort{{Port: 123}, {Name: "web"}}, service.Spec.Ports)
	})
}
