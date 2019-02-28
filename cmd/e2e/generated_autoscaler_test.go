package main

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/zalando-incubator/stackset-controller/controller"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	"testing"
)

func makeCPUAutoscalerMetrics(utilization int32) zv1.AutoscalerMetrics {
	return zv1.AutoscalerMetrics{
		Type:               controller.CPUMetricName,
		AverageUtilization: pint32(utilization),
	}
}

func makeExternalAutoscalerMetrics(queueName, region string, averageQueueLength int64) zv1.AutoscalerMetrics {
	return zv1.AutoscalerMetrics{
		Type:    controller.AmazonSQSMetricName,
		Average: resource.NewQuantity(averageQueueLength, resource.DecimalSI),
		Queue:   &zv1.MetricsQueue{Name: queueName, Region: region},
	}
}

func makeObjectAutoscalerMetrics(average int64) zv1.AutoscalerMetrics {
	return zv1.AutoscalerMetrics{
		Type:    controller.IngressMetricName,
		Average: resource.NewQuantity(average, resource.DecimalSI),
	}
}

func TestGenerateAutoscaler(t *testing.T) {
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
	require.EqualValues(t, metric1.External.TargetAverageValue.Value(), 10)
	metric2 := hpa.Spec.Metrics[1]
	require.EqualValues(t, metric2.Type, v2beta1.ObjectMetricSourceType)
	require.EqualValues(t, metric2.Object.TargetValue.Value(), 20)
	metric3 := hpa.Spec.Metrics[2]
	require.EqualValues(t, metric3.Type, v2beta1.ResourceMetricSourceType)
	require.EqualValues(t, *metric3.Resource.TargetAverageUtilization, 50)
}
