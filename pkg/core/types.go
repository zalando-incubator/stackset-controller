package core

import (
	"encoding/json"
	"fmt"
	"math"
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
	deploymentUpdated  bool
	deploymentReplicas int32
	createdReplicas    int32
	readyReplicas      int32
	updatedReplicas    int32
	desiredReplicas    int32

	// Traffic & scaling
	actualTrafficWeight           float64
	desiredTrafficWeight          float64
	noTrafficSince                time.Time
	prescalingActive              bool
	prescalingReplicas            int32
	prescalingLastTrafficIncrease time.Time
}

func (sc *StackContainer) HasTraffic() bool {
	return sc.actualTrafficWeight > 0 || sc.desiredTrafficWeight > 0
}

func (sc *StackContainer) IsReady() bool {
	// Haven't updated yet
	if !sc.deploymentUpdated {
		return false
	}

	return sc.deploymentReplicas == sc.updatedReplicas && sc.deploymentReplicas == sc.readyReplicas
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

// StackResources describes the resources of a stack.
type StackResources struct {
	Deployment *appsv1.Deployment
	HPA        *autoscaling.HorizontalPodAutoscaler
	Service    *v1.Service
	Ingress    *extensions.Ingress
}

func (ssc *StackSetContainer) stackByName(name string) *StackContainer {
	for _, container := range ssc.StackContainers {
		if container.Stack.Name == name {
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
			stacksetNames[sc.Stack.Name] = struct{}{}
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
		container.desiredTrafficWeight = desired[container.Stack.Name]
		container.actualTrafficWeight = actual[container.Stack.Name]
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

func (sc *StackContainer) updateFromResources() {
	sc.stackReplicas = effectiveReplicas(sc.Stack.Spec.Replicas)

	if sc.Resources.Deployment != nil {
		deployment := sc.Resources.Deployment
		sc.deploymentReplicas = effectiveReplicas(deployment.Spec.Replicas)
		sc.createdReplicas = deployment.Status.Replicas
		sc.readyReplicas = deployment.Status.ReadyReplicas
		sc.updatedReplicas = deployment.Status.UpdatedReplicas
		sc.deploymentUpdated = IsResourceUpToDate(sc.Stack, sc.Resources.Deployment.ObjectMeta) && deployment.Status.ObservedGeneration == deployment.Generation
	} else {
		sc.deploymentUpdated = false
	}

	if sc.Resources.HPA != nil {
		sc.desiredReplicas = sc.Resources.HPA.Status.DesiredReplicas
	}

	status := sc.Stack.Status
	sc.noTrafficSince = unwrapTime(status.NoTrafficSince)
	sc.prescalingActive = status.Prescaling.Active
	sc.prescalingReplicas = status.Prescaling.Replicas
	sc.prescalingLastTrafficIncrease = unwrapTime(status.Prescaling.LastTrafficIncrease)
}
