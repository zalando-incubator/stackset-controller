package core

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

const (
	metricsNamespace = "stackset"

	metricsSubsystemStackset = "stackset"
	metricsSubsystemStack    = "stack"
	metricsSubsystemErrors   = "errors"
)

type MetricsReporter struct {
	stacksetMetricLabels map[resourceKey]prometheus.Labels
	stackMetricLabels    map[resourceKey]prometheus.Labels

	stacksetCount *prometheus.GaugeVec

	stackDesiredTrafficWeight *prometheus.GaugeVec
	stackActualTrafficWeight  *prometheus.GaugeVec
	stackReady                *prometheus.GaugeVec
	stackPrescalingActive     *prometheus.GaugeVec
	stackPrescalingReplicas   *prometheus.GaugeVec
	errorsCount               prometheus.Counter
	panicsCount               prometheus.Counter
}

type resourceKey struct {
	namespace string
	name      string
}

func NewMetricsReporter(registry prometheus.Registerer) (*MetricsReporter, error) {
	stacksetLabelNames := []string{"namespace", "stackset", "application"}
	stackLabelNames := []string{"namespace", "stack", "application"}

	result := &MetricsReporter{
		stacksetMetricLabels: make(map[resourceKey]prometheus.Labels),
		stackMetricLabels:    make(map[resourceKey]prometheus.Labels),
		stacksetCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystemStackset,
			Name:      "stacks",
			Help:      "Number of stacks for this stackset",
		}, stacksetLabelNames),
		stackDesiredTrafficWeight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystemStack,
			Name:      "desired_traffic_weight",
			Help:      "Desired traffic weight of the stack",
		}, stackLabelNames),
		stackActualTrafficWeight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystemStack,
			Name:      "actual_traffic_weight",
			Help:      "Actual traffic weight of the stack",
		}, stackLabelNames),
		stackReady: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystemStack,
			Name:      "ready",
			Help:      "Whether the stack is ready",
		}, stackLabelNames),
		stackPrescalingActive: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystemStack,
			Name:      "prescaling_active",
			Help:      "Whether prescaling is active for the stack",
		}, stackLabelNames),
		stackPrescalingReplicas: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystemStack,
			Name:      "prescaling_replicas",
			Help:      "Amount of replicas needed for prescaling",
		}, stackLabelNames),
		errorsCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystemErrors,
			Name:      "count",
			Help:      "Number of errors encountered",
		}),
		panicsCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystemErrors,
			Name:      "panic_count",
			Help:      "Number of panics encountered",
		}),
	}

	for _, metric := range []prometheus.Collector{
		result.stacksetCount,
		result.stackDesiredTrafficWeight,
		result.stackActualTrafficWeight,
		result.stackReady,
		result.stackPrescalingActive,
		result.stackPrescalingReplicas,
		result.errorsCount,
		result.panicsCount,
	} {
		err := registry.Register(metric)
		if err != nil {
			return nil, err
		}
	}

	// expose Kubernetes errors as metric
	utilruntime.ErrorHandlers = append(
		utilruntime.ErrorHandlers,
		func(_ context.Context, _ error, _ string, _ ...interface{}) {
			result.ReportError()
		},
	)

	return result, nil
}

func (reporter *MetricsReporter) Report(stacksets map[types.UID]*StackSetContainer) error {
	existingStacksets := make(map[resourceKey]struct{})
	existingStacks := make(map[resourceKey]struct{})

	for _, stackset := range stacksets {
		stacksetResource := resourceKey{
			namespace: stackset.StackSet.Namespace,
			name:      stackset.StackSet.Name,
		}
		existingStacksets[stacksetResource] = struct{}{}

		labels, ok := reporter.stacksetMetricLabels[stacksetResource]
		if !ok {
			labels = extractLabels("stackset", stackset.StackSet)
			reporter.stacksetMetricLabels[stacksetResource] = labels
		}
		reporter.reportStacksetMetrics(labels, stackset)

		for _, stack := range stackset.StackContainers {
			stackResource := resourceKey{
				namespace: stack.Namespace(),
				name:      stack.Name(),
			}
			existingStacks[stackResource] = struct{}{}

			labels, ok := reporter.stackMetricLabels[stackResource]
			if !ok {
				labels = extractLabels("stack", stack.Stack)
				reporter.stackMetricLabels[stackResource] = labels
			}
			reporter.reportStackMetrics(labels, stack)
		}
	}

	for resource, labels := range reporter.stacksetMetricLabels {
		if _, ok := existingStacksets[resource]; !ok {
			reporter.removeStacksetMetrics(labels)
			delete(reporter.stacksetMetricLabels, resource)
		}
	}

	for resource, labels := range reporter.stackMetricLabels {
		if _, ok := existingStacks[resource]; !ok {
			reporter.removeStackMetrics(labels)
			delete(reporter.stackMetricLabels, resource)
		}
	}
	return nil
}

func (reporter *MetricsReporter) ReportError() {
	reporter.errorsCount.Inc()
}

func extractLabels(nameKey string, obj metav1.Object) prometheus.Labels {
	return prometheus.Labels{
		"namespace":   obj.GetNamespace(),
		nameKey:       obj.GetName(),
		"application": obj.GetLabels()["application"],
	}
}

func (reporter *MetricsReporter) reportStacksetMetrics(labels prometheus.Labels, stackset *StackSetContainer) {
	reporter.stacksetCount.With(labels).Set(float64(len(stackset.StackContainers)))
}

func (reporter *MetricsReporter) removeStacksetMetrics(labels prometheus.Labels) {
	reporter.stacksetCount.Delete(labels)
}

func (reporter *MetricsReporter) reportStackMetrics(labels prometheus.Labels, stack *StackContainer) {
	reporter.stackDesiredTrafficWeight.With(labels).Set(stack.desiredTrafficWeight)
	reporter.stackActualTrafficWeight.With(labels).Set(stack.actualTrafficWeight)

	if stack.IsReady() {
		reporter.stackReady.With(labels).Set(1.0)
	} else {
		reporter.stackReady.With(labels).Set(0.0)
	}

	if stack.prescalingActive {
		reporter.stackPrescalingActive.With(labels).Set(1.0)
		reporter.stackPrescalingReplicas.With(labels).Set(float64(stack.prescalingReplicas))
	} else {
		reporter.stackPrescalingActive.With(labels).Set(0.0)
		reporter.stackPrescalingReplicas.With(labels).Set(0.0)
	}
}

func (reporter *MetricsReporter) removeStackMetrics(labels prometheus.Labels) {
	reporter.stackDesiredTrafficWeight.Delete(labels)
	reporter.stackActualTrafficWeight.Delete(labels)
	reporter.stackReady.Delete(labels)
	reporter.stackPrescalingActive.Delete(labels)
	reporter.stackPrescalingReplicas.Delete(labels)
}

func (reporter *MetricsReporter) ReportPanic() {
	reporter.panicsCount.Inc()
}
