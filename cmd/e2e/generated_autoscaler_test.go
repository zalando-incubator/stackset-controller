package main

import (
	"fmt"
	"testing"
	"time"

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

func TestAutoscalerWithoutTraffic(t *testing.T) {
	t.Parallel()
	stacksetName := "autoscaler-without-traffic"
	metrics := []zv1.AutoscalerMetrics{
		makeCPUAutoscalerMetrics(50),
	}
	factory := NewTestStacksetSpecFactory(stacksetName).Ingress().Autoscaler(1, 3, metrics).StackGC(1, 30)
	firstStack := "v1"
	fullFirstStack := fmt.Sprintf("%s-%s", stacksetName, firstStack)
	spec := factory.Create(t, firstStack)
	err := createStackSet(stacksetName, 0, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, firstStack)
	require.NoError(t, err)
	_, err = waitForHPA(t, fullFirstStack)
	require.NoError(t, err)

	secondStack := "v2"
	fullSecondStack := fmt.Sprintf("%s-%s", stacksetName, secondStack)
	spec = factory.Create(t, secondStack)
	err = updateStackset(stacksetName, spec)
	require.NoError(t, err)
	_, err = waitForStack(t, stacksetName, secondStack)
	require.NoError(t, err)
	_, err = waitForHPA(t, fullSecondStack)
	require.NoError(t, err)

	desiredTraffic := map[string]float64{
		fullFirstStack:  100,
		fullSecondStack: 0,
	}

	err = setDesiredTrafficWeightsStackset(stacksetName, desiredTraffic)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredTraffic, nil).withTimeout(time.Minute * 1).await()
	require.NoError(t, err)

	err = resourceDeleted(t, "hpa", fullSecondStack, hpaInterface()).withTimeout(time.Minute * 1).await()
	require.NoError(t, err)

	_, err = waitForHPA(t, fullFirstStack)
	require.NoError(t, err)

	// Switch traffic to 50 - 50
	desiredTraffic = map[string]float64{
		fullFirstStack:  50,
		fullSecondStack: 50,
	}

	err = setDesiredTrafficWeightsStackset(stacksetName, desiredTraffic)
	require.NoError(t, err)
	err = trafficWeightsUpdatedStackset(t, stacksetName, weightKindActual, desiredTraffic, nil).withTimeout(time.Minute * 1).await()
	require.NoError(t, err)
	_, err = waitForHPA(t, fullFirstStack)
	require.NoError(t, err)

	_, err = waitForHPA(t, fullSecondStack)
	require.NoError(t, err)
}
