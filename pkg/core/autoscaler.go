package core

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	autoscaling "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	requestsPerSecondName        = "requests-per-second"
	metricConfigJSONPath         = "metric-config.pods.%s.json-path/path"
	metricConfigJSONKey          = "metric-config.pods.%s.json-path/json-key"
	metricConfigJSONPort         = "metric-config.pods.%s.json-path/port"
	sqsQueueLengthTag            = "sqs-queue-length"
	zmonCheckMetricName          = "zmon-check"
	zmonCheckCheckIDTag          = "check-id"
	zmonCheckAggregatorsTag      = "aggregators"
	zmonCheckDurationTag         = "duration"
	zmonCheckStackTag            = "stack"
	zmonCheckKeyAnnotation       = "metric-config.external.zmon-check.zmon/key"
	zmonCheckTagAnnotationPrefix = "metric-config.external.zmon-check.zmon/tag-"
	sqsQueueNameTag              = "queue-name"
	sqsQueueRegionTag            = "region"
	scalingScheduleAPIVersion    = "zalando.org/v1"
)

var (
	errMissingZMONDefinition                   = errors.New("missing ZMON metric definition")
	errMissingScalingScheduleDefinition        = errors.New("missing ScalingSchedule metric definition")
	errMissingClusterScalingScheduleDefinition = errors.New("missing ClusterScalingSchedule metric definition")
	errMissingScalingScheduleName              = errors.New("missing ScalingSchedule metric object name")
	errMissingClusterScalingScheduleName       = errors.New("missing ClusterScalingSchedule metric object name")
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

func convertCustomMetrics(stacksetName, stackName, namespace string, metrics []zv1.AutoscalerMetrics, trafficWeight float64) ([]autoscaling.MetricSpec, map[string]string, error) {
	var resultMetrics MetricsList
	resultAnnotations := make(map[string]string)

	for _, m := range metrics {
		var (
			generated   *autoscaling.MetricSpec
			annotations map[string]string
			err         error
		)
		switch m.Type {
		case zv1.AmazonSQSAutoscalerMetric:
			generated, err = sqsMetric(m)
		case zv1.PodJSONAutoscalerMetric:
			generated, annotations, err = podJsonMetric(m)
		case zv1.IngressAutoscalerMetric:
			generated, err = ingressMetric(m, stacksetName, stackName)
		case zv1.RouteGroupAutoscalerMetric:
			generated, err = routegroupMetric(m, stacksetName, stackName)
		case zv1.ZMONAutoscalerMetric:
			generated, annotations, err = zmonMetric(m, stackName, namespace)
		case zv1.ScalingScheduleMetric:
			generated, err = scalingScheduleMetric(m, stackName, namespace)
		case zv1.ClusterScalingScheduleMetric:
			generated, err = clusterScalingScheduleMetric(m, stackName, namespace)
		case zv1.CPUAutoscalerMetric:
			generated, err = cpuMetric(m)
		case zv1.MemoryAutoscalerMetric:
			generated, err = memoryMetric(m)
		case zv1.ExternalRPSMetric:
			generated, annotations, err = externalRPSMetric(m, stackName, trafficWeight)
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
	if metrics.Container != "" {
		return &autoscaling.MetricSpec{
			Type: autoscaling.ContainerResourceMetricSourceType,
			ContainerResource: &autoscaling.ContainerResourceMetricSource{
				Name: v1.ResourceMemory,
				Target: autoscaling.MetricTarget{
					Type:               autoscaling.UtilizationMetricType,
					AverageUtilization: metrics.AverageUtilization,
				},
				Container: metrics.Container,
			},
		}, nil
	}

	return &autoscaling.MetricSpec{
		Type: autoscaling.ResourceMetricSourceType,
		Resource: &autoscaling.ResourceMetricSource{
			Name: v1.ResourceMemory,
			Target: autoscaling.MetricTarget{
				Type:               autoscaling.UtilizationMetricType,
				AverageUtilization: metrics.AverageUtilization,
			},
		},
	}, nil
}

func cpuMetric(metrics zv1.AutoscalerMetrics) (*autoscaling.MetricSpec, error) {
	if metrics.AverageUtilization == nil {
		return nil, fmt.Errorf("utilization is not specified")
	}

	if metrics.Container != "" {
		return &autoscaling.MetricSpec{
			Type: autoscaling.ContainerResourceMetricSourceType,
			ContainerResource: &autoscaling.ContainerResourceMetricSource{
				Name: v1.ResourceCPU,
				Target: autoscaling.MetricTarget{
					Type:               autoscaling.UtilizationMetricType,
					AverageUtilization: metrics.AverageUtilization,
				},
				Container: metrics.Container,
			},
		}, nil
	}

	return &autoscaling.MetricSpec{
		Type: autoscaling.ResourceMetricSourceType,
		Resource: &autoscaling.ResourceMetricSource{
			Name: v1.ResourceCPU,
			Target: autoscaling.MetricTarget{
				Type:               autoscaling.UtilizationMetricType,
				AverageUtilization: metrics.AverageUtilization,
			},
		},
	}, nil
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
			Metric: autoscaling.MetricIdentifier{
				Name: sqsQueueLengthTag,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{sqsQueueNameTag: metrics.Queue.Name, sqsQueueRegionTag: metrics.Queue.Region},
				},
			},
			Target: autoscaling.MetricTarget{
				Type:         autoscaling.AverageValueMetricType,
				AverageValue: &average,
			},
		},
	}
	return generated, nil
}

func podJsonMetric(metrics zv1.AutoscalerMetrics) (*autoscaling.MetricSpec, map[string]string, error) {
	if metrics.Average == nil {
		return nil, nil, fmt.Errorf("average is not specified for metric")
	}
	average := metrics.Average.DeepCopy()
	generated := &autoscaling.MetricSpec{
		Type: autoscaling.PodsMetricSourceType,
		Pods: &autoscaling.PodsMetricSource{
			Metric: autoscaling.MetricIdentifier{
				Name: metrics.Endpoint.Name,
			},
			Target: autoscaling.MetricTarget{
				Type:         autoscaling.AverageValueMetricType,
				AverageValue: &average,
			},
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
			Metric: autoscaling.MetricIdentifier{
				Name: fmt.Sprintf("%s,%s", requestsPerSecondName, backendName),
				// TODO:
				// Selector: &metav1.LabelSelector{
				// 	MatchLabels: map[string]string{
				// 		"backend": backendName,
				// 	},
				// },
			},
			DescribedObject: autoscaling.CrossVersionObjectReference{
				APIVersion: "networking.k8s.io/v1",
				Kind:       "Ingress",
				Name:       ingressName,
			},
			Target: autoscaling.MetricTarget{
				Type:         autoscaling.AverageValueMetricType,
				AverageValue: &average,
			},
		},
	}
	return generated, nil
}

func routegroupMetric(metrics zv1.AutoscalerMetrics, rgName, backendName string) (*autoscaling.MetricSpec, error) {
	if metrics.Average == nil {
		return nil, fmt.Errorf("average value not specified for metric")
	}

	average := metrics.Average.DeepCopy()

	generated := &autoscaling.MetricSpec{
		Type: autoscaling.ObjectMetricSourceType,
		Object: &autoscaling.ObjectMetricSource{
			Metric: autoscaling.MetricIdentifier{
				Name: requestsPerSecondName,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"backend": backendName,
					},
				},
			},
			DescribedObject: autoscaling.CrossVersionObjectReference{
				APIVersion: "zalando.org/v1",
				Kind:       "RouteGroup",
				Name:       rgName,
			},
			Target: autoscaling.MetricTarget{
				Type:         autoscaling.AverageValueMetricType,
				AverageValue: &average,
			},
		},
	}
	return generated, nil
}

func externalRPSMetric(metrics zv1.AutoscalerMetrics, stackname string, weight float64) (*autoscaling.MetricSpec, map[string]string, error) {
	if metrics.Average == nil {
		return nil, nil, fmt.Errorf("average value not specified for metric")
	}

	if metrics.RequestsPerSecond == nil {
		return nil, nil, fmt.Errorf("RequestsPerSecond value not specified for metric")
	}

	if len(metrics.RequestsPerSecond.Hostnames) == 0 {
		return nil, nil, fmt.Errorf("RequestsPerSecond.hostnames value not specified for metric")
	}

	name := stackname + "-rps"

	average := metrics.Average.DeepCopy()
	generated := &autoscaling.MetricSpec{
		Type: autoscaling.ExternalMetricSourceType,
		External: &autoscaling.ExternalMetricSource{
			Metric: autoscaling.MetricIdentifier{
				Name: name,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"type": "requests-per-second"},
				},
			},
			Target: autoscaling.MetricTarget{
				Type:         autoscaling.AverageValueMetricType,
				AverageValue: &average,
			},
		},
	}

	hostKey := fmt.Sprintf("metric-config.external.%s.requests-per-second/hostnames", name)
	weightKey := fmt.Sprintf("metric-config.external.%s.requests-per-second/weight", name)

	annotations := map[string]string{
		hostKey:   strings.Join(metrics.RequestsPerSecond.Hostnames, ","),
		weightKey: fmt.Sprintf("%d", int(weight)), // weight should be always between 0 and 100 with no decimal points
	}

	return generated, annotations, nil
}

func zmonMetric(metrics zv1.AutoscalerMetrics, stackName, namespace string) (*autoscaling.MetricSpec, map[string]string, error) {
	if metrics.Average == nil {
		return nil, nil, fmt.Errorf("average not specified")
	}
	average := metrics.Average.DeepCopy()

	if metrics.ZMON == nil {
		return nil, nil, errMissingZMONDefinition
	}

	aggregators := make([]string, 0, len(metrics.ZMON.Aggregators))
	for _, agg := range metrics.ZMON.Aggregators {
		aggregators = append(aggregators, string(agg))
	}

	// includes the namespace on the hash to allow stacksets in multiple
	// namespaces with the same metric.
	metricHash, err := metricHash(namespace, stackName)
	if err != nil {
		return nil, nil, fmt.Errorf("could not hash metric name")
	}

	generated := &autoscaling.MetricSpec{
		Type: autoscaling.ExternalMetricSourceType,
		External: &autoscaling.ExternalMetricSource{
			Metric: autoscaling.MetricIdentifier{
				Name: zmonCheckMetricName,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						zmonCheckCheckIDTag:  metrics.ZMON.CheckID,
						zmonCheckDurationTag: metrics.ZMON.Duration,
						// uniquely identifies the metric to this
						// particular stack using a hash
						zmonCheckStackTag: metricHash,
					},
				},
			},
			Target: autoscaling.MetricTarget{
				Type:         autoscaling.AverageValueMetricType,
				AverageValue: &average,
			},
		},
	}

	if len(aggregators) > 0 {
		generated.External.Metric.Selector.MatchLabels[zmonCheckAggregatorsTag] = strings.Join(aggregators, ",")
	}

	annotations := map[string]string{
		zmonCheckKeyAnnotation: metrics.ZMON.Key,
	}
	for k, v := range metrics.ZMON.Tags {
		annotations[zmonCheckTagAnnotationPrefix+k] = v
	}
	return generated, annotations, nil
}

func scalingScheduleMetric(metrics zv1.AutoscalerMetrics, stackName, namespace string) (*autoscaling.MetricSpec, error) {
	if metrics.Average == nil {
		return nil, fmt.Errorf("average not specified")
	}
	average := metrics.Average.DeepCopy()

	if metrics.ScalingSchedule == nil {
		return nil, errMissingScalingScheduleDefinition
	}

	name := metrics.ScalingSchedule.Name
	if name == "" {
		return nil, errMissingScalingScheduleName
	}

	return generateScalingScheduleMetricSpec("ScalingSchedule", name, average), nil
}

func clusterScalingScheduleMetric(metrics zv1.AutoscalerMetrics, stackName, namespace string) (*autoscaling.MetricSpec, error) {
	if metrics.Average == nil {
		return nil, fmt.Errorf("average not specified")
	}
	average := metrics.Average.DeepCopy()

	if metrics.ClusterScalingSchedule == nil {
		return nil, errMissingClusterScalingScheduleDefinition
	}

	name := metrics.ClusterScalingSchedule.Name
	if name == "" {
		return nil, errMissingClusterScalingScheduleName
	}

	return generateScalingScheduleMetricSpec("ClusterScalingSchedule", name, average), nil
}

func generateScalingScheduleMetricSpec(kind, name string, average resource.Quantity) *autoscaling.MetricSpec {
	return &autoscaling.MetricSpec{
		Type: autoscaling.ObjectMetricSourceType,
		Object: &autoscaling.ObjectMetricSource{
			DescribedObject: autoscaling.CrossVersionObjectReference{
				Kind:       kind,
				APIVersion: scalingScheduleAPIVersion,
				Name:       name,
			},
			Metric: autoscaling.MetricIdentifier{
				Name: name,
			},
			Target: autoscaling.MetricTarget{
				Type:         autoscaling.AverageValueMetricType,
				AverageValue: &average,
			},
		},
	}
}

func metricHash(namespace, name string) (string, error) {
	h := sha1.New()
	_, err := h.Write([]byte(namespace + "-" + name))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
