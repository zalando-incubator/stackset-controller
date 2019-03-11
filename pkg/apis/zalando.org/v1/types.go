package v1

import (
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StackSet describes an application resource.
// +k8s:deepcopy-gen=true
type StackSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StackSetSpec   `json:"spec"`
	Status StackSetStatus `json:"status"`
}

// StackSetSpec is the spec part of the StackSet.
// +k8s:deepcopy-gen=true
type StackSetSpec struct {
	Ingress        *StackSetIngressSpec `json:"ingress"`
	StackLifecycle StackLifecycle       `json:"stackLifecycle"`
	StackTemplate  StackTemplate        `json:"stackTemplate"`
}

// StackSetIngressSpec is the ingress defintion of an StackSet. This
// includes ingress annotations and a list of hostnames.
// +k8s:deepcopy-gen=true
type StackSetIngressSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Hosts             []string           `json:"hosts"`
	BackendPort       intstr.IntOrString `json:"backendPort"`
	Path              string             `json:"path"`
}

// StackLifecycle defines lifecycle of the Stacks of a StackSet.
// +k8s:deepcopy-gen=true
type StackLifecycle struct {
	// ScaledownTTLSeconds is the ttl in seconds for when Stacks of a
	// StackSet should be scaled down to 0 replicas in case they are not
	// getting traffic.
	// Defaults to 300 seconds.
	// +optional
	ScaledownTTLSeconds *int64 `json:"scaledownTTLSeconds,omitempty" protobuf:"varint,4,opt,name=scaledownTTLSeconds"`
	// Limit defines the maximum number of Stacks to keep around. If the
	// number of Stacks exceeds the limit then the oldest stacks which are
	// not getting traffic are deleted.
	Limit *int32 `json:"limit,omitempty"`
}

// StackTemplate defines the template used for the Stack created from a
// StackSet definition.
// +k8s:deepcopy-gen=true
type StackTemplate struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              StackSpecTemplate `json:"spec"`
}

// MetricsEndpoint specified the endpoint where the custom endpoint where the metrics
// can be queried
// +k8s:deepcopy-gen=true
type MetricsEndpoint struct {
	Port int32  `json:"port"`
	Path string `json:"path"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// MetricsQueue specifies the SQS queue whose length should be used for scaling
// +k8s:deepcopy-gen=true
type MetricsQueue struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}

// AutoscalerMetrics is the type of metric to be be used for autoscaling
// +k8s:deepcopy-gen=true
type AutoscalerMetrics struct {
	Type               string             `json:"type"`
	Average            *resource.Quantity `json:"average,omitEmpty"`
	Endpoint           *MetricsEndpoint   `json:"endpoint,omitEmpty"`
	AverageUtilization *int32             `json:"averageUtilization,omitempty"`
	Queue              *MetricsQueue      `json:"queue,omitEmpty"`
}

// Autoscaler is the autoscaling definition for a stack
// +k8s:deepcopy-gen=true
type Autoscaler struct {
	// minReplicas is the lower limit for the number of replicas to which the autoscaler can scale down.
	// It defaults to 1 pod.
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty" protobuf:"varint,2,opt,name=minReplicas"`
	// maxReplicas is the upper limit for the number of replicas to which the autoscaler can scale up.
	// It cannot be less that minReplicas.
	MaxReplicas int32 `json:"maxReplicas" protobuf:"varint,3,opt,name=maxReplicas"`

	Metrics []AutoscalerMetrics `json:"metrics"`
}

// HorizontalPodAutoscaler is the Autoscaling configuration of a Stack. If
// defined an HPA will be created for the Stack.
// +k8s:deepcopy-gen=true
type HorizontalPodAutoscaler struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// minReplicas is the lower limit for the number of replicas to which the autoscaler can scale down.
	// It defaults to 1 pod.
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty" protobuf:"varint,2,opt,name=minReplicas"`
	// maxReplicas is the upper limit for the number of replicas to which the autoscaler can scale up.
	// It cannot be less that minReplicas.
	MaxReplicas int32 `json:"maxReplicas" protobuf:"varint,3,opt,name=maxReplicas"`
	// metrics contains the specifications for which to use to calculate the
	// desired replica count (the maximum replica count across all metrics will
	// be used).  The desired replica count is calculated multiplying the
	// ratio between the target value and the current value by the current
	// number of pods.  Ergo, metrics used must decrease as the pod count is
	// increased, and vice-versa.  See the individual metric source types for
	// more information about how each type of metric must respond.
	// +optional
	Metrics []autoscaling.MetricSpec `json:"metrics,omitempty" protobuf:"bytes,4,rep,name=metrics"`
}

// StackSetStatus is the status section of the StackSet resource.
// +k8s:deepcopy-gen=true
type StackSetStatus struct {
	// Stacks is the number of stacks managed by the StackSet.
	// +optional
	Stacks int32 `json:"stacks,omitempty" protobuf:"varint,2,opt,name=stacks"`
	// ReadyStacks is the number of stacks managed by the StackSet which
	// are considered ready. a Stack is considered ready if:
	// replicas == readyReplicas == updatedReplicas.
	// +optional
	ReadyStacks int32 `json:"readyStacks,omitempty" protobuf:"varint,2,opt,name=readyStacks"`
	// StacksWithTraffic is the number of stacks managed by the StackSet
	// which are getting traffic.
	// +optional
	StacksWithTraffic int32 `json:"stacksWithTraffic,omitempty" protobuf:"varint,2,opt,name=stacksWithTraffic"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StackSetList is a list of StackSets.
// +k8s:deepcopy-gen=true
type StackSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []StackSet `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Stack defines one version of an application. It is possible to
// switch traffic between multiple versions of an application.
// +k8s:deepcopy-gen=true
type Stack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StackSpec   `json:"spec"`
	Status StackStatus `json:"status"`
}

// StackSpec is the spec part of the Stack.
// +k8s:deepcopy-gen=true
type StackSpec struct {
	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas                *int32                   `json:"replicas,omitempty" protobuf:"varint,1,opt,name=replicas"`
	HorizontalPodAutoscaler *HorizontalPodAutoscaler `json:"horizontalPodAutoscaler,omitempty"`
	// TODO: Service
	Service *StackServiceSpec `json:"service,omitempty"`
	// PodTemplate describes the pods that will be created.
	PodTemplate v1.PodTemplateSpec `json:"podTemplate" protobuf:"bytes,3,opt,name=template"`

	Autoscaler *Autoscaler `json:"autoscaler,omitempty"`
}

// StackServiceSpec makes it possible to customize the service generated for
// a stack.
// +k8s:deepcopy-gen=true
type StackServiceSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// The list of ports that are exposed by this service.
	// More info: https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies
	// +patchMergeKey=port
	// +patchStrategy=merge
	Ports []v1.ServicePort `json:"ports,omitempty" patchStrategy:"merge" patchMergeKey:"port" protobuf:"bytes,1,rep,name=ports"`
}

// StackSpecTemplate is the spec part of the Stack.
// +k8s:deepcopy-gen=true
type StackSpecTemplate struct {
	StackSpec
	Version string `json:"version"`
}

// StackStatus is the status part of the Stack.
// +k8s:deepcopy-gen=true
type StackStatus struct {
	// ActualTrafficWeight is the actual amount of traffic currently
	// routed to the stack.
	// TODO: should we be using floats in the API?
	// +optional
	ActualTrafficWeight float64 `json:"actualTrafficWeight" protobuf:"varint,2,opt,name=actualTrafficWeight"`
	// DesiredTrafficWeight is desired amount of traffic to be routed to
	// the stack.
	// +optional
	DesiredTrafficWeight float64 `json:"desiredTrafficWeight" protobuf:"varint,2,opt,name=desiredTrafficWeight"`
	// Replicas is the number of replicas in the Deployment managed by the
	// stack.
	// +optional
	Replicas int32 `json:"replicas" protobuf:"varint,2,opt,name=replicas"`
	// ReadyReplicas is the number of ready replicas in the Deployment
	// managed by the stack.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas" protobuf:"varint,2,opt,name=readyReplicas"`
	// UpdatedReplicas is the number of updated replicas in the Deployment
	// managed by the stack.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas" protobuf:"varint,2,opt,name=updatedReplicas"`
	// DesiredReplicas is the number of desired replicas as defined by the
	// optional HortizontalPodAutoscaler defined for the stack.
	// +optional
	DesiredReplicas int32 `json:"desiredReplicas,omitempty" protobuf:"varint,2,opt,name=desiredReplicas"`
	// Prescaling current prescaling information
	// +optional
	Prescaling PrescalingStatus `json:"prescalingStatus,omitempty" protobuf:"varint,2,opt,name=prescalingStatus"`
	// NoTrafficSince is the timestamp defining the last time the stack was
	// observed getting traffic.
	NoTrafficSince *metav1.Time `json:"noTrafficSince,omitempty"`
}

// Prescaling hold prescaling information
// +k8s:deepcopy-gen=true
type PrescalingStatus struct {
	// Active indicates if prescaling is current active
	// +optional
	Active bool `json:"active,omitempty" protobuf:"varint,1,name=active"`
	// Replicas is the number of replicas required for prescaling
	// +optional
	Replicas int32 `json:"replicas,omitempty" protobuf:"varint,2,opt,name=replicas"`
	// LastTrafficIncrease is the timestamp when the traffic was last increased on the stack
	// +optional
	LastTrafficIncrease *metav1.Time `json:"lastTrafficIncrease,omitempty" protobuf:"varint,4,opt,name=lastTrafficIncrease"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StackList is a list of Stacks.
// +k8s:deepcopy-gen=true
type StackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Stack `json:"items"`
}
