package core

import (
	"fmt"
	"sort"
	"strconv"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	amazonSQSMetricName   = "AmazonSQS"
	podJSONMetricName     = "PodJSON"
	ingressMetricName     = "Ingress"
	cpuMetricName         = "CPU"
	memoryMetricName      = "Memory"
	zmonMetricName        = "ZMON"
	requestsPerSecondName = "requests-per-second"
	metricConfigJSONPath  = "metric-config.pods.%s.json-path/path"
	metricConfigJSONKey   = "metric-config.pods.%s.json-path/json-key"
	metricConfigJSONPort  = "metric-config.pods.%s.json-path/port"
	sqsQueueLengthTag     = "sqs-queue-length"
	sqsQueueNameTag       = "queue-name"
	sqsQueueRegionTag     = "region"
)

type MetricsList []autoscaling.MetricSpec

func (l MetricsList) Len() int {
	return len(l)
}

func (l MetricsList) Swap(i, j int) {
	temp := l[i]
	l[i] = l[j]
	l[j] = temp
}

func (l MetricsList) Less(i, j int) bool {
	return l[i].Type < l[j].Type
}

func convertCustomMetrics(stacksetName, stackName string, metrics []zv1.AutoscalerMetrics) ([]autoscaling.MetricSpec, map[string]string, error) {
	var resultMetrics MetricsList
	resultAnnotations := make(map[string]string)

	for _, m := range metrics {
		var (
			generated   *autoscaling.MetricSpec
			annotations map[string]string
			err         error
		)
		switch m.Type {
		case amazonSQSMetricName:
			generated, err = sqsMetric(m)
		case podJSONMetricName:
			generated, annotations, err = podJsonMetric(m)
		case ingressMetricName:
			generated, err = ingressMetric(m, stacksetName, stackName)
		case zmonMetricName:
			err = fmt.Errorf("not implemented metric type: %s", zmonMetricName)
		case cpuMetricName:
			generated, err = cpuMetric(m)
		case memoryMetricName:
			generated, err = memoryMetric(m)
		default:
			err = fmt.Errorf("metric type %s not supported", m.Type)
		}

		if err != nil {
			return nil, nil, err
		}
		resultMetrics = append(resultMetrics, *generated)
		for k, v := range annotations {
			resultAnnotations[k] = v
		}
	}

	sort.Sort(resultMetrics)
	return resultMetrics, resultAnnotations, nil
}

func memoryMetric(metrics zv1.AutoscalerMetrics) (*autoscaling.MetricSpec, error) {
	if metrics.AverageUtilization == nil {
		return nil, fmt.Errorf("utilization is not specified")
	}
	generated := &autoscaling.MetricSpec{
		Type: autoscaling.ResourceMetricSourceType,
		Resource: &autoscaling.ResourceMetricSource{
			Name:                     v1.ResourceMemory,
			TargetAverageUtilization: metrics.AverageUtilization,
		},
	}
	return generated, nil
}

func cpuMetric(metrics zv1.AutoscalerMetrics) (*autoscaling.MetricSpec, error) {
	if metrics.AverageUtilization == nil {
		return nil, fmt.Errorf("utilization is not specified")
	}
	generated := &autoscaling.MetricSpec{
		Type: autoscaling.ResourceMetricSourceType,
		Resource: &autoscaling.ResourceMetricSource{
			Name:                     v1.ResourceCPU,
			TargetAverageUtilization: metrics.AverageUtilization,
		},
	}
	return generated, nil
}

func sqsMetric(metrics zv1.AutoscalerMetrics) (*autoscaling.MetricSpec, error) {
	if metrics.Average == nil {
		return nil, fmt.Errorf("average not specified")
	}
	if metrics.Queue == nil || metrics.Queue.Name == "" || metrics.Queue.Region == "" {
		return nil, fmt.Errorf("queue not specified correctly")
	}
	average := metrics.Average.DeepCopy()
	generated := &autoscaling.MetricSpec{
		Type: autoscaling.ExternalMetricSourceType,
		External: &autoscaling.ExternalMetricSource{
			MetricName: sqsQueueLengthTag,
			MetricSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{sqsQueueNameTag: metrics.Queue.Name, sqsQueueRegionTag: metrics.Queue.Region},
			},
			TargetAverageValue: &average,
		},
	}
	return generated, nil
}

func podJsonMetric(metrics zv1.AutoscalerMetrics) (*autoscaling.MetricSpec, map[string]string, error) {
	if metrics.Average == nil {
		return nil, nil, fmt.Errorf("average is not specified for metric")
	}
	generated := &autoscaling.MetricSpec{
		Type: autoscaling.PodsMetricSourceType,
		Pods: &autoscaling.PodsMetricSource{
			MetricName:         metrics.Endpoint.Name,
			TargetAverageValue: metrics.Average.DeepCopy(),
		},
	}
	if metrics.Endpoint == nil || metrics.Endpoint.Port == 0 || metrics.Endpoint.Path == "" || metrics.Endpoint.Key == "" || metrics.Endpoint.Name == "" {
		return nil, nil, fmt.Errorf("the metrics endpoint is not specified correctly")
	}
	annotations := map[string]string{
		fmt.Sprintf(metricConfigJSONKey, metrics.Endpoint.Name):  metrics.Endpoint.Key,
		fmt.Sprintf(metricConfigJSONPath, metrics.Endpoint.Name): metrics.Endpoint.Path,
		fmt.Sprintf(metricConfigJSONPort, metrics.Endpoint.Name): strconv.Itoa(int(metrics.Endpoint.Port)),
	}
	return generated, annotations, nil
}

func ingressMetric(metrics zv1.AutoscalerMetrics, ingressName, backendName string) (*autoscaling.MetricSpec, error) {
	if metrics.Average == nil {
		return nil, fmt.Errorf("average value not specified for metric")
	}

	average := metrics.Average.DeepCopy()

	generated := &autoscaling.MetricSpec{
		Type: autoscaling.ObjectMetricSourceType,
		Object: &autoscaling.ObjectMetricSource{
			MetricName: fmt.Sprintf("%s,%s", requestsPerSecondName, backendName),
			Target: autoscaling.CrossVersionObjectReference{
				APIVersion: "networking/v1beta1",
				Kind:       "Ingress",
				Name:       ingressName,
			},
			TargetValue:  average, // this has no effect but needs to be set for autoscaling/v2beta1
			AverageValue: &average,
		},
	}
	return generated, nil
}
