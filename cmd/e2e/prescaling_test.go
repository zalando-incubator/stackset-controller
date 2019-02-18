package main

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPrescalingWithoutHPA(t *testing.T) {
	t.Parallel()
	stacksetName := "stackset-prescale-no-hpa"
	specFactory := NewTestStacksetSpecFactory(stacksetName).Ingress().StackGC(3, 0).Replicas(3)

	// create stack with 3 replicas
	firstStack := "v1"
	spec := specFactory.Create(firstStack)
	err := createStackSet(stacksetName, true, spec)
	require.NoError(t, err)
	waitForStack(t, stacksetName, firstStack)

	// create second stack with 3 replicas
	secondStack := "v2"
	spec = specFactory.Create(secondStack)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	waitForStack(t, stacksetName, secondStack)

	// switch traffic so that both stacks are receiving equal traffic and verify traffic has actually switched
	fullFirstStack := fmt.Sprintf("%s-%s", stacksetName, firstStack)
	fullSecondStack := fmt.Sprintf("%s-%s", stacksetName, secondStack)
	_, err = waitForIngress(t, stacksetName)
	require.NoError(t, err)
	desiredTraffic := map[string]float64{
		fullFirstStack:  50,
		fullSecondStack: 50,
	}
	err = setDesiredTrafficWeights(stacksetName, desiredTraffic)
	require.NoError(t, err)
	err = trafficWeightsUpdated(t, stacksetName, weightKindActual, desiredTraffic).withTimeout(time.Minute * 4).await()
	require.NoError(t, err)

	// create third stack with only 1 replica and wait for the deployment to be created
	thirdStack := "v3"
	fullThirdStack := fmt.Sprintf("%s-%s", stacksetName, thirdStack)
	spec = specFactory.Replicas(1).Create(thirdStack)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	deployment, err := waitForDeployment(t, fullThirdStack)
	require.NoError(t, err)
	require.EqualValues(t, 1, *deployment.Spec.Replicas)

	// finally switch 1/10 of the traffic to the new stack
	desiredTraffic = map[string]float64{
		fullThirdStack:  10,
		fullFirstStack:  40,
		fullSecondStack: 50,
	}
	err = setDesiredTrafficWeights(stacksetName, desiredTraffic)
	require.NoError(t, err)
	err = trafficWeightsUpdated(t, stacksetName, weightKindActual, desiredTraffic).withTimeout(time.Minute * 4).await()
	require.NoError(t, err)

	// recheck the deployment of the last stack and verify that the number of replicas is the sum of the previous stacks
	deployment, err = waitForDeployment(t, fullThirdStack)
	require.NoError(t, err)
	require.EqualValues(t, 6, *(deployment.Spec.Replicas))
}

func TestPrescalingWithHPA(t *testing.T) {
	t.Parallel()
	stacksetName := "stackset-prescale-hpa"
	specFactory := NewTestStacksetSpecFactory(stacksetName).Ingress().StackGC(3, 0).
		HPA(1, 10).Replicas(3)

	// create first stack with 3 replicas
	firstStack := "v1"
	spec := specFactory.Create(firstStack)
	err := createStackSet(stacksetName, true, spec)
	require.NoError(t, err)
	waitForStack(t, stacksetName, firstStack)

	// create second stack with 3 replicas
	secondStack := "v2"
	spec = specFactory.Create(secondStack)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	waitForStack(t, stacksetName, secondStack)

	// switch traffic so that both stacks are receiving equal traffic
	fullFirstStack := fmt.Sprintf("%s-%s", stacksetName, firstStack)
	fullSecondStack := fmt.Sprintf("%s-%s", stacksetName, secondStack)
	_, err = waitForIngress(t, stacksetName)
	require.NoError(t, err)
	desiredTraffic := map[string]float64{
		fullFirstStack:  50,
		fullSecondStack: 50,
	}
	err = setDesiredTrafficWeights(stacksetName, desiredTraffic)
	require.NoError(t, err)
	err = trafficWeightsUpdated(t, stacksetName, weightKindActual, desiredTraffic).withTimeout(time.Minute * 4).await()
	require.NoError(t, err)

	// create a third stack with only one replica and verify the deployment has only one pod
	thirdStack := "v3"
	fullThirdStack := fmt.Sprintf("%s-%s", stacksetName, thirdStack)
	spec = specFactory.Replicas(1).Create(thirdStack)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	deployment, err := waitForDeployment(t, fullThirdStack)
	require.NoError(t, err)
	require.EqualValues(t, 1, *deployment.Spec.Replicas)

	// switch 1/10 of the traffic to the third stack and wait for the process to be complete
	desiredTraffic = map[string]float64{
		fullThirdStack:  10,
		fullFirstStack:  40,
		fullSecondStack: 50,
	}

	err = setDesiredTrafficWeights(stacksetName, desiredTraffic)
	require.NoError(t, err)
	err = trafficWeightsUpdated(t, stacksetName, weightKindActual, desiredTraffic).withTimeout(time.Minute * 4).await()
	require.NoError(t, err)

	// verify that the third stack now has 6 replicas
	deployment, err = waitForDeployment(t, fullThirdStack)
	require.NoError(t, err)
	require.EqualValues(t, 6, *(deployment.Spec.Replicas))
}
