package core

import (
	"math"
	"sort"
	"time"
)

type TrafficReconciler interface {
	// Handle the traffic switching and/or scaling logic.
	Reconcile(stacks map[string]*StackContainer, currentTimestamp time.Time) error
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

// roundWeights rounds all the weights to whole numbers while ensuring they
// still add up to 100.
//
// Example:
//
// The weights:
//
//	[33.33, 33.33, 33.33]
//
// will be rounded to:
//
//	[34, 33, 33]
//
// The function assumes that the weights are already normalized to a sum of
// 100.
// It's using the "Largets Remainder Method" for rounding:
// https://en.wikipedia.org/wiki/Largest_remainder_method
func roundWeights(weights map[string]float64) {
	type backendWeight struct {
		Backend string
		Weight  float64
	}

	var weightList []backendWeight
	sum := 0
	// floor all weights of the map
	// sum the rounded weights
	// copy weights map to a slice to sort it later
	for backend, weight := range weights {
		roundedWeight := math.Floor(weight)
		weights[backend] = roundedWeight
		sum += int(roundedWeight)
		weightList = append(weightList, backendWeight{
			Backend: backend,
			Weight:  weight,
		})
	}
	// sort weights by:
	// 1. biggest fraction
	// 2. biggest integer
	// 3. backend name - lexicographical
	sort.Slice(weightList, func(i, j int) bool {
		ii, fi := math.Modf(weightList[i].Weight)
		ij, fj := math.Modf(weightList[j].Weight)
		if fi > fj {
			return true
		}

		if fi == fj && ii > ij {
			return true
		}

		return fi == fj && ii == ij && weightList[i].Backend < weightList[j].Backend
	})
	// check the remaining weight and distribute
	diff := 100 - sum
	for _, backend := range weightList[:diff] {
		weights[backend.Backend]++
	}
}

// ManageTraffic handles the traffic reconciler logic
func (ssc *StackSetContainer) ManageTraffic(currentTimestamp time.Time) error {
	// No ingress -> no traffic management required
	if ssc.StackSet.Spec.Ingress == nil && ssc.StackSet.Spec.RouteGroup == nil && ssc.StackSet.Spec.ExternalIngress == nil {
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
		stacks[stack.Name()] = stack
	}

	// Collect the desired weights
	desiredWeights := make(map[string]float64)
	actualWeights := make(map[string]float64)
	for stackName, stack := range stacks {
		desiredWeights[stackName] = stack.desiredTrafficWeight
		actualWeights[stackName] = stack.actualTrafficWeight
	}

	// Normalize the weights and ensure that at least one stack gets traffic. This is done for both desired
	// and actual weights, because otherwise we might end up in a situation where the desired weights are
	// automagically fixed before reconciling traffic, but the reconciler still has the old actual weights
	// that for example don't add up to 100.
	for _, weights := range []map[string]float64{desiredWeights, actualWeights} {
		// No traffic at all; select a fallback stack and send all traffic there
		if allZero(weights) {
			fallbackStack := findFallbackStack(stacks)
			if fallbackStack == nil {
				return errNoStacks
			}
			weights[fallbackStack.Name()] = 100
		} else {
			normalizeWeights(weights)
			roundWeights(weights)
		}
	}

	for stackName, stack := range stacks {
		stack.desiredTrafficWeight = desiredWeights[stackName]
		stack.actualTrafficWeight = actualWeights[stackName]
	}

	// Run the traffic reconciler which will update the actual weights according to the desired weights. The resulting
	// weights **must** be normalised.
	err := ssc.TrafficReconciler.Reconcile(stacks, currentTimestamp)

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
			stack.noTrafficSince = currentTimestamp
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
		if !stack.noTrafficSince.IsZero() && (recentlyUsed == nil || stack.noTrafficSince.After(recentlyUsed.noTrafficSince)) {
			recentlyUsed = stack
		}
	}

	if recentlyUsed != nil {
		return recentlyUsed
	}
	return earliest
}
