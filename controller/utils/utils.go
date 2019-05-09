package utils

import (
	"fmt"
	"strconv"

	"github.com/zalando-incubator/stackset-controller/controller/keys"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	// set implementation with 0 Byte value
	selectorLabels = map[string]struct{}{
		keys.StacksetHeritageLabelKey: {},
		keys.StackVersionLabelKey:     {},
	}
)

func NewDeploymentFromStack(stack zv1.Stack) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        stack.Name,
			Namespace:   stack.Namespace,
			Annotations: map[string]string{},
			Labels:      MapCopy(stack.Labels),
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
				MatchLabels: LimitLabels(stack.Labels, selectorLabels),
			},
			Template: NewPodTemplateFromStack(stack),
		},
	}
}

func NewServiceFromStack(servicePorts []v1.ServicePort, stack zv1.Stack, deployment *appsv1.Deployment) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stack.Name,
			Namespace: stack.Namespace,
			Labels:    MapCopy(stack.Labels),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: deployment.APIVersion,
					Kind:       deployment.Kind,
					Name:       deployment.Name,
					UID:        deployment.UID,
				},
			},
		},
		Spec: v1.ServiceSpec{
			Selector: LimitLabels(stack.Labels, selectorLabels),
			Type:     v1.ServiceTypeClusterIP,
			Ports:    servicePorts,
		},
	}
}

// LimitLabels returns a limited set of labels based on the validKeys.
func LimitLabels(labels map[string]string, validKeys map[string]struct{}) map[string]string {
	newLabels := make(map[string]string, len(labels))
	for k, v := range labels {
		if _, ok := validKeys[k]; ok {
			newLabels[k] = v
		}
	}
	return newLabels
}

func NewPodTemplateFromStack(stack zv1.Stack) v1.PodTemplateSpec {
	template := *stack.Spec.PodTemplate.DeepCopy()

	// add labels from Stack.Labels to pods
	return TemplateInjectLabels(template, stack.Labels)
}

// templateInjectLabels injects labels into a pod template spec.
func TemplateInjectLabels(template v1.PodTemplateSpec, labels map[string]string) v1.PodTemplateSpec {
	if template.ObjectMeta.Labels == nil {
		template.ObjectMeta.Labels = map[string]string{}
	}

	for key, value := range labels {
		if _, ok := template.ObjectMeta.Labels[key]; !ok {
			template.ObjectMeta.Labels[key] = value
		}
	}
	return template
}

func MapCopy(m map[string]string) map[string]string {
	newMap := map[string]string{}
	for k, v := range m {
		newMap[k] = v
	}
	return newMap
}

// isResourceUpToDate checks whether the stack is assigned to the resource
// by comparing the stack generation with the corresponding resource annotation.
func IsResourceUpToDate(stack zv1.Stack, resourceMeta metav1.ObjectMeta) bool {
	// We only update the resourceMeta if there are changes.
	// We determine changes by comparing the stackGeneration
	// (observed generation) stored on the resourceMeta with the
	// generation of the Stack.
	actualGeneration := GetStackGeneration(resourceMeta)
	return actualGeneration == stack.Generation
}

// GetStackGeneration returns the generation of the stack associated to this resource.
// This value is stored in an annotation of the resource object.
func GetStackGeneration(resource metav1.ObjectMeta) int64 {
	encodedGeneration := resource.GetAnnotations()[keys.StackGenerationAnnotationKey]
	decodedGeneration, err := strconv.ParseInt(encodedGeneration, 10, 64)
	if err != nil {
		return 0
	}
	return decodedGeneration
}

// SetStackGenerationOnResource assigns a stack to a resource by specifying the stack's generation
// in the resource's annotations.
func SetStackGenerationOnResource(stack zv1.Stack, resource metav1.Object) {
	annotations := resource.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[keys.StackGenerationAnnotationKey] = fmt.Sprintf("%d", stack.Generation)
	resource.SetAnnotations(annotations)
}

func UpdateServiceSpecFromStack(service *v1.Service, stack zv1.Stack, backendPort *intstr.IntOrString) error {
	if service == nil {
		return fmt.Errorf(
			"UpdateServiceSpecFromStack expects an existing Service, not a nil pointer")
	}
	SetStackGenerationOnResource(stack, service)

	service.Labels = stack.Labels
	service.Spec.Selector = LimitLabels(stack.Labels, selectorLabels)

	servicePorts, err := GetServicePorts(backendPort, stack)
	if err != nil {
		return err
	}
	service.Spec.Ports = servicePorts
	return nil
}

// GetServicePorts gets the service ports to be used for the stack service.
func GetServicePorts(backendPort *intstr.IntOrString, stack zv1.Stack) ([]v1.ServicePort, error) {
	var servicePorts []v1.ServicePort
	if stack.Spec.Service == nil || len(stack.Spec.Service.Ports) == 0 {
		servicePorts = servicePortsFromContainers(stack.Spec.PodTemplate.Spec.Containers)
	} else {
		servicePorts = stack.Spec.Service.Ports
	}

	// validate that one port in the list maps to the backendPort.
	if backendPort != nil {
		for _, port := range servicePorts {
			switch backendPort.Type {
			case intstr.Int:
				if port.Port == backendPort.IntVal {
					return servicePorts, nil
				}
			case intstr.String:
				if port.Name == backendPort.StrVal {
					return servicePorts, nil
				}
			}
		}

		return nil, fmt.Errorf("no service ports matching backendPort '%s'", backendPort.String())
	}

	return servicePorts, nil
}

// servicePortsFromTemplate gets service port from pod template.
func servicePortsFromContainers(containers []v1.Container) []v1.ServicePort {
	ports := make([]v1.ServicePort, 0)
	for i, container := range containers {
		for j, port := range container.Ports {
			name := fmt.Sprintf("port-%d-%d", i, j)
			if port.Name != "" {
				name = port.Name
			}
			servicePort := v1.ServicePort{
				Name:       name,
				Protocol:   port.Protocol,
				Port:       port.ContainerPort,
				TargetPort: intstr.FromInt(int(port.ContainerPort)),
			}
			// set default protocol if not specified
			if servicePort.Protocol == "" {
				servicePort.Protocol = v1.ProtocolTCP
			}
			ports = append(ports, servicePort)
		}
	}
	return ports
}
