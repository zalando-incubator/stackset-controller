package entities

import (
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewStack creates a new Stack object according to the provided specification.
func NewStack(
	name string,
	namespace string,
	version string,
	owner metav1.OwnerReference,
	spec zv1.StackSpec,
) *zv1.Stack {
	if spec.Service != nil {
		spec.Service = sanitizeServicePorts(spec.Service)
	}
	return &zv1.Stack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				StacksetHeritageLabelKey: owner.Name,
				StackVersionLabelKey:     version,
			},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: spec,
	}
}

// sanitizeServicePorts makes sure the ports has the default fields set if not
// specified.
func sanitizeServicePorts(service *zv1.StackServiceSpec) *zv1.StackServiceSpec {
	for i, port := range service.Ports {
		// set default protocol if not specified
		if port.Protocol == "" {
			port.Protocol = v1.ProtocolTCP
		}
		service.Ports[i] = port
	}
	return service
}
