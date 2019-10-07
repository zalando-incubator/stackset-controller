package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	err = trafficWeightsUpdatedIngress(t, stacksetName, weightKindActual, initialWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(100),
		desiredTrafficWeight: pfloat64(100),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(0),
		desiredTrafficWeight: pfloat64(0),
	}).await()
	require.NoError(t, err)

	// Switch traffic 50/50
	desiredWeights := map[string]float64{firstStack: 50, updatedStack: 50}
	err = setDesiredTrafficWeightsIngress(stacksetName, desiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedIngress(t, stacksetName, weightKindActual, desiredWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(50),
		desiredTrafficWeight: pfloat64(50),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(50),
		desiredTrafficWeight: pfloat64(50),
	}).await()
	require.NoError(t, err)

	// Switch traffic 0/100
	newDesiredWeights := map[string]float64{updatedStack: 100}
	err = setDesiredTrafficWeightsIngress(stacksetName, newDesiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedIngress(t, stacksetName, weightKindActual, newDesiredWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(0),
		desiredTrafficWeight: pfloat64(0),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(100),
		desiredTrafficWeight: pfloat64(100),
	}).await()
	require.NoError(t, err)
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
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, initialWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(100),
		desiredTrafficWeight: pfloat64(100),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(0),
		desiredTrafficWeight: pfloat64(0),
	}).await()
	require.NoError(t, err)

	// Switch traffic 50/50
	desiredWeights := map[string]float64{firstStack: 50, updatedStack: 50}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(50),
		desiredTrafficWeight: pfloat64(50),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(50),
		desiredTrafficWeight: pfloat64(50),
	}).await()
	require.NoError(t, err)

	// Switch traffic 0/100
	newDesiredWeights := map[string]float64{updatedStack: 100}
	err = setDesiredTrafficWeightsStackset(stacksetName, newDesiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, newDesiredWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(0),
		desiredTrafficWeight: pfloat64(0),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(100),
		desiredTrafficWeight: pfloat64(100),
	}).await()
	require.NoError(t, err)
}

func TestTrafficSwitchDesiredStacksetActualIngress(t *testing.T) {
	t.Parallel()

	stacksetName := "switch-traffic-desired-stackset-actual-ing"
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
	err = trafficWeightsUpdatedIngress(t, stacksetName, weightKindActual, initialWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(100),
		desiredTrafficWeight: pfloat64(100),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(0),
		desiredTrafficWeight: pfloat64(0),
	}).await()
	require.NoError(t, err)

	// Switch traffic 50/50
	desiredWeights := map[string]float64{firstStack: 50, updatedStack: 50}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedIngress(t, stacksetName, weightKindActual, desiredWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(50),
		desiredTrafficWeight: pfloat64(50),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(50),
		desiredTrafficWeight: pfloat64(50),
	}).await()
	require.NoError(t, err)

	// Switch traffic 0/100
	newDesiredWeights := map[string]float64{updatedStack: 100}
	err = setDesiredTrafficWeightsStackset(stacksetName, newDesiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedIngress(t, stacksetName, weightKindActual, newDesiredWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(0),
		desiredTrafficWeight: pfloat64(0),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(100),
		desiredTrafficWeight: pfloat64(100),
	}).await()
	require.NoError(t, err)
}

func TestTrafficSwitchDesiredIngressActualStackset(t *testing.T) {
	t.Parallel()

	stacksetName := "switch-traffic-desired-ing-actual-stackset"
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
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, initialWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(100),
		desiredTrafficWeight: pfloat64(100),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(0),
		desiredTrafficWeight: pfloat64(0),
	}).await()
	require.NoError(t, err)

	// Switch traffic 50/50
	desiredWeights := map[string]float64{firstStack: 50, updatedStack: 50}
	err = setDesiredTrafficWeightsIngress(stacksetName, desiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(50),
		desiredTrafficWeight: pfloat64(50),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(50),
		desiredTrafficWeight: pfloat64(50),
	}).await()
	require.NoError(t, err)

	// Switch traffic 0/100
	newDesiredWeights := map[string]float64{updatedStack: 100}
	err = setDesiredTrafficWeightsIngress(stacksetName, newDesiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, newDesiredWeights, nil).await()
	require.NoError(t, err)

	err = stackStatusMatches(t, firstStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(0),
		desiredTrafficWeight: pfloat64(0),
	}).await()
	require.NoError(t, err)
	err = stackStatusMatches(t, updatedStack, expectedStackStatus{
		actualTrafficWeight:  pfloat64(100),
		desiredTrafficWeight: pfloat64(100),
	}).await()
	require.NoError(t, err)
}
