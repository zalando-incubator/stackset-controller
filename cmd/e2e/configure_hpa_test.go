package main

import (
	"fmt"
	"testing"

	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"

	"github.com/stretchr/testify/require"
)

// TestConfigureNewHPA tests Behavior is reflected when stackset is created
func TestConfigureNewHPA(t *testing.T) {
	t.Parallel()
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

// TestBehaviorDefaults tests defaults are applied to HPA Behavior
func TestBehaviorDefaults(t *testing.T) {
	t.Parallel()
	stacksetName := "hpa-behavior"
	factory := NewTestStacksetSpecFactory(stacksetName).
		Ingress().
		HPA(1, 3)

	stackVersion := "v1"
	spec := factory.Create(stackVersion)
	scalePolicy := autoscalingv2beta2.MaxPolicySelect
	spec.StackTemplate.Spec.HorizontalPodAutoscaler.Behavior =
		&autoscalingv2beta2.HorizontalPodAutoscalerBehavior{
			ScaleDown: &autoscalingv2beta2.HPAScalingRules{
				SelectPolicy: &scalePolicy,
			},
		}

	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)

	fullFirstName := fmt.Sprintf("%s-%s", stacksetName, stackVersion)
	stack, err := waitForStack(t, stacksetName, stackVersion)
	require.NoError(t, err, "failed to create stack without stabilization")
	require.Nil(t, stack)

	_, err = waitForHPA(t, fullFirstName)
	require.NoError(t, err, "failed to create HPA")

	hpa, err := waitForHPA(t, fullFirstName)
	require.NoError(t, err)
	require.EqualValues(t, 300, *hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds)
}

// Test Behavior is reflected when Stack is updated
// if the user updates the config *and* stack version, hpa is updated
// if the user updates the config but *not* the stack version, hpa is not updated

// Test both HPA and autoscaling fields
