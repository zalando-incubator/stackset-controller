package core

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/traffic"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	defaultVersion             = "default"
	defaultStackLifecycleLimit = 10
	defaultScaledownTTL        = 300 * time.Second
)

// StackSetContainer is a container for storing the full state of a StackSet
// including the sub-resources which are part of the StackSet. It respresents a
// snapshot of the resources currently in the Cluster. This includes an
// optional Ingress resource as well as the current Traffic distribution. It
// also contains a set of StackContainers which respresents the full state of
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
	Ingress *extensions.Ingress

	// TrafficReconciler is the reconciler implementation used for
	// switching traffic between stacks. E.g. for prescaling stacks before
	// switching traffic.
	TrafficReconciler TrafficReconciler

	// ExternalIngressBackendPort defines the backendPort mapping
	// if an external entity creates ingress objects for us. The
	// Ingress of stackset should be nil in this case.
	externalIngressBackendPort *intstr.IntOrString

	// Whether the stackset should be authoritative for the traffic, and not the ingress
	stacksetManagesTraffic bool

	// BackendWeightsAnnotationKey to store the runtime decision
	// which annotation is used, defaults to
	// traffic.DefaultBackendWeightsAnnotationKey
	BackendWeightsAnnotationKey string
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
	stacksetName string
	ingressSpec  *zv1.StackSetIngressSpec
	scaledownTTL time.Duration
	backendPort  *intstr.IntOrString

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

	// Current number of replicas that the HPA expects deployment to have, from HPA.status
	desiredReplicas int32

	// Traffic & scaling
	currentActualTrafficWeight     float64
	actualTrafficWeight            float64
	desiredTrafficWeight           float64
	noTrafficSince                 time.Time
	prescalingActive               bool
	prescalingReplicas             int32
	prescalingDesiredTrafficWeight float64
	prescalingLastTrafficIncrease  time.Time
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
	// Stacks are considered ready when all subresources have been updated, and we have enough replicas
	return sc.resourcesUpdated && sc.deploymentReplicas > 0 && sc.deploymentReplicas == sc.updatedReplicas && sc.deploymentReplicas == sc.readyReplicas
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

// StackResources describes the resources of a stack.
type StackResources struct {
	Deployment *appsv1.Deployment
	HPA        *autoscaling.HorizontalPodAutoscaler
	Service    *v1.Service
	Ingress    *extensions.Ingress
}

func (ssc *StackSetContainer) stackByName(name string) *StackContainer {
	for _, container := range ssc.StackContainers {
		if container.Name() == name {
			return container
		}
	}
	return nil
}

// updateDesiredTrafficFromIngress updates traffic weights of stack containers from the ingress object
func (ssc *StackSetContainer) updateDesiredTrafficFromIngress() error {
	desired, err := ssc.getNormalizedTrafficFromIngress(traffic.StackTrafficWeightsAnnotationKey)
	if err != nil {
		return fmt.Errorf("failed to get current actual Stack traffic weights: %v", err)
	}

	for _, container := range ssc.StackContainers {
		container.desiredTrafficWeight = desired[container.Name()]
	}

	return nil
}

func (ssc *StackSetContainer) updateActualTrafficFromIngress() error {
	actual, err := ssc.getNormalizedTrafficFromIngress(ssc.BackendWeightsAnnotationKey)
	if err != nil {
		return fmt.Errorf("failed to get current actual Stack traffic weights: %v", err)
	}

	for _, container := range ssc.StackContainers {
		container.actualTrafficWeight = actual[container.Name()]
		container.currentActualTrafficWeight = actual[container.Name()]
	}

	return nil
}

func (ssc *StackSetContainer) getNormalizedTrafficFromIngress(annotationKey string) (map[string]float64, error) {
	weight := make(map[string]float64)

	if ssc.StackSet.Spec.Ingress != nil && ssc.Ingress != nil && len(ssc.StackContainers) > 0 {
		stacksetNames := make(map[string]struct{})
		for _, sc := range ssc.StackContainers {
			stacksetNames[sc.Name()] = struct{}{}
		}

		if w, ok := ssc.Ingress.Annotations[annotationKey]; ok {
			err := json.Unmarshal([]byte(w), &weight)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal json from annotation ingress %s: %v", annotationKey, err)
			}
		}

		// Remove weights for stacks that no longer exist, normalize the result
		for name := range weight {
			if _, ok := stacksetNames[name]; !ok {
				delete(weight, name)
			}
		}

		if !allZero(weight) {
			normalizeWeights(weight)
		}
	}
	return weight, nil
}

func (ssc *StackSetContainer) hasDesiredTrafficFromStackSet() bool {
	return len(ssc.StackSet.Spec.Traffic) > 0
}

func (ssc *StackSetContainer) hasActualTrafficFromStackSet() bool {
	return len(ssc.StackSet.Status.Traffic) > 0
}

func (ssc *StackSetContainer) findFallbackStack() *StackContainer {
	stacks := make(map[string]*StackContainer)
	for _, stack := range ssc.StackContainers {
		stacks[stack.Name()] = stack
	}
	return findFallbackStack(stacks)
}

// updateDesiredTrafficFromStackSet gets desired from stackset spec
// and populates it to stack containers
func (ssc *StackSetContainer) updateDesiredTrafficFromStackSet() error {
	weights := make(map[string]float64)

	if ssc.hasDesiredTrafficFromStackSet() {
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
	}

	// save values in stack containers
	for _, container := range ssc.StackContainers {
		container.desiredTrafficWeight = weights[container.Name()]
	}

	return nil
}

// updateActualTrafficFromStackSet gets actual from stackset status
// and populates it to stack containers
func (ssc *StackSetContainer) updateActualTrafficFromStackSet() error {
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
func (ssc *StackSetContainer) UpdateFromResources(stacksetManageTraffic bool) error {
	ssc.stacksetManagesTraffic = stacksetManageTraffic
	if len(ssc.StackContainers) == 0 {
		return nil
	}

	var ingressSpec *zv1.StackSetIngressSpec
	var externalIngress *zv1.StackSetExternalIngressSpec
	var backendPort *intstr.IntOrString

	if ssc.StackSet.Spec.Ingress != nil {
		ingressSpec = ssc.StackSet.Spec.Ingress
		backendPort = &ingressSpec.BackendPort
	} else if ssc.StackSet.Spec.ExternalIngress != nil {
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
		sc.scaledownTTL = scaledownTTL
		sc.updateFromResources()
	}

	// only populate traffic if traffic management is enabled
	if ingressSpec != nil || externalIngress != nil {
		if ssc.hasDesiredTrafficFromStackSet() || externalIngress != nil {
			err := ssc.updateDesiredTrafficFromStackSet()
			if err != nil {
				return err
			}
			ssc.stacksetManagesTraffic = true
		} else {
			if err := ssc.updateDesiredTrafficFromIngress(); err != nil {
				return err
			}
		}

		if ssc.hasActualTrafficFromStackSet() || externalIngress != nil {
			return ssc.updateActualTrafficFromStackSet()
		}

		// TODO(sszuecs): delete until end of function, if we drop ingress based desired traffic. For step1 we need the fallback but update the stackset status, too
		return ssc.updateActualTrafficFromIngress()
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

func (sc *StackContainer) updateFromResources() {
	sc.stackReplicas = effectiveReplicas(sc.Stack.Spec.Replicas)

	var deploymentUpdated, serviceUpdated, ingressUpdated, hpaUpdated bool

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

	// hpa
	if sc.Resources.HPA != nil {
		hpa := sc.Resources.HPA
		sc.desiredReplicas = hpa.Status.DesiredReplicas
	}
	if sc.IsAutoscaled() {
		hpaUpdated = sc.Resources.HPA != nil && IsResourceUpToDate(sc.Stack, sc.Resources.HPA.ObjectMeta)
	} else {
		hpaUpdated = sc.Resources.HPA == nil
	}

	// aggregated 'resources updated' for the readiness
	sc.resourcesUpdated = deploymentUpdated && serviceUpdated && ingressUpdated && hpaUpdated

	status := sc.Stack.Status
	sc.noTrafficSince = unwrapTime(status.NoTrafficSince)
	if status.Prescaling.Active {
		sc.prescalingActive = true
		sc.prescalingReplicas = status.Prescaling.Replicas
		sc.prescalingDesiredTrafficWeight = status.Prescaling.DesiredTrafficWeight
		sc.prescalingLastTrafficIncrease = unwrapTime(status.Prescaling.LastTrafficIncrease)
	}
}
