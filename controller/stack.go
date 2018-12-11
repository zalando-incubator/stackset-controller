package controller

import (
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	log "github.com/sirupsen/logrus"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/core/v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kube_record "k8s.io/client-go/tools/record"
)

const (
	noTrafficSinceAnnotationKey = "stacksetstacks.zalando.org/no-traffic-since"
)

// stacksReconciler is able to bring a set of Stacks of a StackSet to the
// desired state. This includes managing, Deployment, Service and HPA resources
// of the Stacks.
type stacksReconciler struct {
	logger   *log.Entry
	recorder kube_record.EventRecorder
	client   clientset.Interface
}

// ReconcileStacks brings a set of Stacks of a StackSet to the desired state.
func (c *StackSetController) ReconcileStacks(ssc StackSetContainer) error {
	sr := &stacksReconciler{
		logger: c.logger.WithFields(
			log.Fields{
				"controller": "stacks",
				"stackset":   ssc.StackSet.Name,
				"namespace":  ssc.StackSet.Namespace,
			},
		),
		client:   c.client,
		recorder: c.recorder,
	}
	return sr.reconcile(ssc)
}

func (c *stacksReconciler) reconcile(ssc StackSetContainer) error {
	for _, sc := range ssc.StackContainers {
		err := c.manageStack(*sc, ssc)
		if err != nil {
			log.Errorf("Failed to manage Stack %s/%s: %v", sc.Stack.Namespace, sc.Stack.Name, err)
			continue
		}
	}

	return nil
}

// manageStack manages the stack by managing the related Deployment and Service
// resources.
func (c *stacksReconciler) manageStack(sc StackContainer, ssc StackSetContainer) error {
	err := c.manageDeployment(sc, ssc)
	if err != nil {
		return err
	}

	return nil
}

// manageDeployment manages the deployment owned by the stack.
func (c *stacksReconciler) manageDeployment(sc StackContainer, ssc StackSetContainer) error {
	deployment := sc.Resources.Deployment
	stack := sc.Stack

	var origDeployment *appsv1.Deployment
	if deployment != nil {
		origDeployment = deployment.DeepCopy()
	}

	template := templateInjectLabels(stack.Spec.PodTemplate, stack.Labels)

	// apply default pod template spec values into the stack template.
	// This is sort of a hack to make sure the bare template from the stack
	// resource represents the defaults so we can compare changes later on
	// when deciding if the deployment resource should be updated or not.
	// TODO: find less ugly solution.
	template = applyPodTemplateSpecDefaults(template)

	createDeployment := false

	if deployment == nil {
		createDeployment = true
		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:        stack.Name,
				Namespace:   stack.Namespace,
				Annotations: map[string]string{},
				Labels:      map[string]string{},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: stack.APIVersion,
						Kind:       stack.Kind,
						Name:       stack.Name,
						UID:        stack.UID,
					},
				},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: stack.Labels,
				},
				Replicas: stack.Spec.Replicas,
			},
		}
	}

	for k, v := range stack.Labels {
		deployment.Labels[k] = v
	}

	deployment.Spec.Template = template

	// if autoscaling is disabled or if autoscaling is enabled
	// check if we need to explicitly set replicas on the deployment. There
	// are two cases:
	// 1. Autoscaling is disabled and we should rely on the replicas set on
	//    the stack
	// 2. Autoscaling is enabled, but current replica count is 0. In this
	//    case we have to set a value > 0 otherwise the autoscaler won't do
	//    anything.
	if stack.Spec.HorizontalPodAutoscaler == nil ||
		(deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 0) {
		deployment.Spec.Replicas = stack.Spec.Replicas
	}

	// If the deployment is scaled down by the downscaler then scale it back up again
	if stack.Spec.Replicas != nil && *stack.Spec.Replicas != 0 {
		if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas == 0 {
			replicas := int32(*stack.Spec.Replicas)
			deployment.Spec.Replicas = &replicas
		}
	}

	// The deployment has to be scaled down because the stack has been scaled down then set the replica count
	if stack.Spec.Replicas != nil && *stack.Spec.Replicas == 0 {
		if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 0 {
			replicas := int32(0)
			deployment.Spec.Replicas = &replicas
		}
	}

	currentStackName := generateStackName(ssc.StackSet, currentStackVersion(ssc.StackSet))
	stackUnused := stack.Name != currentStackName && ssc.Traffic != nil && ssc.Traffic[stack.Name].Weight() <= 0

	// Avoid downscaling the current stack
	if stackUnused {
		if ttl, ok := deployment.Annotations[noTrafficSinceAnnotationKey]; ok {
			noTrafficSince, err := time.Parse(time.RFC3339, ttl)
			if err != nil {
				return fmt.Errorf("failed to parse no-traffic-since timestamp '%s': %v", ttl, err)
			}

			if !noTrafficSince.IsZero() && time.Since(noTrafficSince) > ssc.ScaledownTTL() {
				replicas := int32(0)
				deployment.Spec.Replicas = &replicas
			}
		} else {
			deployment.Annotations[noTrafficSinceAnnotationKey] = time.Now().UTC().Format(time.RFC3339)
		}
	} else {
		// ensure the scaledown annotation is removed if the stack has
		// traffic.
		if _, ok := deployment.Annotations[noTrafficSinceAnnotationKey]; ok {
			delete(deployment.Annotations, noTrafficSinceAnnotationKey)
		}
	}

	err := ssc.TrafficReconciler.ReconcileDeployment(ssc.StackContainers, &stack, ssc.Traffic, deployment)
	if err != nil {
		return err
	}

	if createDeployment {
		c.recorder.Eventf(&stack,
			apiv1.EventTypeNormal,
			"CreateDeployment",
			"Creating Deployment %s/%s for StackSet stack %s/%s",
			deployment.Namespace,
			deployment.Name,
			stack.Namespace,
			stack.Name,
		)
		deployment, err = c.client.AppsV1().Deployments(deployment.Namespace).Create(deployment)
		if err != nil {
			return err
		}
	} else {
		// only update the resource if there are changes
		// TODO: still if we add just the annotation it could mess with
		// the HPA.
		if !equality.Semantic.DeepEqual(origDeployment, deployment) {
			c.logger.Debugf("Deployment %s/%s changed: %s",
				deployment.Namespace, deployment.Name,
				cmp.Diff(
					origDeployment,
					deployment,
					cmpopts.IgnoreUnexported(resource.Quantity{}),
				),
			)
			// depends on https://github.com/zalando-incubator/stackset-controller/issues/49
			// c.recorder.Eventf(&stack,
			// 	apiv1.EventTypeNormal,
			// 	"UpdateDeployment",
			// 	"Updating Deployment %s/%s for StackSet stack %s/%s",
			// 	deployment.Namespace,
			// 	deployment.Name,
			// 	stack.Namespace,
			// 	stack.Name,
			// )
			deployment, err = c.client.AppsV1().Deployments(deployment.Namespace).Update(deployment)
			if err != nil {
				return err
			}
		}
	}

	// set TypeMeta manually because of this bug:
	// https://github.com/kubernetes/client-go/issues/308
	deployment.APIVersion = "apps/v1"
	deployment.Kind = "Deployment"

	hpa, err := c.manageAutoscaling(sc, deployment, ssc, stackUnused)
	if err != nil {
		return err
	}

	err = c.manageService(sc, deployment, ssc)
	if err != nil {
		return err
	}

	// update stack status
	newStatus := zv1.StackStatus{
		Replicas:        deployment.Status.Replicas,
		ReadyReplicas:   deployment.Status.ReadyReplicas,
		UpdatedReplicas: deployment.Status.UpdatedReplicas,
	}

	if ssc.Traffic != nil {
		newStatus.ActualTrafficWeight = ssc.Traffic[stack.Name].ActualWeight
		newStatus.DesiredTrafficWeight = ssc.Traffic[stack.Name].DesiredWeight
	}

	if hpa != nil {
		newStatus.DesiredReplicas = hpa.Status.DesiredReplicas
	}

	if !equality.Semantic.DeepEqual(newStatus, stack.Status) {
		c.recorder.Eventf(&stack,
			apiv1.EventTypeNormal,
			"UpdateStackStatus",
			"Status changed for Stack %s/%s: %#v -> %#v",
			stack.Namespace,
			stack.Name,
			stack.Status,
			newStatus,
		)
		stack.Status = newStatus

		// update status of stack
		_, err = c.client.ZalandoV1().Stacks(stack.Namespace).UpdateStatus(&stack)
		if err != nil {
			return err
		}
	}

	return nil
}

// manageAutoscaling manages the HPA defined for the stack.
func (c *stacksReconciler) manageAutoscaling(sc StackContainer, deployment *appsv1.Deployment, ssc StackSetContainer, stackUnused bool) (*autoscaling.HorizontalPodAutoscaler, error) {
	hpa := sc.Resources.HPA
	stack := sc.Stack

	var origHPA *autoscaling.HorizontalPodAutoscaler
	if hpa != nil {
		hpa.Status = autoscaling.HorizontalPodAutoscalerStatus{}
		origHPA = hpa.DeepCopy()
	}

	// cleanup HPA if autoscaling is disabled or the stack has 0 traffic.
	if stack.Spec.HorizontalPodAutoscaler == nil || stackUnused {
		if hpa != nil {
			c.recorder.Eventf(&stack,
				apiv1.EventTypeNormal,
				"DeleteHPA",
				"Deleting obsolete HPA %s/%s for Deployment %s/%s",
				hpa.Namespace,
				hpa.Name,
				deployment.Namespace,
				deployment.Name,
			)
			return nil, c.client.AutoscalingV2beta1().HorizontalPodAutoscalers(hpa.Namespace).Delete(hpa.Name, nil)
		}
		return nil, nil
	}

	createHPA := false

	if hpa == nil {
		createHPA = true
		hpa = &autoscaling.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:        deployment.Name,
				Namespace:   deployment.Namespace,
				Annotations: stack.Spec.HorizontalPodAutoscaler.Annotations,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: deployment.APIVersion,
						Kind:       deployment.Kind,
						Name:       deployment.Name,
						UID:        deployment.UID,
					},
				},
			},
			Spec: autoscaling.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscaling.CrossVersionObjectReference{
					APIVersion: deployment.APIVersion,
					Kind:       deployment.Kind,
					Name:       deployment.Name,
				},
			},
		}
	}

	if hpa.Annotations == nil {
		hpa.Annotations = map[string]string{}
	}

	hpa.Labels = deployment.Labels
	hpa.Spec.Metrics = stack.Spec.HorizontalPodAutoscaler.Metrics

	err := ssc.TrafficReconciler.ReconcileHPA(&stack, hpa, deployment)
	if err != nil {
		return nil, err
	}

	if createHPA {
		c.recorder.Eventf(&stack,
			apiv1.EventTypeNormal,
			"CreateHPA",
			"Creating HPA %s/%s for Deployment %s/%s",
			hpa.Namespace,
			hpa.Name,
			deployment.Namespace,
			deployment.Name,
		)
		_, err := c.client.AutoscalingV2beta1().HorizontalPodAutoscalers(hpa.Namespace).Create(hpa)
		if err != nil {
			return nil, err
		}
	} else {
		if !equality.Semantic.DeepEqual(origHPA, hpa) {
			c.logger.Debugf("HPA %s/%s changed: %s",
				hpa.Namespace, hpa.Name,
				cmp.Diff(
					origHPA,
					hpa,
					cmpopts.IgnoreUnexported(resource.Quantity{}),
				),
			)
			c.recorder.Eventf(&stack,
				apiv1.EventTypeNormal,
				"UpdateHPA",
				"Updating HPA %s/%s for Deployment %s/%s",
				hpa.Namespace,
				hpa.Name,
				deployment.Namespace,
				deployment.Name,
			)
			hpa, err = c.client.AutoscalingV2beta1().HorizontalPodAutoscalers(hpa.Namespace).Update(hpa)
			if err != nil {
				return nil, err
			}
		}
	}

	return hpa, nil
}

// manageService manages the service for a given stack.
func (c *stacksReconciler) manageService(sc StackContainer, deployment *appsv1.Deployment, ssc StackSetContainer) error {
	service := sc.Resources.Service
	stack := sc.Stack

	var origService *v1.Service
	if service != nil {
		origService = service.DeepCopy()
	}

	createService := false

	if service == nil {
		createService = true
		service = &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      stack.Name,
				Namespace: stack.Namespace,
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
				Type: v1.ServiceTypeClusterIP,
			},
		}
	}

	service.Labels = stack.Labels
	service.Spec.Selector = stack.Labels

	// get service ports to be used for the service
	var backendPort *intstr.IntOrString
	if ssc.StackSet.Spec.Ingress != nil {
		backendPort = &ssc.StackSet.Spec.Ingress.BackendPort
	}

	servicePorts, err := getServicePorts(backendPort, stack)
	if err != nil {
		return err
	}
	service.Spec.Ports = servicePorts

	if createService {
		c.recorder.Eventf(&stack,
			apiv1.EventTypeNormal,
			"CreateService",
			"Creating Service %s/%s for StackSet stack %s/%s",
			service.Namespace,
			service.Name,
			stack.Namespace,
			stack.Name,
		)
		_, err := c.client.CoreV1().Services(service.Namespace).Create(service)
		if err != nil {
			return err
		}
	} else {
		if !equality.Semantic.DeepEqual(origService, service) {
			c.logger.Debugf("Service %s/%s changed: %s", service.Namespace, service.Name, cmp.Diff(origService, service))
			c.recorder.Eventf(&stack,
				apiv1.EventTypeNormal,
				"UpdateService",
				"Updating Service %s/%s for StackSet stack %s/%s",
				service.Namespace,
				service.Name,
				stack.Namespace,
				stack.Name,
			)
			_, err := c.client.CoreV1().Services(service.Namespace).Update(service)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// getServicePorts gets the service ports to be used for the stack service.
func getServicePorts(backendPort *intstr.IntOrString, stack zv1.Stack) ([]v1.ServicePort, error) {
	var servicePorts []v1.ServicePort
	if stack.Spec.Service == nil || len(stack.Spec.Service.Ports) == 0 {
		servicePorts = servicePortsFromContainers(stack.Spec.PodTemplate.Spec.Containers)
	} else {
		servicePorts = stack.Spec.Service.Ports
	}

	// validate that one port in the list map to the backendPort.
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
	for _, container := range containers {
		for i, port := range container.Ports {
			name := fmt.Sprintf("port-%d", i)
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

// templateInjectLabels injects labels into a pod template spec.
func templateInjectLabels(template v1.PodTemplateSpec, labels map[string]string) v1.PodTemplateSpec {
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

// applyPodTemplateSpecDefaults inject default values into a pod template spec.
func applyPodTemplateSpecDefaults(template v1.PodTemplateSpec) v1.PodTemplateSpec {
	newTemplate := template.DeepCopy()

	applyContainersDefaults(newTemplate.Spec.InitContainers)
	applyContainersDefaults(newTemplate.Spec.Containers)

	if newTemplate.Spec.RestartPolicy == "" {
		newTemplate.Spec.RestartPolicy = v1.RestartPolicyAlways
	}
	if newTemplate.Spec.TerminationGracePeriodSeconds == nil {
		gracePeriod := int64(v1.DefaultTerminationGracePeriodSeconds)
		newTemplate.Spec.TerminationGracePeriodSeconds = &gracePeriod
	}
	if newTemplate.Spec.DNSPolicy == "" {
		newTemplate.Spec.DNSPolicy = v1.DNSClusterFirst
	}
	if newTemplate.Spec.SecurityContext == nil {
		newTemplate.Spec.SecurityContext = &v1.PodSecurityContext{}
	}
	if newTemplate.Spec.SchedulerName == "" {
		newTemplate.Spec.SchedulerName = v1.DefaultSchedulerName
	}
	if newTemplate.Spec.DeprecatedServiceAccount != newTemplate.Spec.ServiceAccountName {
		newTemplate.Spec.DeprecatedServiceAccount = newTemplate.Spec.ServiceAccountName
	}
	return *newTemplate
}

func applyContainersDefaults(containers []v1.Container) {
	for i, container := range containers {
		for j, port := range container.Ports {
			if port.Protocol == "" {
				containers[i].Ports[j].Protocol = v1.ProtocolTCP
			}
		}

		for j, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.FieldRef != nil && env.ValueFrom.FieldRef.APIVersion == "" {
				containers[i].Env[j].ValueFrom.FieldRef.APIVersion = "v1"
			}
		}
		if container.TerminationMessagePath == "" {
			containers[i].TerminationMessagePath = v1.TerminationMessagePathDefault
		}
		if container.TerminationMessagePolicy == "" {
			containers[i].TerminationMessagePolicy = v1.TerminationMessageReadFile
		}
		if container.ImagePullPolicy == "" {
			containers[i].ImagePullPolicy = v1.PullIfNotPresent
		}
		if container.ReadinessProbe != nil {
			if container.ReadinessProbe.Handler.HTTPGet != nil && container.ReadinessProbe.Handler.HTTPGet.Scheme == "" {
				containers[i].ReadinessProbe.Handler.HTTPGet.Scheme = v1.URISchemeHTTP
			}
			if container.ReadinessProbe.TimeoutSeconds == 0 {
				containers[i].ReadinessProbe.TimeoutSeconds = 1
			}
			if container.ReadinessProbe.PeriodSeconds == 0 {
				containers[i].ReadinessProbe.PeriodSeconds = 10
			}
			if container.ReadinessProbe.SuccessThreshold == 0 {
				containers[i].ReadinessProbe.SuccessThreshold = 1
			}
			if container.ReadinessProbe.FailureThreshold == 0 {
				containers[i].ReadinessProbe.FailureThreshold = 3
			}
		}
		if container.LivenessProbe != nil {
			if container.LivenessProbe.Handler.HTTPGet != nil && container.LivenessProbe.Handler.HTTPGet.Scheme == "" {
				containers[i].LivenessProbe.Handler.HTTPGet.Scheme = v1.URISchemeHTTP
			}
			if container.LivenessProbe.TimeoutSeconds == 0 {
				containers[i].LivenessProbe.TimeoutSeconds = 1
			}
			if container.LivenessProbe.PeriodSeconds == 0 {
				containers[i].LivenessProbe.PeriodSeconds = 10
			}
			if container.LivenessProbe.SuccessThreshold == 0 {
				containers[i].LivenessProbe.SuccessThreshold = 1
			}
			if container.LivenessProbe.FailureThreshold == 0 {
				containers[i].LivenessProbe.FailureThreshold = 3
			}
		}
	}
}
