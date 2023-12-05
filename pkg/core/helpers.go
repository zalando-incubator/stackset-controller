package core

import (
	"fmt"
	"strconv"
	"time"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	APIVersion   = "zalando.org/v1"
	KindStackSet = "StackSet"
	KindStack    = "Stack"

	stackGenerationAnnotationKey = "stackset-controller.zalando.org/stack-generation"
)

func mergeLabels(labelMaps ...map[string]string) map[string]string {
	labels := make(map[string]string)
	for _, labelMap := range labelMaps {
		for k, v := range labelMap {
			labels[k] = v
		}
	}
	return labels
}

// IsResourceUpToDate checks whether the stack is assigned to the resource
// by comparing the stack generation with the corresponding resource annotation.
func IsResourceUpToDate(stack *zv1.Stack, resourceMeta metav1.ObjectMeta) bool {
	// We only update the resourceMeta if there are changes.
	// We determine changes by comparing the stackGeneration
	// (observed generation) stored on the resourceMeta with the
	// generation of the Stack.
	actualGeneration := getStackGeneration(resourceMeta)
	return actualGeneration == stack.Generation
}

// AreAnnotationsUpToDate checks whether the annotations of the existing and
// updated resource are up to date.
func AreAnnotationsUpToDate(updated, existing metav1.ObjectMeta) bool {
	if len(updated.Annotations) != len(existing.Annotations) {
		return false
	}

	for k, v := range updated.Annotations {
		if k == stackGenerationAnnotationKey {
			continue
		}

		existingValue, ok := existing.GetAnnotations()[k]
		if ok && existingValue == v {
			continue
		}

		return false
	}

	return true
}

// getStackGeneration returns the generation of the stack associated to this resource.
// This value is stored in an annotation of the resource object.
func getStackGeneration(resource metav1.ObjectMeta) int64 {
	encodedGeneration := resource.GetAnnotations()[stackGenerationAnnotationKey]
	decodedGeneration, err := strconv.ParseInt(encodedGeneration, 10, 64)
	if err != nil {
		return 0
	}
	return decodedGeneration
}

// findBackendPort - given an ingress, routegroup and externalIngress, determine
// which backendPort to use.
func findBackendPort(
	ingress *zv1.StackSetIngressSpec,
	routeGroup *zv1.RouteGroupSpec,
	externalIngress *zv1.StackSetExternalIngressSpec,
) (*intstr.IntOrString, error) {
	var port *intstr.IntOrString

	if ingress != nil {
		port = &ingress.BackendPort
	}

	if routeGroup != nil {
		if port != nil && port.IntValue() != routeGroup.BackendPort {
			return nil, fmt.Errorf(
				"backendPort for Ingress and RouteGroup does not match %s!=%d",
				port.String(),
				routeGroup.BackendPort,
			)
		}

		rgPort := intstr.FromInt(routeGroup.BackendPort)
		port = &rgPort
	}

	if port == nil && externalIngress != nil {
		return &externalIngress.BackendPort, nil
	}

	return port, nil
}

func wrapTime(time time.Time) *metav1.Time {
	if time.IsZero() {
		return nil
	}
	return &metav1.Time{Time: time}
}

func unwrapTime(tm *metav1.Time) time.Time {
	if tm.IsZero() {
		return time.Time{}
	}
	return tm.Time
}

func effectiveReplicas(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}

func wrapReplicas(replicas int32) *int32 {
	return &replicas
}
