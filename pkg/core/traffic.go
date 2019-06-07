package core

import (
	"fmt"
	"time"
)

const (
	stackTrafficWeightsAnnotationKey = "zalando.org/stack-traffic-weights"
	backendWeightsAnnotationKey      = "zalando.org/backend-weights"
)

type trafficSwitchError struct {
	reason string
}

func (e *trafficSwitchError) Error() string {
	return e.reason
}

func IsTrafficSwitchError(err error) bool {
	switch err.(type) {
	case *trafficSwitchError:
		return true
	default:
		return false
	}
}

func newTrafficSwitchError(format string, args ...interface{}) error {
	return &trafficSwitchError{reason: fmt.Sprintf(format, args...)}
}

type TrafficReconciler interface {
	// Handle the traffic switching and/or scaling logic.
	Reconcile(stacks map[string]*StackContainer) error
}

// allZero returns true if all weights defined in the map are 0.
func allZero(weights map[string]float64) bool {
	for _, weight := range weights {
		if weight > 0 {
			return false
		}
	}
	return true
}

// normalizeWeights normalizes a map of backend weights.
// If all weights are zero the total weight of 100 is distributed equally
// between all backends.
// If not all weights are zero they are normalized to a sum of 100.
// Note this modifies the passed map inplace instead of returning a modified
// copy.
func normalizeWeights(backendWeights map[string]float64) {
	// if all weights are zero distribute them equally to all backends
	if allZero(backendWeights) && len(backendWeights) > 0 {
		eqWeight := 100 / float64(len(backendWeights))
		for backend := range backendWeights {
			backendWeights[backend] = eqWeight
		}
		return
	}

	// if not all weights are zero, normalize them to a sum of 100
	sum := float64(0)
	for _, weight := range backendWeights {
		sum += weight
	}

	for backend, weight := range backendWeights {
		backendWeights[backend] = weight / sum * 100
	}
}

// ManageTraffic handles the traffic reconciler logic
func (ssc *StackSetContainer) ManageTraffic() error {
	// No ingress -> no traffic management required
	if ssc.StackSet.Spec.Ingress == nil {
		for _, sc := range ssc.StackContainers {
			sc.desiredTrafficWeight = 0
			sc.actualTrafficWeight = 0
			sc.noTrafficSince = time.Time{}
			sc.prescalingActive = false
			sc.prescalingReplicas = 0
			sc.prescalingLastTrafficIncrease = time.Time{}
		}
		return nil
	}

	stacks := make(map[string]*StackContainer)
	for _, stack := range ssc.StackContainers {
		stacks[stack.Stack.Name] = stack
	}

	// Collect the desired weights
	desiredWeights := make(map[string]float64)
	actualWeights := make(map[string]float64)
	for stackName, stack := range stacks {
		desiredWeights[stackName] = stack.desiredTrafficWeight
		actualWeights[stackName] = stack.actualTrafficWeight
	}

	// Normalize the desired weights and ensure that at least one stack gets traffic
	if allZero(desiredWeights) {
		fallbackStack := findFallbackStack(stacks)
		if fallbackStack == nil {
			return errNoStacks
		}
		desiredWeights[fallbackStack.Stack.Name] = 100
	} else {
		normalizeWeights(desiredWeights)
	}
	for stackName, stack := range stacks {
		stack.desiredTrafficWeight = desiredWeights[stackName]
	}

	// Run the traffic reconciler which will update the actual weights according to the desired weights
	err := ssc.TrafficReconciler.Reconcile(stacks)

	// Update the actual weights from the reconciled ones
	if err == nil {
		actualWeights = make(map[string]float64)
		for stackName, stack := range stacks {
			actualWeights[stackName] = stack.actualTrafficWeight
		}
	}

	// If none of the stacks are getting traffic, just fallback to desired
	if allZero(actualWeights) {
		actualWeights = desiredWeights
	}
	for stackName, stack := range stacks {
		stack.actualTrafficWeight = actualWeights[stackName]
	}

	// update NoTrafficSince
	for _, stack := range ssc.StackContainers {
		if stack.HasTraffic() {
			stack.noTrafficSince = time.Time{}
		} else if stack.noTrafficSince.IsZero() {
			stack.noTrafficSince = time.Now()
		}
	}
	return err
}

// fallbackStack returns a stack that should be the target of traffic if none of the existing stacks get anything
func findFallbackStack(stacks map[string]*StackContainer) *StackContainer {
	var recentlyUsed *StackContainer
	var earliest *StackContainer

	for _, stack := range stacks {
		if earliest == nil || stack.Stack.CreationTimestamp.Before(&earliest.Stack.CreationTimestamp) {
			earliest = stack
		}
		if !stack.noTrafficSince.IsZero() && (recentlyUsed == nil || !recentlyUsed.noTrafficSince.IsZero() && stack.noTrafficSince.Before(recentlyUsed.noTrafficSince)) {
			recentlyUsed = stack
		}
	}

	if recentlyUsed != nil {
		return recentlyUsed
	}
	return earliest
}
