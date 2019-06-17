package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	testStackSet = zv1.StackSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
			UID:       "123",
		},
	}
	baseTestStack = zv1.Stack{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "foo-v1",
			Namespace:       testStackSet.Namespace,
			UID:             "456",
			Generation:      1,
			OwnerReferences: stacksetOwned(testStackSet).OwnerReferences,
		},
	}
	updatedTestStack = *baseTestStack.DeepCopy()

	baseTestStackOwned    = stackOwned(baseTestStack)
	updatedTestStackOwned = stackOwned(baseTestStack)
)

func init() {
	baseTestStackOwned.Annotations = map[string]string{"stackset-controller.zalando.org/stack-generation": "1"}

	updatedTestStack.Generation = 2
	updatedTestStackOwned.Annotations = map[string]string{"stackset-controller.zalando.org/stack-generation": "2"}
}

func TestReconcileStackDeployment(t *testing.T) {
	exampleReplicas := int32(3)
	updatedReplicas := int32(4)

	examplePodTemplateSpec := v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "foo",
					Image: "nginx",
				},
			},
		},
	}
	updatedPodTemplateSpec := v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "bar",
					Image: "nginx",
				},
			},
		},
	}

	for _, tc := range []struct {
		name     string
		stack    zv1.Stack
		existing *apps.Deployment
		updated  *apps.Deployment
		expected *apps.Deployment
	}{
		{
			name:  "deployment is created if it doesn't exist",
			stack: baseTestStack,
			updated: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
			expected: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
		},
		{
			name:  "deployment is updated if the stack version changes",
			stack: updatedTestStack,
			existing: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
			updated: &apps.Deployment{
				ObjectMeta: updatedTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: updatedPodTemplateSpec,
				},
			},
			expected: &apps.Deployment{
				ObjectMeta: updatedTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: updatedPodTemplateSpec,
				},
			},
		},
		{
			name:  "deployment is updated if the replica count is set",
			stack: baseTestStack,
			existing: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
			updated: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &updatedReplicas,
					Template: updatedPodTemplateSpec,
				},
			},
			expected: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &updatedReplicas,
					Template: updatedPodTemplateSpec,
				},
			},
		},
		{
			name:  "deployment is not updated if the stack version remains the same and replica count is unset",
			stack: baseTestStack,
			existing: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
			updated: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: nil,
					Template: updatedPodTemplateSpec,
				},
			},
			expected: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnvironment()

			err := env.CreateStacksets([]zv1.StackSet{testStackSet})
			require.NoError(t, err)

			err = env.CreateStacks([]zv1.Stack{tc.stack})
			require.NoError(t, err)

			if tc.existing != nil {
				err = env.CreateDeployments([]apps.Deployment{*tc.existing})
				require.NoError(t, err)
			}

			err = env.controller.ReconcileStackDeployment(&tc.stack, tc.existing, func() *apps.Deployment {
				return tc.updated
			})
			require.NoError(t, err)

			updated, err := env.client.AppsV1().Deployments(tc.stack.Namespace).Get(tc.stack.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, tc.expected, updated)
		})
	}
}
