package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTrafficSwitch(t *testing.T) {
	t.Parallel()

	stacksetName := "stackset-switch-traffic"
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
	err = trafficWeightsUpdated(t, stacksetName, weightKindActual, initialWeights, nil).await()
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
	err = setDesiredTrafficWeights(stacksetName, desiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdated(t, stacksetName, weightKindActual, desiredWeights, nil).await()
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
	err = setDesiredTrafficWeights(stacksetName, newDesiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdated(t, stacksetName, weightKindActual, newDesiredWeights, nil).await()
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
