package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	log "github.com/sirupsen/logrus"
	"github.com/zalando-incubator/stackset-controller/controller/entities"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"github.com/zalando-incubator/stackset-controller/pkg/recorder"
	"golang.org/x/sync/errgroup"

	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	kube_record "k8s.io/client-go/tools/record"
)

const (
	defaultVersion             = "default"
	defaultStackLifecycleLimit = 10
	defaultScaledownTTLSeconds = int64(300)
)

// StackSetController is the main controller. It watches for changes to
// stackset resources and starts and maintains other controllers per
// stackset resource.
type StackSetController struct {
	logger         *log.Entry
	client         clientset.Interface
	controllerID   string
	interval       time.Duration
	stacksetEvents chan stacksetEvent
	stacksetStore  map[types.UID]zv1.StackSet
	recorder       kube_record.EventRecorder
	sync.Mutex
}

type stacksetEvent struct {
	Deleted  bool
	StackSet *zv1.StackSet
}

// NewStackSetController initializes a new StackSetController.
func NewStackSetController(client clientset.Interface, controllerID string, interval time.Duration) *StackSetController {

	return &StackSetController{
		logger:         log.WithFields(log.Fields{"controller": "stackset"}),
		client:         client,
		controllerID:   controllerID,
		stacksetEvents: make(chan stacksetEvent, 1),
		stacksetStore:  make(map[types.UID]zv1.StackSet),
		interval:       interval,
		recorder:       recorder.CreateEventRecorder(client),
	}
}

// Run runs the main loop of the StackSetController. Before the loops it
// sets up a watcher to watch StackSet resources. The watch will send
// changes over a channel which is polled from the main loop.
func (c *StackSetController) Run(ctx context.Context) {
	c.startWatch(ctx)

	nextCheck := time.Now().Add(-c.interval)

	for {
		select {
		case <-time.After(time.Until(nextCheck)):
			nextCheck = time.Now().Add(c.interval)

			stackContainers, err := c.collectResources()
			if err != nil {
				c.logger.Errorf("Failed to collect resources: %v", err)
				continue
			}

			var reconcileGroup errgroup.Group
			for stackset, container := range stackContainers {

				container := *container

				reconcileGroup.Go(func() error {
					if _, ok := c.stacksetStore[stackset]; ok {
						err = c.ReconcileStack(container)
						if err != nil {
							c.StackSetStatusUpdateFailed(container, err)
							return err
						}

						err := c.ReconcileStacks(container)
						if err != nil {
							c.StackSetStatusUpdateFailed(container, err)
							return err
						}

						err = c.ReconcileIngress(container)
						if err != nil {
							c.StackSetStatusUpdateFailed(container, err)
							return err
						}

						err = c.ReconcileStackSetStatus(container)
						if err != nil {
							return err
						}

						err = c.StackSetGC(container)
						if err != nil {
							return err
						}
					}
					return nil
				})
			}

			err = reconcileGroup.Wait()
			if err != nil {
				c.logger.Errorf("Failed waiting for reconcilers: %v", err)
			}
		case e := <-c.stacksetEvents:
			stackset := *e.StackSet
			// set TypeMeta manually because of this bug:
			// https://github.com/kubernetes/client-go/issues/308
			stackset.APIVersion = "zalando.org/v1"
			stackset.Kind = "StackSet"

			// set default stackset defaults
			setStackSetDefaults(&stackset)

			// update/delete existing entry
			if _, ok := c.stacksetStore[stackset.UID]; ok {
				if e.Deleted || !c.hasOwnership(&stackset) {
					delete(c.stacksetStore, stackset.UID)
					continue
				}

				// update stackset entry
				c.stacksetStore[stackset.UID] = stackset
				continue
			}

			// check if stackset should be managed by the controller
			if !c.hasOwnership(&stackset) {
				continue
			}

			c.logger.Infof("Adding entry for StackSet %s/%s", stackset.Namespace, stackset.Name)
			c.stacksetStore[stackset.UID] = stackset
		case <-ctx.Done():
			c.logger.Info("Terminating main controller loop.")
			return
		}
	}
}

// setStackSetDefaults sets default values on the stackset in case the fields
// were left empty by the user.
func setStackSetDefaults(stackset *zv1.StackSet) {
	// set default ScaledownTTLSeconds if not defined on
	// the stackset.
	if stackset.Spec.StackLifecycle.ScaledownTTLSeconds == nil {
		scaledownTTLSeconds := defaultScaledownTTLSeconds
		stackset.Spec.StackLifecycle.ScaledownTTLSeconds = &scaledownTTLSeconds
	}
}

// getIngressTraffic parses ingress traffic from an Ingress resource.
func getIngressTraffic(ingress *v1beta1.Ingress) (map[string]entities.TrafficStatus, error) {
	desiredTraffic := make(map[string]float64)
	if weights, ok := ingress.Annotations[stackTrafficWeightsAnnotationKey]; ok {
		err := json.Unmarshal([]byte(weights), &desiredTraffic)
		if err != nil {
			return nil, fmt.Errorf("failed to get current desired Stack traffic weights: %v", err)
		}
	}

	actualTraffic := make(map[string]float64)
	if weights, ok := ingress.Annotations[backendWeightsAnnotationKey]; ok {
		err := json.Unmarshal([]byte(weights), &actualTraffic)
		if err != nil {
			return nil, fmt.Errorf("failed to get current actual Stack traffic weights: %v", err)
		}
	}

	traffic := make(map[string]entities.TrafficStatus, len(desiredTraffic))

	for stackName, weight := range desiredTraffic {
		traffic[stackName] = entities.TrafficStatus{
			ActualWeight:  actualTraffic[stackName],
			DesiredWeight: weight,
		}
	}

	return traffic, nil
}

// collectResources collects resources for all stacksets at once and stores them per StackSet/Stack so that we don't
// overload the API requests with unnecessary requests
func (c *StackSetController) collectResources() (map[types.UID]*entities.StackSetContainer, error) {
	stacksets := make(map[types.UID]*entities.StackSetContainer, len(c.stacksetStore))
	for uid, stackset := range c.stacksetStore {
		stackset := stackset
		stacksetContainer := &entities.StackSetContainer{
			StackSet:          stackset,
			StackContainers:   map[types.UID]*entities.StackContainer{},
			TrafficReconciler: SimpleTrafficReconciler{},
		}

		// use prescaling logic if enabled with an annotation
		if _, ok := stackset.Annotations[entities.PrescaleStacksAnnotationKey]; ok {
			resetDelay := DefaultResetMinReplicasDelay
			if resetDelayValue, ok := getResetMinReplicasDelay(stackset.Annotations); ok {
				resetDelay = resetDelayValue
			}
			stacksetContainer.TrafficReconciler = &PrescaleTrafficReconciler{
				ResetHPAMinReplicasTimeout: resetDelay,
			}
		}

		stacksets[uid] = stacksetContainer
	}

	err := c.collectIngresses(stacksets)
	if err != nil {
		return nil, err
	}

	err = c.collectStacks(stacksets)
	if err != nil {
		return nil, err
	}

	err = c.collectDeployments(stacksets)
	if err != nil {
		return nil, err
	}

	err = c.collectServices(stacksets)
	if err != nil {
		return nil, err
	}

	err = c.collectEndpoints(stacksets)
	if err != nil {
		return nil, err
	}

	err = c.collectHPAs(stacksets)
	if err != nil {
		return nil, err
	}

	// add traffic settings
	for _, ssc := range stacksets {
		if ssc.StackSet.Spec.Ingress != nil && len(ssc.StackContainers) > 0 && ssc.Ingress != nil {
			traffic, err := getIngressTraffic(ssc.Ingress)
			if err != nil {
				// TODO: can fail for all!!!
				c.recorder.Eventf(&ssc.StackSet,
					apiv1.EventTypeWarning,
					"FailedIngressTraffic",
					"Failed to get Ingress traffic: %v", err)
				return nil, err
			}
			ssc.Traffic = traffic
			eventTraffic := ""
			for k, v := range ssc.Traffic {
				if v.ActualWeight != v.DesiredWeight {
					eventTraffic = eventTraffic + fmt.Sprintf("%s: %.2f -> %.2f ", k, v.ActualWeight, v.DesiredWeight)
				} else {
					eventTraffic = eventTraffic + fmt.Sprintf("%s: %.2f ", k, v.ActualWeight)
				}
			}
			c.recorder.Eventf(&ssc.StackSet,
				apiv1.EventTypeNormal,
				"AddedIngressTraffic",
				"Added Ingress traffic: %s", eventTraffic)
		}
	}

	return stacksets, nil
}

func (c *StackSetController) collectIngresses(stacksets map[types.UID]*entities.StackSetContainer) error {
	ingresses, err := c.client.ExtensionsV1beta1().Ingresses(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Ingresses: %v", err)
	}

	for _, i := range ingresses.Items {
		ingress := i
		if uid, ok := getOwnerUID(ingress.ObjectMeta); ok {
			if s, ok := stacksets[uid]; ok {
				s.Ingress = &ingress
			}
		}
	}
	return nil
}

func (c *StackSetController) collectStacks(stacksets map[types.UID]*entities.StackSetContainer) error {
	stacks, err := c.client.ZalandoV1().Stacks(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Stacks: %v", err)
	}

	for _, stack := range stacks.Items {
		if uid, ok := getOwnerUID(stack.ObjectMeta); ok {
			if s, ok := stacksets[uid]; ok {
				s.StackContainers[stack.UID] = &entities.StackContainer{Stack: stack}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectDeployments(stacksets map[types.UID]*entities.StackSetContainer) error {
	deployments, err := c.client.AppsV1().Deployments(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Deployments: %v", err)
	}

	for _, d := range deployments.Items {
		deployment := d
		if uid, ok := getOwnerUID(deployment.ObjectMeta); ok {
			for _, stackset := range stacksets {
				if s, ok := stackset.StackContainers[uid]; ok {
					s.Resources = entities.StackResources{
						Deployment: &deployment,
					}
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectServices(stacksets map[types.UID]*entities.StackSetContainer) error {
	services, err := c.client.CoreV1().Services(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Services: %v", err)
	}

	for _, s := range services.Items {
		service := s
		if uid, ok := getOwnerUID(service.ObjectMeta); ok {
			for _, stackset := range stacksets {
				for _, stack := range stackset.StackContainers {
					if stack.Resources.Deployment != nil && stack.Resources.Deployment.UID == uid {
						stack.Resources.Service = &service
					}
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectEndpoints(stacksets map[types.UID]*entities.StackSetContainer) error {
	endpoints, err := c.client.CoreV1().Endpoints(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Endpoints: %v", err)
	}

	for _, endpoint := range endpoints.Items {
		endpoint := endpoint
		for _, stackset := range stacksets {
			for _, stack := range stackset.StackContainers {
				if stack.Stack.Name == endpoint.Name && stack.Stack.Namespace == endpoint.Namespace {
					stack.Resources.Endpoints = &endpoint
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectHPAs(stacksets map[types.UID]*entities.StackSetContainer) error {
	hpas, err := c.client.AutoscalingV2beta1().HorizontalPodAutoscalers(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list HPAs: %v", err)
	}

	for _, h := range hpas.Items {
		hpa := h
		if uid, ok := getOwnerUID(hpa.ObjectMeta); ok {
			for _, stackset := range stacksets {
				for _, stack := range stackset.StackContainers {
					if stack.Resources.Deployment != nil && stack.Resources.Deployment.UID == uid {
						stack.Resources.HPA = &hpa
					}
				}
			}
		}
	}
	return nil
}

func getOwnerUID(objectMeta metav1.ObjectMeta) (types.UID, bool) {
	if len(objectMeta.OwnerReferences) == 1 {
		return objectMeta.OwnerReferences[0].UID, true
	}
	return "", false
}

// hasOwnership returns true if the controller is the "owner" of the stackset.
// Whether it's owner is determined by the value of the
// 'stackset-controller.zalando.org/controller' annotation. If the value
// matches the controllerID then it owns it, or if the controllerID is
// "" and there's no annotation set.
func (c *StackSetController) hasOwnership(stackset *zv1.StackSet) bool {
	if stackset.Annotations != nil {
		if owner, ok := stackset.Annotations[entities.StacksetControllerControllerAnnotationKey]; ok {
			return owner == c.controllerID
		}
	}
	return c.controllerID == ""
}

func (c *StackSetController) startWatch(ctx context.Context) {
	informer := cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(c.client.ZalandoV1().RESTClient(), "stacksets", v1.NamespaceAll, fields.Everything()),
		&zv1.StackSet{},
		0, // skip resync
		cache.Indexers{},
	)

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.add,
		UpdateFunc: c.update,
		DeleteFunc: c.del,
	})
	go informer.Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		c.logger.Errorf("Timed out waiting for caches to sync")
		return
	}
	c.logger.Info("Synced StackSet watcher")
}

func (c *StackSetController) add(obj interface{}) {
	stackset, ok := obj.(*zv1.StackSet)
	if !ok {
		c.logger.Error("Failed to get StackSet object")
		return
	}

	c.logger.Infof("New StackSet added %s/%s", stackset.Namespace, stackset.Name)
	c.stacksetEvents <- stacksetEvent{
		StackSet: stackset.DeepCopy(),
	}
}

func (c *StackSetController) update(oldObj, newObj interface{}) {
	newStackset, ok := newObj.(*zv1.StackSet)
	if !ok {
		c.logger.Error("Failed to get StackSet object")
		return
	}

	oldStackset, ok := oldObj.(*zv1.StackSet)
	if !ok {
		c.logger.Error("Failed to get StackSet object")
		return
	}

	c.logger.Debugf("StackSet %s/%s changed: %s",
		newStackset.Namespace,
		newStackset.Name,
		cmp.Diff(oldStackset, newStackset, cmpopts.IgnoreUnexported(resource.Quantity{})),
	)

	c.logger.Infof("StackSet updated %s/%s", newStackset.Namespace, newStackset.Name)
	c.stacksetEvents <- stacksetEvent{
		StackSet: newStackset.DeepCopy(),
	}
}

func (c *StackSetController) del(obj interface{}) {
	stackset, ok := obj.(*zv1.StackSet)
	if !ok {
		c.logger.Error("Failed to get StackSet object")
		return
	}

	c.logger.Infof("StackSet deleted %s/%s", stackset.Namespace, stackset.Name)
	c.stacksetEvents <- stacksetEvent{
		StackSet: stackset.DeepCopy(),
		Deleted:  true,
	}
}

// ReconcileStackSetStatus reconciles the status of a StackSet.
func (c *StackSetController) ReconcileStackSetStatus(ssc entities.StackSetContainer) error {
	stackset := ssc.StackSet
	stacks := ssc.Stacks()

	stacksWithTraffic := int32(0)
	for _, stack := range stacks {
		if ssc.Traffic != nil && ssc.Traffic[stack.Name].Weight() > 0 {
			stacksWithTraffic++
		}
	}

	newStatus := zv1.StackSetStatus{
		Stacks:               int32(len(stacks)),
		StacksWithTraffic:    stacksWithTraffic,
		ReadyStacks:          readyStacks(stacks),
		ObservedStackVersion: currentStackVersion(stackset),
	}

	if !equality.Semantic.DeepEqual(newStatus, stackset.Status) {
		stackset.Status = newStatus

		// update status of stackset
		_, err := c.client.ZalandoV1().StackSets(stackset.Namespace).UpdateStatus(&stackset)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *StackSetController) StackSetStatusUpdateFailed(ssc entities.StackSetContainer, err error) {
	stackset := ssc.StackSet
	c.recorder.Eventf(&stackset,
		apiv1.EventTypeWarning,
		"FailedUpdateStack",
		"Failed to create/update stack: %v", err)
}

// readyStacks returns the number of ready Stacks given a slice of Stacks.
func readyStacks(stacks []zv1.Stack) int32 {
	var readyStacks int32
	for _, stack := range stacks {
		replicas := stack.Status.Replicas
		if replicas == stack.Status.ReadyReplicas && replicas == stack.Status.UpdatedReplicas {
			readyStacks++
		}
	}
	return readyStacks
}

// StackSetGC garbage collects Stacks for a single StackSet. Whether Stacks
// should be garbage collected is determined by the StackSet lifecycle limit.
func (c *StackSetController) StackSetGC(ssc entities.StackSetContainer) error {
	stackset := ssc.StackSet
	stacks := c.getStacksToGC(ssc)

	for _, stack := range stacks {
		err := c.client.ZalandoV1().Stacks(stack.Namespace).Delete(stack.Name, nil)
		if err != nil {
			return err
		}
		c.recorder.Eventf(&stackset,
			apiv1.EventTypeNormal,
			"DeletedExcessStack",
			"Deleted excess stack %s/%s",
			stack.Namespace,
			stack.Name,
		)
	}

	return nil
}

func (c *StackSetController) getStacksToGC(ssc entities.StackSetContainer) []zv1.Stack {
	stackset := ssc.StackSet
	stacks := ssc.Stacks()

	historyLimit := defaultStackLifecycleLimit
	if stackset.Spec.StackLifecycle.Limit != nil {
		historyLimit = int(*stackset.Spec.StackLifecycle.Limit)
	}

	gcCandidates := make([]zv1.Stack, 0, len(stacks))
	for _, stack := range ssc.Stacks() {
		// if the stack doesn't have any ingress all stacks are
		// candidates for cleanup
		if ssc.StackSet.Spec.Ingress == nil {
			gcCandidates = append(gcCandidates, stack)
			continue
		}

		// never garbage collect stacks with traffic
		if ssc.Traffic != nil && ssc.Traffic[stack.Name].Weight() > 0 {
			continue
		}

		// never garbage collect stacks without NoTrafficSince status
		if stack.Status.NoTrafficSince == nil {
			continue
		}

		noTrafficSince := stack.Status.NoTrafficSince

		if !noTrafficSince.IsZero() && time.Since(noTrafficSince.Time) > ssc.ScaledownTTL() {
			gcCandidates = append(gcCandidates, stack)
		}
	}

	// only garbage collect if history limit is reached
	if len(gcCandidates) <= historyLimit {
		c.logger.Debugf("No Stacks to clean up for StackSet %s/%s (limit: %d/%d)", stackset.Namespace, stackset.Name, len(stacks), historyLimit)
		return nil
	}

	// sort candidates by oldest
	sort.Slice(gcCandidates, func(i, j int) bool {
		// TODO: maybe we use noTrafficSince instead of CreationTimeStamp to decide oldest
		return gcCandidates[i].CreationTimestamp.Time.Before(gcCandidates[j].CreationTimestamp.Time)
	})

	excessStacks := len(gcCandidates) - historyLimit
	c.recorder.Eventf(&stackset,
		apiv1.EventTypeNormal,
		"ExeedStackHistoryLimit",
		"Found %d Stack(s) exceeding the StackHistoryLimit (%d). %d candidate(s) for GC",
		excessStacks,
		historyLimit,
		len(gcCandidates),
	)

	return gcCandidates[:excessStacks]
}

func currentStackVersion(stackset zv1.StackSet) string {
	version := stackset.Spec.StackTemplate.Spec.Version
	if version == "" {
		version = defaultVersion
	}
	return version
}

func generateStackName(stackset zv1.StackSet, version string) string {
	return stackset.Name + "-" + version
}

// ReconcileStack brings the Stack created from the current StackSet definition
// to the desired state.
func (c *StackSetController) ReconcileStack(ssc entities.StackSetContainer) error {
	stackset := ssc.StackSet
	heritageLabels := map[string]string{
		entities.StacksetHeritageLabelKey: stackset.Name,
	}
	observedStackVersion := stackset.Status.ObservedStackVersion
	stackVersion := currentStackVersion(stackset)
	stackName := generateStackName(stackset, stackVersion)

	stacks := ssc.Stacks()

	var stack *zv1.Stack
	for _, s := range stacks {
		if s.Name == stackName {
			stack = &s
			break
		}
	}

	stackLabels := mergeLabels(
		heritageLabels,
		stackset.Labels,
		map[string]string{entities.StackVersionLabelKey: stackVersion},
	)

	if stack == nil && observedStackVersion != stackVersion {
		stack = &zv1.Stack{
			ObjectMeta: metav1.ObjectMeta{
				Name:      stackName,
				Namespace: ssc.StackSet.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: stackset.APIVersion,
						Kind:       stackset.Kind,
						Name:       stackset.Name,
						UID:        stackset.UID,
					},
				},
			},
		}

		stack.Labels = stackLabels
		stack.Spec.PodTemplate = *stackset.Spec.StackTemplate.Spec.PodTemplate.DeepCopy()
		stack.Spec.Replicas = stackset.Spec.StackTemplate.Spec.Replicas
		stack.Spec.HorizontalPodAutoscaler = stackset.Spec.StackTemplate.Spec.HorizontalPodAutoscaler.DeepCopy()
		stack.Spec.Autoscaler = stackset.Spec.StackTemplate.Spec.Autoscaler.DeepCopy()
		if stackset.Spec.StackTemplate.Spec.Service != nil {
			stack.Spec.Service = sanitizeServicePorts(stackset.Spec.StackTemplate.Spec.Service)
		}

		c.recorder.Eventf(&stackset,
			apiv1.EventTypeNormal,
			"CreatedStack",
			"Created Stack '%s/%s'",
			stack.Namespace, stack.Name,
		)

		_, err := c.client.ZalandoV1().Stacks(stack.Namespace).Create(stack)
		if err != nil {
			return err
		}
	}

	return nil
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

func mergeLabels(labelMaps ...map[string]string) map[string]string {
	labels := make(map[string]string)
	for _, labelMap := range labelMaps {
		for k, v := range labelMap {
			labels[k] = v
		}
	}
	return labels
}

// getResetMinReplicasDelay parses and returns the reset delay if set in the
// stackset annotation.
func getResetMinReplicasDelay(annotations map[string]string) (time.Duration, bool) {
	resetDelayStr, ok := annotations[entities.ResetHPAMinReplicasDelayAnnotationKey]
	if !ok {
		return 0, false
	}
	resetDelay, err := time.ParseDuration(resetDelayStr)
	if err != nil {
		return 0, false
	}
	return resetDelay, true
}

func getStackSetGeneration(metadata metav1.ObjectMeta) int64 {
	if g, ok := metadata.Annotations[entities.StacksetGenerationAnnotationKey]; ok {
		generation, err := strconv.ParseInt(g, 10, 64)
		if err != nil {
			return 0
		}
		return generation
	}
	return 0
}
