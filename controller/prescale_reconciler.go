package controller

import (
	"fmt"
	"math"
	"strconv"
	"time"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	prescaleAnnotationKey        = "stacksetstacks.zalando.org/prescale-replicas"
	resetHPAMinReplicasSinceKey  = "stacksetstacks.zalando.org/min-replicas-prescale-since"
	DefaultResetMinReplicasDelay = 10 * time.Minute
)

type PrescaleTrafficReconciler struct {
	ResetHPAMinReplicasTimeout time.Duration
}

// ReconcileDeployment prescales the deployment if the prescale annotation is
// set. Prescale annotation will only be removed from the deployment after it's
// getting traffic.
func (r *PrescaleTrafficReconciler) ReconcileDeployment(stacks map[types.UID]*StackContainer, stack *zv1.Stack, traffic map[string]TrafficStatus, deployment *appsv1.Deployment) error {
	// prescale logic
	if prescale, ok := deployment.Annotations[prescaleAnnotationKey]; ok {
		// don't prescale if desired weight is 0
		// remove annotation when prescaling is done
		if traffic != nil && (traffic[stack.Name].DesiredWeight <= 0 || traffic[stack.Name].ActualWeight == traffic[stack.Name].DesiredWeight) {
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

	// prescale deployment if desired weight is > 0 and actual weight is <
	// desired weight
	if traffic != nil && traffic[stack.Name].DesiredWeight > 0 && traffic[stack.Name].ActualWeight < traffic[stack.Name].DesiredWeight {
		var prescaleReplicas int32
		// sum replicas of all stacks currently getting traffic
		for _, stackContainer := range stacks {
			if traffic[stackContainer.Stack.Name].ActualWeight > 0 {
				if stackContainer.Resources.Deployment != nil && stackContainer.Resources.Deployment.Spec.Replicas != nil {
					prescaleReplicas += *stackContainer.Resources.Deployment.Spec.Replicas
				}
			}
		}

		if stack.Spec.HorizontalPodAutoscaler != nil {
			prescaleReplicas = int32(math.Min(float64(prescaleReplicas), float64(stack.Spec.HorizontalPodAutoscaler.MaxReplicas)))
		}

		if prescaleReplicas > 0 {
			prescaleReplicasStr := strconv.FormatInt(int64(prescaleReplicas), 10)
			deployment.Annotations[prescaleAnnotationKey] = prescaleReplicasStr

			if stack.Spec.HorizontalPodAutoscaler == nil {
				replicas := int32(prescaleReplicas)
				deployment.Spec.Replicas = &replicas
			}
		}
	}

	return nil
}

// ReconcileHPA sets the MinReplicas to the prescale value defined in the
// annotation of the deployment. If no annotation is defined then the default
// minReplicas value is used from the Stack. This means that the HPA is allowed
// to scale down once the prescaling is done.
func (r *PrescaleTrafficReconciler) ReconcileHPA(stack *zv1.Stack, hpa *autoscaling.HorizontalPodAutoscaler, deployment *appsv1.Deployment) error {
	var minReplicas int32

	if stack.Spec.HorizontalPodAutoscaler.MinReplicas != nil {
		minReplicas = *stack.Spec.HorizontalPodAutoscaler.MinReplicas
	}

	// reuse the existing HPA minReplicas if the "reset HPA MinReplicas
	// timeout" wasn't reached yet.
	if prescaleSince, ok := hpa.Annotations[resetHPAMinReplicasSinceKey]; ok {
		minReplicaPrescaleSince, err := time.Parse(time.RFC3339, prescaleSince)
		if err != nil {
			return fmt.Errorf("failed to parse min-replicas-prescale-since timestamp '%s': %v", prescaleSince, err)
		}

		if !minReplicaPrescaleSince.IsZero() && time.Since(minReplicaPrescaleSince) <= r.ResetHPAMinReplicasTimeout {
			if hpa.Spec.MinReplicas != nil {
				minReplicas = *hpa.Spec.MinReplicas
			}
		} else {
			// remove the annotation if the reset timeout was
			// reached.
			delete(hpa.Annotations, resetHPAMinReplicasSinceKey)
		}
	}

	if prescale, ok := deployment.Annotations[prescaleAnnotationKey]; ok {
		prescaleReplicas, err := strconv.Atoi(prescale)
		if err != nil {
			return err
		}

		if _, ok := hpa.Annotations[resetHPAMinReplicasSinceKey]; !ok {
			hpa.Annotations[resetHPAMinReplicasSinceKey] = time.Now().Format(time.RFC3339)
		}

		minReplicas = int32(prescaleReplicas)
	}

	// cap minReplicas as maxReplicas
	minReplicas = int32(math.Min(float64(minReplicas), float64(stack.Spec.HorizontalPodAutoscaler.MaxReplicas)))

	hpa.Spec.MinReplicas = &minReplicas
	hpa.Spec.MaxReplicas = stack.Spec.HorizontalPodAutoscaler.MaxReplicas

	return nil
}

// getDeploymentPrescale parses and returns the prescale value if set in the
// deployment annotation.
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

// ReconcileIngress calcuates the traffic distribution for the ingress. The
// implementation is optimized for prescaling stacks before directing traffic.
// It works like this:
//
// * If stack has a deployment with prescale annotation then it only gets
//   traffic if it has readyReplicas >= prescaleReplicas.
// * If stack is getting traffic but ReadyReplicas < prescaleReplicas, don't
//   remove traffic from it.
// * If no stacks are currently being prescaled fall back to the current
//   weights.
// * If no stacks are getting traffic fall back to desired weight without
//   checking health.
func (r *PrescaleTrafficReconciler) ReconcileIngress(stacks map[types.UID]*StackContainer, ingress *v1beta1.Ingress, traffic map[string]TrafficStatus) (map[string]float64, map[string]float64) {
	backendWeights := make(map[string]float64, len(stacks))
	currentWeights := make(map[string]float64, len(stacks))
	availableBackends := make(map[string]float64, len(stacks))
	for _, stack := range stacks {
		backendWeights[stack.Stack.Name] = traffic[stack.Stack.Name].DesiredWeight
		currentWeights[stack.Stack.Name] = traffic[stack.Stack.Name].ActualWeight

		deployment := stack.Resources.Deployment

		// prescale if stack is currently less than desired traffic
		if traffic[stack.Stack.Name].ActualWeight < traffic[stack.Stack.Name].DesiredWeight && deployment != nil {
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

	if len(availableBackends) == 0 {
		availableBackends = currentWeights
	}

	normalizeWeights(availableBackends)

	return availableBackends, backendWeights
}
