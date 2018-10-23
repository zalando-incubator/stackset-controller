package controller

import (
	"fmt"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
)

const (
	AmazonSQSMetricName   = "AmazonSQS"
	PodJSONMetricName     = "PodJSON"
	IngressMetricName     = "Ingress"
	CPUMetricName         = "CPU"
	MemoryMetricName      = "Memory"
	ZMONMetricName        = "ZMON"
	requestsPerSecondName = "requests-per-second"
	metricConfigJSONPath  = "metric-config.pods.%s.json-path/path"
	metricConfigJSONKey   = "metric-config.pods.%s.json-path/json-key"
	metricConfigJSONPort  = "metric-config.pods.%s.json-path/port"
	SQSQueueLengthTag     = "sqs-queue-length"
	SQSQueueNameTag       = "queue-name"
	SQSQueueRegionTag     = "region"
)

func (c *StackSetController) ReconcileAutoscalers(container *StackSetContainer) error {
	if container.StackSet.Spec.StackTemplate.Spec.HorizontalPodAutoscaler != nil {
		return nil
	}
	autoscaler := container.StackSet.Spec.StackTemplate.Spec.Autoscaler
	stacksetName := container.StackSet.Name
	if autoscaler != nil {
		var generatedHPA zv1.HorizontalPodAutoscaler
		generatedHPA.MinReplicas = autoscaler.MinReplicas
		generatedHPA.MaxReplicas = autoscaler.MaxReplicas
		generatedHPA.Metrics = make([]autoscaling.MetricSpec, len(autoscaler.Metrics))

		for i, m := range autoscaler.Metrics {
			var (
				generated *autoscaling.MetricSpec
				err       error
			)
			switch m.Type {
			case AmazonSQSMetricName:
				generated, err = SQSMetric(m)
				if err != nil {
					return err
				}
			case PodJSONMetricName:
				g, annotations, err := PodJsonMetric(m)
				if err != nil {
					return err
				}
				generatedHPA.Annotations = make(map[string]string)
				for k, v := range annotations {
					generatedHPA.Annotations[k] = v
				}
				generated = g
			case IngressMetricName:
				generated, err = IngressMetric(m, stacksetName)
				if err != nil {
					return err
				}
			case ZMONMetricName:
				return fmt.Errorf("not implemented metric type: %s", ZMONMetricName)
			case CPUMetricName:
				generated, err = CPUMetric(m)
				if err != nil {
					return err
				}
			case MemoryMetricName:
				generated, err = MemoryMetric(m)
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("metric type %s not supported", m.Type)
			}

			generatedHPA.Metrics[i] = *generated
		}
		container.StackSet.Spec.StackTemplate.Spec.HorizontalPodAutoscaler = &generatedHPA
	}
	return nil
}

func MemoryMetric(metrics zv1.AutoscalerMetrics) (*autoscaling.MetricSpec, error) {
	if metrics.AverageUtilization == nil {
		return nil, fmt.Errorf("utilization is not specified")
	}
	generated := &autoscaling.MetricSpec{
		Type:autoscaling.ResourceMetricSourceType,
		Resource:&autoscaling.ResourceMetricSource{
			Name: v1.ResourceMemory,
			TargetAverageUtilization:metrics.AverageUtilization,
		},
	}
	return generated, nil
}

func CPUMetric(metrics zv1.AutoscalerMetrics) (*autoscaling.MetricSpec, error) {
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

func SQSMetric(metrics zv1.AutoscalerMetrics) (*autoscaling.MetricSpec, error) {
	if metrics.Average == nil {
		return nil, fmt.Errorf("average not specified")
	}
	quantity, err := resource.ParseQuantity(strconv.Itoa(int(*metrics.Average)))
	if err != nil {
		return nil, err
	}
	if metrics.Queue == nil || metrics.Queue.Name == "" || metrics.Queue.Region == "" {
		return nil, fmt.Errorf("queue not specified correctly")
	}
	generated := &autoscaling.MetricSpec{
		Type: autoscaling.ExternalMetricSourceType,
		External: &autoscaling.ExternalMetricSource{
			MetricName: SQSQueueLengthTag,
			MetricSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{SQSQueueNameTag: metrics.Queue.Name, SQSQueueRegionTag: metrics.Queue.Region},
			},
			TargetAverageValue: &quantity,
		},
	}
	return generated, nil
}

func PodJsonMetric(metrics zv1.AutoscalerMetrics) (*autoscaling.MetricSpec, map[string]string, error) {
	if metrics.Average == nil {
		return nil, nil, fmt.Errorf("average is not specified for metric")
	}
	quantity, err := resource.ParseQuantity(strconv.Itoa(int(*metrics.Average)))
	if err != nil {
		return nil, nil, err
	}
	generated := &autoscaling.MetricSpec{
		Type: autoscaling.PodsMetricSourceType,
		Pods: &autoscaling.PodsMetricSource{
			MetricName:         metrics.Endpoint.Name,
			TargetAverageValue: quantity,
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
func IngressMetric(metrics zv1.AutoscalerMetrics, ingressName string) (*autoscaling.MetricSpec, error) {
	if metrics.Average == nil {
		return nil, fmt.Errorf("average value not specified for metric")
	}
	quantity, err := resource.ParseQuantity(strconv.Itoa(int(*metrics.Average)))
	if err != nil {
		return nil, err
	}
	generated := &autoscaling.MetricSpec{
		Type: autoscaling.ObjectMetricSourceType,
		Object: &autoscaling.ObjectMetricSource{
			MetricName: requestsPerSecondName,
			Target: autoscaling.CrossVersionObjectReference{
				APIVersion: "extensions/v1beta1",
				Kind:       "Ingress",
				Name:       ingressName,
			},
			TargetValue: quantity,
		},
	}
	return generated, nil
}
