package core

import (
	"sort"
	"strings"
	"time"
)

// PrescalingTrafficReconciler is the most simple traffic reconciler which
// implements the default traffic switching supported in the
// stackset-controller.
type PrescalingTrafficReconciler struct {
	ResetHPAMinReplicasTimeout time.Duration
}

func (r PrescalingTrafficReconciler) Reconcile(stacks map[string]*StackContainer, currentTimestamp time.Time) error {
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
		if stack.desiredTrafficWeight > stack.actualTrafficWeight {
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
			stack.prescalingLastTrafficIncrease = currentTimestamp
		}

		// If prescaling is active and the prescaling timeout has expired then deactivate the prescaling
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
	var nonReadyStacks []string
	actualWeights := make(map[string]float64, len(stacks))
	for stackName, stack := range stacks {
		// Check if we're increasing traffic but the stack is not ready
		if stack.desiredTrafficWeight > stack.actualTrafficWeight {
			var desiredReplicas = stack.deploymentReplicas
			if stack.prescalingActive {
				desiredReplicas = stack.prescalingReplicas
			}
			if !stack.IsReady() || stack.updatedReplicas < desiredReplicas || stack.readyReplicas < desiredReplicas {
				nonReadyStacks = append(nonReadyStacks, stackName)
				continue
			}
		}

		actualWeights[stackName] = stack.desiredTrafficWeight
	}

	if len(nonReadyStacks) > 0 {
		sort.Strings(nonReadyStacks)
		return newTrafficSwitchError("stacks %s not ready", strings.Join(nonReadyStacks, ", "))
	}

	// TODO: think of case were all are zero and the service/deployment is deleted.
	normalizeWeights(actualWeights)

	for stackName, stack := range stacks {
		stack.actualTrafficWeight = actualWeights[stackName]
	}

	return nil
}
