package controller

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func generateSSAutoscalerStub(minReplicas, maxReplicas int32) StackSetContainer {
	container := StackSetContainer{
		StackSet: zv1.StackSet{
			Spec: zv1.StackSetSpec{
				StackTemplate: zv1.StackTemplate{
					Spec: zv1.StackSpecTemplate{
						StackSpec: zv1.StackSpec{
							Autoscaler: &zv1.Autoscaler{
								MinReplicas: &minReplicas,
								MaxReplicas: maxReplicas,
								Metrics: []zv1.AutoscalerMetrics{
								},
							},
						},
					},
				},
			},
		},
	}
	return container
}

func generateSSAutoscalerCPU(minReplicas, maxReplicas, utilization int32) StackSetContainer {
	container := generateSSAutoscalerStub(minReplicas, maxReplicas)
	container.StackSet.Spec.StackTemplate.Spec.Autoscaler.Metrics = append(
		container.StackSet.Spec.StackTemplate.Spec.Autoscaler.Metrics, zv1.AutoscalerMetrics{
			Type:               CPUMetricName,
			AverageUtilization: &utilization,
		})
	return container
}

func generateSSAutoscalerMemory(minReplicas, maxReplicas, utilization int32) StackSetContainer {
	container := generateSSAutoscalerStub(minReplicas, maxReplicas)
	container.StackSet.Spec.StackTemplate.Spec.Autoscaler.Metrics = append(
		container.StackSet.Spec.StackTemplate.Spec.Autoscaler.Metrics, zv1.AutoscalerMetrics{
			Type:               MemoryMetricName,
			AverageUtilization: &utilization,
		})
	return container
}
func generateSSAutoscalerSQS(minReplicas, maxReplicas, utilization int32, queueName, queueRegion string) StackSetContainer {
	container := generateSSAutoscalerStub(minReplicas, maxReplicas)
	container.StackSet.Spec.StackTemplate.Spec.Autoscaler.Metrics = append(
		container.StackSet.Spec.StackTemplate.Spec.Autoscaler.Metrics, zv1.AutoscalerMetrics{
			Type: AmazonSQSMetricName,
			Queue: &zv1.MetricsQueue{
				Name:   queueName,
				Region: queueRegion,
			},
			Average: &utilization,
		},
	)
	return container
}

func generateSSAutoscalerPodJson(minReplicas, maxReplicas, utilization, port int32, name, path, key string) StackSetContainer {
	container := generateSSAutoscalerStub(minReplicas, maxReplicas)
	container.StackSet.Spec.StackTemplate.Spec.Autoscaler.Metrics = append(
		container.StackSet.Spec.StackTemplate.Spec.Autoscaler.Metrics, zv1.AutoscalerMetrics{
			Type: PodJSONMetricName,
			Endpoint: &zv1.MetricsEndpoint{
				Path: path,
				Name: name,
				Key:  key,
				Port: port,
			},
			Average: &utilization,
		},
	)
	return container
}
func generateSSAutoscalerIngress(minReplicas, maxReplicas, utilization int32) StackSetContainer {
	container := generateSSAutoscalerStub(minReplicas, maxReplicas)
	container.StackSet.Spec.StackTemplate.Spec.Autoscaler.Metrics = append(
		container.StackSet.Spec.StackTemplate.Spec.Autoscaler.Metrics, zv1.AutoscalerMetrics{
			Type:    IngressMetricName,
			Average: &utilization,
		},
	)
	return container
}

func TestStackSetController_ReconcileAutoscalersCPU(t *testing.T) {
	ssc := generateSSAutoscalerCPU(1, 10, 80)
	reconciler := NewAutoscalerReconciler(ssc)
	sc := &StackContainer{}
	err := reconciler.Reconcile(sc)
	assert.NoError(t, err, "failed to reconcile autoscaler")
	hpa := sc.Stack.Spec.HorizontalPodAutoscaler
	assert.NotNil(t, hpa, "hpa not generated")
	assert.Equal(t, int32(1), *hpa.MinReplicas, "min replicas not generated correctly")
	assert.Equal(t, int32(10), hpa.MaxReplicas, "max replicas generated incorrectly")
	assert.Len(t, hpa.Metrics, 1, "expected HPA to have 1 metric. instead got %d", len(hpa.Metrics))
	cpuMetric := hpa.Metrics[0]
	assert.Equal(t, cpuMetric.Type, v2beta1.ResourceMetricSourceType)
	assert.Equal(t, cpuMetric.Resource.Name, corev1.ResourceCPU)
	assert.Equal(t, *cpuMetric.Resource.TargetAverageUtilization, int32(80))
}

func TestStackSetController_ReconcileAutoscalersMemory(t *testing.T) {
	ssc := generateSSAutoscalerMemory(1, 10, 80)
	reconciler := NewAutoscalerReconciler(ssc)
	sc := &StackContainer{}
	err := reconciler.Reconcile(sc)
	assert.NoError(t, err, "failed to reconcile autoscaler")
	hpa := sc.Stack.Spec.HorizontalPodAutoscaler
	assert.NotNil(t, hpa, "hpa not generated")
	assert.Equal(t, int32(1), *hpa.MinReplicas, "min replicas not generated correctly")
	assert.Equal(t, int32(10), hpa.MaxReplicas, "max replicas generated incorrectly")
	assert.Len(t, hpa.Metrics, 1, "expected HPA to have 1 metric. instead got %d", len(hpa.Metrics))
	memoryMetric := hpa.Metrics[0]
	assert.Equal(t, memoryMetric.Type, v2beta1.ResourceMetricSourceType)
	assert.Equal(t, memoryMetric.Resource.Name, corev1.ResourceMemory)
	assert.Equal(t, *memoryMetric.Resource.TargetAverageUtilization, int32(80))
}
func TestStackSetController_ReconcileAutoscalersSQS(t *testing.T) {
	ssc := generateSSAutoscalerSQS(1, 10, 80, "test-queue", "test-region")
	reconciler := NewAutoscalerReconciler(ssc)
	sc := &StackContainer{}
	err := reconciler.Reconcile(sc)
	assert.NoError(t, err, "failed to reconcile autoscaler")
	hpa := sc.Stack.Spec.HorizontalPodAutoscaler
	assert.NotNil(t, hpa, "hpa not generated")
	assert.Equal(t, int32(1), *hpa.MinReplicas, "min replicas not generated correctly")
	assert.Equal(t, int32(10), hpa.MaxReplicas, "max replicas generated incorrectly")
	assert.Len(t, hpa.Metrics, 1, "expected HPA to have 1 metric. instead got %d", len(hpa.Metrics))
	externalMetric := hpa.Metrics[0]
	assert.Equal(t, externalMetric.Type, v2beta1.ExternalMetricSourceType)
	assert.Equal(t, externalMetric.External.MetricName, "sqs-queue-length")
	assert.Equal(t, externalMetric.External.MetricSelector.MatchLabels["queue-name"], "test-queue")
	assert.Equal(t, externalMetric.External.MetricSelector.MatchLabels["queue-name"], "test-queue")
	assert.Equal(t, externalMetric.External.TargetAverageValue.Value(), int64(80))
}

func TestStackSetController_ReconcileAutoscalersPodJson(t *testing.T) {
	ssc := generateSSAutoscalerPodJson(1, 10, 80, 8080, "current-load", "/metrics", "$.current-load.counter")
	reconciler := NewAutoscalerReconciler(ssc)
	sc := &StackContainer{}
	err := reconciler.Reconcile(sc)
	assert.NoError(t, err, "failed to reconcile autoscaler")
	hpa := sc.Stack.Spec.HorizontalPodAutoscaler
	assert.NotNil(t, hpa, "hpa not generated")
	assert.Equal(t, int32(1), *hpa.MinReplicas, "min replicas not generated correctly")
	assert.Equal(t, int32(10), hpa.MaxReplicas, "max replicas generated incorrectly")
	assert.Len(t, hpa.Metrics, 1, "expected HPA to have 1 metric. instead got %d", len(hpa.Metrics))
	podMetrics := hpa.Metrics[0]
	assert.Equal(t, podMetrics.Type, v2beta1.PodsMetricSourceType)
	assert.Equal(t, hpa.Annotations["metric-config.pods.current-load.json-path/json-key"], "$.current-load.counter")
	assert.Equal(t, hpa.Annotations["metric-config.pods.current-load.json-path/path"], "/metrics")
	assert.Equal(t, hpa.Annotations["metric-config.pods.current-load.json-path/port"], "8080")
	assert.Equal(t, podMetrics.Pods.TargetAverageValue.Value(), int64(80))
	assert.Equal(t, podMetrics.Pods.MetricName, "current-load")
}
func TestStackSetController_ReconcileAutoscalersIngress(t *testing.T) {
	ssc := generateSSAutoscalerIngress(1, 10, 80)
	reconciler := NewAutoscalerReconciler(ssc)
	sc := &StackContainer{Stack: zv1.Stack{ObjectMeta: metav1.ObjectMeta{Name: "test-stack"}}}
	err := reconciler.Reconcile(sc)
	assert.NoError(t, err, "failed to reconcile autoscaler")
	hpa := sc.Stack.Spec.HorizontalPodAutoscaler
	assert.NotNil(t, hpa, "hpa not generated")
	assert.Equal(t, int32(1), *hpa.MinReplicas, "min replicas not generated correctly")
	assert.Equal(t, int32(10), hpa.MaxReplicas, "max replicas generated incorrectly")
	assert.Len(t, hpa.Metrics, 1, "expected HPA to have 1 metric. instead got %d", len(hpa.Metrics))
	ingressMetrics := hpa.Metrics[0]
	assert.Equal(t, ingressMetrics.Type, v2beta1.ObjectMetricSourceType)
	assert.Equal(t, ingressMetrics.Object.TargetValue.Value(), int64(80))
	assert.Equal(t, ingressMetrics.Object.MetricName, fmt.Sprintf("%s,%s", "requests-per-second", "test-stack"))
}

func TestCPUMetricValid(t *testing.T) {
	var utilization int32 = 80
	metrics := zv1.AutoscalerMetrics{Type: "cpu", AverageUtilization: &utilization}
	metric, err := CPUMetric(metrics)
	assert.NoError(t, err, "could not create hpa metric")
	assert.Equal(t, metric.Resource.Name, corev1.ResourceCPU)
}

func TestCPUMetricInValid(t *testing.T) {
	metrics := zv1.AutoscalerMetrics{Type: "cpu", AverageUtilization: nil}
	_, err := CPUMetric(metrics)
	assert.Error(t, err, "created metric even when utilization not specified")
}
func TestMemoryMetricValid(t *testing.T) {
	var utilization int32 = 80
	metrics := zv1.AutoscalerMetrics{Type: "memory", AverageUtilization: &utilization}
	metric, err := MemoryMetric(metrics)
	assert.NoError(t, err, "could not create hpa metric")
	assert.Equal(t, metric.Resource.Name, corev1.ResourceMemory)
}

func TestMemoryMetricInValid(t *testing.T) {
	metrics := zv1.AutoscalerMetrics{Type: "memory", AverageUtilization: nil}
	_, err := MemoryMetric(metrics)
	assert.Error(t, err, "created metric even when utilization not specified")
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
		metrics := zv1.AutoscalerMetrics{Type: PodJSONMetricName, Endpoint: &e}
		_, _, err := PodJsonMetric(metrics)
		assert.Error(t, err, "created metric with invalid configuration")
	}
}

func TestIngressMetricInvalid(t *testing.T) {
	metrics := zv1.AutoscalerMetrics{Type: IngressMetricName, Average: nil}
	_, err := IngressMetric(metrics, "stack-name", "test-stack")
	assert.Errorf(t, err, "created metric with invalid configuration")
}
