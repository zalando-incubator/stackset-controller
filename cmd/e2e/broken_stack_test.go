package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestBrokenStacks(t *testing.T) {
	t.Parallel()

	stacksetName := "stackset-broken-stacks"
	factory := NewTestStacksetSpecFactory(stacksetName).Ingress().StackGC(1, 30)

	firstVersion := "v1"
	firstStack := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	spec := factory.Create(firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, firstVersion)
	require.NoError(t, err)

	unhealthyVersion := "v2"
	unhealthyStack := fmt.Sprintf("%s-%s", stacksetName, unhealthyVersion)
	spec = factory.Create(unhealthyVersion)
	spec.StackTemplate.Spec.Service.Ports = []v1.ServicePort{
		{
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromString("foobar"),
		},
	}
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, unhealthyVersion)
	require.NoError(t, err)

	_, err = waitForIngress(t, stacksetName)
	require.NoError(t, err)

	initialWeights := map[string]float64{firstStack: 100}
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, initialWeights, nil).await()
	require.NoError(t, err)

	// Switch traffic to the second stack, this should fail
	desiredWeights := map[string]float64{unhealthyStack: 100}
	err = setDesiredTrafficWeightsStackset(stacksetName, desiredWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredWeights, nil).await()
	require.Error(t, err)

	// Create a healthy stack
	healthyVersion := "v3"
	healthyStack := fmt.Sprintf("%s-%s", stacksetName, healthyVersion)
	spec = factory.Create(healthyVersion)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, healthyVersion)
	require.NoError(t, err)

	healthyWeights := map[string]float64{healthyStack: 100}
	err = setDesiredTrafficWeightsStackset(stacksetName, healthyWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, healthyWeights, nil).await()
	require.NoError(t, err)

	// Create another healthy stack so we can test GC
	finalVersion := "v4"
	finalStack := fmt.Sprintf("%s-%s", stacksetName, finalVersion)
	spec = factory.Create(finalVersion)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, finalVersion)
	require.NoError(t, err)

	finalWeights := map[string]float64{finalStack: 100}
	err = setDesiredTrafficWeightsStackset(stacksetName, finalWeights)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, finalWeights, nil).await()
	require.NoError(t, err)

	// Check that the unhealthy stack was deleted
	for _, stack := range []string{unhealthyStack, firstStack} {
		err := resourceDeleted(t, "stack", stack, stackInterface()).withTimeout(time.Second * 60).await()
		require.NoError(t, err)
	}


}
