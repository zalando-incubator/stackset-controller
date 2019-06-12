package core

import (
	"sort"
	"strings"
	"time"
)

// SimpleTrafficReconciler is the most simple traffic reconciler which
// implements the default traffic switching supported in the
// stackset-controller.
type SimpleTrafficReconciler struct{}

func (SimpleTrafficReconciler) Reconcile(stacks map[string]*StackContainer, currentTimestamp time.Time) error {
	actualWeights := make(map[string]float64, len(stacks))

	var nonReadyStacks []string
	for stackName, stack := range stacks {
		if stack.desiredTrafficWeight > stack.actualTrafficWeight && !stack.IsReady() {
			nonReadyStacks = append(nonReadyStacks, stackName)
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
