package entities

import (
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
)

// StackContainer is a container for storing the full state of a Stack
// including all the managed sub-resources. This includes the Stack resource
// itself and all the sub resources like Deployment, HPA, Service and
// Endpoints.
type StackContainer struct {
	Stack     zv1.Stack
	Resources StackResources
}
