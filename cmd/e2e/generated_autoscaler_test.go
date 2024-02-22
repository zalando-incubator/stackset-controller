package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	v2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/resource"
)

func makeCPUAutoscalerMetrics(utilization int32) zv1.AutoscalerMetrics {
	return zv1.AutoscalerMetrics{
		Type:               "CPU",
		AverageUtilization: pint32(utilization),
	}
}

func makeAmazonSQSAutoscalerMetrics(queueName, region string, averageQueueLength int64) zv1.AutoscalerMetrics {
	return zv1.AutoscalerMetrics{
		Type:    "AmazonSQS",
		Average: resource.NewQuantity(averageQueueLength, resource.DecimalSI),
		Queue:   &zv1.MetricsQueue{Name: queueName, Region: region},
	}
}

func makeIngressAutoscalerMetrics(average int64) zv1.AutoscalerMetrics {
	return zv1.AutoscalerMetrics{
		Type:    "Ingress",
		Average: resource.NewQuantity(average, resource.DecimalSI),
	}
}

func TestGenerateAutoscaler(t *testing.T) {
	t.Parallel()
	stacksetName := "generated-autoscaler"
	metrics := []zv1.AutoscalerMetrics{
		makeAmazonSQSAutoscalerMetrics("test", "eu-central-1", 10),
		makeCPUAutoscalerMetrics(50),
		makeIngressAutoscalerMetrics(20),
	}
	require.Len(t, metrics, 3)

	factory := NewTestStacksetSpecFactory(stacksetName).Ingress().Autoscaler(1, 10, metrics)
	firstVersion := "v1"
	spec := factory.Create(t, firstVersion)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)

	fullFirstName := fmt.Sprintf("%s-%s", stacksetName, firstVersion)
	hpa, err := waitForHPA(t, fullFirstName)
	require.NoError(t, err)
	require.EqualValues(t, 1, *hpa.Spec.MinReplicas)
	require.EqualValues(t, 10, hpa.Spec.MaxReplicas)
	require.Len(t, hpa.Spec.Metrics, 3)

	// we intentionally don't care about the order of the metrics because
	// it's not guaranteed in Kubernetes versions below v1.28
	// See https://github.com/zalando-incubator/stackset-controller/pull/591#issuecomment-1959751276
	metricTypes := map[v2.MetricSourceType]int64{
		v2.ExternalMetricSourceType: 10,
		v2.ResourceMetricSourceType: 50,
		v2.ObjectMetricSourceType:   20,
	}

	for _, metric := range hpa.Spec.Metrics {
		switch metric.Type {
		case v2.ExternalMetricSourceType:
			require.EqualValues(t, metricTypes[metric.Type], metric.External.Target.AverageValue.Value())
		case v2.ResourceMetricSourceType:
			require.EqualValues(t, metricTypes[metric.Type], *metric.Resource.Target.AverageUtilization)
		case v2.ObjectMetricSourceType:
			require.EqualValues(t, metricTypes[metric.Type], metric.Object.Target.AverageValue.Value())
		}
	}
}
