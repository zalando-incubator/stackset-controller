package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
)

// TestConfigureHPA tests Behavior is reflected when stackset is created
func TestConfigureHPA(t *testing.T) {
	t.Parallel()
	stacksetName := "configured-hpa"
	var stabilizationWindow int32 = 60
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

// TestConfigureHPA tests Behavior is reflected when stackset is created
func TestConfigureAutoscaling(t *testing.T) {
	t.Parallel()
	stacksetName := "configured-autoscaler"
	var stabilizationWindow int32 = 60
	metrics := []zv1.AutoscalerMetrics{
		makeCPUAutoscalerMetrics(50),
		makeExternalAutoscalerMetrics("test", "eu-central-1", 10),
		makeObjectAutoscalerMetrics(20),
	}
	require.Len(t, metrics, 3)

	factory := NewTestStacksetSpecFactory(stacksetName).
		Ingress().
		Autoscaler(1, 10, metrics).
		Behavior(stabilizationWindow)
	firstVersion := "v1"
	spec := factory.Create(firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)

	fullFirstName := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	hpa, err := waitForHPA(t, fullFirstName)
	require.NoError(t, err)

	require.NotNil(t, hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds, "HPA StabilizationWindowSeconds is nil")
	require.EqualValues(t, stabilizationWindow, *hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds)
}
