package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	log "github.com/sirupsen/logrus"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"github.com/zalando-incubator/stackset-controller/pkg/core"
	"github.com/zalando-incubator/stackset-controller/pkg/recorder"
	"golang.org/x/sync/errgroup"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	kube_record "k8s.io/client-go/tools/record"
)

const (
	PrescaleStacksAnnotationKey               = "alpha.stackset-controller.zalando.org/prescale-stacks"
	ResetHPAMinReplicasDelayAnnotationKey     = "alpha.stackset-controller.zalando.org/reset-hpa-min-replicas-delay"
	StacksetControllerControllerAnnotationKey = "stackset-controller.zalando.org/controller"

	defaultResetMinReplicasDelay = 10 * time.Minute
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

// eventedError wraps an error that was already exposed as an event to the user
type eventedError struct {
	err error
}

func (ee *eventedError) Error() string {
	return ee.err.Error()
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

func (c *StackSetController) stacksetLogger(ssc *core.StackSetContainer) *log.Entry {
	return c.logger.WithFields(map[string]interface{}{
		"namespace": ssc.StackSet.Namespace,
		"name":      ssc.StackSet.Name,
	})
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
				container := container

				reconcileGroup.Go(func() error {
					if _, ok := c.stacksetStore[stackset]; ok {
						err := c.ReconcileStackSet(container)
						if err != nil {
							c.stacksetLogger(container).Errorf("unable to reconcile a stackset: %v", err)
							return c.errorEventf(container.StackSet, "FailedManageStackSet", err)
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
			stackset.APIVersion = core.APIVersion
			stackset.Kind = core.KindStackSet

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

// collectResources collects resources for all stacksets at once and stores them per StackSet/Stack so that we don't
// overload the API requests with unnecessary requests
func (c *StackSetController) collectResources() (map[types.UID]*core.StackSetContainer, error) {
	stacksets := make(map[types.UID]*core.StackSetContainer, len(c.stacksetStore))
	for uid, stackset := range c.stacksetStore {
		stackset := stackset
		stacksetContainer := &core.StackSetContainer{
			StackSet:          &stackset,
			StackContainers:   map[types.UID]*core.StackContainer{},
			TrafficReconciler: &core.SimpleTrafficReconciler{},
		}

		// use prescaling logic if enabled with an annotation
		if _, ok := stackset.Annotations[PrescaleStacksAnnotationKey]; ok {
			resetDelay := defaultResetMinReplicasDelay
			if resetDelayValue, ok := getResetMinReplicasDelay(stackset.Annotations); ok {
				resetDelay = resetDelayValue
			}
			stacksetContainer.TrafficReconciler = &core.PrescalingTrafficReconciler{
				ResetHPAMinReplicasTimeout: resetDelay,
			}
		}

		stacksets[uid] = stacksetContainer
	}

	err := c.collectStacks(stacksets)
	if err != nil {
		return nil, err
	}

	err = c.collectIngresses(stacksets)
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

	err = c.collectHPAs(stacksets)
	if err != nil {
		return nil, err
	}

	return stacksets, nil
}

func (c *StackSetController) collectIngresses(stacksets map[types.UID]*core.StackSetContainer) error {
	ingresses, err := c.client.ExtensionsV1beta1().Ingresses(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Ingresses: %v", err)
	}

Items:
	for _, i := range ingresses.Items {
		ingress := i
		if uid, ok := getOwnerUID(ingress.ObjectMeta); ok {
			// stackset ingress
			if s, ok := stacksets[uid]; ok {
				s.Ingress = &ingress
				continue Items
			}

			// stack ingress
			for _, stackset := range stacksets {
				if s, ok := stackset.StackContainers[uid]; ok {
					s.Resources.Ingress = &ingress
					continue Items
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectStacks(stacksets map[types.UID]*core.StackSetContainer) error {
	stacks, err := c.client.ZalandoV1().Stacks(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Stacks: %v", err)
	}

	for _, stack := range stacks.Items {
		if uid, ok := getOwnerUID(stack.ObjectMeta); ok {
			if s, ok := stacksets[uid]; ok {
				stack := stack

				// set TypeMeta manually because of this bug:
				// https://github.com/kubernetes/client-go/issues/308
				stack.APIVersion = core.APIVersion
				stack.Kind = core.KindStack

				s.StackContainers[stack.UID] = &core.StackContainer{
					Stack: &stack,
				}
				continue
			}
		}
	}
	return nil
}

func (c *StackSetController) collectDeployments(stacksets map[types.UID]*core.StackSetContainer) error {
	deployments, err := c.client.AppsV1().Deployments(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Deployments: %v", err)
	}

	for _, d := range deployments.Items {
		deployment := d
		if uid, ok := getOwnerUID(deployment.ObjectMeta); ok {
			for _, stackset := range stacksets {
				if s, ok := stackset.StackContainers[uid]; ok {
					s.Resources.Deployment = &deployment
					break
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectServices(stacksets map[types.UID]*core.StackSetContainer) error {
	services, err := c.client.CoreV1().Services(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Services: %v", err)
	}

Items:
	for _, s := range services.Items {
		service := s
		if uid, ok := getOwnerUID(service.ObjectMeta); ok {
			for _, stackset := range stacksets {
				if s, ok := stackset.StackContainers[uid]; ok {
					s.Resources.Service = &service
					continue Items
				}

				// service/HPA used to be owned by the deployment for some reason
				for _, stack := range stackset.StackContainers {
					if stack.Resources.Deployment != nil && stack.Resources.Deployment.UID == uid {
						stack.Resources.Service = &service
						continue Items
					}
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectHPAs(stacksets map[types.UID]*core.StackSetContainer) error {
	hpas, err := c.client.AutoscalingV2beta1().HorizontalPodAutoscalers(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list HPAs: %v", err)
	}

Items:
	for _, h := range hpas.Items {
		hpa := h
		if uid, ok := getOwnerUID(hpa.ObjectMeta); ok {
			for _, stackset := range stacksets {
				if s, ok := stackset.StackContainers[uid]; ok {
					s.Resources.HPA = &hpa
					continue Items
				}

				// service/HPA used to be owned by the deployment for some reason
				for _, stack := range stackset.StackContainers {
					if stack.Resources.Deployment != nil && stack.Resources.Deployment.UID == uid {
						stack.Resources.HPA = &hpa
						continue Items
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

func (c *StackSetController) errorEventf(object runtime.Object, reason string, err error) error {
	switch err.(type) {
	case *eventedError:
		// already notified
		return err
	default:
		c.recorder.Eventf(
			object,
			apiv1.EventTypeWarning,
			reason,
			err.Error())
		return &eventedError{err: err}
	}
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
		return
	}

	oldStackset, ok := oldObj.(*zv1.StackSet)
	if !ok {
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
		return
	}

	c.logger.Infof("StackSet deleted %s/%s", stackset.Namespace, stackset.Name)
	c.stacksetEvents <- stacksetEvent{
		StackSet: stackset.DeepCopy(),
		Deleted:  true,
	}
}

func retryUpdate(updateFn func(retry bool) error) error {
	retry := false
	for {
		err := updateFn(retry)
		if err != nil {
			if errors.IsConflict(err) {
				retry = true
				continue
			}
			return err
		}
		return nil
	}
}

// ReconcileStatuses reconciles the statuses of StackSets and Stacks.
func (c *StackSetController) ReconcileStatuses(ssc *core.StackSetContainer) error {
	for _, sc := range ssc.StackContainers {
		stack := sc.Stack.DeepCopy()
		status := *sc.GenerateStackStatus()
		err := retryUpdate(func(retry bool) error {
			if retry {
				updated, err := c.client.ZalandoV1().Stacks(sc.Namespace()).Get(stack.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				stack = updated
			}
			stack.Status = status
			_, err := c.client.ZalandoV1().Stacks(sc.Namespace()).UpdateStatus(stack)
			return err
		})
		if err != nil {
			return c.errorEventf(sc.Stack, "FailedUpdateStackStatus", err)
		}
	}

	stackset := ssc.StackSet.DeepCopy()
	status := *ssc.GenerateStackSetStatus()
	err := retryUpdate(func(retry bool) error {
		if retry {
			updated, err := c.client.ZalandoV1().StackSets(ssc.StackSet.Namespace).Get(ssc.StackSet.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			stackset = updated
		}
		if !equality.Semantic.DeepEqual(status, stackset.Status) {
			stackset.Status = status
			_, err := c.client.ZalandoV1().StackSets(ssc.StackSet.Namespace).UpdateStatus(stackset)
			return err
		}
		return nil
	})

	if err != nil {
		return c.errorEventf(ssc.StackSet, "FailedUpdateStackSetStatus", err)
	}
	return nil
}

// CreateCurrentStack creates a new Stack object for the current stack, if needed
func (c *StackSetController) CreateCurrentStack(ssc *core.StackSetContainer) error {
	newStack, newStackVersion := ssc.NewStack()
	if newStack == nil {
		return nil
	}

	created, err := c.client.ZalandoV1().Stacks(newStack.Namespace()).Create(newStack.Stack)
	if err != nil {
		return c.errorEventf(ssc.StackSet, "FailedCreateStack", err)
	}

	// set TypeMeta manually because of this bug:
	// https://github.com/kubernetes/client-go/issues/308
	created.APIVersion = core.APIVersion
	created.Kind = core.KindStack

	c.recorder.Eventf(
		ssc.StackSet,
		apiv1.EventTypeNormal,
		"CreatedStack",
		"Created stack %s",
		newStack.Name())

	// Persist ObservedStackVersion in the status
	updated := ssc.StackSet.DeepCopy()
	updated.Status.ObservedStackVersion = newStackVersion
	result, err := c.client.ZalandoV1().StackSets(ssc.StackSet.Namespace).UpdateStatus(updated)
	if err != nil {
		return err
	}
	ssc.StackSet = result

	ssc.StackContainers[created.UID] = &core.StackContainer{
		Stack:          created,
		PendingRemoval: false,
		Resources:      core.StackResources{},
	}
	return nil
}

// CleanupOldStacks deletes stacks that are no longer needed.
func (c *StackSetController) CleanupOldStacks(ssc *core.StackSetContainer) error {
	for _, sc := range ssc.StackContainers {
		if !sc.PendingRemoval {
			continue
		}

		stack := sc.Stack
		err := c.client.ZalandoV1().Stacks(stack.Namespace).Delete(stack.Name, nil)
		if err != nil {
			return c.errorEventf(ssc.StackSet, "FailedDeleteStack", err)
		}
		c.recorder.Eventf(
			ssc.StackSet,
			apiv1.EventTypeNormal,
			"DeletedExcessStack",
			"Deleted excess stack %s",
			stack.Name)
	}

	return nil
}

func (c *StackSetController) ReconcileStackSetIngress(stackset *zv1.StackSet, existing *extensions.Ingress, generateUpdated func() (*extensions.Ingress, error)) error {
	ingress, err := generateUpdated()
	if err != nil {
		return err
	}

	// Ingress removed
	if ingress == nil {
		if existing != nil {
			err := c.client.ExtensionsV1beta1().Ingresses(existing.Namespace).Delete(existing.Name, &metav1.DeleteOptions{})
			if err != nil {
				return err
			}
			c.recorder.Eventf(
				stackset,
				apiv1.EventTypeNormal,
				"DeletedIngress",
				"Deleted Ingress %s",
				existing.Namespace)
		}
		return nil
	}

	// Create new Ingress
	if existing == nil {
		_, err := c.client.ExtensionsV1beta1().Ingresses(ingress.Namespace).Create(ingress)
		if err != nil {
			return err
		}
		c.recorder.Eventf(
			stackset,
			apiv1.EventTypeNormal,
			"CreatedIngress",
			"Created Ingress %s",
			ingress.Name)
		return nil
	}

	// Check if we need to update the Ingress
	if equality.Semantic.DeepDerivative(ingress.Spec, existing.Spec) && equality.Semantic.DeepEqual(ingress.Annotations, existing.Annotations) {
		return nil
	}

	updated := existing.DeepCopy()
	updated.Spec = ingress.Spec

	_, err = c.client.ExtensionsV1beta1().Ingresses(updated.Namespace).Update(ingress)
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		stackset,
		apiv1.EventTypeNormal,
		"UpdatedIngress",
		"Updated Ingress %s",
		ingress.Name)
	return nil
}

func (c *StackSetController) ReconcileResources(ssc *core.StackSetContainer) error {
	for _, sc := range ssc.StackContainers {
		err := c.ReconcileStackResources(ssc, sc)
		if err != nil {
			return c.errorEventf(sc.Stack, "FailedManageStack", err)
		}
	}

	err := c.ReconcileStackSetIngress(ssc.StackSet, ssc.Ingress, ssc.GenerateIngress)
	if err != nil {
		return c.errorEventf(ssc.StackSet, "FailedManageIngress", err)
	}

	trafficChanges := ssc.TrafficChanges()
	if len(trafficChanges) != 0 {
		var changeMessages []string
		for _, change := range trafficChanges {
			changeMessages = append(changeMessages, change.String())
		}

		c.recorder.Eventf(
			ssc.StackSet,
			apiv1.EventTypeNormal,
			"TrafficSwitched",
			"Switched traffic: %s",
			strings.Join(changeMessages, ", "))
	}

	return nil
}

func (c *StackSetController) ReconcileStackResources(ssc *core.StackSetContainer, sc *core.StackContainer) error {
	err := c.ReconcileStackDeployment(sc.Stack, sc.Resources.Deployment, sc.GenerateDeployment)
	if err != nil {
		return c.errorEventf(sc.Stack, "FailedManageDeployment", err)
	}

	err = c.ReconcileStackHPA(sc.Stack, sc.Resources.HPA, sc.GenerateHPA)
	if err != nil {
		return c.errorEventf(sc.Stack, "FailedManageHPA", err)
	}

	err = c.ReconcileStackService(sc.Stack, sc.Resources.Service, sc.GenerateService)
	if err != nil {
		return c.errorEventf(sc.Stack, "FailedManageService", err)
	}

	err = c.ReconcileStackIngress(sc.Stack, sc.Resources.Ingress, sc.GenerateIngress)
	if err != nil {
		return c.errorEventf(sc.Stack, "FailedManageIngress", err)
	}

	return nil
}

func (c *StackSetController) ReconcileStackSet(container *core.StackSetContainer) error {
	// Create current stack, if needed. Errors don't cause reconciliation to fail.
	err := c.CreateCurrentStack(container)
	if err != nil {
		c.stacksetLogger(container).Errorf("Unable to create new stack: %v", err)
	}

	// Update statuses from external resources (ingresses, deployments, etc). Errors here shouldn't happen and abort reconciliation.
	err = container.UpdateFromResources()
	if err != nil {
		return err
	}

	// Update the stacks with the currently selected traffic reconciler
	err = container.ManageTraffic(time.Now())
	if err != nil {
		c.stacksetLogger(container).Errorf("Traffic reconciliation failed: %v", err)
		c.recorder.Eventf(
			container.StackSet,
			v1.EventTypeWarning,
			"TrafficNotSwitched",
			"Failed to switch traffic: "+err.Error())
	}

	// Mark stacks that should be removed
	container.MarkExpiredStacks()

	// Create or update resources
	err = c.ReconcileResources(container)
	if err != nil {
		return err
	}

	// Delete old stacks
	err = c.CleanupOldStacks(container)
	if err != nil {
		return err
	}

	// Update statuses
	err = c.ReconcileStatuses(container)
	if err != nil {
		return err
	}

	return nil
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
