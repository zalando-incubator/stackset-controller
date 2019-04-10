package controller

import (
	"github.com/zalando-incubator/stackset-controller/controller/entities"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

// SimpleTrafficReconciler is the most simple traffic reconciler which
// implements the default traffic switching supported in the
// stackset-controller.
type SimpleTrafficReconciler struct{}

// ReconcileDeployment does not do anything for the simple reconciler.
func (r SimpleTrafficReconciler) ReconcileDeployment(stacks map[types.UID]*entities.StackContainer, stack *zv1.Stack, traffic map[string]entities.TrafficStatus, deployment *appsv1.Deployment) error {
	return nil
}

// ReconcileHPA sets the min and max replicas as defined on the Stack.
func (r SimpleTrafficReconciler) ReconcileHPA(stack *zv1.Stack, hpa *autoscaling.HorizontalPodAutoscaler, deployment *appsv1.Deployment) error {
	hpa.Spec.MinReplicas = stack.Spec.HorizontalPodAutoscaler.MinReplicas
	hpa.Spec.MaxReplicas = stack.Spec.HorizontalPodAutoscaler.MaxReplicas
	return nil
}

// ReconcileIngress computes the ingress traffic to distribute to stacks.
func (r SimpleTrafficReconciler) ReconcileIngress(stacks map[types.UID]*entities.StackContainer, ingress *v1beta1.Ingress, traffic map[string]entities.TrafficStatus) (map[string]float64, map[string]float64, error) {
	stackStatuses := entities.GetStackStatuses(stacks)
	availableBackends, allBackends := computeBackendWeights(stackStatuses, traffic)
	return availableBackends, allBackends, nil
}

func computeBackendWeights(stacks []entities.StackStatus, traffic map[string]entities.TrafficStatus) (map[string]float64, map[string]float64) {
	backendWeights := make(map[string]float64, len(stacks))
	availableBackends := make(map[string]float64, len(stacks))
	for _, stack := range stacks {
		backendWeights[stack.Stack.Name] = traffic[stack.Stack.Name].DesiredWeight

		if stack.Available {
			availableBackends[stack.Stack.Name] = traffic[stack.Stack.Name].DesiredWeight
		}
	}

	// TODO: validate this logic
	if !allZero(backendWeights) {
		normalizeWeights(backendWeights)
	}

	if len(availableBackends) == 0 {
		availableBackends = backendWeights
	}

	// TODO: think of case were all are zero and the service/deployment is
	// deleted.
	normalizeWeights(availableBackends)

	return availableBackends, backendWeights
}

func (r SimpleTrafficReconciler) ReconcileStacks(ssc *entities.StackSetContainer) error {
	return nil
}
