package entities

import (
	"time"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

// StackSetContainer is a container for storing the full state of a StackSet
// including the sub-resources which are part of the StackSet. It respresents a
// snapshot of the resources currently in the Cluster. This includes an
// optional Ingress resource as well as the current Traffic distribution. It
// also contains a set of StackContainers which respresents the full state of
// the individual Stacks part of the StackSet.
type StackSetContainer struct {
	StackSet zv1.StackSet

	// StackContainers is a set of stacks belonging to the StackSet
	// including the Stack sub resources like Deployments and Services.
	StackContainers map[types.UID]*StackContainer

	// Ingress defines the current Ingress resource belonging to the
	// StackSet. This is a reference to the actual resource while
	// `StackSet.Spec.Ingress` defines the ingress configuration specified
	// by the user on the StackSet.
	Ingress *v1beta1.Ingress

	// Traffic is the current traffic distribution across stacks of the
	// StackSet. The values of this are derived from the related Ingress
	// resource. The key of the map is the Stack name.
	Traffic map[string]TrafficStatus

	// TrafficReconciler is the reconciler implementation used for
	// switching traffic between stacks. E.g. for prescaling stacks before
	// switching traffic.
	TrafficReconciler TrafficReconciler
}

// Stacks returns a slice of Stack resources.
func (ssc StackSetContainer) Stacks() []zv1.Stack {
	stacks := make([]zv1.Stack, 0, len(ssc.StackContainers))
	for _, stackContainer := range ssc.StackContainers {
		stacks = append(stacks, stackContainer.Stack)
	}
	return stacks
}

func (ssc StackSetContainer) StackFromName(name string) *zv1.Stack {
	for _, sc := range ssc.StackContainers {
		if sc.Stack.Name == name {
			return &sc.Stack
		}
	}
	return nil
}

// ScaledownTTL returns the ScaledownTTLSeconds value of a StackSet as a
// time.Duration.
func (ssc StackSetContainer) ScaledownTTL() time.Duration {
	if ttlSec := ssc.StackSet.Spec.StackLifecycle.ScaledownTTLSeconds; ttlSec != nil {
		return time.Second * time.Duration(*ttlSec)
	}
	return 0
}
