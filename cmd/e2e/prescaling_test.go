package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
)

func TestPrescalingWithoutHPA(t *testing.T) {
	t.Parallel()
	stacksetName := "stackset-prescale-no-hpa"
	specFactory := NewTestStacksetSpecFactory(stacksetName).Ingress().StackGC(3, 15).Replicas(3)

	// create stack with 3 replicas
	firstStack := "v1"
	spec := specFactory.Create(t, firstStack)
	err := createStackSet(stacksetName, 1, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, firstStack)
	require.NoError(t, err)

	// create second stack with 3 replicas
	secondStack := "v2"
	spec = specFactory.Create(t, secondStack)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, secondStack)
	require.NoError(t, err)

	// switch traffic so that both stacks are receiving equal traffic and verify traffic has actually switched
	fullFirstStack := fmt.Sprintf("%s-%s", stacksetName, firstStack)
	fullSecondStack := fmt.Sprintf("%s-%s", stacksetName, secondStack)
	_, err = waitForIngress(t, stacksetName)
	require.NoError(t, err)
	desiredTraffic := map[string]float64{
		fullFirstStack:  50,
		fullSecondStack: 50,
	}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredTraffic)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredTraffic, nil).withTimeout(time.Minute * 4).await()
	require.NoError(t, err)

	// create third stack with only 1 replica and wait for the deployment to be created
	thirdStack := "v3"
	fullThirdStack := fmt.Sprintf("%s-%s", stacksetName, thirdStack)
	spec = specFactory.Replicas(1).Create(t, thirdStack)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	deployment, err := waitForDeployment(t, fullThirdStack)
	require.NoError(t, err)
	require.EqualValues(t, 1, *deployment.Spec.Replicas)

	// switch 50% of the traffic to the new stack
	desiredTraffic = map[string]float64{
		fullThirdStack:  50,
		fullFirstStack:  25,
		fullSecondStack: 25,
	}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredTraffic)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredTraffic, nil).withTimeout(time.Minute * 4).await()
	require.NoError(t, err)

	// recheck the deployment of the last stack and verify that the number of replicas is 3 till the end of the
	// prescaling timeout
	for i := 1; i <= 6; i++ {
		deployment, err = waitForDeployment(t, fullThirdStack)
		require.NoError(t, err)
		require.EqualValues(t, 3, *(deployment.Spec.Replicas))
		time.Sleep(time.Second * 10)
	}
	time.Sleep(time.Second * 10)
	deployment, err = waitForDeployment(t, fullThirdStack)
	require.NoError(t, err)
	require.EqualValues(t, 1, *(deployment.Spec.Replicas))
}

func TestPrescalingWithHPA(t *testing.T) {
	t.Parallel()
	stacksetName := "stackset-prescale-hpa"
	specFactory := NewTestStacksetSpecFactory(stacksetName).Ingress().StackGC(3, 15).
		Autoscaler(1, 3, []zv1.AutoscalerMetrics{makeCPUAutoscalerMetrics(50)}).Replicas(3)

	// create first stack with 3 replicas
	firstStack := "v1"
	spec := specFactory.Create(t, firstStack)
	err := createStackSet(stacksetName, 1, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, firstStack)
	require.NoError(t, err)

	// create second stack with 3 replicas
	secondStack := "v2"
	spec = specFactory.Create(t, secondStack)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, secondStack)
	require.NoError(t, err)

	// switch traffic so that both stacks are receiving equal traffic
	fullFirstStack := fmt.Sprintf("%s-%s", stacksetName, firstStack)
	fullSecondStack := fmt.Sprintf("%s-%s", stacksetName, secondStack)
	_, err = waitForIngress(t, stacksetName)
	require.NoError(t, err)
	desiredTraffic := map[string]float64{
		fullFirstStack:  50,
		fullSecondStack: 50,
	}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredTraffic)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredTraffic, nil).withTimeout(time.Minute * 4).await()
	require.NoError(t, err)

	// create a third stack with only one replica and verify the deployment has only one pod
	thirdStack := "v3"
	fullThirdStack := fmt.Sprintf("%s-%s", stacksetName, thirdStack)
	spec = specFactory.Replicas(1).Create(t, thirdStack)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	deployment, err := waitForDeployment(t, fullThirdStack)
	require.NoError(t, err)
	require.EqualValues(t, 1, *deployment.Spec.Replicas)

	// switch 50% of the traffic to the third stack and wait for the process to be complete
	desiredTraffic = map[string]float64{
		fullThirdStack:  50,
		fullFirstStack:  25,
		fullSecondStack: 25,
	}

	err = setDesiredTrafficWeightsStackset(stacksetName, desiredTraffic)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredTraffic, nil).withTimeout(time.Minute * 4).await()
	require.NoError(t, err)

	// verify that the third stack now has 3 replicas till the end of the prescaling period
	for i := 1; i <= 6; i++ {
		hpa, err := waitForHPA(t, fullThirdStack)
		require.NoError(t, err)
		require.EqualValues(t, 3, *(hpa.Spec.MinReplicas))
		time.Sleep(time.Second * 10)
	}
	time.Sleep(time.Second * 10)
	hpa, err := waitForHPA(t, fullThirdStack)
	require.NoError(t, err)
	require.EqualValues(t, 1, *(hpa.Spec.MinReplicas))
}

func TestPrescalingPreventDelete(t *testing.T) {
	stackPrescalingTimeout := 5
	t.Parallel()
	stacksetName := "stackset-prevent-delete"
	factory := NewTestStacksetSpecFactory(stacksetName).StackGC(1, 15).Ingress().Replicas(3)

	// create stackset with first version
	firstVersion := "v1"
	fullFirstStack := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	firstCreateTimestamp := time.Now()
	err := createStackSet(stacksetName, stackPrescalingTimeout, factory.Create(t, firstVersion))
	require.NoError(t, err)
	_, err = waitForDeployment(t, fullFirstStack)
	require.NoError(t, err)
	_, err = waitForIngress(t, fullFirstStack)
	require.NoError(t, err)

	// update stackset with second version
	secondVersion := "v2"
	fullSecondStack := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	secondCreateTimestamp := time.Now()
	err = updateStackset(stacksetName, factory.Create(t, secondVersion))
	require.NoError(t, err)
	_, err = waitForDeployment(t, fullSecondStack)
	require.NoError(t, err)
	_, err = waitForIngress(t, fullSecondStack)
	require.NoError(t, err)

	// switch all traffic to the new stack
	desiredTrafficMap := map[string]float64{
		fullSecondStack: 100,
	}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredTrafficMap)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredTrafficMap, nil).withTimeout(2 * time.Minute).await()
	require.NoError(t, err)

	// update stackset with third version
	thirdVersion := "v3"
	fullThirdStack := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	thirdCreateTimestamp := time.Now()
	err = updateStackset(stacksetName, factory.Create(t, thirdVersion))
	require.NoError(t, err)
	_, err = waitForDeployment(t, fullThirdStack)
	require.NoError(t, err)
	_, err = waitForIngress(t, fullThirdStack)
	require.NoError(t, err)

	desiredTrafficMap = map[string]float64{
		fullThirdStack: 100,
	}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredTrafficMap)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredTrafficMap, nil).withTimeout(2 * time.Minute).await()
	require.NoError(t, err)

	// verify that all stack deployments are still present and their prescaling is active
	for time.Now().Before(firstCreateTimestamp.Add(time.Minute * time.Duration(stackPrescalingTimeout))) {
		firstDeployment, err := waitForDeployment(t, fullFirstStack)
		require.NoError(t, err)
		require.EqualValues(t, 3, *firstDeployment.Spec.Replicas)
		time.Sleep(15 * time.Second)
	}
	for time.Now().Before(secondCreateTimestamp.Add(time.Minute * time.Duration(stackPrescalingTimeout))) {
		secondDeployment, err := waitForDeployment(t, fullSecondStack)
		require.NoError(t, err)
		require.EqualValues(t, 3, *secondDeployment.Spec.Replicas)
		time.Sleep(15 * time.Second)
	}

	for time.Now().Before(thirdCreateTimestamp.Add(time.Minute * time.Duration(stackPrescalingTimeout))) {
		thirdDeployment, err := waitForDeployment(t, fullThirdStack)
		require.NoError(t, err)
		require.EqualValues(t, 3, *thirdDeployment.Spec.Replicas)
		time.Sleep(15 * time.Second)
	}
}

func TestPrescalingWaitsForBackends(t *testing.T) {
	// Create 3 stacks with traffic 0.0, 50.0 & 50.0
	// Switch traffic to be 0.0, 30, 70.0
	// Verify that actual traffic changes correctly

	t.Parallel()
	stacksetName := "stackset-prescale-backends-wait"
	specFactory := NewTestStacksetSpecFactory(stacksetName).Ingress().StackGC(3, 15).Replicas(3)

	// create stack with 3 replicas
	firstStack := "v1"
	spec := specFactory.Create(t, firstStack)
	err := createStackSet(stacksetName, 1, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, firstStack)
	require.NoError(t, err)

	// create second stack with 3 replicas
	secondStack := "v2"
	spec = specFactory.Create(t, secondStack)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, secondStack)
	require.NoError(t, err)

	// create third stack with 3 replicas
	thirdStack := "v3"
	spec = specFactory.Create(t, thirdStack)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, thirdStack)
	require.NoError(t, err)

	_, err = waitForIngress(t, stacksetName)
	require.NoError(t, err)

	// switch traffic so that all three stacks are receiving 0%, 50% & 50% traffic and verify traffic has actually switched
	fullFirstStack := fmt.Sprintf("%s-%s", stacksetName, firstStack)
	fullSecondStack := fmt.Sprintf("%s-%s", stacksetName, secondStack)
	fullThirdStack := fmt.Sprintf("%s-%s", stacksetName, thirdStack)

	desiredTraffic := map[string]float64{
		fullFirstStack:  0,
		fullSecondStack: 50,
		fullThirdStack:  50,
	}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredTraffic)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredTraffic, nil).withTimeout(time.Minute * 4).await()
	require.NoError(t, err)

	// switch traffic so that all three stacks are receiving 0%, 30% & 70% traffic respectively
	desiredTraffic = map[string]float64{
		fullFirstStack:  0,
		fullSecondStack: 30,
		fullThirdStack:  70,
	}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredTraffic)
	require.NoError(t, err)

	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredTraffic, func(actualTraffic map[string]float64) error {
		// err out if the traffic for any of the stacks is outside of the expected range
		if actualTraffic[fullFirstStack] > 0 {
			return fmt.Errorf("%v traffic not exactly %v", actualTraffic[fullFirstStack], 0)
		}

		if actualTraffic[fullSecondStack] > 50 || actualTraffic[fullSecondStack] < 30 {
			return fmt.Errorf("%v traffic not between %v and %v", actualTraffic[fullSecondStack], 30, 50)
		}

		if actualTraffic[fullThirdStack] > 70 || actualTraffic[fullThirdStack] < 50 {
			return fmt.Errorf("%v traffic not between %v and %v", actualTraffic[fullThirdStack], 50, 70)
		}

		return nil

	}).withTimeout(time.Minute * 4).await()
	require.NoError(t, err)
}
