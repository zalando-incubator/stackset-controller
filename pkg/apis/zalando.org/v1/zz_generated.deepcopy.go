//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by deepcopy-gen. DO NOT EDIT.

package v1

import (
	zalandoorgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	v2beta1 "k8s.io/api/autoscaling/v2beta1"
	v2beta2 "k8s.io/api/autoscaling/v2beta2"
	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Autoscaler) DeepCopyInto(out *Autoscaler) {
	*out = *in
	if in.MinReplicas != nil {
		in, out := &in.MinReplicas, &out.MinReplicas
		*out = new(int32)
		**out = **in
	}
	if in.Metrics != nil {
		in, out := &in.Metrics, &out.Metrics
		*out = make([]AutoscalerMetrics, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Behavior != nil {
		in, out := &in.Behavior, &out.Behavior
		*out = new(v2beta2.HorizontalPodAutoscalerBehavior)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Autoscaler.
func (in *Autoscaler) DeepCopy() *Autoscaler {
	if in == nil {
		return nil
	}
	out := new(Autoscaler)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AutoscalerMetrics) DeepCopyInto(out *AutoscalerMetrics) {
	*out = *in
	if in.Average != nil {
		in, out := &in.Average, &out.Average
		x := (*in).DeepCopy()
		*out = &x
	}
	if in.Endpoint != nil {
		in, out := &in.Endpoint, &out.Endpoint
		*out = new(MetricsEndpoint)
		**out = **in
	}
	if in.AverageUtilization != nil {
		in, out := &in.AverageUtilization, &out.AverageUtilization
		*out = new(int32)
		**out = **in
	}
	if in.Queue != nil {
		in, out := &in.Queue, &out.Queue
		*out = new(MetricsQueue)
		**out = **in
	}
	if in.ZMON != nil {
		in, out := &in.ZMON, &out.ZMON
		*out = new(MetricsZMON)
		(*in).DeepCopyInto(*out)
	}
	if in.ScalingSchedule != nil {
		in, out := &in.ScalingSchedule, &out.ScalingSchedule
		*out = new(MetricsScalingSchedule)
		**out = **in
	}
	if in.ClusterScalingSchedule != nil {
		in, out := &in.ClusterScalingSchedule, &out.ClusterScalingSchedule
		*out = new(MetricsClusterScalingSchedule)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AutoscalerMetrics.
func (in *AutoscalerMetrics) DeepCopy() *AutoscalerMetrics {
	if in == nil {
		return nil
	}
	out := new(AutoscalerMetrics)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EmbeddedObjectMeta) DeepCopyInto(out *EmbeddedObjectMeta) {
	*out = *in
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EmbeddedObjectMeta.
func (in *EmbeddedObjectMeta) DeepCopy() *EmbeddedObjectMeta {
	if in == nil {
		return nil
	}
	out := new(EmbeddedObjectMeta)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EmbeddedObjectMetaWithAnnotations) DeepCopyInto(out *EmbeddedObjectMetaWithAnnotations) {
	*out = *in
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EmbeddedObjectMetaWithAnnotations.
func (in *EmbeddedObjectMetaWithAnnotations) DeepCopy() *EmbeddedObjectMetaWithAnnotations {
	if in == nil {
		return nil
	}
	out := new(EmbeddedObjectMetaWithAnnotations)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HorizontalPodAutoscaler) DeepCopyInto(out *HorizontalPodAutoscaler) {
	*out = *in
	if in.MinReplicas != nil {
		in, out := &in.MinReplicas, &out.MinReplicas
		*out = new(int32)
		**out = **in
	}
	if in.Metrics != nil {
		in, out := &in.Metrics, &out.Metrics
		*out = make([]v2beta1.MetricSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Behavior != nil {
		in, out := &in.Behavior, &out.Behavior
		*out = new(v2beta2.HorizontalPodAutoscalerBehavior)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HorizontalPodAutoscaler.
func (in *HorizontalPodAutoscaler) DeepCopy() *HorizontalPodAutoscaler {
	if in == nil {
		return nil
	}
	out := new(HorizontalPodAutoscaler)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MetricsClusterScalingSchedule) DeepCopyInto(out *MetricsClusterScalingSchedule) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MetricsClusterScalingSchedule.
func (in *MetricsClusterScalingSchedule) DeepCopy() *MetricsClusterScalingSchedule {
	if in == nil {
		return nil
	}
	out := new(MetricsClusterScalingSchedule)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MetricsEndpoint) DeepCopyInto(out *MetricsEndpoint) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MetricsEndpoint.
func (in *MetricsEndpoint) DeepCopy() *MetricsEndpoint {
	if in == nil {
		return nil
	}
	out := new(MetricsEndpoint)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MetricsQueue) DeepCopyInto(out *MetricsQueue) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MetricsQueue.
func (in *MetricsQueue) DeepCopy() *MetricsQueue {
	if in == nil {
		return nil
	}
	out := new(MetricsQueue)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MetricsScalingSchedule) DeepCopyInto(out *MetricsScalingSchedule) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MetricsScalingSchedule.
func (in *MetricsScalingSchedule) DeepCopy() *MetricsScalingSchedule {
	if in == nil {
		return nil
	}
	out := new(MetricsScalingSchedule)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MetricsZMON) DeepCopyInto(out *MetricsZMON) {
	*out = *in
	if in.Aggregators != nil {
		in, out := &in.Aggregators, &out.Aggregators
		*out = make([]ZMONMetricAggregatorType, len(*in))
		copy(*out, *in)
	}
	if in.Tags != nil {
		in, out := &in.Tags, &out.Tags
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MetricsZMON.
func (in *MetricsZMON) DeepCopy() *MetricsZMON {
	if in == nil {
		return nil
	}
	out := new(MetricsZMON)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodTemplateSpec) DeepCopyInto(out *PodTemplateSpec) {
	*out = *in
	in.EmbeddedObjectMeta.DeepCopyInto(&out.EmbeddedObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodTemplateSpec.
func (in *PodTemplateSpec) DeepCopy() *PodTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(PodTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PrescalingStatus) DeepCopyInto(out *PrescalingStatus) {
	*out = *in
	if in.LastTrafficIncrease != nil {
		in, out := &in.LastTrafficIncrease, &out.LastTrafficIncrease
		*out = (*in).DeepCopy()
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PrescalingStatus.
func (in *PrescalingStatus) DeepCopy() *PrescalingStatus {
	if in == nil {
		return nil
	}
	out := new(PrescalingStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RouteGroupSpec) DeepCopyInto(out *RouteGroupSpec) {
	*out = *in
	in.EmbeddedObjectMetaWithAnnotations.DeepCopyInto(&out.EmbeddedObjectMetaWithAnnotations)
	if in.Hosts != nil {
		in, out := &in.Hosts, &out.Hosts
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.AdditionalBackends != nil {
		in, out := &in.AdditionalBackends, &out.AdditionalBackends
		*out = make([]zalandoorgv1.RouteGroupBackend, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Routes != nil {
		in, out := &in.Routes, &out.Routes
		*out = make([]zalandoorgv1.RouteGroupRouteSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RouteGroupSpec.
func (in *RouteGroupSpec) DeepCopy() *RouteGroupSpec {
	if in == nil {
		return nil
	}
	out := new(RouteGroupSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Stack) DeepCopyInto(out *Stack) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Stack.
func (in *Stack) DeepCopy() *Stack {
	if in == nil {
		return nil
	}
	out := new(Stack)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Stack) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackIngressRouteGroupOverrides) DeepCopyInto(out *StackIngressRouteGroupOverrides) {
	*out = *in
	in.EmbeddedObjectMetaWithAnnotations.DeepCopyInto(&out.EmbeddedObjectMetaWithAnnotations)
	if in.Enabled != nil {
		in, out := &in.Enabled, &out.Enabled
		*out = new(bool)
		**out = **in
	}
	if in.Hosts != nil {
		in, out := &in.Hosts, &out.Hosts
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackIngressRouteGroupOverrides.
func (in *StackIngressRouteGroupOverrides) DeepCopy() *StackIngressRouteGroupOverrides {
	if in == nil {
		return nil
	}
	out := new(StackIngressRouteGroupOverrides)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackLifecycle) DeepCopyInto(out *StackLifecycle) {
	*out = *in
	if in.ScaledownTTLSeconds != nil {
		in, out := &in.ScaledownTTLSeconds, &out.ScaledownTTLSeconds
		*out = new(int64)
		**out = **in
	}
	if in.Limit != nil {
		in, out := &in.Limit, &out.Limit
		*out = new(int32)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackLifecycle.
func (in *StackLifecycle) DeepCopy() *StackLifecycle {
	if in == nil {
		return nil
	}
	out := new(StackLifecycle)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackList) DeepCopyInto(out *StackList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Stack, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackList.
func (in *StackList) DeepCopy() *StackList {
	if in == nil {
		return nil
	}
	out := new(StackList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *StackList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackServiceSpec) DeepCopyInto(out *StackServiceSpec) {
	*out = *in
	in.EmbeddedObjectMetaWithAnnotations.DeepCopyInto(&out.EmbeddedObjectMetaWithAnnotations)
	if in.Ports != nil {
		in, out := &in.Ports, &out.Ports
		*out = make([]corev1.ServicePort, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackServiceSpec.
func (in *StackServiceSpec) DeepCopy() *StackServiceSpec {
	if in == nil {
		return nil
	}
	out := new(StackServiceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackSet) DeepCopyInto(out *StackSet) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackSet.
func (in *StackSet) DeepCopy() *StackSet {
	if in == nil {
		return nil
	}
	out := new(StackSet)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *StackSet) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackSetIngressSpec) DeepCopyInto(out *StackSetIngressSpec) {
	*out = *in
	in.EmbeddedObjectMetaWithAnnotations.DeepCopyInto(&out.EmbeddedObjectMetaWithAnnotations)
	if in.Hosts != nil {
		in, out := &in.Hosts, &out.Hosts
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	out.BackendPort = in.BackendPort
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackSetIngressSpec.
func (in *StackSetIngressSpec) DeepCopy() *StackSetIngressSpec {
	if in == nil {
		return nil
	}
	out := new(StackSetIngressSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackSetList) DeepCopyInto(out *StackSetList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]StackSet, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackSetList.
func (in *StackSetList) DeepCopy() *StackSetList {
	if in == nil {
		return nil
	}
	out := new(StackSetList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *StackSetList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackSetSpec) DeepCopyInto(out *StackSetSpec) {
	*out = *in
	if in.Ingress != nil {
		in, out := &in.Ingress, &out.Ingress
		*out = new(StackSetIngressSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.ExternalIngress != nil {
		in, out := &in.ExternalIngress, &out.ExternalIngress
		*out = new(StackSetExternalIngressSpec)
		**out = **in
	}
	if in.RouteGroup != nil {
		in, out := &in.RouteGroup, &out.RouteGroup
		*out = new(RouteGroupSpec)
		(*in).DeepCopyInto(*out)
	}
	in.StackLifecycle.DeepCopyInto(&out.StackLifecycle)
	in.StackTemplate.DeepCopyInto(&out.StackTemplate)
	if in.Traffic != nil {
		in, out := &in.Traffic, &out.Traffic
		*out = make([]*DesiredTraffic, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(DesiredTraffic)
				**out = **in
			}
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackSetSpec.
func (in *StackSetSpec) DeepCopy() *StackSetSpec {
	if in == nil {
		return nil
	}
	out := new(StackSetSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackSetStatus) DeepCopyInto(out *StackSetStatus) {
	*out = *in
	if in.Traffic != nil {
		in, out := &in.Traffic, &out.Traffic
		*out = make([]*ActualTraffic, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(ActualTraffic)
				**out = **in
			}
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackSetStatus.
func (in *StackSetStatus) DeepCopy() *StackSetStatus {
	if in == nil {
		return nil
	}
	out := new(StackSetStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackSpec) DeepCopyInto(out *StackSpec) {
	*out = *in
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	if in.HorizontalPodAutoscaler != nil {
		in, out := &in.HorizontalPodAutoscaler, &out.HorizontalPodAutoscaler
		*out = new(HorizontalPodAutoscaler)
		(*in).DeepCopyInto(*out)
	}
	if in.Service != nil {
		in, out := &in.Service, &out.Service
		*out = new(StackServiceSpec)
		(*in).DeepCopyInto(*out)
	}
	in.PodTemplate.DeepCopyInto(&out.PodTemplate)
	if in.Autoscaler != nil {
		in, out := &in.Autoscaler, &out.Autoscaler
		*out = new(Autoscaler)
		(*in).DeepCopyInto(*out)
	}
	if in.IngressOverrides != nil {
		in, out := &in.IngressOverrides, &out.IngressOverrides
		*out = new(StackIngressRouteGroupOverrides)
		(*in).DeepCopyInto(*out)
	}
	if in.RouteGroupOverrides != nil {
		in, out := &in.RouteGroupOverrides, &out.RouteGroupOverrides
		*out = new(StackIngressRouteGroupOverrides)
		(*in).DeepCopyInto(*out)
	}
	if in.Strategy != nil {
		in, out := &in.Strategy, &out.Strategy
		*out = new(appsv1.DeploymentStrategy)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackSpec.
func (in *StackSpec) DeepCopy() *StackSpec {
	if in == nil {
		return nil
	}
	out := new(StackSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackSpecTemplate) DeepCopyInto(out *StackSpecTemplate) {
	*out = *in
	in.StackSpec.DeepCopyInto(&out.StackSpec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackSpecTemplate.
func (in *StackSpecTemplate) DeepCopy() *StackSpecTemplate {
	if in == nil {
		return nil
	}
	out := new(StackSpecTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackStatus) DeepCopyInto(out *StackStatus) {
	*out = *in
	in.Prescaling.DeepCopyInto(&out.Prescaling)
	if in.NoTrafficSince != nil {
		in, out := &in.NoTrafficSince, &out.NoTrafficSince
		*out = (*in).DeepCopy()
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackStatus.
func (in *StackStatus) DeepCopy() *StackStatus {
	if in == nil {
		return nil
	}
	out := new(StackStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StackTemplate) DeepCopyInto(out *StackTemplate) {
	*out = *in
	in.EmbeddedObjectMetaWithAnnotations.DeepCopyInto(&out.EmbeddedObjectMetaWithAnnotations)
	in.Spec.DeepCopyInto(&out.Spec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StackTemplate.
func (in *StackTemplate) DeepCopy() *StackTemplate {
	if in == nil {
		return nil
	}
	out := new(StackTemplate)
	in.DeepCopyInto(out)
	return out
}
