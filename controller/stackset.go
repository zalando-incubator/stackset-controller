package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	log "github.com/sirupsen/logrus"
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
	StacksetControllerControllerAnnotationKey = "stackset-controller.zalando.org/controller"
	PrescaleStacksAnnotationKey               = "alpha.stackset-controller.zalando.org/prescale-stacks"
	ResetHPAMinReplicasDelayAnnotationKey     = "alpha.stackset-controller.zalando.org/reset-hpa-min-replicas-delay"
	stacksetGenerationAnnotationKey           = "stackset-controller.zalando.org/stackset-generation"

	stacksetHeritageLabelKey   = "stackset"
	stackVersionLabelKey       = "stack-version"
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
							c.StackSetStatusUpdateFailed(container)
							return err
						}

						err := c.ReconcileStacks(container)
						if err != nil {
							c.StackSetStatusUpdateFailed(container)
							return err
						}

						err = c.ReconcileIngress(container)
						if err != nil {
							c.StackSetStatusUpdateFailed(container)
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
					c.recorder.Eventf(e.StackSet, apiv1.EventTypeNormal, "DeleteStackSet", "StackSet '%s/%s' deleted, removing references", stackset.Namespace, stackset.Name)
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

// StackSetContainer is a container for storing the full state of a StackSet
// including the sub-resources which are part of the StackSet. It respresents a
// snapshot of the resources currently in the Cluster. This includes an
// optional Ingress resource as well as the current Traffic distribution. It
// also contains a set of StackContainers which respresents the full state of
// the individual Stacks part of the StackSet.
type StackSetContainer struct {
	StackSet zv1.StackSet

	// StackContainers is a set of stacks belonging to the StackSet
	// including the Stack sub resources like Deployments and Services.
	StackContainers map[types.UID]*StackContainer

	// Ingress defines the current Ingress resource belonging to the
	// StackSet. This is a reference to the actual resource while
	// `StackSet.Spec.Ingress` defines the ingress configuration specified
	// by the user on the StackSet.
	Ingress *v1beta1.Ingress

	// Traffic is the current traffic distribution across stacks of the
	// StackSet. The values of this are derived from the related Ingress
	// resource. The key of the map is the Stack name.
	Traffic map[string]TrafficStatus

	// TrafficReconciler is the reconciler implementation used for
	// switching traffic between stacks. E.g. for prescaling stacks before
	// switching traffic.
	TrafficReconciler TrafficReconciler
}

// Stacks returns a slice of Stack resources.
func (sc StackSetContainer) Stacks() []zv1.Stack {
	stacks := make([]zv1.Stack, 0, len(sc.StackContainers))
	for _, stackContainer := range sc.StackContainers {
		stacks = append(stacks, stackContainer.Stack)
	}
	return stacks
}

// ScaledownTTL returns the ScaledownTTLSeconds value of a StackSet as a
// time.Duration.
func (sc StackSetContainer) ScaledownTTL() time.Duration {
	if ttlSec := sc.StackSet.Spec.StackLifecycle.ScaledownTTLSeconds; ttlSec != nil {
		return time.Second * time.Duration(*ttlSec)
	}
	return 0
}

// StackContainer is a container for storing the full state of a Stack
// including all the managed sub-resources. This includes the Stack resource
// itself and all the sub resources like Deployment, HPA, Service and
// Endpoints.
type StackContainer struct {
	Stack     zv1.Stack
	Resources StackResources
}

// TrafficStatus represents the traffic status of an Ingress. ActualWeight is
// the actual traffic a particular backend is getting and DesiredWeight is the
// user specified value that it should try to achieve.
type TrafficStatus struct {
	ActualWeight  float64
	DesiredWeight float64
}

// Weight returns the max of ActualWeight and DesiredWeight.
func (t TrafficStatus) Weight() float64 {
	return math.Max(t.ActualWeight, t.DesiredWeight)
}

// getIngressTraffic parses ingress traffic from an Ingress resource.
func getIngressTraffic(ingress *v1beta1.Ingress) (map[string]TrafficStatus, error) {
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

	traffic := make(map[string]TrafficStatus, len(desiredTraffic))

	for stackName, weight := range desiredTraffic {
		traffic[stackName] = TrafficStatus{
			ActualWeight:  actualTraffic[stackName],
			DesiredWeight: weight,
		}
	}

	return traffic, nil
}

// collectResources collects resources for all stacksets at once and stores them per StackSet/Stack so that we don't
// overload the API requests with unnecessary requests
func (c *StackSetController) collectResources() (map[types.UID]*StackSetContainer, error) {
	stacksets := make(map[types.UID]*StackSetContainer, len(c.stacksetStore))
	for uid, stackset := range c.stacksetStore {
		stackset := stackset
		stacksetContainer := &StackSetContainer{
			StackSet:          stackset,
			StackContainers:   map[types.UID]*StackContainer{},
			TrafficReconciler: SimpleTrafficReconciler{},
		}

		// use prescaling logic if enabled with an annotation
		if _, ok := stackset.Annotations[PrescaleStacksAnnotationKey]; ok {
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
					"GetIngressTraffic",
					"Failed to get Ingress traffic for StackSet %s/%s: %v", ssc.StackSet.Namespace, ssc.StackSet.Name, err)
				return nil, err
			}
			ssc.Traffic = traffic
		}
	}

	return stacksets, nil
}

func (c *StackSetController) collectIngresses(stacksets map[types.UID]*StackSetContainer) error {
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

func (c *StackSetController) collectStacks(stacksets map[types.UID]*StackSetContainer) error {
	stacks, err := c.client.ZalandoV1().Stacks(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Stacks: %v", err)
	}

	for _, stack := range stacks.Items {
		if uid, ok := getOwnerUID(stack.ObjectMeta); ok {
			if s, ok := stacksets[uid]; ok {
				s.StackContainers[stack.UID] = &StackContainer{Stack: stack}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectDeployments(stacksets map[types.UID]*StackSetContainer) error {
	deployments, err := c.client.AppsV1().Deployments(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Deployments: %v", err)
	}

	for _, d := range deployments.Items {
		deployment := d
		if uid, ok := getOwnerUID(deployment.ObjectMeta); ok {
			for _, stackset := range stacksets {
				if s, ok := stackset.StackContainers[uid]; ok {
					s.Resources = StackResources{
						Deployment: &deployment,
					}
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectServices(stacksets map[types.UID]*StackSetContainer) error {
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

func (c *StackSetController) collectEndpoints(stacksets map[types.UID]*StackSetContainer) error {
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

func (c *StackSetController) collectHPAs(stacksets map[types.UID]*StackSetContainer) error {
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
		if owner, ok := stackset.Annotations[StacksetControllerControllerAnnotationKey]; ok {
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
func (c *StackSetController) ReconcileStackSetStatus(ssc StackSetContainer) error {
	stackset := ssc.StackSet
	stacks := ssc.Stacks()

	stacksWithTraffic := int32(0)
	for _, stack := range stacks {
		if ssc.Traffic != nil && ssc.Traffic[stack.Name].Weight() > 0 {
			stacksWithTraffic++
		}
	}

	newStatus := zv1.StackSetStatus{
		Stacks:            int32(len(stacks)),
		StacksWithTraffic: stacksWithTraffic,
		ReadyStacks:       readyStacks(stacks),
	}

	if !equality.Semantic.DeepEqual(newStatus, stackset.Status) {
		c.recorder.Eventf(&stackset,
			apiv1.EventTypeNormal,
			"UpdateStackSetStatus",
			"Status changed for StackSet %s/%s: %#v -> %#v",
			stackset.Namespace,
			stackset.Name,
			stackset.Status,
			newStatus,
		)
		stackset.Status = newStatus

		// update status of stackset
		_, err := c.client.ZalandoV1().StackSets(stackset.Namespace).UpdateStatus(&stackset)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *StackSetController) StackSetStatusUpdateFailed(ssc StackSetContainer) {
	stackset := ssc.StackSet
	c.recorder.Eventf(&stackset,
		apiv1.EventTypeWarning,
		"FailedUpdateStack",
		"Failed to create/update stack")
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
func (c *StackSetController) StackSetGC(ssc StackSetContainer) error {
	stackset := ssc.StackSet
	stacks := c.getStacksToGC(ssc)

	for _, stack := range stacks {
		c.recorder.Eventf(&stackset,
			apiv1.EventTypeNormal,
			"DeleteExcessStack",
			"Deleting excess stack %s/%s for StackSet %s/%s",
			stack.Namespace,
			stack.Name,
			stackset.Namespace,
			stackset.Name,
		)
		err := c.client.ZalandoV1().Stacks(stack.Namespace).Delete(stack.Name, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *StackSetController) getStacksToGC(ssc StackSetContainer) []zv1.Stack {
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
	if len(stacks) <= historyLimit {
		c.logger.Debugf("No Stacks to clean up for StackSet %s/%s (limit: %d/%d)", stackset.Namespace, stackset.Name, len(stacks), historyLimit)
		return nil
	}

	// sort candidates by oldest
	sort.Slice(gcCandidates, func(i, j int) bool {
		return gcCandidates[i].CreationTimestamp.Time.Before(gcCandidates[j].CreationTimestamp.Time)
	})

	excessStacks := len(stacks) - historyLimit
	c.recorder.Eventf(&stackset,
		apiv1.EventTypeNormal,
		"ExeedStackHistoryLimit",
		"Found %d Stack(s) exeeding the StackHistoryLimit (%d) for StackSet %s/%s. %d candidate(s) for GC",
		excessStacks,
		historyLimit,
		stackset.Namespace,
		stackset.Name,
		len(gcCandidates),
	)

	gcLimit := int(math.Min(float64(excessStacks), float64(len(gcCandidates))))
	return gcCandidates[:gcLimit]
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
func (c *StackSetController) ReconcileStack(ssc StackSetContainer) error {
	stackset := ssc.StackSet
	heritageLabels := map[string]string{
		stacksetHeritageLabelKey: stackset.Name,
	}

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
		map[string]string{stackVersionLabelKey: stackVersion},
	)

	createStack := false

	if stack == nil {
		createStack = true
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
	}

	stack.Labels = stackLabels
	stack.Spec.PodTemplate = *stackset.Spec.StackTemplate.Spec.PodTemplate.DeepCopy()
	stack.Spec.Replicas = stackset.Spec.StackTemplate.Spec.Replicas
	stack.Spec.HorizontalPodAutoscaler = stackset.Spec.StackTemplate.Spec.HorizontalPodAutoscaler.DeepCopy()
	stack.Spec.Autoscaler = stackset.Spec.StackTemplate.Spec.Autoscaler.DeepCopy()
	if stackset.Spec.StackTemplate.Spec.Service != nil {
		stack.Spec.Service = sanitizeServicePorts(stackset.Spec.StackTemplate.Spec.Service)
	}

	if createStack {
		c.recorder.Eventf(&stackset,
			apiv1.EventTypeNormal,
			"CreateStackSetStack",
			"Creating Stack '%s/%s' for StackSet",
			stack.Namespace, stack.Name,
		)
		_, err := c.client.ZalandoV1().Stacks(stack.Namespace).Create(stack)
		if err != nil {
			return err
		}
	} else {
		stacksetGeneration := getStackSetGeneration(stack.ObjectMeta)

		// only update the resource if there are changes
		// We determine changes by comparing the stacksetGeneration
		// (observed generation) stored on the stack with the
		// generation of the StackSet.
		if stacksetGeneration != stackset.Generation {
			if stack.Annotations == nil {
				stack.Annotations = make(map[string]string, 1)
			}
			stack.Annotations[stacksetGenerationAnnotationKey] = fmt.Sprintf("%d", stackset.Generation)

			c.recorder.Eventf(&stackset,
				apiv1.EventTypeNormal,
				"UpdateStackSetStack",
				"Updating Stack '%s/%s' for StackSet",
				stack.Namespace, stack.Name,
			)
			_, err := c.client.ZalandoV1().Stacks(stack.Namespace).Update(stack)
			if err != nil {
				return err
			}
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
	resetDelayStr, ok := annotations[ResetHPAMinReplicasDelayAnnotationKey]
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
	if g, ok := metadata.Annotations[stacksetGenerationAnnotationKey]; ok {
		generation, err := strconv.ParseInt(g, 10, 64)
		if err != nil {
			return 0
		}
		return generation
	}
	return 0
}
