package controller

import (
	"fmt"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
)

// StackResources describes the resources of a stack.
type StackResources struct {
	Deployment *appsv1.Deployment
	HPA        *autoscaling.HorizontalPodAutoscaler
	Service    *v1.Service
	// Endpoints field will be filled by
	// the StackSetController.collectEndpoints
	Endpoints *v1.Endpoints
}

func newDeploymentFromStack(stack zv1.Stack) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        stack.Name,
			Namespace:   stack.Namespace,
			Annotations: map[string]string{},
			Labels:      mapCopy(stack.Labels),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: stack.APIVersion,
					Kind:       stack.Kind,
					Name:       stack.Name,
					UID:        stack.UID,
				},
			},
		},
		// set TypeMeta manually because of this bug:
		// https://github.com/kubernetes/client-go/issues/308
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: stack.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: limitLabels(stack.Labels, selectorLabels),
			},
			Template: newPodTemplateFromStack(stack),
		},
	}
}

func newPodTemplateFromStack(stack zv1.Stack) v1.PodTemplateSpec {
	template := *stack.Spec.PodTemplate.DeepCopy()

	// Copy Labels from Stack.Labels to the Deployment
	template.ObjectMeta.Labels = mapCopy(stack.Labels)

	return template
}

func mapCopy(m map[string]string) map[string]string {
	copy := map[string]string{}
	for k, v := range m {
		copy[k] = v
	}
	return copy
}

func newHPAFromStack(stack zv1.Stack) *autoscaling.HorizontalPodAutoscaler {
	return &autoscaling.HorizontalPodAutoscaler{}
}

func newServiceFromStack(stack zv1.Stack, servicePorts []v1.ServicePort) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stack.Name,
			Namespace: stack.Namespace,
			Labels:    mapCopy(stack.Labels),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: stack.APIVersion,
					Kind:       stack.Kind,
					Name:       stack.Name,
					UID:        stack.UID,
				},
			},
		},
		Spec: v1.ServiceSpec{
			Selector: limitLabels(stack.Labels, selectorLabels),
			Type:     v1.ServiceTypeClusterIP,
			Ports:    servicePorts,
		},
	}
}

// stackAssignedToResource checks whether the stack is assigned to the resource
// by comparing the stack generation with the corresponding resource annotation.
func stackAssignedToResource(stack zv1.Stack, resource metav1.Object) bool {
	// We only update the resource if there are changes.
	// We determine changes by comparing the stackGeneration
	// (observed generation) stored on the resource with the
	// generation of the Stack.
	actualGeneration := getStackGeneration(resource)
	return actualGeneration == stack.Generation
}

// getStackGeneration returns the generation of the stack associated to this resource.
// This value is stored in an annotation of the resource object.
func getStackGeneration(resource metav1.Object) int64 {
	encodedGeneration := resource.GetAnnotations()[stackGenerationAnnotationKey]
	decodedGeneration, err := strconv.ParseInt(encodedGeneration, 10, 64)
	if err != nil {
		return 0
	}
	return decodedGeneration
}

// assignStackToResource assigns a stack to a resource by specifying the stack's generation
// in the resource's annotations.
func assignStackToResource(stack zv1.Stack, resource metav1.Object) {
	annotations := resource.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[stackGenerationAnnotationKey] = fmt.Sprintf("%d", stack.Generation)
	resource.SetAnnotations(annotations)
}
