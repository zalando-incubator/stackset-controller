package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// expectActualTrafficWeights waits until that both stackset.status and the ingress have the expected actual traffic weight,
// and all stacks have their weights populated correctly
func expectActualTrafficWeights(t *testing.T, stacksetName string, weights map[string]float64) {
	err := trafficWeightsUpdatedIngress(t, stacksetName, weightKindActual, weights, nil).await()
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, weights, nil).await()
	require.NoError(t, err)
}

// expectStackTrafficWeights waits until the stack has the correct traffic weight values
func expectStackTrafficWeights(t *testing.T, stackName string, actualTrafficWeight, desiredTrafficWeight float64) {
	err := stackStatusMatches(t, stackName, expectedStackStatus{
		actualTrafficWeight:  pfloat64(actualTrafficWeight),
		desiredTrafficWeight: pfloat64(desiredTrafficWeight),
	}).await()
	require.NoError(t, err)
}

func TestTrafficSwitchIngress(t *testing.T) {
	t.Parallel()

	stacksetName := "switch-traffic-ingress"
	firstVersion := "v1"
	firstStack := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	updatedVersion := "v2"
	updatedStack := fmt.Sprintf("%s-%s", stacksetName, updatedVersion)
	factory := NewTestStacksetSpecFactory(stacksetName).Ingress()
	spec := factory.Create(firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, firstVersion)
	require.NoError(t, err)
	spec = factory.Create(updatedVersion)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, updatedVersion)
	require.NoError(t, err)

	_, err = waitForIngress(t, stacksetName)
	require.NoError(t, err)

	initialWeights := map[string]float64{firstStack: 100}
	expectActualTrafficWeights(t, stacksetName, initialWeights)
	err = ingressTrafficAuthoritative(t, stacksetName, true).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 100, 100)
	expectStackTrafficWeights(t, updatedStack, 0, 0)

	// Switch traffic 50/50
	desiredWeights := map[string]float64{firstStack: 50, updatedStack: 50}
	err = setDesiredTrafficWeightsIngress(stacksetName, desiredWeights)
	require.NoError(t, err)
	expectActualTrafficWeights(t, stacksetName, desiredWeights)
	err = ingressTrafficAuthoritative(t, stacksetName, true).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 50, 50)
	expectStackTrafficWeights(t, updatedStack, 50, 50)

	// Switch traffic 0/100
	newDesiredWeights := map[string]float64{updatedStack: 100}
	err = setDesiredTrafficWeightsIngress(stacksetName, newDesiredWeights)
	require.NoError(t, err)
	expectActualTrafficWeights(t, stacksetName, desiredWeights)
	err = ingressTrafficAuthoritative(t, stacksetName, true).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 0, 0)
	expectStackTrafficWeights(t, updatedStack, 100, 100)
}

func TestTrafficSwitchIngressThenStackset(t *testing.T) {
	t.Parallel()

	stacksetName := "switch-traffic-stackset-to-ingress"
	firstVersion := "v1"
	firstStack := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	updatedVersion := "v2"
	updatedStack := fmt.Sprintf("%s-%s", stacksetName, updatedVersion)
	factory := NewTestStacksetSpecFactory(stacksetName).Ingress()
	spec := factory.Create(firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, firstVersion)
	require.NoError(t, err)
	spec = factory.Create(updatedVersion)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, updatedVersion)
	require.NoError(t, err)

	_, err = waitForIngress(t, stacksetName)
	require.NoError(t, err)

	initialWeights := map[string]float64{firstStack: 100}
	expectActualTrafficWeights(t, stacksetName, initialWeights)
	err = trafficWeightsUpdatedIngress(t, stacksetName, weightKindDesired, initialWeights, nil).await()
	require.NoError(t, err)
	err = ingressTrafficAuthoritative(t, stacksetName, true).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 100, 100)
	expectStackTrafficWeights(t, updatedStack, 0, 0)

	// Switch traffic 50/50 via ingress
	desiredWeights := map[string]float64{firstStack: 50, updatedStack: 50}
	err = setDesiredTrafficWeightsIngress(stacksetName, desiredWeights)
	require.NoError(t, err)
	expectActualTrafficWeights(t, stacksetName, desiredWeights)
	err = ingressTrafficAuthoritative(t, stacksetName, true).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 50, 50)
	expectStackTrafficWeights(t, updatedStack, 50, 50)

	// Switch traffic 75/25 via stackset
	newDesiredWeights := map[string]float64{firstStack: 75, updatedStack: 25}
	err = setDesiredTrafficWeightsStackset(stacksetName, newDesiredWeights)
	require.NoError(t, err)
	expectActualTrafficWeights(t, stacksetName, newDesiredWeights)
	err = ingressTrafficAuthoritative(t, stacksetName, false).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 75, 75)
	expectStackTrafficWeights(t, updatedStack, 25, 25)

	// Switch traffic 0/100
	finalDesiredWeights := map[string]float64{updatedStack: 100}
	err = setDesiredTrafficWeightsStackset(stacksetName, finalDesiredWeights)
	require.NoError(t, err)
	expectActualTrafficWeights(t, stacksetName, finalDesiredWeights)
	err = ingressTrafficAuthoritative(t, stacksetName, false).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 0, 0)
	expectStackTrafficWeights(t, updatedStack, 100, 100)
}

func TestTrafficSwitchStackset(t *testing.T) {
	t.Parallel()

	stacksetName := "switch-traffic-stackset"
	firstVersion := "v1"
	firstStack := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	updatedVersion := "v2"
	updatedStack := fmt.Sprintf("%s-%s", stacksetName, updatedVersion)
	factory := NewTestStacksetSpecFactory(stacksetName).Ingress()
	spec := factory.Create(firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, firstVersion)
	require.NoError(t, err)
	spec = factory.Create(updatedVersion)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, updatedVersion)
	require.NoError(t, err)

	_, err = waitForIngress(t, stacksetName)
	require.NoError(t, err)

	initialWeights := map[string]float64{firstStack: 100}
	expectActualTrafficWeights(t, stacksetName, initialWeights)
	err = trafficWeightsUpdatedIngress(t, stacksetName, weightKindDesired, initialWeights, nil).await()
	require.NoError(t, err)
	err = ingressTrafficAuthoritative(t, stacksetName, true).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 100, 100)
	expectStackTrafficWeights(t, updatedStack, 0, 0)

	// Switch traffic 50/50
	desiredWeights := map[string]float64{firstStack: 50, updatedStack: 50}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredWeights)
	require.NoError(t, err)
	expectActualTrafficWeights(t, stacksetName, desiredWeights)
	err = ingressTrafficAuthoritative(t, stacksetName, false).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 50, 50)
	expectStackTrafficWeights(t, updatedStack, 50, 50)

	// Switch traffic 0/100
	newDesiredWeights := map[string]float64{updatedStack: 100}
	err = setDesiredTrafficWeightsStackset(stacksetName, newDesiredWeights)
	require.NoError(t, err)
	expectActualTrafficWeights(t, stacksetName, newDesiredWeights)
	err = ingressTrafficAuthoritative(t, stacksetName, false).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 0, 0)
	expectStackTrafficWeights(t, updatedStack, 100, 100)
}

func TestTrafficSwitchStacksetExternalIngress(t *testing.T) {
	t.Parallel()

	stacksetName := "switch-traffic-stackset-external"
	firstVersion := "v1"
	firstStack := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	updatedVersion := "v2"
	updatedStack := fmt.Sprintf("%s-%s", stacksetName, updatedVersion)
	factory := NewTestStacksetSpecFactory(stacksetName).ExternalIngress()
	spec := factory.Create(firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, firstVersion)
	require.NoError(t, err)
	spec = factory.Create(updatedVersion)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, updatedVersion)
	require.NoError(t, err)

	initialWeights := map[string]float64{firstStack: 100}
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, initialWeights, nil).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 100, 100)
	expectStackTrafficWeights(t, updatedStack, 0, 0)

	// Switch traffic 50/50
	desiredWeights := map[string]float64{firstStack: 50, updatedStack: 50}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredWeights, nil).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 50, 50)
	expectStackTrafficWeights(t, updatedStack, 50, 50)

	// Switch traffic 0/100
	newDesiredWeights := map[string]float64{updatedStack: 100}
	err = setDesiredTrafficWeightsStackset(stacksetName, newDesiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, newDesiredWeights, nil).await()
	require.NoError(t, err)

	expectStackTrafficWeights(t, firstStack, 0, 0)
	expectStackTrafficWeights(t, updatedStack, 100, 100)
}
