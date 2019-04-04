package controller

import (
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// StackResources describes the resources of a stack.
type StackResources struct {
	Deployment *appsv1.Deployment
	HPA        *autoscaling.HorizontalPodAutoscaler
	Service    *v1.Service
	Endpoints  *v1.Endpoints
}

// NewStackResources creates stack resources corresponding to the desired state described in stack.
// This function contains "pure" logic extracted from stack.go/manageDeployment.
func NewStackResources(stack zv1.Stack) StackResources {
	var sr StackResources
	sr.Deployment = newDeploymentFromStack(stack)
	sr.HPA = newHPAFromStack(stack)
	sr.Service = newServiceFromStack(stack)
	// Returning empty Endpoints field because it will be filled by
	// the StackSetController.collectEndpoints

	return sr
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

func newServiceFromStack(stack zv1.Stack, backendPort *intstr.IntOrString) *v1.Service {
	servicePorts, err := getServicePorts(backendPort, stack)
	// TODO: figure out if we really need to return these errors
	if err != nil {
		return err
	}

	return 	&v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stack.Name,
			Namespace: stack.Namespace,
			Labels: mapCopy(stack.Labels),
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
			Type: v1.ServiceTypeClusterIP,
			Ports: servicePorts,
		},
	}

}
