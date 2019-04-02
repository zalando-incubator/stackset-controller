package controller

import (
	"fmt"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kube_record "k8s.io/client-go/tools/record"
)

const (
	stackGenerationAnnotationKey = "stackset-controller.zalando.org/stack-generation"
)

var (
	// set implementation with 0 Byte value
	selectorLabels = map[string]struct{}{
		stacksetHeritageLabelKey: struct{}{},
		stackVersionLabelKey:     struct{}{},
	}
)

// stacksReconciler is able to bring a set of Stacks of a StackSet to the
// desired state. This includes managing, Deployment, Service and HPA resources
// of the Stacks.
type stacksReconciler struct {
	logger               *log.Entry
	recorder             kube_record.EventRecorder
	client               clientset.Interface
	autoscalerReconciler *AutoscalerReconciler
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
		client:               c.client,
		recorder:             c.recorder,
		autoscalerReconciler: NewAutoscalerReconciler(ssc),
	}
	return sr.reconcile(ssc)
}

func (c *stacksReconciler) reconcile(ssc StackSetContainer) error {
	for _, sc := range ssc.StackContainers {
		err := c.autoscalerReconciler.Reconcile(sc)
		if err != nil {
			c.recorder.Event(&ssc.StackSet, v1.EventTypeWarning, "GenerateHPA",
				fmt.Sprintf("Failed to generate HPA %v", err.Error()))
			continue
		}
		err = c.manageStack(*sc, ssc)
		if err != nil {
			c.recorder.Event(&sc.Stack, v1.EventTypeWarning, "ManageStackFailed",
				fmt.Sprintf("Failed to reconcile stack: %v", err.Error()))
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

	origReplicas := int32(0)
	createDeployment := false

	if deployment == nil {
		createDeployment = true
		deployment = NewDeploymentFromStack(stack)
	} else {
		origReplicas = *deployment.Spec.Replicas
		template := templateInjectLabels(stack.Spec.PodTemplate, stack.Labels)
		deployment.Spec.Template = template
		for k, v := range stack.Labels {
			deployment.Labels[k] = v
		}
	}

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

	noTrafficSince := stack.Status.NoTrafficSince

	// Avoid downscaling the current stack
	if stackUnused {
		if noTrafficSince != nil {
			if !noTrafficSince.IsZero() && time.Since(noTrafficSince.Time) > ssc.ScaledownTTL() {
				replicas := int32(0)
				deployment.Spec.Replicas = &replicas
			}
		} else {
			noTrafficSince = &metav1.Time{Time: time.Now().UTC()}
		}
	} else {
		// ensure noTrafficSince status is not set
		noTrafficSince = nil
	}

	err := ssc.TrafficReconciler.ReconcileDeployment(ssc.StackContainers, &stack, ssc.Traffic, deployment)
	if err != nil {
		return err
	}

	if createDeployment {
		c.recorder.Eventf(&stack,
			apiv1.EventTypeNormal,
			"CreateDeployment",
			"Creating Deployment '%s/%s' for Stack",
			deployment.Namespace,
			deployment.Name,
		)
		deployment, err = c.client.AppsV1().Deployments(deployment.Namespace).Create(deployment)
		if err != nil {
			return err
		}
	} else {
		stackGeneration := getStackGeneration(deployment.ObjectMeta)

		replicas := *deployment.Spec.Replicas

		// only update the resource if there are changes
		// We determine changes by comparing the stackGeneration
		// (observed generation) stored on the deployment with the
		// generation of the Stack.
		// Since replicas are modified independently of the replicas
		// defined on the stack we need to also check if those get
		// changed.
		// TODO: still if we add just the annotation it could mess with
		// the HPA.
		if stackGeneration != stack.Generation || origReplicas != replicas {
			if deployment.Annotations == nil {
				deployment.Annotations = make(map[string]string, 1)
			}
			deployment.Annotations[stackGenerationAnnotationKey] = fmt.Sprintf("%d", stack.Generation)
			c.recorder.Eventf(&stack,
				apiv1.EventTypeNormal,
				"UpdateDeployment",
				"Updating Deployment '%s/%s' for Stack",
				deployment.Namespace,
				deployment.Name,
			)
			deployment, err = c.client.AppsV1().Deployments(deployment.Namespace).Update(deployment)
			if err != nil {
				return err
			}
		}
	}

	// TODO: Add this to NewDeploymentFromStack once we figure out why it matters that
	//  it is so late compared to other deployment-related stuff (i.e. why tests fails
	//  when we put it in this function's first else branch).
	// set TypeMeta manually because of this bug:
	// https://github.com/kubernetes/client-go/issues/308
	deployment.APIVersion = "apps/v1"
	deployment.Kind = "Deployment"

	hpa, err := c.manageAutoscaling(stack, sc.Resources.HPA, deployment, ssc, stackUnused)
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
		NoTrafficSince:  noTrafficSince,
	}

	newStatus.Prescaling = stack.Status.Prescaling
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
			"Status changed: %#v -> %#v",
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
func (c *stacksReconciler) manageAutoscaling(stack zv1.Stack, hpa *autoscalingv2.HorizontalPodAutoscaler, deployment *appsv1.Deployment, ssc StackSetContainer, stackUnused bool) (*autoscaling.HorizontalPodAutoscaler, error) {
	origMinReplicas := int32(0)
	origMaxReplicas := int32(0)
	if hpa != nil {
		hpa.Status = autoscaling.HorizontalPodAutoscalerStatus{}
		origMinReplicas = *hpa.Spec.MinReplicas
		origMaxReplicas = hpa.Spec.MaxReplicas
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
			"Creating HPA '%s/%s' for Stack",
			hpa.Namespace,
			hpa.Name,
		)
		_, err := c.client.AutoscalingV2beta1().HorizontalPodAutoscalers(hpa.Namespace).Create(hpa)
		if err != nil {
			return nil, err
		}
	} else {
		stackGeneration := getStackGeneration(hpa.ObjectMeta)

		// only update the resource if there are changes
		// We determine changes by comparing the stackGeneration
		// (observed generation) stored on the hpa with the
		// generation of the Stack.
		// Since min/max replicas are modified independently of the
		// replicas defined on the stack during reconciliation, we
		// need to also check if those get changed.
		if stackGeneration != stack.Generation ||
			origMinReplicas != *hpa.Spec.MinReplicas ||
			origMaxReplicas != hpa.Spec.MaxReplicas {
			if hpa.Annotations == nil {
				hpa.Annotations = make(map[string]string, 1)
			}
			hpa.Annotations[stackGenerationAnnotationKey] = fmt.Sprintf("%d", stack.Generation)
			c.recorder.Eventf(&stack,
				apiv1.EventTypeNormal,
				"UpdateHPA",
				"Updating HPA '%s/%s' for Stack",
				hpa.Namespace,
				hpa.Name,
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
	service.Spec.Selector = limitLabels(stack.Labels, selectorLabels)

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
			"Creating Service '%s/%s' for Stack",
			service.Namespace,
			service.Name,
		)
		_, err := c.client.CoreV1().Services(service.Namespace).Create(service)
		if err != nil {
			return err
		}
	} else {
		stackGeneration := getStackGeneration(service.ObjectMeta)

		// only update the resource if there are changes
		// We determine changes by comparing the stackGeneration
		// (observed generation) stored on the service with the
		// generation of the Stack.
		if stackGeneration != stack.Generation {
			if service.Annotations == nil {
				service.Annotations = make(map[string]string, 1)
			}
			service.Annotations[stackGenerationAnnotationKey] = fmt.Sprintf("%d", stack.Generation)
			c.recorder.Eventf(&stack,
				apiv1.EventTypeNormal,
				"UpdateService",
				"Updating Service '%s/%s' for Stack",
				service.Namespace,
				service.Name,
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

func getStackGeneration(metadata metav1.ObjectMeta) int64 {
	if g, ok := metadata.Annotations[stackGenerationAnnotationKey]; ok {
		generation, err := strconv.ParseInt(g, 10, 64)
		if err != nil {
			return 0
		}
		return generation
	}
	return 0
}
