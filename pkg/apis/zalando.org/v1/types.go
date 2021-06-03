package v1

import (
	rg "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// StackSet describes an application resource.
// +k8s:deepcopy-gen=true
// +kubebuilder:resource:categories=all
// +kubebuilder:printcolumn:name="Stacks",type=integer,JSONPath=`.status.stacks`,description="Number of Stacks belonging to the StackSet"
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyStacks`,description="Number of Ready Stacks"
// +kubebuilder:printcolumn:name="Traffic",type=integer,JSONPath=`.status.stacksWithTraffic`,description="Number of Ready Stacks with traffic"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age of the stack"
// +kubebuilder:subresource:status
type StackSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec StackSetSpec `json:"spec"`
	// +optional
	Status StackSetStatus `json:"status"`
}

// StackSetSpec is the spec part of the StackSet.
// +k8s:deepcopy-gen=true
type StackSetSpec struct {
	// Ingress is the information we need to create ingress and
	// service. Ingress is optional, because other controller
	// might create ingress objects, but stackset owns the traffic
	// switch. In this case we would only have a Traffic, but no
	// ingress.
	// +optional
	Ingress *StackSetIngressSpec `json:"ingress,omitempty"`
	// ExternalIngress is used to specify the backend port to
	// generate the services for the stacks.
	// +optional
	ExternalIngress *StackSetExternalIngressSpec `json:"externalIngress,omitempty"`
	// RouteGroup is an alternative to ingress allowing more advanced
	// routing configuration while still maintaining the ability to switch
	// traffic to stacks. Use this if you need skipper filters or
	// predicates.
	// +optional
	RouteGroup *RouteGroupSpec `json:"routegroup,omitempty"`
	// StackLifecycle defines the cleanup rules for old stacks.
	StackLifecycle StackLifecycle `json:"stackLifecycle"`
	// StackTemplate container for resources to be created that
	// belong to one stack.
	StackTemplate StackTemplate `json:"stackTemplate"`
	// Traffic is the mapping from a stackset to stack with
	// weights. It defines the desired traffic. Clients that
	// orchestrate traffic switching should write this part.
	Traffic []*DesiredTraffic `json:"traffic,omitempty"`
}

// EmbeddedObjectMetaWithAnnotations defines the metadata which can be attached
// to a resource. It's a slimmed down version of metav1.ObjectMeta only
// containing annotations.
// +k8s:deepcopy-gen=true
type EmbeddedObjectMetaWithAnnotations struct {
	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty" protobuf:"bytes,12,rep,name=annotations"`
}

// EmbeddedObject defines the metadata which can be attached
// to a resource. It's a slimmed down version of metav1.ObjectMeta only
// containing labels and annotations.
// +k8s:deepcopy-gen=true
type EmbeddedObjectMeta struct {
	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	// More info: http://kubernetes.io/docs/user-guide/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty" protobuf:"bytes,11,rep,name=labels"`

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty" protobuf:"bytes,12,rep,name=annotations"`
}

// +k8s:deepcopy-gen=true
type StackIngressRouteGroupOverrides struct {
	EmbeddedObjectMetaWithAnnotations `json:"metadata,omitempty"`

	// Whether to enable per-stack ingresses or routegroups. Defaults to enabled if unset.
	// +optional
	Enabled *bool `json:"enabled"`

	// Hostnames to use for the per-stack ingresses (or route groups). These must contain the special $(STACK_NAME)
	// token, which will be replaced with the stack's name. Would be automatically generated based on the hosts in the
	// ingress/routegroup entry if unset.
	Hosts []string `json:"hosts,omitempty"`
}

func (o *StackIngressRouteGroupOverrides) IsEnabled() bool {
	if o == nil || o.Enabled == nil {
		return true
	}
	return *o.Enabled
}

func (o *StackIngressRouteGroupOverrides) GetAnnotations() map[string]string {
	if o == nil {
		return nil
	}
	return o.Annotations
}

// StackSetIngressSpec is the ingress defintion of an StackSet. This
// includes ingress annotations and a list of hostnames.
// +k8s:deepcopy-gen=true
type StackSetIngressSpec struct {
	EmbeddedObjectMetaWithAnnotations `json:"metadata,omitempty"`
	Hosts                             []string           `json:"hosts"`
	BackendPort                       intstr.IntOrString `json:"backendPort"`

	// Settings for the per-stack ingresses
	// +optional
	StackIngressOverrides *StackIngressRouteGroupOverrides `json:"stackOverrides"`
	// +optional
	Path string `json:"path"`
}

func (s *StackSetIngressSpec) GetHosts() []string {
	return s.Hosts
}

func (s *StackSetIngressSpec) GetOverrides() *StackIngressRouteGroupOverrides {
	return s.StackIngressOverrides
}

// StackSetExternalIngressSpec defines the required service
// backendport for ingress managed outside of stackset.
type StackSetExternalIngressSpec struct {
	BackendPort intstr.IntOrString `json:"backendPort"`
}

// RouteGroupSpec defines the specification for defining a RouteGroup attached
// to a StackSet.
// +k8s:deepcopy-gen=true
type RouteGroupSpec struct {
	EmbeddedObjectMetaWithAnnotations `json:"metadata,omitempty"`
	// Hosts is the list of hostnames to add to the routegroup.
	Hosts []string `json:"hosts"`
	// Settings for the per-stack route groups
	// +optional
	StackRouteGroupOverrides *StackIngressRouteGroupOverrides `json:"stackOverrides"`
	// AdditionalBackends is the list of additional backends to use for
	// routing.
	// +optional
	AdditionalBackends []rg.RouteGroupBackend `json:"additionalBackends,omitempty"`
	// Routes is the list of routes to be applied to the routegroup.
	// +kubebuilder:validation:MinItems=1
	Routes      []rg.RouteGroupRouteSpec `json:"routes"`
	BackendPort int                      `json:"backendPort"`
}

func (s *RouteGroupSpec) GetHosts() []string {
	return s.Hosts
}

func (s *RouteGroupSpec) GetOverrides() *StackIngressRouteGroupOverrides {
	return s.StackRouteGroupOverrides
}

// StackLifecycle defines lifecycle of the Stacks of a StackSet.
// +k8s:deepcopy-gen=true
type StackLifecycle struct {
	// ScaledownTTLSeconds is the ttl in seconds for when Stacks of a
	// StackSet should be scaled down to 0 replicas in case they are not
	// getting traffic.
	// Defaults to 300 seconds.
	// +optional
	ScaledownTTLSeconds *int64 `json:"scaledownTTLSeconds,omitempty"`
	// Limit defines the maximum number of Stacks to keep around. If the
	// number of Stacks exceeds the limit then the oldest stacks which are
	// not getting traffic are deleted.
	// +kubebuilder:validation:Minimum=1
	Limit *int32 `json:"limit,omitempty"`
}

// StackTemplate defines the template used for the Stack created from a
// StackSet definition.
// +k8s:deepcopy-gen=true
type StackTemplate struct {
	EmbeddedObjectMetaWithAnnotations `json:"metadata,omitempty"`
	Spec                              StackSpecTemplate `json:"spec"`
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

// MetricsQueue specifies the SQS queue whose length should be used for
// scaling.
// +k8s:deepcopy-gen=true
type MetricsQueue struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}

// ZMONMetricAggregatorType is the type of aggregator used in a ZMON based
// metric.
// +kubebuilder:validation:Enum=avg;dev;count;first;last;max;min;sum;diff
type ZMONMetricAggregatorType string

const (
	AvgZMONMetricAggregator   ZMONMetricAggregatorType = "avg"
	DevZMONMetricAggregator   ZMONMetricAggregatorType = "dev"
	CountZMONMetricAggregator ZMONMetricAggregatorType = "count"
	FirstZMONMetricAggregator ZMONMetricAggregatorType = "first"
	LastZMONMetricAggregator  ZMONMetricAggregatorType = "last"
	MaxZMONMetricAggregator   ZMONMetricAggregatorType = "max"
	MinZMONMetricAggregator   ZMONMetricAggregatorType = "min"
	SumZMONMetricAggregator   ZMONMetricAggregatorType = "sum"
	DiffZMONMetricAggregator  ZMONMetricAggregatorType = "diff"
)

// MetricsZMON specifies the ZMON check which should be used for scaling.
// +k8s:deepcopy-gen=true
type MetricsZMON struct {
	// +kubebuilder:validation:Pattern:="^[0-9]+$"
	CheckID string `json:"checkID"`
	Key     string `json:"key"`
	// +kubebuilder:default:="5m"
	// +optional
	Duration string `json:"duration"`
	// +optional
	Aggregators []ZMONMetricAggregatorType `json:"aggregators,omitempty"`
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// MetricsScalingSchedule specifies the ScalingSchedule object which
// should be used for scaling.
// +k8s:deepcopy-gen=true
type MetricsScalingSchedule struct {
	// The name of the referenced ScalingSchedule object.
	// +kubebuilder:validation:Pattern:=^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$
	Name string `json:"name"`
}

// MetricsClusterScalingSchedule specifies the ClusterScalingSchedule
// object which should be used for scaling.
// +k8s:deepcopy-gen=true
type MetricsClusterScalingSchedule struct {
	// The name of the referenced ClusterScalingSchedule object.
	// +kubebuilder:validation:Pattern:=^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$
	Name string `json:"name"`
}

// AutoscalerMetricType is the type of the metric used for scaling.
// +kubebuilder:validation:Enum=CPU;Memory;AmazonSQS;PodJSON;Ingress;ZMON;ScalingSchedule;ClusterScalingSchedule
type AutoscalerMetricType string

const (
	CPUAutoscalerMetric          AutoscalerMetricType = "CPU"
	MemoryAutoscalerMetric       AutoscalerMetricType = "Memory"
	AmazonSQSAutoscalerMetric    AutoscalerMetricType = "AmazonSQS"
	PodJSONAutoscalerMetric      AutoscalerMetricType = "PodJSON"
	IngressAutoscalerMetric      AutoscalerMetricType = "Ingress"
	ZMONAutoscalerMetric         AutoscalerMetricType = "ZMON"
	ClusterScalingScheduleMetric AutoscalerMetricType = "ClusterScalingSchedule"
	ScalingScheduleMetric        AutoscalerMetricType = "ScalingSchedule"
)

// AutoscalerMetrics is the type of metric to be be used for autoscaling.
// +k8s:deepcopy-gen=true
type AutoscalerMetrics struct {
	Type                   AutoscalerMetricType           `json:"type"`
	Average                *resource.Quantity             `json:"average,omitempty"`
	Endpoint               *MetricsEndpoint               `json:"endpoint,omitempty"`
	AverageUtilization     *int32                         `json:"averageUtilization,omitempty"`
	Queue                  *MetricsQueue                  `json:"queue,omitempty"`
	ZMON                   *MetricsZMON                   `json:"zmon,omitempty"`
	ScalingSchedule        *MetricsScalingSchedule        `json:"scalingSchedule,omitempty"`
	ClusterScalingSchedule *MetricsClusterScalingSchedule `json:"clusterScalingSchedule,omitempty"`
	// optional container name that can be used to scale based on CPU or
	// Memory metrics of a specific container as opposed to an average of
	// all containers in a pod.
	// +optional
	Container string `json:"container,omitempty"`
}

// Autoscaler is the autoscaling definition for a stack
// +k8s:deepcopy-gen=true
type Autoscaler struct {
	// minReplicas is the lower limit for the number of replicas to which the autoscaler can scale down.
	// It defaults to 1 pod.
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// maxReplicas is the upper limit for the number of replicas to which the autoscaler can scale up.
	// It cannot be less that minReplicas.
	MaxReplicas int32 `json:"maxReplicas"`

	Metrics []AutoscalerMetrics `json:"metrics"`

	// behavior configures the scaling behavior of the target
	// in both Up and Down directions (scaleUp and scaleDown fields respectively).
	// If not set, the default HPAScalingRules for scale up and scale down are used.
	// +optional
	Behavior *autoscalingv2beta2.HorizontalPodAutoscalerBehavior `json:"behavior,omitempty" protobuf:"bytes,5,opt,name=behavior"`
}

// HorizontalPodAutoscaler is the Autoscaling configuration of a Stack. If
// defined an HPA will be created for the Stack.
// +k8s:deepcopy-gen=true
type HorizontalPodAutoscaler struct {
	// minReplicas is the lower limit for the number of replicas to which the autoscaler can scale down.
	// It defaults to 1 pod.
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// maxReplicas is the upper limit for the number of replicas to which the autoscaler can scale up.
	// It cannot be less that minReplicas.
	MaxReplicas int32 `json:"maxReplicas"`
	// metrics contains the specifications for which to use to calculate the
	// desired replica count (the maximum replica count across all metrics will
	// be used).  The desired replica count is calculated multiplying the
	// ratio between the target value and the current value by the current
	// number of pods.  Ergo, metrics used must decrease as the pod count is
	// increased, and vice-versa.  See the individual metric source types for
	// more information about how each type of metric must respond.
	// +optional
	Metrics []autoscaling.MetricSpec `json:"metrics,omitempty"`

	// behavior configures the scaling behavior of the target
	// in both Up and Down directions (scaleUp and scaleDown fields respectively).
	// If not set, the default HPAScalingRules for scale up and scale down are used.
	// +optional
	Behavior *autoscalingv2beta2.HorizontalPodAutoscalerBehavior `json:"behavior,omitempty" protobuf:"bytes,5,opt,name=behavior"`
}

// StackSetStatus is the status section of the StackSet resource.
// +k8s:deepcopy-gen=true
type StackSetStatus struct {
	// Stacks is the number of stacks managed by the StackSet.
	// +optional
	Stacks int32 `json:"stacks"`
	// ReadyStacks is the number of stacks managed by the StackSet which
	// are considered ready. a Stack is considered ready if:
	// replicas == readyReplicas == updatedReplicas.
	// +optional
	ReadyStacks int32 `json:"readyStacks"`
	// StacksWithTraffic is the number of stacks managed by the StackSet
	// which are getting traffic.
	// +optional
	StacksWithTraffic int32 `json:"stacksWithTraffic"`
	// ObservedStackVersion is the version of Stack generated from the current StackSet definition.
	// TODO: add a more detailed comment
	// +optional
	ObservedStackVersion string `json:"observedStackVersion,omitempty"`
	// Traffic is the actual traffic setting on services for this stackset
	// +optional
	Traffic []*ActualTraffic `json:"traffic,omitempty"`
}

// Traffic is the actual traffic setting on services for this
// stackset, controllers interested in current traffic decision should
// read this.
type ActualTraffic struct {
	StackName   string             `json:"stackName"`
	ServiceName string             `json:"serviceName"`
	ServicePort intstr.IntOrString `json:"servicePort"`

	// +kubebuilder:validation:Format=float
	// +kubebuilder:validation:Type=number
	Weight float64 `json:"weight"`
}

// DesiredTraffic is the desired traffic setting to direct traffic to
// a stack. This is meant to use by clients to orchestrate traffic
// switching.
type DesiredTraffic struct {
	StackName string `json:"stackName"`
	// +kubebuilder:validation:Type=number
	// +kubebuilder:validation:Format=float
	Weight float64 `json:"weight"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
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
// +kubebuilder:resource:categories=all
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.spec.replicas`,description="Number of desired replicas"
// +kubebuilder:printcolumn:name="Current",type=integer,JSONPath=`.status.replicas`,description="Number of current replicas"
// +kubebuilder:printcolumn:name="Up-To-Date",type=integer,JSONPath=`.status.updatedReplicas`,description="Number of up-to-date replicas"
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`,description="Number of ready replicas"
// +kubebuilder:printcolumn:name="Traffic",type=number,JSONPath=`.status.actualTrafficWeight`,description="Current traffic weight for the stack"
// +kubebuilder:printcolumn:name="No-Traffic-Since",type=date,JSONPath=`.status.noTrafficSince`,description="Time since the stack didn't get any traffic"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age of the stack"
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.labelSelector
type Stack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec StackSpec `json:"spec"`
	// +optional
	Status StackStatus `json:"status"`
}

// PodTemplateSpec describes the data a pod should have when created from a template
// +k8s:deepcopy-gen=true
type PodTemplateSpec struct {
	// Object's metadata.
	// +optional
	EmbeddedObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Specification of the desired behavior of the pod.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec v1.PodSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

// StackSpec is the spec part of the Stack.
// +k8s:deepcopy-gen=true
type StackSpec struct {
	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas                *int32                   `json:"replicas,omitempty"`
	HorizontalPodAutoscaler *HorizontalPodAutoscaler `json:"horizontalPodAutoscaler,omitempty"`
	// Service can be used to configure a custom service, if not
	// set stackset-controller will generate a service based on
	// container port and ingress backendport.
	Service *StackServiceSpec `json:"service,omitempty"`
	// PodTemplate describes the pods that will be created.
	PodTemplate PodTemplateSpec `json:"podTemplate"`

	Autoscaler *Autoscaler `json:"autoscaler,omitempty"`

	// Strategy describe the rollout strategy for the underlying deployment
	Strategy *appsv1.DeploymentStrategy `json:"strategy,omitempty"`
}

// StackServiceSpec makes it possible to customize the service generated for
// a stack.
// +k8s:deepcopy-gen=true
type StackServiceSpec struct {
	EmbeddedObjectMetaWithAnnotations `json:"metadata,omitempty"`

	// The list of ports that are exposed by this service.
	// More info: https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies
	// +patchMergeKey=port
	// +patchStrategy=merge
	Ports []v1.ServicePort `json:"ports,omitempty" patchStrategy:"merge" patchMergeKey:"port"`
}

// StackSpecTemplate is the spec part of the Stack.
// +k8s:deepcopy-gen=true
type StackSpecTemplate struct {
	StackSpec `json:",inline"`
	// +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"
	Version string `json:"version"`
}

// StackStatus is the status part of the Stack.
// +k8s:deepcopy-gen=true
type StackStatus struct {
	// ActualTrafficWeight is the actual amount of traffic currently
	// routed to the stack.
	// TODO: should we be using floats in the API?
	// +optional
	// +kubebuilder:validation:Format=float
	// +kubebuilder:validation:Type=number
	ActualTrafficWeight float64 `json:"actualTrafficWeight"`
	// DesiredTrafficWeight is desired amount of traffic to be routed to
	// the stack.
	// +optional
	// +kubebuilder:validation:Format=float
	// +kubebuilder:validation:Type=number
	DesiredTrafficWeight float64 `json:"desiredTrafficWeight"`
	// Replicas is the number of replicas in the Deployment managed by the
	// stack.
	// +optional
	Replicas int32 `json:"replicas"`
	// ReadyReplicas is the number of ready replicas in the Deployment
	// managed by the stack.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas"`
	// UpdatedReplicas is the number of updated replicas in the Deployment
	// managed by the stack.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas"`
	// DesiredReplicas is the number of desired replicas in the Deployment
	// +optional
	DesiredReplicas int32 `json:"desiredReplicas"`
	// Prescaling current prescaling information
	// +optional
	Prescaling PrescalingStatus `json:"prescalingStatus"`
	// NoTrafficSince is the timestamp defining the last time the stack was
	// observed getting traffic.
	NoTrafficSince *metav1.Time `json:"noTrafficSince,omitempty"`
	// LabelSelector is the label selector used to find all pods managed by
	// a stack.
	LabelSelector string `json:"labelSelector,omitempty"`
}

// Prescaling hold prescaling information
// +k8s:deepcopy-gen=true
type PrescalingStatus struct {
	// Active indicates if prescaling is current active
	// +optional
	Active bool `json:"active"`
	// Replicas is the number of replicas required for prescaling
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
	// DesiredTrafficWeight is the desired traffic weight that the stack was prescaled for
	// +optional
	// +kubebuilder:validation:Format=float
	// +kubebuilder:validation:Type=number
	DesiredTrafficWeight float64 `json:"desiredTrafficWeight,omitempty"`
	// LastTrafficIncrease is the timestamp when the traffic was last increased on the stack
	// +optional
	LastTrafficIncrease *metav1.Time `json:"lastTrafficIncrease,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// StackList is a list of Stacks.
// +k8s:deepcopy-gen=true
type StackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Stack `json:"items"`
}
