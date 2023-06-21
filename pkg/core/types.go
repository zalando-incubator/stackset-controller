package core

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
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

var (
	tsRe = regexp.MustCompile(`TrafficSegment\((?P<Low>.*?),(?P<High>.*?)\)`)
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
	ingressSpec    *zv1.StackSetIngressSpec
	routeGroupSpec *zv1.RouteGroupSpec
	scaledownTTL   time.Duration
	backendPort    *intstr.IntOrString
	clusterDomains []string

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
	if sc.Stack.Spec.Autoscaler != nil {
		return sc.Stack.Spec.Autoscaler.MaxReplicas
	}
	if sc.Stack.Spec.HorizontalPodAutoscaler != nil {
		return sc.Stack.Spec.HorizontalPodAutoscaler.MaxReplicas
	}
	return math.MaxInt32
}

func (sc *StackContainer) IsAutoscaled() bool {
	return sc.Stack.Spec.HorizontalPodAutoscaler != nil || sc.Stack.Spec.Autoscaler != nil
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

func (sc *StackContainer) GetSegmentLimits() (float64, float64, error) {
	if sc.Resources.Ingress == nil && sc.Resources.RouteGroup == nil {
		return 0.0, 0.0, nil
	}

	predicates := ""
	if sc.Resources.IngressSegment != nil {
		annotations := sc.Resources.IngressSegment.Annotations
		predicates = annotations[IngressPredicateKey]
	} else {
		return -1.0, -1.0, fmt.Errorf(
			"IngressSegment not found for %s",
			sc.Name(),
		)
	}

	vals := tsRe.FindStringSubmatch(predicates)
	if len(vals) != 3 {
 		return -1.0, -1.0, fmt.Errorf("invalid TrafficSegment annotation")
 	}

	low, err := strconv.ParseFloat(vals[1], 64)
	if err != nil {
		return -1.0, -1.0, fmt.Errorf("invalid low limit: %w", err)
	}

	high, err := strconv.ParseFloat(vals[2], 64)
	if err != nil {
		return -1.0, -1.0, fmt.Errorf("invalid high limit: %w", err)
	}

	return low, high, nil
}

func (sc *StackContainer) SetSegmentLimits(low, high float64) error {
 	if high > low {
 		return fmt.Errorf("high limit must be greater than low limit")
 	}

 	predicates := sc.getPredicates()
 	newPredicates := tsRe.ReplaceAllString(
 		predicates,
 		fmt.Sprintf("TrafficSegment(%.2f, %.2f)", low, high),
 	)

	if sc.Resources.IngressSegment != nil {
		sc.Resources.IngressSegment.Annotations[IngressPredicateKey] = newPredicates
	}

	return nil
}

func (sc *StackContainer) getPredicates() string {
	fmt.Printf("Aqui %v\n", sc.Resources.IngressSegment)
	predicates := ""
	if sc.Resources.IngressSegment != nil {
		annotations := sc.Resources.IngressSegment.Annotations
		predicates = annotations[IngressPredicateKey]
		fmt.Printf("Pred %v\n", predicates)
	}

	// TODO RouteGroup

	return predicates
}

// func (sc *StackContainer) GetSegmentLimits() (float64, float64, error) {
// 	// TODO ignore traffic switch from central ingress
// 	// Set 100% traffic for initial stack
// 	{
// 	predicates := getPredicates()
// 	vals := tsRe.FindStringSubmatch(predicates)
// 	if len(vals) != 3 {
// 		return 0, 0, fmt.Errorf("invalid TrafficSegment annotation")
// 	}

// 	low, err := strconv.ParseFloat(vals[1], 64)
// 	if err != nil {
// 		return 0, 0, fmt.Errorf("invalid low limit: %w", err)
// 	}

// 	high, err := strconv.ParseFloat(vals[2], 64)
// 	if err != nil {
// 		return 0, 0, fmt.Errorf("invalid high limit: %w", err)
// 	}

// 	return 0, 0, nil
// }

// StackResources describes the resources of a stack.
type StackResources struct {
	Deployment        *appsv1.Deployment
	HPA               *autoscaling.HorizontalPodAutoscaler
	Service           *v1.Service
	Ingress           *networking.Ingress
	IngressSegment    *networking.Ingress
	RouteGroup        *rgv1.RouteGroup
	RouteGroupSegment *rgv1.RouteGroup
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

// UpdateFromResources populates stack state information (e.g. replica counts or traffic) from related resources
func (ssc *StackSetContainer) UpdateFromResources() error {
	if len(ssc.StackContainers) == 0 {
		return nil
	}

	var ingressSpec *zv1.StackSetIngressSpec
	var routeGroupSpec *zv1.RouteGroupSpec
	var externalIngress *zv1.StackSetExternalIngressSpec
	var backendPort *intstr.IntOrString

	if ssc.StackSet.Spec.Ingress != nil {
		ingressSpec = ssc.StackSet.Spec.Ingress
		backendPort = &ingressSpec.BackendPort
	}

	if ssc.StackSet.Spec.RouteGroup != nil {
		routeGroupSpec = ssc.StackSet.Spec.RouteGroup
		if backendPort != nil && backendPort.IntValue() != routeGroupSpec.BackendPort {
			return fmt.Errorf("backendPort for Ingress and RouteGroup does not match %s!=%d", ssc.StackSet.Spec.Ingress.BackendPort.String(), routeGroupSpec.BackendPort)
		}
		rgBackendPort := intstr.FromInt(routeGroupSpec.BackendPort)
		backendPort = &rgBackendPort
	}

	// if backendPort is not defined from Ingress or Routegroup fall back
	// to externalIngress if defined
	if backendPort == nil && ssc.StackSet.Spec.ExternalIngress != nil {
		externalIngress = ssc.StackSet.Spec.ExternalIngress
		backendPort = &externalIngress.BackendPort
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
		sc.ingressSpec = ingressSpec
		sc.backendPort = backendPort
		sc.routeGroupSpec = routeGroupSpec
		sc.scaledownTTL = scaledownTTL
		sc.clusterDomains = ssc.clusterDomains
		sc.updateFromResources()
	}

	// only populate traffic if traffic management is enabled
	if ingressSpec != nil || routeGroupSpec != nil || externalIngress != nil {
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

// ComputeTrafficSegments computes the stack segments to fulfill the actual  returns updates the TrafficSegment predicates on all
// ingresses/routegroups to match the actual traffic weights.
func (ssc *StackSetContainer) ComputeTrafficSegments() ([]segment, error) {
	segments, growing, shrinking := []segment{}, []segment{}, []segment{}
	existingStacks := map[types.UID]bool{}
	newWeights := map[types.UID]float64{}

	for uid, sc := range ssc.StackContainers {
		// Active Stacks, as StackSet CRD traffic defines
		if sc.actualTrafficWeight > 0 {
			newWeights[uid] = sc.actualTrafficWeight
		}

		// Currently assigend segments
		low, high, err := sc.GetSegmentLimits()
		if err != nil {
			return nil, err
		}

		if low == high {
			// unused stack, skip
			continue
		}

		segments = append(
			segments,
			segment{id: uid, lowLimit: low, highLimit: high},
		)
	}

	sort.SliceStable(segments, func(i, j int) bool {
		return segments[i].lowLimit < segments[j].highLimit
	})

	index := 0.0
	for _, s := range segments {
		existingStacks[s.id] = true
		w, ok := newWeights[s.id]
		if !ok {
			// Remove traffic
			s.lowLimit = 0.0
			s.highLimit = 0.0
			shrinking = append(shrinking, s)
			continue
		}

		beforeSize := s.size()

		s.lowLimit = index
		s.highLimit = index + w
		index += w

		if s.size() > beforeSize {
			growing = append(growing, s)
		} else {
			shrinking = append(shrinking, s)
		}
	}

	// Add new stacks
	for id, w := range newWeights {
		if !existingStacks[id] {
			growing = append(growing, segment{id, index, index + w})
			index += w
		}
	}

	return append(growing, shrinking...), nil
}

type segment struct {
	id types.UID
	lowLimit  float64
	highLimit float64
}

func (s *segment) size() float64 {
	return s.highLimit - s.lowLimit
}

func (sc *StackContainer) updateFromResources() {
	sc.stackReplicas = effectiveReplicas(sc.Stack.Spec.Replicas)

	var deploymentUpdated, serviceUpdated, ingressUpdated, routeGroupUpdated, hpaUpdated bool
	var ingressSegmentUpdated bool

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
		ingressSegmentUpdated = sc.Resources.IngressSegment != nil &&
			IsResourceUpToDate(sc.Stack, sc.Resources.IngressSegment.ObjectMeta)
	} else {
		// ignore if ingress is not set
		ingressUpdated = sc.Resources.Ingress == nil
	}

	// routegroup: ignore if routegroup is not set or check if we are up to date
	if sc.routeGroupSpec != nil {
		routeGroupUpdated = sc.Resources.RouteGroup != nil && IsResourceUpToDate(sc.Stack, sc.Resources.RouteGroup.ObjectMeta)
	} else {
		// ignore if ingress is not set
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
		ingressSegmentUpdated &&
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
