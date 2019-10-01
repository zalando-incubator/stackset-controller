package core

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"k8s.io/api/autoscaling/v2beta1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func generateAutoscalerStub(minReplicas, maxReplicas int32) StackContainer {
	return StackContainer{
		Stack: &zv1.Stack{
			ObjectMeta: metav1.ObjectMeta{
				Name: "stackset-v1",
			},
			Spec: zv1.StackSpec{
				Autoscaler: &zv1.Autoscaler{
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics:     []zv1.AutoscalerMetrics{},
				},
			},
		},
		stacksetName: "stackset",
	}
}

func generateAutoscalerCPU(minReplicas, maxReplicas, utilization int32) StackContainer {
	container := generateAutoscalerStub(minReplicas, maxReplicas)
	container.Stack.Spec.Autoscaler.Metrics = append(
		container.Stack.Spec.Autoscaler.Metrics, zv1.AutoscalerMetrics{
			Type:               cpuMetricName,
			AverageUtilization: &utilization,
		})
	return container
}

func generateAutoscalerMemory(minReplicas, maxReplicas, utilization int32) StackContainer {
	container := generateAutoscalerStub(minReplicas, maxReplicas)
	container.Stack.Spec.Autoscaler.Metrics = append(
		container.Stack.Spec.Autoscaler.Metrics, zv1.AutoscalerMetrics{
			Type:               memoryMetricName,
			AverageUtilization: &utilization,
		})
	return container
}
func generateAutoscalerSQS(minReplicas, maxReplicas, utilization int32, queueName, queueRegion string) StackContainer {
	container := generateAutoscalerStub(minReplicas, maxReplicas)
	container.Stack.Spec.Autoscaler.Metrics = append(
		container.Stack.Spec.Autoscaler.Metrics, zv1.AutoscalerMetrics{
			Type: amazonSQSMetricName,
			Queue: &zv1.MetricsQueue{
				Name:   queueName,
				Region: queueRegion,
			},
			Average: resource.NewQuantity(int64(utilization), resource.DecimalSI),
		},
	)
	return container
}

func generateAutoscalerPodJson(minReplicas, maxReplicas, utilization, port int32, name, path, key string) StackContainer {
	container := generateAutoscalerStub(minReplicas, maxReplicas)
	container.Stack.Spec.Autoscaler.Metrics = append(
		container.Stack.Spec.Autoscaler.Metrics, zv1.AutoscalerMetrics{
			Type: podJSONMetricName,
			Endpoint: &zv1.MetricsEndpoint{
				Path: path,
				Name: name,
				Key:  key,
				Port: port,
			},
			Average: resource.NewQuantity(int64(utilization), resource.DecimalSI),
		},
	)
	return container
}
func generateAutoscalerIngress(minReplicas, maxReplicas, utilization int32) StackContainer {
	container := generateAutoscalerStub(minReplicas, maxReplicas)
	container.Stack.Spec.Autoscaler.Metrics = append(
		container.Stack.Spec.Autoscaler.Metrics, zv1.AutoscalerMetrics{
			Type:    ingressMetricName,
			Average: resource.NewQuantity(int64(utilization), resource.DecimalSI),
		},
	)
	return container
}

func TestStackSetController_ReconcileAutoscalersCPU(t *testing.T) {
	ssc := generateAutoscalerCPU(1, 10, 80)
	hpa, err := ssc.GenerateHPA()
	require.NoError(t, err, "failed to create an HPA")
	require.NotNil(t, hpa, "hpa not generated")
	require.Equal(t, int32(1), *hpa.Spec.MinReplicas, "min replicas not generated correctly")
	require.Equal(t, int32(10), hpa.Spec.MaxReplicas, "max replicas generated incorrectly")
	require.Len(t, hpa.Spec.Metrics, 1, "expected HPA to have 1 metric. instead got %d", len(hpa.Spec.Metrics))
	cpuMetric := hpa.Spec.Metrics[0]
	require.Equal(t, cpuMetric.Type, v2beta1.ResourceMetricSourceType)
	require.Equal(t, cpuMetric.Resource.Name, corev1.ResourceCPU)
	require.Equal(t, *cpuMetric.Resource.TargetAverageUtilization, int32(80))
}

func TestStackSetController_ReconcileAutoscalersMemory(t *testing.T) {
	ssc := generateAutoscalerMemory(1, 10, 80)
	hpa, err := ssc.GenerateHPA()
	require.NoError(t, err, "failed to create an HPA")
	require.NotNil(t, hpa, "hpa not generated")
	require.Equal(t, int32(1), *hpa.Spec.MinReplicas, "min replicas not generated correctly")
	require.Equal(t, int32(10), hpa.Spec.MaxReplicas, "max replicas generated incorrectly")
	require.Len(t, hpa.Spec.Metrics, 1, "expected HPA to have 1 metric. instead got %d", len(hpa.Spec.Metrics))
	memoryMetric := hpa.Spec.Metrics[0]
	require.Equal(t, memoryMetric.Type, v2beta1.ResourceMetricSourceType)
	require.Equal(t, memoryMetric.Resource.Name, corev1.ResourceMemory)
	require.Equal(t, *memoryMetric.Resource.TargetAverageUtilization, int32(80))
}

func TestStackSetController_ReconcileAutoscalersSQS(t *testing.T) {
	ssc := generateAutoscalerSQS(1, 10, 80, "test-queue", "test-region")
	hpa, err := ssc.GenerateHPA()
	require.NoError(t, err, "failed to create an HPA")
	require.NotNil(t, hpa, "hpa not generated")
	require.Equal(t, int32(1), *hpa.Spec.MinReplicas, "min replicas not generated correctly")
	require.Equal(t, int32(10), hpa.Spec.MaxReplicas, "max replicas generated incorrectly")
	require.Len(t, hpa.Spec.Metrics, 1, "expected HPA to have 1 metric. instead got %d", len(hpa.Spec.Metrics))
	externalMetric := hpa.Spec.Metrics[0]
	require.Equal(t, externalMetric.Type, v2beta1.ExternalMetricSourceType)
	require.Equal(t, externalMetric.External.MetricName, "sqs-queue-length")
	require.Equal(t, externalMetric.External.MetricSelector.MatchLabels["queue-name"], "test-queue")
	require.Equal(t, externalMetric.External.MetricSelector.MatchLabels["queue-name"], "test-queue")
	require.Equal(t, externalMetric.External.TargetAverageValue.Value(), int64(80))
}

func TestStackSetController_ReconcileAutoscalersPodJson(t *testing.T) {
	ssc := generateAutoscalerPodJson(1, 10, 80, 8080, "current-load", "/metrics", "$.current-load.counter")
	hpa, err := ssc.GenerateHPA()
	require.NoError(t, err, "failed to create an HPA")
	require.NotNil(t, hpa, "hpa not generated")
	require.Equal(t, int32(1), *hpa.Spec.MinReplicas, "min replicas not generated correctly")
	require.Equal(t, int32(10), hpa.Spec.MaxReplicas, "max replicas generated incorrectly")
	require.Len(t, hpa.Spec.Metrics, 1, "expected HPA to have 1 metric. instead got %d", len(hpa.Spec.Metrics))
	podMetrics := hpa.Spec.Metrics[0]
	require.Equal(t, podMetrics.Type, v2beta1.PodsMetricSourceType)
	require.Equal(t, hpa.Annotations["metric-config.pods.current-load.json-path/json-key"], "$.current-load.counter")
	require.Equal(t, hpa.Annotations["metric-config.pods.current-load.json-path/path"], "/metrics")
	require.Equal(t, hpa.Annotations["metric-config.pods.current-load.json-path/port"], "8080")
	require.Equal(t, podMetrics.Pods.TargetAverageValue.Value(), int64(80))
	require.Equal(t, podMetrics.Pods.MetricName, "current-load")
}

func TestStackSetController_ReconcileAutoscalersIngress(t *testing.T) {
	ssc := generateAutoscalerIngress(1, 10, 80)
	hpa, err := ssc.GenerateHPA()
	require.NoError(t, err, "failed to create an HPA")
	require.NotNil(t, hpa, "hpa not generated")
	require.Equal(t, int32(1), *hpa.Spec.MinReplicas, "min replicas not generated correctly")
	require.Equal(t, int32(10), hpa.Spec.MaxReplicas, "max replicas generated incorrectly")
	require.Len(t, hpa.Spec.Metrics, 1, "expected HPA to have 1 metric. instead got %d", len(hpa.Spec.Metrics))
	ingressMetrics := hpa.Spec.Metrics[0]
	require.Equal(t, ingressMetrics.Type, v2beta1.ObjectMetricSourceType)
	require.Equal(t, ingressMetrics.Object.TargetValue.Value(), int64(80))
	require.Equal(t, ingressMetrics.Object.MetricName, fmt.Sprintf("%s,%s", "requests-per-second", "stackset-v1"))
}

func TestCPUMetricValid(t *testing.T) {
	var utilization int32 = 80
	metrics := zv1.AutoscalerMetrics{Type: "cpu", AverageUtilization: &utilization}
	metric, err := cpuMetric(metrics)
	require.NoError(t, err, "could not create hpa metric")
	require.Equal(t, metric.Resource.Name, corev1.ResourceCPU)
}

func TestCPUMetricInValid(t *testing.T) {
	metrics := zv1.AutoscalerMetrics{Type: "cpu", AverageUtilization: nil}
	_, err := cpuMetric(metrics)
	require.Error(t, err, "created metric even when utilization not specified")
}
func TestMemoryMetricValid(t *testing.T) {
	var utilization int32 = 80
	metrics := zv1.AutoscalerMetrics{Type: "memory", AverageUtilization: &utilization}
	metric, err := memoryMetric(metrics)
	require.NoError(t, err, "could not create hpa metric")
	require.Equal(t, metric.Resource.Name, corev1.ResourceMemory)
}

func TestMemoryMetricInValid(t *testing.T) {
	metrics := zv1.AutoscalerMetrics{Type: "memory", AverageUtilization: nil}
	_, err := memoryMetric(metrics)
	require.Error(t, err, "created metric even when utilization not specified")
}

func TestPodJsonMetricInvalid(t *testing.T) {
	endpoints := []zv1.MetricsEndpoint{
		{
			Path: "/metrics",
			Port: 8080,
			Key:  "$.metrics_key",
		},
		{
			Path: "/metrics",
			Port: 8080,
			Name: "metric-name",
		},
		{
			Path: "/metrics",
			Key:  "$.metrics_key",
			Name: "metric-name",
		},
		{
			Port: 8080,
			Key:  "$.metrics_key",
			Name: "metric-name",
		},
	}
	for _, e := range endpoints {
		metrics := zv1.AutoscalerMetrics{Type: podJSONMetricName, Endpoint: &e}
		_, _, err := podJsonMetric(metrics)
		require.Error(t, err, "created metric with invalid configuration")
	}
}

func TestIngressMetricInvalid(t *testing.T) {
	metrics := zv1.AutoscalerMetrics{Type: ingressMetricName, Average: nil}
	_, err := ingressMetric(metrics, "stack-name", "test-stack")
	require.Errorf(t, err, "created metric with invalid configuration")
}

func TestSortingMetrics(t *testing.T) {
	container := generateAutoscalerStub(1, 10)
	metrics := []zv1.AutoscalerMetrics{
		{Type: cpuMetricName, AverageUtilization: pint32(50)},
		{Type: ingressMetricName, Average: resource.NewQuantity(10, resource.DecimalSI)},
		{Type: podJSONMetricName, Average: resource.NewQuantity(10, resource.DecimalSI), Endpoint: &zv1.MetricsEndpoint{Name: "abc", Path: "/metrics", Port: 1222, Key: "test.abc"}},
		{Type: amazonSQSMetricName, Average: resource.NewQuantity(10, resource.DecimalSI), Queue: &zv1.MetricsQueue{Name: "test", Region: "region"}},
	}
	container.Stack.Spec.Autoscaler.Metrics = metrics
	hpa, err := container.GenerateHPA()
	require.NoError(t, err, "failed to create an HPA")
	require.NotNil(t, hpa, "hpa not generated")
	require.Len(t, hpa.Spec.Metrics, 4)
	require.EqualValues(t, v2beta1.ExternalMetricSourceType, hpa.Spec.Metrics[0].Type)
	require.EqualValues(t, v2beta1.ObjectMetricSourceType, hpa.Spec.Metrics[1].Type)
	require.EqualValues(t, v2beta1.PodsMetricSourceType, hpa.Spec.Metrics[2].Type)
	require.EqualValues(t, v2beta1.ResourceMetricSourceType, hpa.Spec.Metrics[3].Type)
}

func pint32(val int) *int32 {
	return &[]int32{int32(val)}[0]
}

func generateHPA(minReplicas, maxReplicas int32) StackContainer {
	return StackContainer{
		Stack: &zv1.Stack{
			ObjectMeta: metav1.ObjectMeta{
				Name: "stackset-v1",
			},
			Spec: zv1.StackSpec{
				HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics:     []autoscaling.MetricSpec{},
				},
			},
		},
		stacksetName: "stackset",
	}
}

func TestStackSetController_ReconcileHPA(t *testing.T) {
	ssc := generateHPA(1, 10)
	hpa, err := ssc.GenerateHPA()
	require.NoError(t, err, "failed to create an HPA")
	require.NotNil(t, hpa, "hpa not generated")
	require.Equal(t, int32(1), *hpa.Spec.MinReplicas, "min replicas not generated correctly")
	require.Equal(t, int32(10), hpa.Spec.MaxReplicas, "max replicas generated incorrectly")
}
