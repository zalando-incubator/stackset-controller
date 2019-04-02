package controller

import (
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	v1 "k8s.io/api/core/v1"
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
	sr.Deployment = NewDeploymentFromStack(stack)
	sr.HPA = NewHPAFromStack(stack)
	sr.Service = NewServiceFromStack(stack)
	sr.Endpoints = NewEndpointsFromStack(stack)
	return sr
}

func NewDeploymentFromStack(stack zv1.Stack) *appsv1.Deployment {
	return &appsv1.Deployment{
		Spec:       appsv1.DeploymentSpec{
			Replicas:                stack.Spec.Replicas,
			Selector:                nil, // comes from StackSet instead of Stack
			Template:                *stack.Spec.PodTemplate.DeepCopy(),
		},
	}
}

func NewHPAFromStack(stack zv1.Stack) *autoscaling.HorizontalPodAutoscaler {
	return &autoscaling.HorizontalPodAutoscaler{}
}

func NewServiceFromStack(stack zv1.Stack) *v1.Service {
	return &v1.Service{}
}

func NewEndpointsFromStack(stack zv1.Stack) *v1.Endpoints {
	return &v1.Endpoints{}
}
