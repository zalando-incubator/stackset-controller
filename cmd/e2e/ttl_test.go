package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStackTTLWithoutIngress(t *testing.T) {
	t.Parallel()
	stacksetName := "stackset-ttl-noingress"
	specFactory := NewTestStacksetSpecFactory(stacksetName).StackGC(3, 0)

	// Create 5 stacks in total and wait for their deployments to come up
	for i := 0; i < 5; i++ {
		stackVersion := fmt.Sprintf("v%d", i)
		var err error
		spec := specFactory.Create(stackVersion)
		if !stacksetExists(stacksetName) {
			err = createStackSet(stacksetName, 1, spec)
		} else {
			err = updateStackset(stacksetName, spec)
		}
		require.NoError(t, err)
		_, err = waitForStack(t, stacksetName, stackVersion)
		require.NoError(t, err)
		_, err = waitForDeployment(t, fmt.Sprintf("%s-%s", stacksetName, stackVersion))
		require.NoError(t, err)
	}

	// verify that only 3 stacks are present and the last 2 have been deleted
	for i := 2; i < 5; i++ {
		require.True(t, stackExists(stacksetName, fmt.Sprintf("v%d", i)))
	}

	// verify that the first 2 stacks which were created have been deleted
	for i := 0; i < 2; i++ {
		deploymentName := fmt.Sprintf("%s-v%d", stacksetName, i)
		err := resourceDeleted(t, "stack", deploymentName, deploymentInterface()).withTimeout(time.Second * 60).await()
		require.NoError(t, err)
		require.False(t, stackExists(stacksetName, fmt.Sprintf("v%d", i)))
	}
}

func TestStackTTLWithIngress(t *testing.T) {
	t.Parallel()
	stacksetName := "stackset-ttl-ingress"
	specFactory := NewTestStacksetSpecFactory(stacksetName).StackGC(3, 0).Ingress()

	// Create 5 stacks each with an ingress
	for i := 0; i < 5; i++ {
		stackVersion := fmt.Sprintf("v%d", i)
		var err error
		spec := specFactory.Create(stackVersion)
		if !stacksetExists(stacksetName) {
			err = createStackSet(stacksetName, 1, spec)
		} else {
			err = updateStackset(stacksetName, spec)
		}
		require.NoError(t, err)
		_, err = waitForStack(t, stacksetName, stackVersion)
		require.NoError(t, err)
		fullStackName := fmt.Sprintf("%s-%s", stacksetName, stackVersion)
		_, err = waitForIngress(t, fullStackName)
		require.NoError(t, err)

		// once the stack is created switch full traffic to it
		newWeight := map[string]float64{fullStackName: 100}
		err = setDesiredTrafficWeights(stacksetName, newWeight)
		require.NoError(t, err)
		err = trafficWeightsUpdated(t, stacksetName, weightKindActual, newWeight).withTimeout(10 * time.Minute).await()
		require.NoError(t, err)
	}

	// verify that only the last 3 created stacks are present
	for i := 2; i < 5; i++ {
		deploymentName := fmt.Sprintf("%s-v%d", stacksetName, i)
		require.True(t, stackExists(stacksetName, fmt.Sprintf("v%d", i)))
		_, err := waitForDeployment(t, deploymentName)
		require.NoError(t, err)
	}

	// verify that the first 2 created stacks have been deleted
	for i := 0; i < 2; i++ {
		deploymentName := fmt.Sprintf("%s-v%d", stacksetName, i)
		err := resourceDeleted(t, "stack", deploymentName, deploymentInterface()).withTimeout(time.Second * 60).await()
		require.NoError(t, err)
		require.False(t, stackExists(stacksetName, fmt.Sprintf("v%d", i)))
	}
}

// TestStackTTLForLatestStack tests that the latest stack gets scaled down and isn't treated differently
func TestStackTTLForLatestStack(t *testing.T) {
	t.Parallel()
	stacksetName := "stackset-ttl-last-stack"
	specFactory := NewTestStacksetSpecFactory(stacksetName).StackGC(1, 0).Ingress()

	// Create 2 stacks in total and wait for their deployments to come up
	for i := 0; i < 2; i++ {
		stackVersion := fmt.Sprintf("v%d", i)
		var err error
		spec := specFactory.Create(stackVersion)
		if !stacksetExists(stacksetName) {
			err = createStackSet(stacksetName, 1, spec)
		} else {
			err = updateStackset(stacksetName, spec)
		}
		require.NoError(t, err)

		_, err = waitForStack(t, stacksetName, stackVersion)
		require.NoError(t, err)

		fullStackName := fmt.Sprintf("%s-%s", stacksetName, stackVersion)
		_, err = waitForIngress(t, fullStackName)
		require.NoError(t, err)

		if i == 0 {
			// Explicitly switch traffic to the first stack
			newWeight := map[string]float64{fullStackName: 100}
			err = setDesiredTrafficWeights(stacksetName, newWeight)
			require.NoError(t, err)

			err = trafficWeightsUpdated(t, stacksetName, weightKindActual, newWeight).withTimeout(10 * time.Minute).await()
			require.NoError(t, err)
		}
	}

	// verify that the 1st stack exists and the latest stack was scaled down
	stackVersion := 0
	require.True(t, stackExists(stacksetName, fmt.Sprintf("v%d", stackVersion)))

	stackVersion = 1
	fullStackName := fmt.Sprintf("%s-v%d", stacksetName, stackVersion)

	err := stackStatusMatches(t, fullStackName, expectedStackStatus{
		replicas: pint32(0),
	}).await()
	require.NoError(t, err)
}
