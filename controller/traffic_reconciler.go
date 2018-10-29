package controller

import (
	"fmt"
	"math"
	"strconv"
	"time"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	prescaleAnnotationKey = "stacksetstacks.zalando.org/prescale-replicas"
)

type TrafficReconciler interface {
	ReconcileDeployment(stackset zv1.StackSet, stacks map[types.UID]*StackContainer, stack *zv1.Stack, traffic map[string]TrafficStatus, deployment *appsv1.Deployment, scaledownTTL time.Duration) error
	ReconcileHPA(stack *zv1.Stack, hpa *autoscaling.HorizontalPodAutoscaler, deployment *appsv1.Deployment) error
	ReconcileIngress(stacks map[types.UID]*StackContainer, ingress *v1beta1.Ingress, traffic map[string]TrafficStatus) (map[string]float64, map[string]float64)
}

type SimpleTrafficReconciler struct{}

func (r SimpleTrafficReconciler) ReconcileDeployment(stackset zv1.StackSet, stacks map[types.UID]*StackContainer, stack *zv1.Stack, traffic map[string]TrafficStatus, deployment *appsv1.Deployment, scaledownTTL time.Duration) error {
	// if autoscaling is disabled or if autoscaling is enabled
	// check if we need to explicitly set replicas on the deployment. There
	// are two cases:
	// 1. Autoscaling is disabled and we should rely on the replicas set on
	//    the stack
	// 2. Autoscaling is enabled, but current replica count is 0. In this
	//    case we have to set a value > 0 otherwise the autoscaler won't do
	//    anything.
	if stack.Spec.HorizontalPodAutoscaler == nil ||
		(deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 0) {
		deployment.Spec.Replicas = stack.Spec.Replicas
	}

	// If the deployment is scaled down by the downscaler then scale it back up again
	if stack.Spec.Replicas != nil && *stack.Spec.Replicas != 0 {
		if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas == 0 {
			replicas := int32(*stack.Spec.Replicas)
			deployment.Spec.Replicas = &replicas
		}
	}

	// The deployment has to be scaled down because the stack has been scaled down then set the replica count
	if stack.Spec.Replicas != nil && *stack.Spec.Replicas == 0 {
		if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 0 {
			replicas := int32(0)
			deployment.Spec.Replicas = &replicas
		}
	}

	currentStackName := generateStackName(stackset, currentStackVersion(stackset))

	// Avoid downscaling the current stack
	if stack.Name != currentStackName && traffic != nil && traffic[stack.Name].Weight() <= 0 {
		if ttl, ok := deployment.Annotations[noTrafficSinceAnnotationKey]; ok {
			noTrafficSince, err := time.Parse(time.RFC3339, ttl)
			if err != nil {
				return fmt.Errorf("failed to parse no-traffic-since timestamp '%s': %v", ttl, err)
			}

			if !noTrafficSince.IsZero() && time.Since(noTrafficSince) > scaledownTTL {
				replicas := int32(0)
				deployment.Spec.Replicas = &replicas
			}
		} else {
			deployment.Annotations[noTrafficSinceAnnotationKey] = time.Now().UTC().Format(time.RFC3339)
		}
	} else {
		// ensure the scaledown annotation is removed if the stack has
		// traffic.
		if _, ok := deployment.Annotations[noTrafficSinceAnnotationKey]; ok {
			delete(deployment.Annotations, noTrafficSinceAnnotationKey)
		}
	}

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

type PrescaleTrafficReconciler struct {
	base TrafficReconciler
}

func (r *PrescaleTrafficReconciler) ReconcileDeployment(stackset zv1.StackSet, stacks map[types.UID]*StackContainer, stack *zv1.Stack, traffic map[string]TrafficStatus, deployment *appsv1.Deployment, scaledownTTL time.Duration) error {
	err := r.base.ReconcileDeployment(stackset, stacks, stack, traffic, deployment, scaledownTTL)
	if err != nil {
		return err
	}

	// prescale logic
	if prescale, ok := deployment.Annotations[prescaleAnnotationKey]; ok {
		// don't prescale if desired weight is 0
		// remove annotation when prescaling is done
		if traffic != nil && (traffic[stack.Name].DesiredWeight <= 0 || traffic[stack.Name].ActualWeight > 0) {
			delete(deployment.Annotations, prescaleAnnotationKey)
			return nil
		}

		if stack.Spec.HorizontalPodAutoscaler == nil {
			prescaleReplicas, err := strconv.Atoi(prescale)
			if err != nil {
				return err
			}

			replicas := int32(prescaleReplicas)
			deployment.Spec.Replicas = &replicas
		}
		return nil
	}

	// prescale deployment if desired weight is > 0 and actual weight is 0
	if traffic != nil && traffic[stack.Name].DesiredWeight > 0 && traffic[stack.Name].ActualWeight == 0 {
		var prescaleReplicas int32
		// sum replicas of all stacks currently getting traffic
		for _, stackContainer := range stacks {
			if traffic[stackContainer.Stack.Name].ActualWeight > 0 {
				if stackContainer.Resources.HPA != nil {
					prescaleReplicas += stackContainer.Resources.HPA.Status.CurrentReplicas
					continue
				}

				if stackContainer.Resources.Deployment != nil && stackContainer.Resources.Deployment.Spec.Replicas != nil {
					prescaleReplicas += *stackContainer.Resources.Deployment.Spec.Replicas
				}
			}
		}

		prescaleReplicasStr := strconv.FormatInt(int64(prescaleReplicas), 10)
		deployment.Annotations[prescaleAnnotationKey] = prescaleReplicasStr

		if stack.Spec.HorizontalPodAutoscaler == nil {
			replicas := int32(prescaleReplicas)
			deployment.Spec.Replicas = &replicas
		}
	}

	return nil
}

func (r *PrescaleTrafficReconciler) ReconcileHPA(stack *zv1.Stack, hpa *autoscaling.HorizontalPodAutoscaler, deployment *appsv1.Deployment) error {
	hpa.Spec.MinReplicas = stack.Spec.HorizontalPodAutoscaler.MinReplicas
	hpa.Spec.MaxReplicas = stack.Spec.HorizontalPodAutoscaler.MaxReplicas

	if prescale, ok := deployment.Annotations[prescaleAnnotationKey]; ok {
		prescaleReplicas, err := strconv.Atoi(prescale)
		if err != nil {
			return err
		}

		replicas := int32(math.Min(float64(prescaleReplicas), float64(stack.Spec.HorizontalPodAutoscaler.MaxReplicas)))
		hpa.Spec.MinReplicas = &replicas
	}

	return nil
}

func getDeploymentPrescale(deployment *appsv1.Deployment) (int32, bool) {
	prescaleReplicasStr, ok := deployment.Annotations[prescaleAnnotationKey]
	if !ok {
		return 0, false
	}
	prescaleReplicas, err := strconv.Atoi(prescaleReplicasStr)
	if err != nil {
		return 0, false
	}
	return int32(prescaleReplicas), true
}

func (r *PrescaleTrafficReconciler) ReconcileIngress(stacks map[types.UID]*StackContainer, ingress *v1beta1.Ingress, traffic map[string]TrafficStatus) (map[string]float64, map[string]float64) {
	backendWeights := make(map[string]float64, len(stacks))
	currentWeights := make(map[string]float64, len(stacks))
	availableBackends := make(map[string]float64, len(stacks))
	for _, stack := range stacks {
		backendWeights[stack.Stack.Name] = traffic[stack.Stack.Name].DesiredWeight
		currentWeights[stack.Stack.Name] = traffic[stack.Stack.Name].ActualWeight

		deployment := stack.Resources.Deployment

		// prescale if stack is currently not getting traffic
		if traffic[stack.Stack.Name].ActualWeight == 0 && deployment != nil {
			if prescale, ok := getDeploymentPrescale(deployment); ok {
				var desired int32 = 1
				if deployment.Spec.Replicas != nil {
					desired = *deployment.Spec.Replicas
				}

				if desired >= prescale && deployment.Status.ReadyReplicas >= prescale {
					availableBackends[stack.Stack.Name] = traffic[stack.Stack.Name].DesiredWeight
				}
			}
			continue
		}
	}

	if !allZero(currentWeights) {
		normalizeWeights(currentWeights)
	}

	if !allZero(backendWeights) {
		normalizeWeights(backendWeights)
	}

	if len(availableBackends) == 0 {
		availableBackends = currentWeights
	}

	normalizeWeights(availableBackends)

	return availableBackends, backendWeights
}
