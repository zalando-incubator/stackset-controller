package core

import (
	"encoding/json"
	"errors"
	"sort"

	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	StacksetHeritageLabelKey = "stackset"
	StackVersionLabelKey     = "stack-version"

	ingressTrafficAuthoritativeAnnotation = "zalando.org/traffic-authoritative"
	forwardBackendAnnotation              = "zalando.org/forward-backend"
	forwardBackendName                    = "fwd"
)

var (
	errNoPaths             = errors.New("invalid ingress, no paths defined")
	errNoStacks            = errors.New("no stacks to assign traffic to")
	errStackServiceBackend = errors.New("additionalBackends must not reference a Stack Service")
)

func currentStackVersion(stackset *zv1.StackSet) string {
	version := stackset.Spec.StackTemplate.Spec.Version
	if version == "" {
		version = defaultVersion
	}
	return version
}

func generateStackName(stackset *zv1.StackSet, version string) string {
	return stackset.Name + "-" + version
}

// sanitizeServicePorts makes sure the ports has the default fields set if not
// specified.
func sanitizeServicePorts(service *zv1.StackServiceSpec) *zv1.StackServiceSpec {
	for i, port := range service.Ports {
		// set default protocol if not specified
		if port.Protocol == "" {
			port.Protocol = corev1.ProtocolTCP
		}
		service.Ports[i] = port
	}
	return service
}

// NewStack returns an (optional) stack that should be created
func (ssc *StackSetContainer) NewStack() (*StackContainer, string) {
	_, forwardMigration := ssc.StackSet.ObjectMeta.Annotations[forwardBackendAnnotation]
	observedStackVersion := ssc.StackSet.Status.ObservedStackVersion
	stackVersion := currentStackVersion(ssc.StackSet)
	stackName := generateStackName(ssc.StackSet, stackVersion)
	stack := ssc.stackByName(stackName)

	// If the current stack doesn't exist, check that we haven't created it
	// before. We shouldn't recreate it if it was removed for any reason.
	if stack == nil && observedStackVersion != stackVersion {
		spec := &zv1.StackSpecInternal{}

		parentSpec := ssc.StackSet.Spec.StackTemplate.Spec.StackSpec.DeepCopy()
		if parentSpec.Service != nil {
			parentSpec.Service = sanitizeServicePorts(parentSpec.Service)
		}
		spec.StackSpec = *parentSpec

		if ssc.StackSet.Spec.Ingress != nil {
			spec.Ingress = ssc.StackSet.Spec.Ingress.DeepCopy()
		}

		if ssc.StackSet.Spec.ExternalIngress != nil {
			spec.ExternalIngress = ssc.StackSet.Spec.ExternalIngress.DeepCopy()
		}

		if ssc.StackSet.Spec.RouteGroup != nil {
			spec.RouteGroup = ssc.StackSet.Spec.RouteGroup.DeepCopy()
		}

		stackAnnotations := make(map[string]string)
		if a := ssc.StackSet.Spec.StackTemplate.Annotations; a != nil {
			stackAnnotations = a
		}

		if forwardMigration {
			stackAnnotations[forwardBackendAnnotation] = forwardBackendName
		}
		if len(stackAnnotations) == 0 {
			stackAnnotations = nil
		}

		return &StackContainer{
			Stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      stackName,
					Namespace: ssc.StackSet.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: ssc.StackSet.APIVersion,
							Kind:       ssc.StackSet.Kind,
							Name:       ssc.StackSet.Name,
							UID:        ssc.StackSet.UID,
						},
					},
					Labels: mergeLabels(
						map[string]string{
							StacksetHeritageLabelKey: ssc.StackSet.Name,
						},
						ssc.StackSet.Labels,
						map[string]string{StackVersionLabelKey: stackVersion},
					),
					Annotations: stackAnnotations,
				},
				Spec: *spec,
			},
		}, stackVersion
	}

	return nil, ""
}

// MarkExpiredStacks marks stacks that should be deleted
func (ssc *StackSetContainer) MarkExpiredStacks() {
	historyLimit := defaultStackLifecycleLimit
	if ssc.StackSet.Spec.StackLifecycle.Limit != nil {
		historyLimit = int(*ssc.StackSet.Spec.StackLifecycle.Limit)
	}

	gcCandidates := make([]*StackContainer, 0, len(ssc.StackContainers))

	for _, sc := range ssc.StackContainers {
		// Stacks are considered for cleanup if we don't have RouteGroup nor an ingress or if the stack is scaled down because of inactivity
		hasIngress := sc.routeGroupSpec != nil || sc.ingressSpec != nil || ssc.StackSet.Spec.ExternalIngress != nil
		if !hasIngress || sc.ScaledDown() {
			gcCandidates = append(gcCandidates, sc)
		}
	}

	// only garbage collect if history limit is reached
	if len(gcCandidates) <= historyLimit {
		return
	}

	// sort candidates by when they last had traffic.
	sort.Slice(gcCandidates, func(i, j int) bool {
		// First check if NoTrafficSince is set. If not, fall back to the creation timestamp
		iTime := gcCandidates[i].noTrafficSince
		if iTime.IsZero() {
			iTime = gcCandidates[i].Stack.CreationTimestamp.Time
		}

		jTime := gcCandidates[j].noTrafficSince
		if jTime.IsZero() {
			jTime = gcCandidates[j].Stack.CreationTimestamp.Time
		}
		return iTime.Before(jTime)
	})

	excessStacks := len(gcCandidates) - historyLimit
	for _, sc := range gcCandidates[:excessStacks] {
		sc.PendingRemoval = true
	}
}

func (ssc *StackSetContainer) GenerateRouteGroup() (*rgv1.RouteGroup, error) {
	stackset := ssc.StackSet
	if stackset.Spec.RouteGroup == nil {
		return nil, nil
	}

	labels := mergeLabels(
		map[string]string{StacksetHeritageLabelKey: stackset.Name},
		stackset.Labels,
	)

	result := &rgv1.RouteGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:        stackset.Name,
			Namespace:   stackset.Namespace,
			Labels:      labels,
			Annotations: stackset.Spec.RouteGroup.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: stackset.APIVersion,
					Kind:       stackset.Kind,
					Name:       stackset.Name,
					UID:        stackset.UID,
				},
			},
		},
		Spec: rgv1.RouteGroupSpec{
			Hosts:    stackset.Spec.RouteGroup.Hosts,
			Backends: make([]rgv1.RouteGroupBackend, 0, len(ssc.StackContainers)),
			Routes:   stackset.Spec.RouteGroup.Routes,
		},
	}

	// Generate backends
	stacks := make(map[string]struct{}, len(ssc.StackContainers))
	for _, sc := range ssc.StackContainers {
		stacks[sc.Name()] = struct{}{}
		result.Spec.Backends = append(result.Spec.Backends, rgv1.RouteGroupBackend{
			Name:        sc.Name(),
			Type:        rgv1.ServiceRouteGroupBackend,
			ServiceName: sc.Name(),
			ServicePort: stackset.Spec.RouteGroup.BackendPort,
			Algorithm:   stackset.Spec.RouteGroup.LBAlgorithm,
		})
		if sc.actualTrafficWeight > 0 {
			result.Spec.DefaultBackends = append(result.Spec.DefaultBackends, rgv1.RouteGroupBackendReference{
				BackendName: sc.Name(),
				Weight:      int(sc.actualTrafficWeight),
			})
		}
	}

	// validate that additional backends don't overlap with the generated
	// backends.
	for _, additionalBackend := range stackset.Spec.RouteGroup.AdditionalBackends {
		if _, ok := stacks[additionalBackend.Name]; ok {
			return nil, errStackServiceBackend
		}
		if _, ok := stacks[additionalBackend.ServiceName]; ok {
			return nil, errStackServiceBackend
		}
		result.Spec.Backends = append(result.Spec.Backends, additionalBackend)
	}

	// sort backends/defaultBackends to ensure have a consistent generated RoutGroup resource
	sort.Slice(result.Spec.Backends, func(i, j int) bool {
		return result.Spec.Backends[i].Name < result.Spec.Backends[j].Name
	})
	sort.Slice(result.Spec.DefaultBackends, func(i, j int) bool {
		return result.Spec.DefaultBackends[i].BackendName < result.Spec.DefaultBackends[j].BackendName
	})

	return result, nil
}

func (ssc *StackSetContainer) GenerateIngress() (*networking.Ingress, error) {
	stackset := ssc.StackSet
	if stackset.Spec.Ingress == nil {
		return nil, nil
	}

	labels := mergeLabels(
		map[string]string{StacksetHeritageLabelKey: stackset.Name},
		stackset.Labels,
	)

	trafficAuthoritative := map[string]string{
		ingressTrafficAuthoritativeAnnotation: "false",
	}

	result := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        stackset.Name,
			Namespace:   stackset.Namespace,
			Labels:      labels,
			Annotations: mergeLabels(stackset.Spec.Ingress.Annotations, trafficAuthoritative),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: stackset.APIVersion,
					Kind:       stackset.Kind,
					Name:       stackset.Name,
					UID:        stackset.UID,
				},
			},
		},
		Spec: networking.IngressSpec{
			Rules: make([]networking.IngressRule, 0),
		},
	}

	rule := networking.IngressRule{
		IngressRuleValue: networking.IngressRuleValue{
			HTTP: &networking.HTTPIngressRuleValue{
				Paths: make([]networking.HTTPIngressPath, 0),
			},
		},
	}

	actualWeights := make(map[string]float64)

	for _, sc := range ssc.StackContainers {
		if sc.actualTrafficWeight > 0 {
			actualWeights[sc.Name()] = sc.actualTrafficWeight

			rule.IngressRuleValue.HTTP.Paths = append(rule.IngressRuleValue.HTTP.Paths, networking.HTTPIngressPath{
				Path:     stackset.Spec.Ingress.Path,
				PathType: &PathTypeImplementationSpecific,
				Backend: networking.IngressBackend{
					Service: &networking.IngressServiceBackend{
						Name: sc.Name(),
						Port: networking.ServiceBackendPort{
							Number: stackset.Spec.Ingress.BackendPort.IntVal,
							Name:   stackset.Spec.Ingress.BackendPort.StrVal,
						},
					},
				},
			})
		}
	}

	if len(rule.IngressRuleValue.HTTP.Paths) == 0 {
		return nil, errNoPaths
	}

	// sort backends by name to have a consistent generated ingress resource.
	sort.Slice(rule.IngressRuleValue.HTTP.Paths, func(i, j int) bool {
		return rule.IngressRuleValue.HTTP.Paths[i].Backend.Service.Name < rule.IngressRuleValue.HTTP.Paths[j].Backend.Service.Name
	})

	// create rule per hostname
	for _, host := range stackset.Spec.Ingress.Hosts {
		r := rule
		r.Host = host
		result.Spec.Rules = append(result.Spec.Rules, r)
	}

	// sort rules by host to have a consistent generated ingress resource.
	sort.Slice(result.Spec.Rules, func(i, j int) bool {
		return result.Spec.Rules[i].Host < result.Spec.Rules[j].Host
	})

	actualWeightsData, err := json.Marshal(&actualWeights)
	if err != nil {
		return nil, err
	}

	result.Annotations[ssc.backendWeightsAnnotationKey] = string(actualWeightsData)
	return result, nil
}

func (ssc *StackSetContainer) GenerateStackSetStatus() *zv1.StackSetStatus {
	result := &zv1.StackSetStatus{
		Stacks:               0,
		ReadyStacks:          0,
		StacksWithTraffic:    0,
		ObservedStackVersion: ssc.StackSet.Status.ObservedStackVersion,
	}
	var traffic []*zv1.ActualTraffic

	for _, sc := range ssc.StackContainers {
		if sc.PendingRemoval {
			continue
		}
		if sc.HasBackendPort() {
			t := &zv1.ActualTraffic{
				StackName:   sc.Name(),
				ServiceName: sc.Name(),
				ServicePort: *sc.backendPort,
				Weight:      sc.actualTrafficWeight,
			}
			traffic = append(traffic, t)
		}

		result.Stacks += 1
		if sc.HasTraffic() {
			result.StacksWithTraffic += 1
		}
		if sc.IsReady() {
			result.ReadyStacks += 1
		}
	}
	sort.Slice(traffic, func(i, j int) bool {
		return traffic[i].StackName < traffic[j].StackName
	})
	result.Traffic = traffic
	return result
}

func (ssc *StackSetContainer) GenerateStackSetTraffic() []*zv1.DesiredTraffic {
	var traffic []*zv1.DesiredTraffic
	for _, sc := range ssc.StackContainers {
		if sc.PendingRemoval {
			continue
		}
		if sc.HasBackendPort() && sc.desiredTrafficWeight > 0 {
			t := &zv1.DesiredTraffic{
				StackName: sc.Name(),
				Weight:    sc.desiredTrafficWeight,
			}
			traffic = append(traffic, t)
		}
	}
	sort.Slice(traffic, func(i, j int) bool {
		return traffic[i].StackName < traffic[j].StackName
	})
	return traffic
}
