package core

import (
	"strconv"
	"time"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// findBackendPort 
func findBackendPort() {

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
