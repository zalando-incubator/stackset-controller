package core

import (
	"time"
)

// PrescalingTrafficReconciler is the most simple traffic reconciler which
// implements the default traffic switching supported in the
// stackset-controller.
type PrescalingTrafficReconciler struct {
	ResetHPAMinReplicasTimeout time.Duration
}

func (r PrescalingTrafficReconciler) Reconcile(stacks map[string]*StackContainer) error {
	// Calculate how many replicas stacks with traffic have at the moment
	totalReplicas := int32(0)
	for _, stack := range stacks {
		if stack.actualTrafficWeight > 0 {
			totalReplicas += stack.deploymentReplicas
		}
	}

	// Prescale stacks if needed
	for _, stack := range stacks {
		// If traffic needs to be increased
		if stack.desiredTrafficWeight > 0 && stack.desiredTrafficWeight > stack.actualTrafficWeight {
			// If prescaling is not active then calculate the replicas required
			if !stack.prescalingActive {
				stack.prescalingReplicas = totalReplicas

				// If no other stacks are currently active
				if stack.prescalingReplicas == 0 {
					stack.prescalingReplicas = effectiveReplicas(stack.Stack.Spec.Replicas)
				}

				// Limit to MaxReplicas
				if stack.prescalingReplicas > stack.MaxReplicas() {
					stack.prescalingReplicas = stack.MaxReplicas()
				}

			}

			stack.prescalingActive = true
			stack.prescalingLastTrafficIncrease = time.Now()
		}

		if stack.prescalingActive && !stack.prescalingLastTrafficIncrease.IsZero() && time.Since(stack.prescalingLastTrafficIncrease) > r.ResetHPAMinReplicasTimeout {
			stack.prescalingActive = false
			stack.prescalingReplicas = 0
			stack.prescalingLastTrafficIncrease = time.Time{}
		}
	}

	// Update the traffic weights:
	// * If prescaling is active on the stack then it only gets traffic if it has readyReplicas >= prescaleReplicas.
	// * If stack is getting traffic but ReadyReplicas < prescaleReplicas, don't remove traffic from it.
	// * If no stacks are currently being prescaled fall back to the current weights.
	// * If no stacks are getting traffic fall back to desired weight without checking health.
	currentWeights := make(map[string]float64, len(stacks))
	actualWeights := make(map[string]float64, len(stacks))
	for _, stack := range stacks {
		currentWeights[stack.Stack.Name] = stack.actualTrafficWeight

		deployment := stack.Resources.Deployment

		if stack.actualTrafficWeight < stack.desiredTrafficWeight && stack.deploymentUpdated {
			if stack.Stack.Status.Prescaling.Active {
				var desired int32 = 1
				if deployment.Spec.Replicas != nil {
					desired = *deployment.Spec.Replicas
				}

				if desired >= stack.Stack.Status.Prescaling.Replicas && deployment.Status.ReadyReplicas >= stack.Stack.Status.Prescaling.Replicas {
					actualWeights[stack.Stack.Name] = stack.desiredTrafficWeight
				}
			}
			continue
		} else if stack.actualTrafficWeight > 0 && stack.desiredTrafficWeight > 0 {
			actualWeights[stack.Stack.Name] = stack.desiredTrafficWeight
		}
	}

	if !allZero(currentWeights) {
		normalizeWeights(currentWeights)
	}

	if len(actualWeights) == 0 {
		actualWeights = currentWeights
	}

	normalizeWeights(actualWeights)

	for stackName, stack := range stacks {
		stack.actualTrafficWeight = actualWeights[stackName]
	}

	return nil
}
