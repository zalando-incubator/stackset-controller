package controller

import (
	"math"
	"time"

	"github.com/zalando-incubator/stackset-controller/controller/entities"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	DefaultResetMinReplicasDelay = 10 * time.Minute
)

type PrescaleTrafficReconciler struct {
	ResetHPAMinReplicasTimeout time.Duration
}

// ReconcileDeployment calculates the number of replicas required when prescaling is active. If there is no associated
// HPA then the replicas of the deployment are increased directly. Finally once traffic switching is complete the
// prescaling status is set to false
func (r *PrescaleTrafficReconciler) ReconcileDeployment(stacks map[types.UID]*entities.StackContainer, stack *zv1.Stack, traffic map[string]entities.TrafficStatus, deployment *appsv1.Deployment) error {
	// If traffic needs to be increased
	if traffic != nil && traffic[stack.Name].DesiredWeight > 0 && traffic[stack.Name].ActualWeight < traffic[stack.Name].DesiredWeight {
		// If prescaling is not active then calculate the replicas required
		if !stack.Status.Prescaling.Active {
			for _, stackContainer := range stacks {
				if traffic[stackContainer.Stack.Name].ActualWeight > 0 {
					if stackContainer.Resources.Deployment != nil && stackContainer.Resources.Deployment.Spec.Replicas != nil {
						stack.Status.Prescaling.Replicas += int32(*stackContainer.Resources.Deployment.Spec.Replicas)
					}
				}
			}
			// If no other stacks are currently active
			if stack.Status.Prescaling.Replicas == 0 {
				stack.Status.Prescaling.Replicas = *stack.Spec.Replicas
			}

			// Restrict the prescaling replicas to the maximum allowed by HPA if present
			if stack.Spec.HorizontalPodAutoscaler != nil {
				stack.Status.Prescaling.Replicas = int32(math.Min(float64(stack.Status.Prescaling.Replicas), float64(stack.Spec.HorizontalPodAutoscaler.MaxReplicas)))
			}
		}
		stack.Status.Prescaling.Active = true
		// Update the timestamp in the prescaling information. This bumps the prescaling timeout
		currentTime := metav1.NewTime(time.Now())
		stack.Status.Prescaling.LastTrafficIncrease = &currentTime
	}

	// If prescaling is active and the prescaling timeout has expired then deactivate the prescaling
	if stack.Status.Prescaling.Active {
		lastTraffic := *stack.Status.Prescaling.LastTrafficIncrease
		if !lastTraffic.IsZero() && time.Since(lastTraffic.Time) > r.ResetHPAMinReplicasTimeout {
			stack.Status.Prescaling.Active = false
			return nil
		}

		// If there is no associated HPA then manually update the replicas
		if stack.Spec.HorizontalPodAutoscaler == nil {
			replicas := stack.Status.Prescaling.Replicas
			deployment.Spec.Replicas = &replicas
		}
	}

	return nil
}

// ReconcileHPA sets the MinReplicas to the prescale value defined in the
// status of the stack if prescaling is active. If prescaling is not active then it sets it to the
// minReplicas value from the Stack. This means that the HPA is allowed to scale down once the prescaling is done.
func (r *PrescaleTrafficReconciler) ReconcileHPA(stack *zv1.Stack, hpa *autoscaling.HorizontalPodAutoscaler, deployment *appsv1.Deployment) error {
	hpa.Spec.MaxReplicas = stack.Spec.HorizontalPodAutoscaler.MaxReplicas
	if stack.Status.Prescaling.Active {
		prescalingReplicas := stack.Status.Prescaling.Replicas
		hpa.Spec.MinReplicas = &prescalingReplicas
		return nil
	}
	hpa.Spec.MinReplicas = stack.Spec.HorizontalPodAutoscaler.MinReplicas
	return nil
}

// ReconcileIngress calculates the traffic distribution for the ingress. The
// implementation is optimized for prescaling stacks before directing traffic.
// It works like this:
//
// * If prescaling is active on the stack then it only gets
//   traffic if it has readyReplicas >= prescaleReplicas.
// * If stack is getting traffic but ReadyReplicas < prescaleReplicas, don't
//   remove traffic from it.
// * If no stacks are currently being prescaled fall back to the current
//   weights.
// * If no stacks are getting traffic fall back to desired weight without
//   checking health.
func (r *PrescaleTrafficReconciler) ReconcileIngress(stacks map[types.UID]*entities.StackContainer, ingress *v1beta1.Ingress, traffic map[string]entities.TrafficStatus) (map[string]float64, map[string]float64) {
	backendWeights := make(map[string]float64, len(stacks))
	currentWeights := make(map[string]float64, len(stacks))
	availableBackends := make(map[string]float64, len(stacks))
	var notReadyBackends int32
	for _, stack := range stacks {
		backendWeights[stack.Stack.Name] = traffic[stack.Stack.Name].DesiredWeight
		currentWeights[stack.Stack.Name] = traffic[stack.Stack.Name].ActualWeight

		deployment := stack.Resources.Deployment

		// prescale if stack is currently less than desired traffic
		if traffic[stack.Stack.Name].ActualWeight < traffic[stack.Stack.Name].DesiredWeight && deployment != nil {
			if stack.Stack.Status.Prescaling.Active {
				var actualReplicas int32 = 1
				desiredReplicas := stack.Stack.Status.Prescaling.Replicas
				if deployment.Spec.Replicas != nil {
					actualReplicas = *deployment.Spec.Replicas
				}

				if actualReplicas >= desiredReplicas &&
					deployment.Status.ReadyReplicas >= desiredReplicas {
					availableBackends[stack.Stack.Name] = traffic[stack.Stack.Name].DesiredWeight
				} else {
					notReadyBackends++
				}
			}
			continue
		} else if traffic[stack.Stack.Name].ActualWeight > 0 && traffic[stack.Stack.Name].DesiredWeight > 0 {
			availableBackends[stack.Stack.Name] = traffic[stack.Stack.Name].DesiredWeight
		}
	}

	if !allZero(currentWeights) {
		normalizeWeights(currentWeights)
	}

	if !allZero(backendWeights) {
		normalizeWeights(backendWeights)
	}

	// don't switch traffic (return currentWeights as is) if 1 or more backends aren't ready yet
	if len(availableBackends) == 0 || notReadyBackends > 0 {
		availableBackends = currentWeights
	}

	normalizeWeights(availableBackends)

	return availableBackends, backendWeights
}
