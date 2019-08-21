package core

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
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

// UpdateTrafficFromIngress updates traffic weights of stack containers from the ingress object
func (ssc *StackSetContainer) updateTrafficFromIngress() error {
	desired := make(map[string]float64)
	actual := make(map[string]float64)

	if ssc.StackSet.Spec.Ingress != nil && ssc.Ingress != nil && len(ssc.StackContainers) > 0 {
		stacksetNames := make(map[string]struct{})
		for _, sc := range ssc.StackContainers {
			stacksetNames[sc.Name()] = struct{}{}
		}

		if weights, ok := ssc.Ingress.Annotations[stackTrafficWeightsAnnotationKey]; ok {
			err := json.Unmarshal([]byte(weights), &desired)
			if err != nil {
				return fmt.Errorf("failed to get current desired Stack traffic weights: %v", err)
			}
		}

		if weights, ok := ssc.Ingress.Annotations[backendWeightsAnnotationKey]; ok {
			err := json.Unmarshal([]byte(weights), &actual)
			if err != nil {
				return fmt.Errorf("failed to get current actual Stack traffic weights: %v", err)
			}
		}

		// Remove weights for stacks that no longer exist, normalize the result
		for _, weights := range []map[string]float64{desired, actual} {
			for name := range weights {
				if _, ok := stacksetNames[name]; !ok {
					delete(weights, name)
				}
			}

			if !allZero(weights) {
				normalizeWeights(weights)
			}
		}
	}

	for _, container := range ssc.StackContainers {
		container.desiredTrafficWeight = desired[container.Name()]
		container.actualTrafficWeight = actual[container.Name()]
		container.currentActualTrafficWeight = actual[container.Name()]
	}

	return nil
}

// UpdateFromResources populates stack state information (e.g. replica counts or traffic) from related resources
func (ssc *StackSetContainer) UpdateFromResources() error {
	for _, sc := range ssc.StackContainers {
		sc.stacksetName = ssc.StackSet.Name
		sc.ingressSpec = ssc.StackSet.Spec.Ingress
		if ssc.StackSet.Spec.StackLifecycle.ScaledownTTLSeconds == nil {
			sc.scaledownTTL = defaultScaledownTTL
		} else {
			sc.scaledownTTL = time.Duration(*ssc.StackSet.Spec.StackLifecycle.ScaledownTTLSeconds) * time.Second
		}
		sc.updateFromResources()
	}

	return ssc.updateTrafficFromIngress()
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

	// ingress
	if sc.ingressSpec != nil {
		ingressUpdated = sc.Resources.Ingress != nil && IsResourceUpToDate(sc.Stack, sc.Resources.Ingress.ObjectMeta)
	} else {
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
