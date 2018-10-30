package controller

import (
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

type TrafficReconciler interface {
	ReconcileDeployment(stacks map[types.UID]*StackContainer, stack *zv1.Stack, traffic map[string]TrafficStatus, deployment *appsv1.Deployment) error
	ReconcileHPA(stack *zv1.Stack, hpa *autoscaling.HorizontalPodAutoscaler, deployment *appsv1.Deployment) error
	ReconcileIngress(stacks map[types.UID]*StackContainer, ingress *v1beta1.Ingress, traffic map[string]TrafficStatus) (map[string]float64, map[string]float64)
}

type SimpleTrafficReconciler struct{}

func (r SimpleTrafficReconciler) ReconcileDeployment(stacks map[types.UID]*StackContainer, stack *zv1.Stack, traffic map[string]TrafficStatus, deployment *appsv1.Deployment) error {
	return nil
}

func (r SimpleTrafficReconciler) ReconcileHPA(stack *zv1.Stack, hpa *autoscaling.HorizontalPodAutoscaler, deployment *appsv1.Deployment) error {
	hpa.Spec.MinReplicas = stack.Spec.HorizontalPodAutoscaler.MinReplicas
	hpa.Spec.MaxReplicas = stack.Spec.HorizontalPodAutoscaler.MaxReplicas
	return nil
}

func (r SimpleTrafficReconciler) ReconcileIngress(stacks map[types.UID]*StackContainer, ingress *v1beta1.Ingress, traffic map[string]TrafficStatus) (map[string]float64, map[string]float64) {
	stackStatuses := getStackStatuses(stacks)
	return computeBackendWeights(stackStatuses, traffic)
}

func computeBackendWeights(stacks []stackStatus, traffic map[string]TrafficStatus) (map[string]float64, map[string]float64) {
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
