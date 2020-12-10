package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func makeCPUAutoscalerMetrics(utilization int32) zv1.AutoscalerMetrics {
	return zv1.AutoscalerMetrics{
		Type:               "CPU",
		AverageUtilization: pint32(utilization),
	}
}

func makeExternalAutoscalerMetrics(queueName, region string, averageQueueLength int64) zv1.AutoscalerMetrics {
	return zv1.AutoscalerMetrics{
		Type:    "AmazonSQS",
		Average: resource.NewQuantity(averageQueueLength, resource.DecimalSI),
		Queue:   &zv1.MetricsQueue{Name: queueName, Region: region},
	}
}

func makeObjectAutoscalerMetrics(average int64) zv1.AutoscalerMetrics {
	return zv1.AutoscalerMetrics{
		Type:    "Ingress",
		Average: resource.NewQuantity(average, resource.DecimalSI),
	}
}

func TestGenerateAutoscaler(t *testing.T) {
	t.Parallel()
	stacksetName := "generated-autoscaler"
	metrics := []zv1.AutoscalerMetrics{
		makeCPUAutoscalerMetrics(50),
		makeExternalAutoscalerMetrics("test", "eu-central-1", 10),
		makeObjectAutoscalerMetrics(20),
	}
	require.Len(t, metrics, 3)

	factory := NewTestStacksetSpecFactory(stacksetName).Ingress().Autoscaler(1, 10, metrics)
	firstVersion := "v1"
	spec := factory.Create(firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)

	fullFirstName := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	hpa, err := waitForHPA(t, fullFirstName)
	require.NoError(t, err)
	require.EqualValues(t, 1, *hpa.Spec.MinReplicas)
	require.EqualValues(t, 10, hpa.Spec.MaxReplicas)
	require.Len(t, hpa.Spec.Metrics, 3)
	metric1 := hpa.Spec.Metrics[0]
	require.EqualValues(t, metric1.Type, v2beta1.ExternalMetricSourceType)
	require.EqualValues(t, metric1.External.Target.AverageValue.Value(), 10)
	metric2 := hpa.Spec.Metrics[1]
	require.EqualValues(t, v2beta1.ObjectMetricSourceType, metric2.Type)
	require.EqualValues(t, 20, metric2.Object.Target.AverageValue.Value())
	metric3 := hpa.Spec.Metrics[2]
	require.EqualValues(t, v2beta1.ResourceMetricSourceType, metric3.Type)
	require.EqualValues(t, 50, *metric3.Resource.Target.AverageUtilization)
}
