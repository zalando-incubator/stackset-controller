package core

import (
	"math"
	"sort"
	"strings"
	"time"
)

// PrescalingTrafficReconciler is a traffic reconciler that forcibly scales up the deployment
// before switching traffic
type PrescalingTrafficReconciler struct {
	ResetHPAMinReplicasTimeout time.Duration
}

func (r PrescalingTrafficReconciler) Reconcile(stacks map[string]*StackContainer, currentTimestamp time.Time) error {
	// Calculate how many replicas we need per unit of traffic
	totalReplicas := 0.0
	totalTraffic := 0.0

	for _, stack := range stacks {
		if stack.prescalingActive {
			// Stack is prescaled, there are several possibilities
			if stack.deploymentReplicas <= stack.prescalingReplicas && stack.prescalingDesiredTrafficWeight > 0 {
				// We can't get information out of the HPA, so let's use the information captured previously
				totalReplicas += float64(stack.prescalingReplicas)
				totalTraffic += stack.prescalingDesiredTrafficWeight
			} else if stack.deploymentReplicas > stack.prescalingReplicas && stack.actualTrafficWeight > 0 {
				// Even though prescaling is active, stack is scaled up to more replicas and it has traffic,
				// let's assume that we can get more precise replicas/traffic information this way
				totalReplicas += float64(stack.deploymentReplicas)
				totalTraffic += stack.actualTrafficWeight
			}
		} else if stack.actualTrafficWeight > 0 {
			// Stack has traffic and is not prescaled
			totalReplicas += float64(stack.deploymentReplicas)
			totalTraffic += stack.actualTrafficWeight
		}
	}

	// Prescale stacks if needed
	for _, stack := range stacks {
		// If traffic needs to be increased
		if stack.desiredTrafficWeight > stack.actualTrafficWeight {
			// If prescaling is not active, or desired weight changed since the last prescaling attempt, update
			// the target replica count
			if !stack.prescalingActive || stack.prescalingDesiredTrafficWeight < stack.desiredTrafficWeight {
				stack.prescalingDesiredTrafficWeight = stack.desiredTrafficWeight

				if totalTraffic != 0 {
					stack.prescalingReplicas = int32(math.Ceil(stack.desiredTrafficWeight * totalReplicas / totalTraffic))
				}

				// Unable to determine target scale, fallback to stack replicas
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
			stack.prescalingDesiredTrafficWeight = 0
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
		return newTrafficSwitchError("stacks not ready: %s", strings.Join(nonReadyStacks, ", "))
	}

	// TODO: think of case were all are zero and the service/deployment is deleted.
	normalizeWeights(actualWeights)

	for stackName, stack := range stacks {
		stack.actualTrafficWeight = actualWeights[stackName]
	}

	return nil
}
