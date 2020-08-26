package core

import (
	"fmt"
	"strconv"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta2"
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	apiVersionAppsV1 = "apps/v1"
	kindDeployment   = "Deployment"
	// defaults for scaleUp and scaleDown StabilizationWindowSeconds
	// ScaleUp:
	// https://github.com/kubernetes/kubernetes/blob/9f2892aab98fe339f3bd70e3c470144299398ace/pkg/apis/autoscaling/v2beta2/defaults.go#L31
	// ScaleDown:
	// https://github.com/kubernetes/kubernetes/blob/9f2892aab98fe339f3bd70e3c470144299398ace/pkg/apis/autoscaling/v2beta2/defaults.go#L50-L52
	// Not a fixed value because it can be set via a flag on the
	// controller-manager. We use the default flag value:
	// https://github.com/kubernetes/kubernetes/blob/9f2892aab98fe339f3bd70e3c470144299398ace/pkg/controller/podautoscaler/config/v1alpha1/defaults.go#L48
	defaultScaleUpStabilizationSeconds   = 0
	defaultScaleDownStabilizationSeconds = 300
)

var (
	// set implementation with 0 Byte value
	selectorLabels = map[string]struct{}{
		StacksetHeritageLabelKey: {},
		StackVersionLabelKey:     {},
	}
)

func mapCopy(m map[string]string) map[string]string {
	newMap := map[string]string{}
	for k, v := range m {
		newMap[k] = v
	}
	return newMap
}

// limitLabels returns a limited set of labels based on the validKeys.
func limitLabels(labels map[string]string, validKeys map[string]struct{}) map[string]string {
	newLabels := make(map[string]string, len(labels))
	for k, v := range labels {
		if _, ok := validKeys[k]; ok {
			newLabels[k] = v
		}
	}
	return newLabels
}

// templateObjectMetaInjectLabels injects labels into a pod template spec.
func objectMetaInjectLabels(objectMeta metav1.ObjectMeta, labels map[string]string) metav1.ObjectMeta {
	if objectMeta.Labels == nil {
		objectMeta.Labels = map[string]string{}
	}
	for key, value := range labels {
		if _, ok := objectMeta.Labels[key]; !ok {
			objectMeta.Labels[key] = value
		}
	}
	return objectMeta
}

func embeddedToObjectMeta(embedded zv1.EmbeddedObjectMeta) metav1.ObjectMeta {
	c := embedded.DeepCopy()
	return metav1.ObjectMeta{
		Annotations: c.Annotations,
		Labels:      c.Labels,
	}
}

func (sc *StackContainer) resourceMeta() metav1.ObjectMeta {
	resourceLabels := mapCopy(sc.Stack.Labels)

	return metav1.ObjectMeta{
		Name:      sc.Name(),
		Namespace: sc.Namespace(),
		Annotations: map[string]string{
			stackGenerationAnnotationKey: strconv.FormatInt(sc.Stack.Generation, 10),
		},
		Labels: resourceLabels,
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: APIVersion,
				Kind:       KindStack,
				Name:       sc.Name(),
				UID:        sc.Stack.UID,
			},
		},
	}
}

// getServicePorts gets the service ports to be used for the stack service.
func getServicePorts(stackSpec zv1.StackSpec, backendPort *intstr.IntOrString) ([]v1.ServicePort, error) {
	var servicePorts []v1.ServicePort
	if stackSpec.Service == nil || len(stackSpec.Service.Ports) == 0 {
		servicePorts = servicePortsFromContainers(stackSpec.PodTemplate.Spec.Containers)
	} else {
		servicePorts = stackSpec.Service.Ports
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

func (sc *StackContainer) selector() map[string]string {
	return limitLabels(sc.Stack.Labels, selectorLabels)
}

func (sc *StackContainer) GenerateDeployment() *appsv1.Deployment {
	stack := sc.Stack

	desiredReplicas := sc.stackReplicas
	if sc.prescalingActive {
		desiredReplicas = sc.prescalingReplicas
	}

	var updatedReplicas *int32

	if desiredReplicas != 0 && !sc.ScaledDown() {
		// Stack scaled up, rescale the deployment if it's at 0 replicas, or if HPA is unused and we don't run autoscaling
		if sc.deploymentReplicas == 0 || (!sc.IsAutoscaled() && desiredReplicas != sc.deploymentReplicas) {
			updatedReplicas = wrapReplicas(desiredReplicas)
		}
	} else {
		// Stack scaled down (manually or because it doesn't receive traffic), check if we need to scale down the deployment
		if sc.deploymentReplicas != 0 {
			updatedReplicas = wrapReplicas(0)
		}
	}

	if updatedReplicas == nil {
		updatedReplicas = wrapReplicas(sc.deploymentReplicas)
	}

	var strategy *appsv1.DeploymentStrategy
	if stack.Spec.Strategy != nil {
		strategy = stack.Spec.Strategy.DeepCopy()
	}

	embeddedCopy := stack.Spec.PodTemplate.EmbeddedObjectMeta.DeepCopy()

	templateObjectMeta := metav1.ObjectMeta{
		Annotations: embeddedCopy.Annotations,
		Labels:      embeddedCopy.Labels,
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: sc.resourceMeta(),
		Spec: appsv1.DeploymentSpec{
			Replicas: updatedReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: sc.selector(),
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: objectMetaInjectLabels(templateObjectMeta, stack.Labels),
				Spec:       *stack.Spec.PodTemplate.Spec.DeepCopy(),
			},
		},
	}
	if strategy != nil {
		deployment.Spec.Strategy = *strategy
	}
	return deployment
}

func (sc *StackContainer) GenerateHPA() (*autoscaling.HorizontalPodAutoscaler, error) {
	autoscalerSpec := sc.Stack.Spec.Autoscaler
	hpaSpec := sc.Stack.Spec.HorizontalPodAutoscaler

	if autoscalerSpec == nil && hpaSpec == nil {
		return nil, nil
	}

	result := &autoscaling.HorizontalPodAutoscaler{
		ObjectMeta: sc.resourceMeta(),
		TypeMeta: metav1.TypeMeta{
			Kind:       "HorizontalPodAutoscaler",
			APIVersion: "autoscaling/v2beta2",
		},
		Spec: autoscaling.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscaling.CrossVersionObjectReference{
				APIVersion: apiVersionAppsV1,
				Kind:       kindDeployment,
				Name:       sc.Name(),
			},
		},
	}

	if autoscalerSpec != nil {
		result.Spec.MinReplicas = autoscalerSpec.MinReplicas
		result.Spec.MaxReplicas = autoscalerSpec.MaxReplicas

		metrics, annotations, err := convertCustomMetrics(sc.stacksetName, sc.Name(), autoscalerSpec.Metrics)
		if err != nil {
			return nil, err
		}
		result.Spec.Metrics = metrics
		result.Annotations = mergeLabels(result.Annotations, annotations)
		result.Spec.Behavior = defaultAutoscaleBehavior(autoscalerSpec.Behavior)
	} else {
		result.Spec.MinReplicas = hpaSpec.MinReplicas
		result.Spec.MaxReplicas = hpaSpec.MaxReplicas
		metrics := make([]autoscaling.MetricSpec, 0, len(hpaSpec.Metrics))
		for _, m := range hpaSpec.Metrics {
			m := m
			metric := autoscaling.MetricSpec{}
			err := Convert_v2beta1_MetricSpec_To_autoscaling_MetricSpec(&m, &metric, nil)
			if err != nil {
				return nil, err
			}
			metrics = append(metrics, metric)
		}
		result.Spec.Metrics = metrics
		result.Spec.Behavior = defaultAutoscaleBehavior(hpaSpec.Behavior)
	}

	// If prescaling is enabled, ensure we have at least `precalingReplicas` pods
	if sc.prescalingActive && (result.Spec.MinReplicas == nil || *result.Spec.MinReplicas < sc.prescalingReplicas) {
		pr := sc.prescalingReplicas
		result.Spec.MinReplicas = &pr
	}

	return result, nil
}

// defaultAutoscaleBehavior sets the defaults of the autoscale behavior if no
// value is specified.
// This is required for scalDown and scaleUp stabilizationWindowSeconds because
// an int32 is expected instead of `null`.
func defaultAutoscaleBehavior(behavior *autoscaling.HorizontalPodAutoscalerBehavior) *autoscaling.HorizontalPodAutoscalerBehavior {
	if behavior != nil && behavior.ScaleDown != nil && behavior.ScaleDown.StabilizationWindowSeconds == nil {
		stabilizationWindowSeconds := (int32)(defaultScaleDownStabilizationSeconds)
		behavior.ScaleDown.StabilizationWindowSeconds = &stabilizationWindowSeconds
	}

	if behavior != nil && behavior.ScaleUp != nil && behavior.ScaleUp.StabilizationWindowSeconds == nil {
		stabilizationWindowSeconds := (int32)(defaultScaleUpStabilizationSeconds)
		behavior.ScaleUp.StabilizationWindowSeconds = &stabilizationWindowSeconds
	}

	return behavior
}

func (sc *StackContainer) GenerateService() (*v1.Service, error) {
	// get service ports to be used for the service
	var backendPort *intstr.IntOrString
	// Ingress or external managed Ingress
	if sc.HasBackendPort() {
		backendPort = sc.backendPort
	}

	servicePorts, err := getServicePorts(sc.Stack.Spec, backendPort)
	if err != nil {
		return nil, err
	}

	metaObj := sc.resourceMeta()
	stackSpec := sc.Stack.Spec
	if stackSpec.Service != nil {
		metaObj.Annotations = mergeLabels(metaObj.Annotations, stackSpec.Service.Annotations)
	}
	return &v1.Service{
		ObjectMeta: metaObj,
		Spec: v1.ServiceSpec{
			Selector: sc.selector(),
			Type:     v1.ServiceTypeClusterIP,
			Ports:    servicePorts,
		},
	}, nil
}

func (sc *StackContainer) GenerateIngress() (*networking.Ingress, error) {
	if !sc.HasBackendPort() || sc.ingressSpec == nil {
		return nil, nil
	}

	result := &networking.Ingress{
		ObjectMeta: sc.resourceMeta(),
		Spec: networking.IngressSpec{
			Rules: []networking.IngressRule{
				{
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path: sc.ingressSpec.Path,
									Backend: networking.IngressBackend{
										ServiceName: sc.Name(),
										ServicePort: *sc.backendPort,
									},
								},
							},
						},
					},
					Host: fmt.Sprintf("%s.%s", sc.Name(), sc.clusterDomain),
				},
			},
		},
	}

	// insert annotations
	result.Annotations = mergeLabels(result.Annotations, sc.ingressSpec.Annotations)
	return result, nil
}

func (sc *StackContainer) GenerateStackStatus() *zv1.StackStatus {
	prescaling := zv1.PrescalingStatus{}
	if sc.prescalingActive {
		prescaling = zv1.PrescalingStatus{
			Active:               sc.prescalingActive,
			Replicas:             sc.prescalingReplicas,
			DesiredTrafficWeight: sc.prescalingDesiredTrafficWeight,
			LastTrafficIncrease:  wrapTime(sc.prescalingLastTrafficIncrease),
		}
	}
	return &zv1.StackStatus{
		ActualTrafficWeight:  sc.actualTrafficWeight,
		DesiredTrafficWeight: sc.desiredTrafficWeight,
		Replicas:             sc.createdReplicas,
		ReadyReplicas:        sc.readyReplicas,
		UpdatedReplicas:      sc.updatedReplicas,
		DesiredReplicas:      sc.deploymentReplicas,
		Prescaling:           prescaling,
		NoTrafficSince:       wrapTime(sc.noTrafficSince),
		LabelSelector:        labels.Set(sc.selector()).String(),
	}
}
