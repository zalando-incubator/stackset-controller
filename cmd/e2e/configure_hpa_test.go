package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfigureNewHPA tests Behavior is reflected when stackset is create
func TestConfigureNewHPA(t *testing.T) {
	stacksetName := "configured-hpa"
	var stabilizationWindow int32 = 300
	factory := NewTestStacksetSpecFactory(stacksetName).
		Ingress().
		HPA(1, 3).
		Behavior(stabilizationWindow)
	firstVersion := "v1"
	spec := factory.Create(firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)

	fullFirstName := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	hpa, err := waitForHPA(t, fullFirstName)
	require.NoError(t, err)
	require.EqualValues(t, 1, *hpa.Spec.MinReplicas)
	require.EqualValues(t, stabilizationWindow, *hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds)
}

// Test Behavior is reflected when Stack is updated
// if the user updates the config *and* stack version, hpa is updated
// if the user updates the config but *not* the stack version, hpa is not updated

// Test both HPA and autoscaling fields
