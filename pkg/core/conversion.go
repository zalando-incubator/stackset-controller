package core

import (
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	autoscaling "k8s.io/api/autoscaling/v2"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/conversion"
)

// from: https://github.com/kubernetes/kubernetes/blob/v1.14.4/pkg/apis/autoscaling/v2beta1/conversion.go

func Convert_v2beta1_ResourceMetricSource_To_autoscaling_ResourceMetricSource(in *autoscalingv2beta1.ResourceMetricSource, out *autoscaling.ResourceMetricSource, s conversion.Scope) error {
	out.Name = core.ResourceName(in.Name)
	utilization := in.TargetAverageUtilization
	averageValue := in.TargetAverageValue

	var metricType autoscaling.MetricTargetType
	if utilization == nil {
		metricType = autoscaling.AverageValueMetricType
	} else {
		metricType = autoscaling.UtilizationMetricType
	}
	out.Target = autoscaling.MetricTarget{
		Type:               metricType,
		AverageValue:       averageValue,
		AverageUtilization: utilization,
	}
	return nil
}

func Convert_v2beta1_ExternalMetricSource_To_autoscaling_ExternalMetricSource(in *autoscalingv2beta1.ExternalMetricSource, out *autoscaling.ExternalMetricSource, s conversion.Scope) error {
	value := in.TargetValue
	averageValue := in.TargetAverageValue

	var metricType autoscaling.MetricTargetType
	if value == nil {
		metricType = autoscaling.AverageValueMetricType
	} else {
		metricType = autoscaling.ValueMetricType
	}

	out.Target = autoscaling.MetricTarget{
		Type:         metricType,
		Value:        value,
		AverageValue: averageValue,
	}

	out.Metric = autoscaling.MetricIdentifier{
		Name:     in.MetricName,
		Selector: in.MetricSelector,
	}
	return nil
}

func Convert_v2beta1_ObjectMetricSource_To_autoscaling_ObjectMetricSource(in *autoscalingv2beta1.ObjectMetricSource, out *autoscaling.ObjectMetricSource, s conversion.Scope) error {
	var metricType autoscaling.MetricTargetType
	if in.AverageValue == nil {
		metricType = autoscaling.ValueMetricType
	} else {
		metricType = autoscaling.AverageValueMetricType
	}
	out.Target = autoscaling.MetricTarget{
		Type:         metricType,
		Value:        &in.TargetValue,
		AverageValue: in.AverageValue,
	}
	out.DescribedObject = autoscaling.CrossVersionObjectReference{
		Kind:       in.Target.Kind,
		Name:       in.Target.Name,
		APIVersion: in.Target.APIVersion,
	}
	out.Metric = autoscaling.MetricIdentifier{
		Name:     in.MetricName,
		Selector: in.Selector,
	}
	return nil
}

func Convert_v2beta1_PodsMetricSource_To_autoscaling_PodsMetricSource(in *autoscalingv2beta1.PodsMetricSource, out *autoscaling.PodsMetricSource, s conversion.Scope) error {
	targetAverageValue := &in.TargetAverageValue
	metricType := autoscaling.AverageValueMetricType

	out.Target = autoscaling.MetricTarget{
		Type:         metricType,
		AverageValue: targetAverageValue,
	}
	out.Metric = autoscaling.MetricIdentifier{
		Name:     in.MetricName,
		Selector: in.Selector,
	}
	return nil
}
