package core

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	apiVersionAppsV1 = "apps/v1"
	kindDeployment   = "Deployment"

	SegmentSuffix       = "-traffic-segment"
	IngressPredicateKey = "zalando.org/skipper-predicate"
)

type ingressOrRouteGroupSpec interface {
	GetAnnotations() map[string]string
	GetHosts() []string
}

var (
	// set implementation with 0 Byte value
	selectorLabels = map[string]struct{}{
		StacksetHeritageLabelKey: {},
		StackVersionLabelKey:     {},
	}

	// PathTypeImplementationSpecific is the used implementation path type
	// for k8s.io/api/networking/v1.HTTPIngressPath resources.
	PathTypeImplementationSpecific = networking.PathTypeImplementationSpecific
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

func (sc *StackContainer) objectMeta(segment bool) metav1.ObjectMeta {
	resourceLabels := mapCopy(sc.Stack.Labels)

	name := sc.Name()
	if segment {
		name += SegmentSuffix
	}

	return metav1.ObjectMeta{
		Name:      name,
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

func (sc *StackContainer) resourceMeta() metav1.ObjectMeta {
	return sc.objectMeta(false)
}

// getServicePorts gets the service ports to be used for the stack service.
func getServicePorts(stackSpec zv1.StackSpecInternal, backendPort *intstr.IntOrString) ([]v1.ServicePort, error) {
	var servicePorts []v1.ServicePort
	if stackSpec.StackSpec.Service == nil ||
		len(stackSpec.StackSpec.Service.Ports) == 0 {

		servicePorts = servicePortsFromContainers(
			stackSpec.StackSpec.PodTemplate.Spec.Containers,
		)
	} else {
		servicePorts = stackSpec.StackSpec.Service.Ports
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
	if stack.Spec.StackSpec.Strategy != nil {
		strategy = stack.Spec.StackSpec.Strategy.DeepCopy()
	}

	embeddedCopy := stack.Spec.StackSpec.PodTemplate.EmbeddedObjectMeta.DeepCopy()

	templateObjectMeta := metav1.ObjectMeta{
		Annotations: embeddedCopy.Annotations,
		Labels:      embeddedCopy.Labels,
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: sc.resourceMeta(),
		Spec: appsv1.DeploymentSpec{
			Replicas:        updatedReplicas,
			MinReadySeconds: sc.Stack.Spec.StackSpec.MinReadySeconds,
			Selector: &metav1.LabelSelector{
				MatchLabels: sc.selector(),
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: objectMetaInjectLabels(templateObjectMeta, stack.Labels),
				Spec:       *stack.Spec.StackSpec.PodTemplate.Spec.DeepCopy(),
			},
		},
	}
	if strategy != nil {
		deployment.Spec.Strategy = *strategy
	}
	return deployment
}

func (sc *StackContainer) GenerateHPA() (
	*autoscaling.HorizontalPodAutoscaler,
	error,
) {
	autoscalerSpec := sc.Stack.Spec.StackSpec.Autoscaler
	trafficWeight := sc.actualTrafficWeight

	if autoscalerSpec == nil {
		return nil, nil
	}

	if sc.ScaledDown() {
		return nil, nil
	}

	result := &autoscaling.HorizontalPodAutoscaler{
		ObjectMeta: sc.resourceMeta(),
		TypeMeta: metav1.TypeMeta{
			Kind:       "HorizontalPodAutoscaler",
			APIVersion: "autoscaling/v2",
		},
		Spec: autoscaling.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscaling.CrossVersionObjectReference{
				APIVersion: apiVersionAppsV1,
				Kind:       kindDeployment,
				Name:       sc.Name(),
			},
		},
	}

	result.Spec.MinReplicas = autoscalerSpec.MinReplicas
	result.Spec.MaxReplicas = autoscalerSpec.MaxReplicas

	metrics, annotations, err := convertCustomMetrics(
		sc.Name()+SegmentSuffix,
		sc.Name(),
		sc.Namespace(),
		autoscalerMetricsList(autoscalerSpec.Metrics),
		trafficWeight,
	)

	if err != nil {
		return nil, err
	}
	result.Spec.Metrics = metrics
	result.Annotations = mergeLabels(result.Annotations, annotations)
	result.Spec.Behavior = autoscalerSpec.Behavior

	// If prescaling is enabled, ensure we have at least `precalingReplicas` pods
	if sc.prescalingActive && (result.Spec.MinReplicas == nil || *result.Spec.MinReplicas < sc.prescalingReplicas) {
		pr := sc.prescalingReplicas
		result.Spec.MinReplicas = &pr
	}

	return result, nil
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
	if stackSpec.StackSpec.Service != nil {
		metaObj.Annotations = mergeLabels(
			metaObj.Annotations,
			stackSpec.StackSpec.Service.Annotations,
		)
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

func (sc *StackContainer) stackHostnames(
	spec ingressOrRouteGroupSpec,
	segment bool,
) ([]string, error) {
	// The Ingress segment uses the original hostnames
	if segment {
		return spec.GetHosts(), nil
	}
	result := sets.NewString()

	// Old-style autogenerated hostnames
	for _, host := range spec.GetHosts() {
		if strings.HasSuffix(host, sc.perStackDomain) {
			result.Insert(fmt.Sprintf("%s.%s", sc.Name(), sc.perStackDomain))
		} else {
			log.Debugf(
				"Ingress host: %s suffix did not match cluster-domain %s",
				host,
				sc.perStackDomain,
			)
		}
	}
	return result.List(), nil
}

func (sc *StackContainer) GenerateIngress() (*networking.Ingress, error) {
	return sc.generateIngress(false)
}

func (sc *StackContainer) GenerateIngressSegment() (
	*networking.Ingress,
	error,
) {
	res, err := sc.generateIngress(true)
	if err != nil || res == nil {
		return res, err
	}

	// Synchronize annotations specified in the StackSet.
	res.Annotations = syncAnnotations(
		res.Annotations,
		sc.syncAnnotationsInIngress,
		sc.ingressAnnotationsToSync,
	)

	if predVal, ok := res.Annotations[IngressPredicateKey]; !ok || predVal == "" {
		res.Annotations = mergeLabels(
			res.Annotations,
			map[string]string{IngressPredicateKey: sc.trafficSegment()},
		)
	} else {
		res.Annotations = mergeLabels(
			res.Annotations,
			map[string]string{
				IngressPredicateKey: sc.trafficSegment() + " && " + predVal,
			},
		)
	}

	return res, nil
}

func (sc *StackContainer) generateIngress(segment bool) (
	*networking.Ingress,
	error,
) {

	if !sc.HasBackendPort() || sc.ingressSpec == nil {
		return nil, nil
	}

	hostnames, err := sc.stackHostnames(sc.ingressSpec, segment)
	if err != nil {
		return nil, err
	}
	if len(hostnames) == 0 {
		return nil, nil
	}

	rules := make([]networking.IngressRule, 0, len(hostnames))
	for _, hostname := range hostnames {
		rules = append(rules, networking.IngressRule{
			IngressRuleValue: networking.IngressRuleValue{
				HTTP: &networking.HTTPIngressRuleValue{
					Paths: []networking.HTTPIngressPath{
						{
							PathType: &PathTypeImplementationSpecific,
							Path:     sc.ingressSpec.Path,
							Backend: networking.IngressBackend{
								Service: &networking.IngressServiceBackend{
									Name: sc.Name(),
									Port: networking.ServiceBackendPort{
										Name:   sc.backendPort.StrVal,
										Number: sc.backendPort.IntVal,
									},
								},
							},
						},
					},
				},
			},
			Host: hostname,
		})
	}

	// sort rules by hostname for a stable order
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Host < rules[j].Host
	})

	result := &networking.Ingress{
		ObjectMeta: sc.objectMeta(segment),
		Spec: networking.IngressSpec{
			Rules: rules,
		},
	}

	// insert annotations
	result.Annotations = mergeLabels(
		result.Annotations,
		sc.ingressSpec.GetAnnotations(),
	)

	return result, nil
}

func (sc *StackContainer) GenerateRouteGroup() (*rgv1.RouteGroup, error) {
	return sc.generateRouteGroup(false)
}

func (sc *StackContainer) GenerateRouteGroupSegment() (
	*rgv1.RouteGroup,
	error,
) {
	res, err := sc.generateRouteGroup(true)
	if err != nil || res == nil {
		return res, err
	}

	// Synchronize annotations specified in the StackSet.
	res.Annotations = syncAnnotations(
		res.Annotations,
		sc.syncAnnotationsInRouteGroup,
		sc.ingressAnnotationsToSync,
	)

	segmentedRoutes := []rgv1.RouteGroupRouteSpec{}
	for _, r := range res.Spec.Routes {
		r.Predicates = append(r.Predicates, sc.trafficSegment())
		segmentedRoutes = append(segmentedRoutes, r)
	}
	res.Spec.Routes = segmentedRoutes

	return res, nil
}

func (sc *StackContainer) generateRouteGroup(segment bool) (
	*rgv1.RouteGroup,
	error,
) {
	if !sc.HasBackendPort() || sc.routeGroupSpec == nil {
		return nil, nil
	}

	hostnames, err := sc.stackHostnames(sc.routeGroupSpec, segment)
	if err != nil {
		return nil, err
	}
	if len(hostnames) == 0 {
		return nil, nil
	}

	result := &rgv1.RouteGroup{
		ObjectMeta: sc.objectMeta(segment),
		Spec: rgv1.RouteGroupSpec{
			Hosts: hostnames,
			Backends: []rgv1.RouteGroupBackend{
				{
					Name:        sc.Name(),
					Type:        rgv1.ServiceRouteGroupBackend,
					ServiceName: sc.Name(),
					ServicePort: sc.backendPort.IntValue(),
					Algorithm:   sc.routeGroupSpec.LBAlgorithm,
				},
			},
			DefaultBackends: []rgv1.RouteGroupBackendReference{
				{
					BackendName: sc.Name(),
					Weight:      100,
				},
			},
		},
	}

	result.Spec.Routes = sc.routeGroupSpec.Routes

	// validate not overlapping with main backend
	for _, backend := range sc.routeGroupSpec.AdditionalBackends {
		if backend.Name == sc.Name() {
			return nil, fmt.Errorf("invalid additionalBackend '%s', overlaps with Stack name", backend.Name)
		}
		if backend.ServiceName == sc.Name() {
			return nil, fmt.Errorf("invalid additionalBackend '%s', serviceName '%s' overlaps with Stack name", backend.Name, backend.ServiceName)
		}
		result.Spec.Backends = append(result.Spec.Backends, backend)
	}

	// sort backends to ensure have a consistent generated RouteGroup resource
	sort.Slice(result.Spec.Backends, func(i, j int) bool {
		return result.Spec.Backends[i].Name < result.Spec.Backends[j].Name
	})

	// insert annotations
	result.Annotations = mergeLabels(
		result.Annotations,
		sc.routeGroupSpec.GetAnnotations(),
	)

	return result, nil
}

func (sc *StackContainer) trafficSegment() string {
	return fmt.Sprintf(
		segmentString,
		sc.segmentLowerLimit,
		sc.segmentUpperLimit,
	)
}

func (sc *StackContainer) UpdateObjectMeta(objMeta *metav1.ObjectMeta) *metav1.ObjectMeta {
	metaObj := sc.resourceMeta()
	objMeta.OwnerReferences = metaObj.OwnerReferences
	objMeta.Labels = mergeLabels(metaObj.Labels, objMeta.Labels)
	objMeta.Annotations = mergeLabels(metaObj.Annotations, objMeta.Annotations)

	return objMeta
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

func (sc *StackContainer) GeneratePlatformCredentialsSet(pcs *zv1.PCS) (*zv1.PlatformCredentialsSet, error) {
	if pcs.Tokens == nil {
		return nil, fmt.Errorf("platformCredentialsSet has no tokens")
	}

	metaObj := sc.resourceMeta()
	if _, ok := sc.Stack.Labels["application"]; !ok {
		return nil, fmt.Errorf("stack has no label application")
	}
	metaObj.Name = pcs.Name

	result := &zv1.PlatformCredentialsSet{
		ObjectMeta: metaObj,
		TypeMeta: metav1.TypeMeta{
			Kind:       "PlatformCredentialsSet",
			APIVersion: "zalando.org/v1",
		},
		Spec: zv1.PlatformCredentialsSpec{
			Application:  metaObj.Labels["application"],
			TokenVersion: "v2",
			Tokens:       pcs.Tokens,
		},
	}

	return result, nil
}
