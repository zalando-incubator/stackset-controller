package entities

import (
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	v1 "k8s.io/api/core/v1"
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
