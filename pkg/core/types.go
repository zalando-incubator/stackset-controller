package core

import (
	"fmt"
	"math"
	"sort"
	"time"

	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	defaultVersion             = "default"
	defaultStackLifecycleLimit = 10
	defaultScaledownTTL        = 300 * time.Second
)

// StackSetContainer is a container for storing the full state of a StackSet
// including the sub-resources which are part of the StackSet. It represents a
// snapshot of the resources currently in the Cluster. This includes an
// optional Ingress resource as well as the current Traffic distribution. It
// also contains a set of StackContainers which represents the full state of
// the individual Stacks part of the StackSet.
type StackSetContainer struct {
	StackSet *zv1.StackSet

	// StackContainers is a set of stacks belonging to the StackSet
	// including the Stack sub resources like Deployments and Services.
	StackContainers map[types.UID]*StackContainer

	// Ingress defines the current Ingress resource belonging to the
	// StackSet. This is a reference to the actual resource while
	// `StackSet.Spec.Ingress` defines the ingress configuration specified
	// by the user on the StackSet.
	Ingress *networking.Ingress

	// RouteGroups defines the current RouteGroup resource belonging to the
	// StackSet. This is a reference to the actual resource while
	// `StackSet.Spec.RouteGroup` defines the route group configuration
	// specified by the user on the StackSet.
	RouteGroup *rgv1.RouteGroup

	// TrafficReconciler is the reconciler implementation used for
	// switching traffic between stacks. E.g. for prescaling stacks before
	// switching traffic.
	TrafficReconciler TrafficReconciler

	// ExternalIngressBackendPort defines the backendPort mapping
	// if an external entity creates ingress objects for us. The
	// Ingress of stackset should be nil in this case.
	externalIngressBackendPort *intstr.IntOrString

	// backendWeightsAnnotationKey to store the runtime decision
	// which annotation is used, defaults to
	// traffic.DefaultBackendWeightsAnnotationKey
	backendWeightsAnnotationKey string

	// clusterDomains stores the main domain names of the cluster;
	// per-stack ingress hostnames are not generated for names outside of them
	clusterDomains []string

	// segmentTraffic controls support of managing traffic weight by using
	// dedicated Ingress and RouteGroup resources with Skipper's TrafficSegment
	// predicate.
	segmentTraffic bool
}

// StackContainer is a container for storing the full state of a Stack
// including all the managed sub-resources. This includes the Stack resource
// itself and all the sub resources like Deployment, HPA and Service.
type StackContainer struct {
	// Stack represents the desired state of the stack, updated by the reconciliation logic
	Stack *zv1.Stack

	// PendingRemoval is set to true if the stack should be deleted
	PendingRemoval bool

	// Resources contains Kubernetes entities for the Stack's resources (Deployment, Ingress, etc)
	Resources StackResources

	// Fields from the parent stackset
	stacksetName   string
	scaledownTTL   time.Duration
	clusterDomains []string

	// Fields from the stack itself.
	ingressSpec    *zv1.StackSetIngressSpec
	routeGroupSpec *zv1.RouteGroupSpec
	backendPort    *intstr.IntOrString

	// Fields from the stack itself, with some defaults applied
	stackReplicas int32

	// Fields from the stack resources

	// Set to true only if all related resources have been updated according to the latest stack version
	resourcesUpdated bool

	// Current number of replicas that the deployment is expected to have, from Deployment.spec
	deploymentReplicas int32

	// Current number of replicas that the deployment has, from Deployment.status
	createdReplicas int32

	// Current number of replicas that the deployment has, from Deployment.status
	readyReplicas int32

	// Current number of up-to-date replicas that the deployment has, from Deployment.status
	updatedReplicas int32

	// When using traffic segments: the traffic segment associated to this stack
	IngressSegmentToUpdate    *networking.Ingress
	RouteGroupSegmentToUpdate *rgv1.RouteGroup

	// Traffic & scaling
	currentActualTrafficWeight     float64
	actualTrafficWeight            float64
	desiredTrafficWeight           float64
	noTrafficSince                 time.Time
	prescalingActive               bool
	prescalingReplicas             int32
	prescalingDesiredTrafficWeight float64
	prescalingLastTrafficIncrease  time.Time
	minReadyPercent                float64
}

// TrafficChange contains information about a traffic change event
type TrafficChange struct {
	StackName        string
	OldTrafficWeight float64
	NewTrafficWeight float64
}

func (tc TrafficChange) String() string {
	return fmt.Sprintf("%s: %.1f%% to %.1f%%", tc.StackName, tc.OldTrafficWeight, tc.NewTrafficWeight)
}

func (sc *StackContainer) HasBackendPort() bool {
	return sc.backendPort != nil
}

func (sc *StackContainer) HasTraffic() bool {
	return sc.actualTrafficWeight > 0 || sc.desiredTrafficWeight > 0
}

func (sc *StackContainer) IsReady() bool {
	// Calculate minimum required replicas for the Deployment to be considered ready
	minRequiredReplicas := int32(math.Ceil(float64(sc.deploymentReplicas) * sc.minReadyPercent))

	// Stacks are considered ready when all subresources have been updated
	// and the minimum ready percentage is hit on and replicas
	return (sc.resourcesUpdated && sc.deploymentReplicas > 0 &&
		minRequiredReplicas <= sc.updatedReplicas &&
		minRequiredReplicas <= sc.readyReplicas)
}

func (sc *StackContainer) MaxReplicas() int32 {
	if sc.Stack.Spec.StackSpec.Autoscaler != nil {
		return sc.Stack.Spec.StackSpec.Autoscaler.MaxReplicas
	}
	return math.MaxInt32
}

func (sc *StackContainer) IsAutoscaled() bool {
	return sc.Stack.Spec.StackSpec.Autoscaler != nil
}

func (sc *StackContainer) ScaledDown() bool {
	if sc.HasTraffic() {
		return false
	}
	return !sc.noTrafficSince.IsZero() && time.Since(sc.noTrafficSince) > sc.scaledownTTL
}

func (sc *StackContainer) Name() string {
	return sc.Stack.Name
}

func (sc *StackContainer) Namespace() string {
	return sc.Stack.Namespace
}

// StackResources describes the resources of a stack.
type StackResources struct {
	Deployment        *appsv1.Deployment
	HPA               *autoscaling.HorizontalPodAutoscaler
	Service           *v1.Service
	Ingress           *networking.Ingress
	IngressSegment    *networking.Ingress
	RouteGroup        *rgv1.RouteGroup
	RouteGroupSegment *rgv1.RouteGroup
	ConfigMaps []*v1.ConfigMap
}

func NewContainer(stackset *zv1.StackSet, reconciler TrafficReconciler, backendWeightsAnnotationKey string, clusterDomains []string) *StackSetContainer {
	return &StackSetContainer{
		StackSet:                    stackset,
		StackContainers:             map[types.UID]*StackContainer{},
		TrafficReconciler:           reconciler,
		backendWeightsAnnotationKey: backendWeightsAnnotationKey,
		clusterDomains:              clusterDomains,
	}
}

func (ssc *StackSetContainer) stackByName(name string) *StackContainer {
	for _, container := range ssc.StackContainers {
		if container.Name() == name {
			return container
		}
	}
	return nil
}

// updateDesiredTraffic gets desired from stackset spec
// and populates it to stack containers
func (ssc *StackSetContainer) updateDesiredTraffic() error {
	weights := make(map[string]float64)

	for _, desiredTraffic := range ssc.StackSet.Spec.Traffic {
		weights[desiredTraffic.StackName] = desiredTraffic.Weight
	}

	// filter stacks and normalize weights
	stacksetNames := make(map[string]struct{})
	for _, sc := range ssc.StackContainers {
		stacksetNames[sc.Name()] = struct{}{}
	}
	for name := range weights {
		if _, ok := stacksetNames[name]; !ok {
			delete(weights, name)
		}
	}

	if !allZero(weights) {
		normalizeWeights(weights)
	}

	// save values in stack containers
	for _, container := range ssc.StackContainers {
		container.desiredTrafficWeight = weights[container.Name()]
	}

	return nil
}

// updateActualTraffic gets actual from stackset status
// and populates it to stack containers
func (ssc *StackSetContainer) updateActualTraffic() error {
	weights := make(map[string]float64)
	for _, actualTraffic := range ssc.StackSet.Status.Traffic {
		weights[actualTraffic.ServiceName] = actualTraffic.Weight
	}

	// filter stacks and normalize weights
	stacksetNames := make(map[string]struct{})
	for _, sc := range ssc.StackContainers {
		stacksetNames[sc.Name()] = struct{}{}
	}
	for name := range weights {
		if _, ok := stacksetNames[name]; !ok {
			delete(weights, name)
		}
	}
	if !allZero(weights) {
		normalizeWeights(weights)
	}

	// save values in stack containers
	for _, container := range ssc.StackContainers {
		container.actualTrafficWeight = weights[container.Name()]
		container.currentActualTrafficWeight = weights[container.Name()]
	}

	return nil
}

// UpdateFromResources populates stack state information (e.g. replica counts or
// traffic) from related resources
func (ssc *StackSetContainer) UpdateFromResources() error {
	if len(ssc.StackContainers) == 0 {
		return nil
	}

	backendPort, err := findBackendPort(
		ssc.StackSet.Spec.Ingress,
		ssc.StackSet.Spec.RouteGroup,
		ssc.StackSet.Spec.ExternalIngress,
	)
	if err != nil {
		return err
	}

	// if backendPort is not defined from Ingress or Routegroup fallback
	// to externalIngress if defined
	if ssc.StackSet.Spec.ExternalIngress != nil {
		ssc.externalIngressBackendPort = backendPort
	}

	var scaledownTTL time.Duration
	if ssc.StackSet.Spec.StackLifecycle.ScaledownTTLSeconds == nil {
		scaledownTTL = defaultScaledownTTL
	} else {
		scaledownTTL = time.Duration(*ssc.StackSet.Spec.StackLifecycle.ScaledownTTLSeconds) * time.Second
	}

	for _, sc := range ssc.StackContainers {
		sc.stacksetName = ssc.StackSet.Name
		sc.backendPort = backendPort
		sc.scaledownTTL = scaledownTTL
		sc.clusterDomains = ssc.clusterDomains
		err := sc.updateStackResources()
		if err != nil {
			return err
		}

		sc.updateFromResources()
		// This is to support both central and segment-based traffic.
		if ssc.SupportsSegmentTraffic() {
			sc.updateFromSegmentResources()
		}
	}

	// only populate traffic if traffic management is enabled
	if ssc.StackSet.Spec.Ingress != nil ||
		ssc.StackSet.Spec.RouteGroup != nil ||
		ssc.StackSet.Spec.ExternalIngress != nil {

		err := ssc.updateDesiredTraffic()
		if err != nil {
			return err
		}
		return ssc.updateActualTraffic()
	}

	return nil
}

func (ssc *StackSetContainer) TrafficChanges() []TrafficChange {
	var result []TrafficChange

	for _, sc := range ssc.StackContainers {
		oldWeight := sc.currentActualTrafficWeight
		newWeight := sc.actualTrafficWeight

		if oldWeight != newWeight {
			result = append(result, TrafficChange{
				StackName:        sc.Name(),
				OldTrafficWeight: oldWeight,
				NewTrafficWeight: newWeight,
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StackName < result[j].StackName
	})
	return result
}

// updateStackResources writes sets the Ingress and RouteGroup
// resources, based on this container's Spec.
func (sc *StackContainer) updateStackResources() error {
	sc.ingressSpec = sc.Stack.Spec.Ingress
	sc.routeGroupSpec = sc.Stack.Spec.RouteGroup

	backendPort, err := findBackendPort(
		sc.ingressSpec,
		sc.routeGroupSpec,
		sc.Stack.Spec.ExternalIngress,
	)
	if err != nil {
		return err
	}

	if backendPort != nil {
		sc.backendPort = backendPort
	}

	return nil
}

func (sc *StackContainer) updateFromResources() {
	sc.stackReplicas = effectiveReplicas(sc.Stack.Spec.StackSpec.Replicas)

	var deploymentUpdated, serviceUpdated, ingressUpdated, routeGroupUpdated, hpaUpdated bool

	// deployment
	if sc.Resources.Deployment != nil {
		deployment := sc.Resources.Deployment
		sc.deploymentReplicas = effectiveReplicas(deployment.Spec.Replicas)
		sc.createdReplicas = deployment.Status.Replicas
		sc.readyReplicas = deployment.Status.ReadyReplicas
		sc.updatedReplicas = deployment.Status.UpdatedReplicas
		deploymentUpdated = IsResourceUpToDate(sc.Stack, sc.Resources.Deployment.ObjectMeta) && deployment.Status.ObservedGeneration == deployment.Generation
	}

	// service
	serviceUpdated = sc.Resources.Service != nil && IsResourceUpToDate(sc.Stack, sc.Resources.Service.ObjectMeta)

	// ingress: ignore if ingress is not set or check if we are up to date
	if sc.ingressSpec != nil {
		ingressUpdated = sc.Resources.Ingress != nil && IsResourceUpToDate(sc.Stack, sc.Resources.Ingress.ObjectMeta)
	} else {
		// ignore if ingress is not set
		ingressUpdated = sc.Resources.Ingress == nil
	}

	// routegroup: ignore if routegroup is not set or check if we are up to date
	if sc.routeGroupSpec != nil {
		routeGroupUpdated = sc.Resources.RouteGroup != nil && IsResourceUpToDate(sc.Stack, sc.Resources.RouteGroup.ObjectMeta)
	} else {
		// ignore if route group is not set
		routeGroupUpdated = sc.Resources.RouteGroup == nil
	}

	// hpa
	if sc.IsAutoscaled() {
		hpaUpdated = sc.Resources.HPA != nil && IsResourceUpToDate(sc.Stack, sc.Resources.HPA.ObjectMeta)
	} else {
		hpaUpdated = sc.Resources.HPA == nil
	}

	// aggregated 'resources updated' for the readiness
	sc.resourcesUpdated = deploymentUpdated &&
		serviceUpdated &&
		ingressUpdated &&
		routeGroupUpdated &&
		hpaUpdated

	status := sc.Stack.Status
	sc.noTrafficSince = unwrapTime(status.NoTrafficSince)
	if status.Prescaling.Active {
		sc.prescalingActive = true
		sc.prescalingReplicas = status.Prescaling.Replicas
		sc.prescalingDesiredTrafficWeight = status.Prescaling.DesiredTrafficWeight
		sc.prescalingLastTrafficIncrease = unwrapTime(status.Prescaling.LastTrafficIncrease)
	}
}

func (sc *StackContainer) updateFromSegmentResources() {
	var ingressSegmentUpdated, routeGroupSegmentUpdated bool

	// ingress: ignore if ingress is not set or check if we are up to date
	if sc.ingressSpec != nil {
		ingressSegmentUpdated = sc.Resources.IngressSegment != nil &&
			IsResourceUpToDate(sc.Stack, sc.Resources.IngressSegment.ObjectMeta)
	} else {
		// ignore if ingress is not set
		ingressSegmentUpdated = sc.Resources.Ingress == nil
	}

	// routegroup: ignore if routegroup is not set or check if we are up to date
	if sc.routeGroupSpec != nil {
		routeGroupSegmentUpdated = sc.Resources.RouteGroupSegment != nil &&
			IsResourceUpToDate(
				sc.Stack,
				sc.Resources.RouteGroupSegment.ObjectMeta,
			)
	} else {
		// ignore if route group is not set
		routeGroupSegmentUpdated = sc.Resources.RouteGroup == nil
	}

	sc.resourcesUpdated = sc.resourcesUpdated &&
		ingressSegmentUpdated &&
		routeGroupSegmentUpdated
}
