package controller

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/heptiolabs/healthcheck"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	routegroupinformers "github.com/szuecs/routegroup-client/client/informers/externalversions"
	rginformerv1 "github.com/szuecs/routegroup-client/client/informers/externalversions/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	stacksetinformers "github.com/zalando-incubator/stackset-controller/pkg/client/informers/externalversions"
	stacksetinformerv1 "github.com/zalando-incubator/stackset-controller/pkg/client/informers/externalversions/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"github.com/zalando-incubator/stackset-controller/pkg/core"
	"github.com/zalando-incubator/stackset-controller/pkg/recorder"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	appsinformerv1 "k8s.io/client-go/informers/apps/v1"
	autoscalinginformerv2 "k8s.io/client-go/informers/autoscaling/v2"
	coreinformer "k8s.io/client-go/informers/core/v1"
	ingressinformerv1 "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/tools/cache"
	kube_record "k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

const (
	PrescaleStacksAnnotationKey               = "alpha.stackset-controller.zalando.org/prescale-stacks"
	ResetHPAMinReplicasDelayAnnotationKey     = "alpha.stackset-controller.zalando.org/reset-hpa-min-replicas-delay"
	StacksetControllerControllerAnnotationKey = "stackset-controller.zalando.org/controller"
	ControllerLastUpdatedAnnotationKey        = "stackset-controller.zalando.org/updated-timestamp"

	reasonFailedManageStackSet = "FailedManageStackSet"

	defaultResetMinReplicasDelay = 10 * time.Minute
)

var configurationResourceNameError = "ConfigurationResource name must be prefixed by Stack name. ConfigurationResource: %s, Stack: %s"

type controllerInformers struct {
	stacksetInformer   stacksetinformerv1.StackSetInformer
	stackInformer      stacksetinformerv1.StackInformer
	ingressInformer    ingressinformerv1.IngressInformer
	routegroupInformer rginformerv1.RouteGroupInformer
	deploymentInformer appsinformerv1.DeploymentInformer
	serviceInformer    coreinformer.ServiceInformer
	hpaInformer        autoscalinginformerv2.HorizontalPodAutoscalerInformer
	configMapInformer  coreinformer.ConfigMapInformer
	secretInformer     coreinformer.SecretInformer
	pcsInformer        stacksetinformerv1.PlatformCredentialsSetInformer
}

// StackSetController is the main controller. It watches for changes to
// stackset resources and starts and maintains other controllers per
// stackset resource.
type StackSetController struct {
	logger          *log.Entry
	client          clientset.Interface
	informers       controllerInformers
	config          StackSetConfig
	stacksetEvents  chan stacksetEvent
	stacksetStore   map[types.UID]zv1.StackSet
	recorder        kube_record.EventRecorder
	metricsReporter *core.MetricsReporter
	HealthReporter  healthcheck.Handler
	now             func() string
	sync.Mutex
	queue workqueue.TypedRateLimitingInterface[types.NamespacedName]
}

type StackSetConfig struct {
	Namespace    string
	ControllerID string

	ClusterDomains              []string
	BackendWeightsAnnotationKey string
	SyncIngressAnnotations      []string

	ReconcileWorkers int
	Interval         time.Duration

	RouteGroupSupportEnabled bool
	ConfigMapSupportEnabled  bool
	SecretSupportEnabled     bool
	PcsSupportEnabled        bool
	ForwardSupportEnabled    bool
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

func now() string {
	return time.Now().Format(time.RFC3339)
}

// NewStackSetController initializes a new StackSetController.
func NewStackSetController(
	client clientset.Interface,
	registry prometheus.Registerer,
	config StackSetConfig,
) (*StackSetController, error) {
	metricsReporter, err := core.NewMetricsReporter(registry)
	if err != nil {
		return nil, err
	}

	logger := log.WithField("controller", "stackset")

	if config.ControllerID != "" {
		logger = logger.WithField("controller_id", config.ControllerID)
	}

	return &StackSetController{
		logger:          logger,
		client:          client,
		config:          config,
		stacksetEvents:  make(chan stacksetEvent, 1),
		stacksetStore:   make(map[types.UID]zv1.StackSet),
		recorder:        recorder.CreateEventRecorder(client),
		metricsReporter: metricsReporter,
		HealthReporter:  healthcheck.NewHandler(),
		now:             now,
		queue: workqueue.NewTypedRateLimitingQueue(workqueue.NewTypedMaxOfRateLimiter(
			workqueue.NewTypedItemExponentialFailureRateLimiter[types.NamespacedName](5*time.Millisecond, 5*time.Second),
			// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
			&workqueue.TypedBucketRateLimiter[types.NamespacedName]{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		)),
	}, nil
}

func (c *StackSetController) stacksetLogger(ssc *core.StackSetContainer) *log.Entry {
	return c.logger.WithFields(map[string]interface{}{
		"namespace": ssc.StackSet.Namespace,
		"stackset":  ssc.StackSet.Name,
	})
}

func (c *StackSetController) stackLogger(ssc *core.StackSetContainer, sc *core.StackContainer) *log.Entry {
	return c.logger.WithFields(map[string]interface{}{
		"namespace": ssc.StackSet.Namespace,
		"stackset":  ssc.StackSet.Name,
		"stack":     sc.Name(),
	})
}

// Run runs the main loop of the StackSetController. Before the loops it
// sets up a watcher to watch StackSet resources. The watch will send
// changes over a channel which is polled from the main loop.
func (c *StackSetController) Run(ctx context.Context) error {
	var nextCheck time.Time

	// We're not alive if nextCheck is too far in the past
	c.HealthReporter.AddLivenessCheck("nextCheck", func() error {
		if time.Since(nextCheck) > 5*c.config.Interval {
			return fmt.Errorf("nextCheck too old")
		}
		return nil
	})

	err := c.startWatch(ctx)
	if err != nil {
		return err
	}

	http.HandleFunc("/healthz", c.HealthReporter.LiveEndpoint)

	nextCheck = time.Now().Add(-c.config.Interval)

	for {
		select {
		case <-time.After(time.Until(nextCheck)):

			nextCheck = time.Now().Add(c.config.Interval)

			stackSetContainers, err := c.collectResources(ctx)
			if err != nil {
				c.logger.Errorf("Failed to collect resources: %v", err)
				continue
			}

			var reconcileGroup errgroup.Group
			reconcileGroup.SetLimit(c.config.ReconcileWorkers)
			for stackset, container := range stackSetContainers {
				container := container
				stackset := stackset

				reconcileGroup.Go(func() error {
					if _, ok := c.stacksetStore[stackset]; ok {
						err := c.ReconcileStackSet(ctx, container)
						if err != nil {
							c.stacksetLogger(container).Errorf("unable to reconcile a stackset: %v", err)
							return c.errorEventf(container.StackSet, reasonFailedManageStackSet, err)
						}
					}
					return nil
				})
			}

			err = reconcileGroup.Wait()
			if err != nil {
				c.logger.Errorf("Failed waiting for reconcilers: %v", err)
			}
			err = c.metricsReporter.Report(stackSetContainers)
			if err != nil {
				c.logger.Errorf("Failed reporting metrics: %v", err)
			}
		case e := <-c.stacksetEvents:
			stackset := *e.StackSet
			fixupStackSetTypeMeta(&stackset)

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
			return nil
		}
	}
}

func (c *StackSetController) Run2(ctx context.Context) error {
	// defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	// TODO: health
	// TODO: metricsReporter
	c.logger.Info("Starting Controller")

	err := c.setupInformers(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup informers: %v", err)
	}

	for i := 0; i < c.config.ReconcileWorkers; i++ {
		go wait.UntilWithContext(ctx, c.worker, time.Second)
	}

	<-ctx.Done()
	c.logger.Info("Shutting down controller")
	return nil
}

func (c *StackSetController) worker(ctx context.Context) {
	for c.processNextItem(ctx) {
	}
}

func (c *StackSetController) processNextItem(ctx context.Context) bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.syncHandler(ctx, key)
	if err == nil {
		c.queue.Forget(key)
		return true
	}

	// runtime.HandleError(fmt.Errorf("sync %q failed: %v", key, err))
	c.queue.AddRateLimited(key)
	return true
}

func (c *StackSetController) syncHandler(ctx context.Context, key types.NamespacedName) error {
	obj, exists, err := c.informers.stacksetInformer.Informer().GetIndexer().GetByKey(key.String())
	if err != nil {
		return fmt.Errorf("failed to get StackSet %s from informer: %v", key.String(), err)
	}
	if !exists {
		return fmt.Errorf("StackSet %s not found in informer", key.String())
	}

	stackset, ok := obj.(*zv1.StackSet)
	if !ok {
		return fmt.Errorf("failed to cast object to StackSet for %s", key.String())
	}

	if !c.hasOwnership(stackset) {
		return nil
	}

	container, err := c.stacksetContainer(stackset.DeepCopy())
	if err != nil {
		return err
	}

	err = c.ReconcileStackSet(ctx, container)
	if err != nil {
		c.stacksetLogger(container).Errorf("unable to reconcile a stackset: %v", err)
		return err
	}
	return nil
}

// collectResources collects resources for all stacksets at once and stores them per StackSet/Stack so that we don't
// overload the API requests with unnecessary requests
func (c *StackSetController) collectResources(ctx context.Context) (map[types.UID]*core.StackSetContainer, error) {
	stacksets := make(map[types.UID]*core.StackSetContainer, len(c.stacksetStore))
	for uid, stackset := range c.stacksetStore {
		stackset := stackset

		reconciler := core.TrafficReconciler(&core.SimpleTrafficReconciler{})

		// use prescaling logic if enabled with an annotation
		if _, ok := stackset.Annotations[PrescaleStacksAnnotationKey]; ok {
			resetDelay := defaultResetMinReplicasDelay
			if resetDelayValue, ok := getResetMinReplicasDelay(stackset.Annotations); ok {
				resetDelay = resetDelayValue
			}
			reconciler = &core.PrescalingTrafficReconciler{
				ResetHPAMinReplicasTimeout: resetDelay,
			}
		}

		stacksetContainer := core.NewContainer(
			&stackset,
			reconciler,
			c.config.BackendWeightsAnnotationKey,
			c.config.ClusterDomains,
			c.config.SyncIngressAnnotations,
		)
		stacksets[uid] = stacksetContainer
	}

	err := c.collectStacks(ctx, stacksets)
	if err != nil {
		return nil, err
	}

	err = c.collectIngresses(ctx, stacksets)
	if err != nil {
		return nil, err
	}

	if c.config.RouteGroupSupportEnabled {
		err = c.collectRouteGroups(ctx, stacksets)
		if err != nil {
			return nil, err
		}
	}

	err = c.collectDeployments(ctx, stacksets)
	if err != nil {
		return nil, err
	}

	err = c.collectServices(ctx, stacksets)
	if err != nil {
		return nil, err
	}

	err = c.collectHPAs(ctx, stacksets)
	if err != nil {
		return nil, err
	}

	if c.config.ConfigMapSupportEnabled {
		err = c.collectConfigMaps(ctx, stacksets)
		if err != nil {
			return nil, err
		}
	}

	if c.config.SecretSupportEnabled {
		err = c.collectSecrets(ctx, stacksets)
		if err != nil {
			return nil, err
		}
	}

	if c.config.PcsSupportEnabled {
		err = c.collectPlatformCredentialsSet(ctx, stacksets)
		if err != nil {
			return nil, err
		}
	}

	return stacksets, nil
}

func (c *StackSetController) collectIngresses(ctx context.Context, stacksets map[types.UID]*core.StackSetContainer) error {
	ingresses, err := c.client.NetworkingV1().Ingresses(c.config.Namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		return fmt.Errorf("failed to list Ingresses: %v", err)
	}

	for _, i := range ingresses.Items {
		ingress := i
		if uid, ok := getOwnerUID(ingress.ObjectMeta); ok {
			// stackset ingress
			if s, ok := stacksets[uid]; ok {
				s.Ingress = &ingress
				continue
			}

			// stack ingress
			for _, stackset := range stacksets {
				if s, ok := stackset.StackContainers[uid]; ok {
					if strings.HasSuffix(
						ingress.ObjectMeta.Name,
						core.SegmentSuffix,
					) {
						// Traffic Segment
						s.Resources.IngressSegment = &ingress
					} else {
						s.Resources.Ingress = &ingress
					}
					break
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectRouteGroups(ctx context.Context, stacksets map[types.UID]*core.StackSetContainer) error {
	rgs, err := c.client.RouteGroupV1().RouteGroups(c.config.Namespace).List(
		ctx,
		metav1.ListOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to list RouteGroups: %v", err)
	}

	for _, rg := range rgs.Items {
		routegroup := rg
		if uid, ok := getOwnerUID(routegroup.ObjectMeta); ok {
			// stackset routegroups
			if s, ok := stacksets[uid]; ok {
				s.RouteGroup = &routegroup
				continue
			}

			// stack routegroups
			for _, stackset := range stacksets {
				if s, ok := stackset.StackContainers[uid]; ok {
					if strings.HasSuffix(
						routegroup.ObjectMeta.Name,
						core.SegmentSuffix,
					) {
						// Traffic Segment
						s.Resources.RouteGroupSegment = &routegroup
					} else {
						s.Resources.RouteGroup = &routegroup
					}
					break
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectStacks(ctx context.Context, stacksets map[types.UID]*core.StackSetContainer) error {
	stacks, err := c.client.ZalandoV1().Stacks(c.config.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Stacks: %v", err)
	}

	for _, stack := range stacks.Items {
		if uid, ok := getOwnerUID(stack.ObjectMeta); ok {
			if s, ok := stacksets[uid]; ok {
				stack := stack
				fixupStackTypeMeta(&stack)

				s.StackContainers[stack.UID] = &core.StackContainer{
					Stack: &stack,
				}
				continue
			}
		}
	}
	return nil
}

func (c *StackSetController) collectDeployments(ctx context.Context, stacksets map[types.UID]*core.StackSetContainer) error {
	deployments, err := c.client.AppsV1().Deployments(c.config.Namespace).List(ctx, metav1.ListOptions{})
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

func (c *StackSetController) collectServices(ctx context.Context, stacksets map[types.UID]*core.StackSetContainer) error {
	services, err := c.client.CoreV1().Services(c.config.Namespace).List(ctx, metav1.ListOptions{})
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

func (c *StackSetController) collectHPAs(ctx context.Context, stacksets map[types.UID]*core.StackSetContainer) error {
	hpas, err := c.client.AutoscalingV2().HorizontalPodAutoscalers(c.config.Namespace).List(ctx, metav1.ListOptions{})
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

func (c *StackSetController) collectConfigMaps(
	ctx context.Context,
	stacksets map[types.UID]*core.StackSetContainer,
) error {
	configMaps, err := c.client.CoreV1().ConfigMaps(c.config.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list ConfigMaps: %v", err)
	}

	for _, cm := range configMaps.Items {
		configMap := cm
		if uid, ok := getOwnerUID(configMap.ObjectMeta); ok {
			for _, stackset := range stacksets {
				if s, ok := stackset.StackContainers[uid]; ok {
					s.Resources.ConfigMaps = append(s.Resources.ConfigMaps, &configMap)
					break
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectSecrets(
	ctx context.Context,
	stacksets map[types.UID]*core.StackSetContainer,
) error {
	secrets, err := c.client.CoreV1().Secrets(c.config.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Secrets: %v", err)
	}

	for _, sct := range secrets.Items {
		secret := sct
		if uid, ok := getOwnerUID(secret.ObjectMeta); ok {
			for _, stackset := range stacksets {
				if s, ok := stackset.StackContainers[uid]; ok {
					s.Resources.Secrets = append(s.Resources.Secrets, &secret)
					break
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) collectPlatformCredentialsSet(
	ctx context.Context,
	stacksets map[types.UID]*core.StackSetContainer,
) error {
	platformCredentialsSets, err := c.client.ZalandoV1().PlatformCredentialsSets(c.config.Namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list PlatformCredentialsSet: %v", err)
	}

	for _, platformCredentialsSet := range platformCredentialsSets.Items {
		pcs := platformCredentialsSet
		if uid, ok := getOwnerUID(platformCredentialsSet.ObjectMeta); ok {
			for _, stackset := range stacksets {
				if s, ok := stackset.StackContainers[uid]; ok {
					s.Resources.PlatformCredentialsSets = append(
						s.Resources.PlatformCredentialsSets,
						&pcs,
					)
					break
				}
			}
		}
	}
	return nil
}

// collectResources collects resources for all stacksets at once and stores them per StackSet/Stack so that we don't
// overload the API requests with unnecessary requests
func (c *StackSetController) stacksetContainer(stackset *zv1.StackSet) (*core.StackSetContainer, error) {
	key := types.NamespacedName{
		Namespace: stackset.Namespace,
		Name:      stackset.Name,
	}

	fixupStackSetTypeMeta(stackset)

	reconciler := core.TrafficReconciler(&core.SimpleTrafficReconciler{})

	// use prescaling logic if enabled with an annotation
	if _, ok := stackset.Annotations[PrescaleStacksAnnotationKey]; ok {
		resetDelay := defaultResetMinReplicasDelay
		if resetDelayValue, ok := getResetMinReplicasDelay(stackset.Annotations); ok {
			resetDelay = resetDelayValue
		}
		reconciler = &core.PrescalingTrafficReconciler{
			ResetHPAMinReplicasTimeout: resetDelay,
		}
	}

	stacksetContainer := core.NewContainer(
		stackset,
		reconciler,
		c.config.BackendWeightsAnnotationKey,
		c.config.ClusterDomains,
		c.config.SyncIngressAnnotations,
	)

	labelSelector := labels.SelectorFromSet(labels.Set{(core.StacksetHeritageLabelKey): stackset.Name})
	err := c.addStacks(stacksetContainer, key, labelSelector)
	if err != nil {
		return nil, err
	}

	err = c.addIngresses(stacksetContainer, key, labelSelector)
	if err != nil {
		return nil, err
	}

	if c.config.RouteGroupSupportEnabled {
		err = c.addRouteGroups(stacksetContainer, key, labelSelector)
		if err != nil {
			return nil, err
		}
	}

	err = c.addDeployments(stacksetContainer, key, labelSelector)
	if err != nil {
		return nil, err
	}

	err = c.addServices(stacksetContainer, key, labelSelector)
	if err != nil {
		return nil, err
	}

	err = c.addHPAs(stacksetContainer, key, labelSelector)
	if err != nil {
		return nil, err
	}

	if c.config.ConfigMapSupportEnabled {
		err = c.addConfigMaps(stacksetContainer, key, labelSelector)
		if err != nil {
			return nil, err
		}
	}

	if c.config.SecretSupportEnabled {
		err = c.addSecrets(stacksetContainer, key, labelSelector)
		if err != nil {
			return nil, err
		}
	}

	if c.config.PcsSupportEnabled {
		err = c.addPlatformCredentialsSet(stacksetContainer, key, labelSelector)
		if err != nil {
			return nil, err
		}
	}

	return stacksetContainer, nil
}

func (c *StackSetController) addStacks(stacksetContainer *core.StackSetContainer, key types.NamespacedName, labelSelector labels.Selector) error {
	stacks, err := c.informers.stackInformer.Lister().Stacks(key.Namespace).List(labelSelector)
	if err != nil {
		return fmt.Errorf("failed to list Stacks for StackSet %s: %v", key.String(), err)
	}
	for _, stack := range stacks {
		stack := stack.DeepCopy()
		if uid, ok := getOwnerUID(stack.ObjectMeta); ok {
			if uid == stacksetContainer.StackSet.UID {
				fixupStackTypeMeta(stack)

				stacksetContainer.StackContainers[stack.UID] = &core.StackContainer{
					Stack: stack,
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) addIngresses(stacksetContainer *core.StackSetContainer, key types.NamespacedName, labelSelector labels.Selector) error {
	ingresses, err := c.informers.ingressInformer.Lister().Ingresses(key.Namespace).List(labelSelector)
	if err != nil {
		return fmt.Errorf("failed to list Ingresses for StackSet %s: %v", key.String(), err)
	}
	for _, ingress := range ingresses {
		ingress := ingress.DeepCopy()
		if uid, ok := getOwnerUID(ingress.ObjectMeta); ok {
			// stackset ingress
			if uid == stacksetContainer.StackSet.UID {
				stacksetContainer.Ingress = ingress
				continue
			}

			// stack ingress
			if s, ok := stacksetContainer.StackContainers[uid]; ok {
				if strings.HasSuffix(
					ingress.ObjectMeta.Name,
					core.SegmentSuffix,
				) {
					// Traffic Segment
					s.Resources.IngressSegment = ingress
				} else {
					s.Resources.Ingress = ingress
				}
			}
		}
	}

	return nil
}

func (c *StackSetController) addRouteGroups(stacksetContainer *core.StackSetContainer, key types.NamespacedName, labelSelector labels.Selector) error {
	routegroups, err := c.informers.routegroupInformer.Lister().RouteGroups(key.Namespace).List(labelSelector)
	if err != nil {
		return fmt.Errorf("failed to list Ingresses for StackSet %s: %v", key.String(), err)
	}
	for _, routegroup := range routegroups {
		routegroup := routegroup.DeepCopy()
		if uid, ok := getOwnerUID(routegroup.ObjectMeta); ok {
			// stackset routegroup
			if uid == stacksetContainer.StackSet.UID {
				stacksetContainer.RouteGroup = routegroup
				continue
			}

			// stack routegroup
			if s, ok := stacksetContainer.StackContainers[uid]; ok {
				if strings.HasSuffix(
					routegroup.ObjectMeta.Name,
					core.SegmentSuffix,
				) {
					// Traffic Segment
					s.Resources.RouteGroupSegment = routegroup
				} else {
					s.Resources.RouteGroup = routegroup
				}
			}
		}
	}

	return nil
}

func (c *StackSetController) addDeployments(stacksetContainer *core.StackSetContainer, key types.NamespacedName, labelSelector labels.Selector) error {
	deployments, err := c.informers.deploymentInformer.Lister().Deployments(key.Namespace).List(labelSelector)
	if err != nil {
		return fmt.Errorf("failed to list Deployments for StackSet %s: %v", key.String(), err)
	}

	for _, deployment := range deployments {
		deployment := deployment.DeepCopy()
		if uid, ok := getOwnerUID(deployment.ObjectMeta); ok {
			if s, ok := stacksetContainer.StackContainers[uid]; ok {
				s.Resources.Deployment = deployment
			}
		}
	}
	return nil
}

func (c *StackSetController) addServices(stacksetContainer *core.StackSetContainer, key types.NamespacedName, labelSelector labels.Selector) error {
	services, err := c.informers.serviceInformer.Lister().Services(key.Namespace).List(labelSelector)
	if err != nil {
		return fmt.Errorf("failed to list Services for StackSet %s: %v", key.String(), err)
	}

Items:
	for _, service := range services {
		service := service.DeepCopy()
		if uid, ok := getOwnerUID(service.ObjectMeta); ok {
			if s, ok := stacksetContainer.StackContainers[uid]; ok {
				s.Resources.Service = service
			}

			// service/HPA used to be owned by the deployment for some reason
			// TODO: check if this can be removed
			for _, stack := range stacksetContainer.StackContainers {
				if stack.Resources.Deployment != nil && stack.Resources.Deployment.UID == uid {
					stack.Resources.Service = service
					continue Items
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) addHPAs(stacksetContainer *core.StackSetContainer, key types.NamespacedName, labelSelector labels.Selector) error {
	hpas, err := c.informers.hpaInformer.Lister().HorizontalPodAutoscalers(key.Namespace).List(labelSelector)
	if err != nil {
		return fmt.Errorf("failed to list HPAs for StackSet %s: %v", key.String(), err)
	}

Items:
	for _, hpa := range hpas {
		hpa := hpa.DeepCopy()
		if uid, ok := getOwnerUID(hpa.ObjectMeta); ok {
			if s, ok := stacksetContainer.StackContainers[uid]; ok {
				s.Resources.HPA = hpa
			}

			// service/HPA used to be owned by the deployment for some reason
			// TODO: check if this can be removed
			for _, stack := range stacksetContainer.StackContainers {
				if stack.Resources.Deployment != nil && stack.Resources.Deployment.UID == uid {
					stack.Resources.HPA = hpa
					continue Items
				}
			}
		}
	}
	return nil
}

func (c *StackSetController) addConfigMaps(stacksetContainer *core.StackSetContainer, key types.NamespacedName, labelSelector labels.Selector) error {
	configMaps, err := c.informers.configMapInformer.Lister().ConfigMaps(key.Namespace).List(labelSelector)
	if err != nil {
		return fmt.Errorf("failed to list ConfigMaps for StackSet %s: %v", key.String(), err)
	}

	for _, configMap := range configMaps {
		configMap := configMap.DeepCopy()
		if uid, ok := getOwnerUID(configMap.ObjectMeta); ok {
			if s, ok := stacksetContainer.StackContainers[uid]; ok {
				s.Resources.ConfigMaps = append(s.Resources.ConfigMaps, configMap)
			}
		}
	}
	return nil
}

func (c *StackSetController) addSecrets(stacksetContainer *core.StackSetContainer, key types.NamespacedName, labelSelector labels.Selector) error {
	secrets, err := c.informers.secretInformer.Lister().Secrets(key.Namespace).List(labelSelector)
	if err != nil {
		return fmt.Errorf("failed to list Secrets for StackSet %s: %v", key.String(), err)
	}

	for _, secret := range secrets {
		secret := secret.DeepCopy()
		if uid, ok := getOwnerUID(secret.ObjectMeta); ok {
			if s, ok := stacksetContainer.StackContainers[uid]; ok {
				s.Resources.Secrets = append(s.Resources.Secrets, secret)
			}
		}
	}
	return nil
}

func (c *StackSetController) addPlatformCredentialsSet(stacksetContainer *core.StackSetContainer, key types.NamespacedName, labelSelector labels.Selector) error {
	platformCredentialsSets, err := c.informers.pcsInformer.Lister().PlatformCredentialsSets(key.Namespace).List(labelSelector)
	if err != nil {
		return fmt.Errorf("failed to list PlatformCredentialsSets for StackSet %s: %v", key.String(), err)
	}

	for _, pcs := range platformCredentialsSets {
		pcs := pcs.DeepCopy()
		if uid, ok := getOwnerUID(pcs.ObjectMeta); ok {
			if s, ok := stacksetContainer.StackContainers[uid]; ok {
				s.Resources.PlatformCredentialsSets = append(
					s.Resources.PlatformCredentialsSets,
					pcs,
				)
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
			v1.EventTypeWarning,
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
			return owner == c.config.ControllerID
		}
	}
	return c.config.ControllerID == ""
}

func (c *StackSetController) startWatch(ctx context.Context) error {
	informer := cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(c.client.ZalandoV1().RESTClient(), "stacksets", c.config.Namespace, fields.Everything()),
		&zv1.StackSet{},
		0, // skip resync
		cache.Indexers{},
	)

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.add,
		UpdateFunc: c.update,
		DeleteFunc: c.del,
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	go informer.Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("timed out waiting for caches to sync")
	}
	c.logger.Info("Synced StackSet watcher")

	return nil
}

type informerInterface interface {
	Informer() cache.SharedIndexInformer
}

func (c *StackSetController) setupInformers(ctx context.Context) error {
	factory := informers.NewSharedInformerFactoryWithOptions(c.client, 0, informers.WithNamespace(c.config.Namespace))

	stackSetFactory := stacksetinformers.NewSharedInformerFactoryWithOptions(c.client, 24*time.Hour, stacksetinformers.WithNamespace(c.config.Namespace))
	rgClient := c.client.(*clientset.Clientset)
	routegroupFactory := routegroupinformers.NewSharedInformerFactoryWithOptions(rgClient.RouteGroup, 0, routegroupinformers.WithNamespace(c.config.Namespace))

	c.informers.stacksetInformer = stackSetFactory.Zalando().V1().StackSets()
	c.informers.stackInformer = stackSetFactory.Zalando().V1().Stacks()
	c.informers.ingressInformer = factory.Networking().V1().Ingresses()
	c.informers.routegroupInformer = routegroupFactory.Zalando().V1().RouteGroups()
	c.informers.deploymentInformer = factory.Apps().V1().Deployments()
	c.informers.serviceInformer = factory.Core().V1().Services()
	c.informers.hpaInformer = factory.Autoscaling().V2().HorizontalPodAutoscalers()
	c.informers.configMapInformer = factory.Core().V1().ConfigMaps()
	c.informers.secretInformer = factory.Core().V1().Secrets()
	c.informers.pcsInformer = stackSetFactory.Zalando().V1().PlatformCredentialsSets()

	resourceInformers := []struct {
		informer informerInterface
		resync   time.Duration
	}{
		{c.informers.stacksetInformer, c.config.Interval},
		{c.informers.stackInformer, 0},
		{c.informers.ingressInformer, 0},
		{c.informers.routegroupInformer, 0},
		{c.informers.deploymentInformer, 0},
		{c.informers.serviceInformer, 0},
		{c.informers.hpaInformer, 0},
		{c.informers.configMapInformer, 0},
		{c.informers.secretInformer, 0},
		{c.informers.pcsInformer, 0},
	}

	var hasSynced []cache.InformerSynced
	for _, informer := range resourceInformers {
		_, err := informer.informer.Informer().AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
			AddFunc:    c.addResource,
			UpdateFunc: c.updateResource,
			DeleteFunc: c.deleteResource,
		},
			informer.resync)
		if err != nil {
			return fmt.Errorf("failed to add event handler: %v", err)
		}

		hasSynced = append(hasSynced, informer.informer.Informer().HasSynced)
		go informer.informer.Informer().Run(ctx.Done())
	}

	if !cache.WaitForCacheSync(ctx.Done(),
		hasSynced...,
	) {
		return fmt.Errorf("failed to sync informers")
	}

	return nil
}

func objToNamespacedName(obj any) (types.NamespacedName, bool) {
	switch typedObj := obj.(type) {
	case *zv1.StackSet:
		return types.NamespacedName{
			Namespace: typedObj.Namespace,
			Name:      typedObj.Name,
		}, true
	case *zv1.Stack, *networking.Ingress, *rgv1.RouteGroup, *appsv1.Deployment, *v1.Service, *autoscalingv2.HorizontalPodAutoscaler, *v1.ConfigMap, *v1.Secret, *zv1.PlatformCredentialsSet:
		meta, ok := obj.(metav1.ObjectMeta)
		if !ok {
			return types.NamespacedName{}, false
		}

		if stackset, ok := meta.GetLabels()[core.StacksetHeritageLabelKey]; ok {
			return types.NamespacedName{
				Namespace: meta.Namespace,
				Name:      stackset,
			}, true
		}

		return types.NamespacedName{}, false
	default:
		return types.NamespacedName{}, false
	}
}

func (c *StackSetController) addResource(obj any) {
	key, ok := objToNamespacedName(obj)
	if !ok {
		return
	}
	c.queue.Add(key)
}

func (c *StackSetController) updateResource(oldObj, newObj any) {
	c.addResource(newObj)
}

func (c *StackSetController) deleteResource(obj any) {
	stackset, ok := obj.(*zv1.StackSet)
	if !ok {
		// non-stackset deletions indicate refresh to resource
		// associated with a stackset
		c.addResource(obj)
		return
	}

	key := types.NamespacedName{
		Namespace: stackset.Namespace,
		Name:      stackset.Name,
	}

	c.queue.Forget(key)
	c.queue.Done(key)
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
func (c *StackSetController) ReconcileStatuses(ctx context.Context, ssc *core.StackSetContainer) error {
	for _, sc := range ssc.StackContainers {
		stack := sc.Stack.DeepCopy()
		status := *sc.GenerateStackStatus()
		err := retryUpdate(func(retry bool) error {
			if retry {
				updated, err := c.client.ZalandoV1().Stacks(sc.Namespace()).Get(ctx, stack.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				stack = updated
			}
			if !equality.Semantic.DeepEqual(status, stack.Status) {
				stack.Status = status
				_, err := c.client.ZalandoV1().Stacks(sc.Namespace()).UpdateStatus(ctx, stack, metav1.UpdateOptions{})
				return err
			}
			return nil
		})
		if err != nil {
			return c.errorEventf(sc.Stack, "FailedUpdateStackStatus", err)
		}
	}

	stackset := ssc.StackSet.DeepCopy()
	status := *ssc.GenerateStackSetStatus()
	err := retryUpdate(func(retry bool) error {
		if retry {
			updated, err := c.client.ZalandoV1().StackSets(ssc.StackSet.Namespace).Get(ctx, ssc.StackSet.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			stackset = updated
		}
		if !equality.Semantic.DeepEqual(status, stackset.Status) {
			stackset.Status = status
			_, err := c.client.ZalandoV1().StackSets(ssc.StackSet.Namespace).UpdateStatus(ctx, stackset, metav1.UpdateOptions{})
			return err
		}
		return nil
	})
	if err != nil {
		return c.errorEventf(ssc.StackSet, "FailedUpdateStackSetStatus", err)
	}
	return nil
}

// ReconcileTrafficSegments updates the traffic segments according to the actual
// traffic weight of each stack.
//
// Returns the ordered list of Trafic Segments that need to be updated.
func (c *StackSetController) ReconcileTrafficSegments(
	ctx context.Context,
	ssc *core.StackSetContainer,
) ([]types.UID, error) {
	// Compute segments
	toUpdate, err := ssc.ComputeTrafficSegments()
	if err != nil {
		return nil, c.errorEventf(ssc.StackSet, "FailedManageSegments", err)
	}

	return toUpdate, nil
}

// CreateCurrentStack creates a new Stack object for the current stack, if needed
func (c *StackSetController) CreateCurrentStack(ctx context.Context, ssc *core.StackSetContainer) error {
	newStack, newStackVersion := ssc.NewStack(c.config.ForwardSupportEnabled)
	if newStack == nil {
		return nil
	}

	if c.config.ConfigMapSupportEnabled || c.config.SecretSupportEnabled {
		// ensure that ConfigurationResources are prefixed by Stack name.
		if err := validateAllConfigurationResourcesNames(newStack.Stack); err != nil {
			return err
		}
	}

	created, err := c.client.ZalandoV1().Stacks(newStack.Namespace()).Create(ctx, newStack.Stack, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	fixupStackTypeMeta(created)

	c.recorder.Eventf(
		ssc.StackSet,
		v1.EventTypeNormal,
		"CreatedStack",
		"Created stack %s",
		newStack.Name(),
	)

	// Persist ObservedStackVersion in the status
	updated := ssc.StackSet.DeepCopy()
	updated.Status.ObservedStackVersion = newStackVersion

	result, err := c.client.ZalandoV1().StackSets(ssc.StackSet.Namespace).UpdateStatus(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	fixupStackSetTypeMeta(result)
	ssc.StackSet = result

	ssc.StackContainers[created.UID] = &core.StackContainer{
		Stack:          created,
		PendingRemoval: false,
		Resources:      core.StackResources{},
	}
	return nil
}

// CleanupOldStacks deletes stacks that are no longer needed.
func (c *StackSetController) CleanupOldStacks(ctx context.Context, ssc *core.StackSetContainer) error {
	for _, sc := range ssc.StackContainers {
		if !sc.PendingRemoval {
			continue
		}

		stack := sc.Stack
		err := c.client.ZalandoV1().Stacks(stack.Namespace).Delete(ctx, stack.Name, metav1.DeleteOptions{})
		if err != nil {
			return c.errorEventf(ssc.StackSet, "FailedDeleteStack", err)
		}
		c.recorder.Eventf(
			ssc.StackSet,
			v1.EventTypeNormal,
			"DeletedExcessStack",
			"Deleted excess stack %s",
			stack.Name)
	}

	return nil
}

// AddUpdateStackSetIngress reconciles the Ingress but never deletes it, it returns the existing/new Ingress
func (c *StackSetController) AddUpdateStackSetIngress(ctx context.Context, stackset *zv1.StackSet, existing *networking.Ingress, routegroup *rgv1.RouteGroup, ingress *networking.Ingress) (*networking.Ingress, error) {
	// Ingress removed, handled outside
	if ingress == nil {
		return existing, nil
	}

	if existing == nil {
		if ingress.Annotations == nil {
			ingress.Annotations = make(map[string]string)
		}
		ingress.Annotations[ControllerLastUpdatedAnnotationKey] = c.now()

		createdIng, err := c.client.NetworkingV1().Ingresses(ingress.Namespace).Create(ctx, ingress, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
		c.recorder.Eventf(
			stackset,
			v1.EventTypeNormal,
			"CreatedIngress",
			"Created Ingress %s",
			ingress.Name)
		return createdIng, nil
	}

	lastUpdateValue, existingHaveUpdateTimeStamp := existing.Annotations[ControllerLastUpdatedAnnotationKey]
	if existingHaveUpdateTimeStamp {
		delete(existing.Annotations, ControllerLastUpdatedAnnotationKey)
	}

	// Check if we need to update the Ingress
	if existingHaveUpdateTimeStamp && equality.Semantic.DeepDerivative(ingress.Spec, existing.Spec) &&
		equality.Semantic.DeepEqual(ingress.Annotations, existing.Annotations) &&
		equality.Semantic.DeepEqual(ingress.Labels, existing.Labels) {
		// add the annotation back after comparing
		existing.Annotations[ControllerLastUpdatedAnnotationKey] = lastUpdateValue
		return existing, nil
	}

	updated := existing.DeepCopy()
	updated.Spec = ingress.Spec
	if ingress.Annotations != nil {
		updated.Annotations = ingress.Annotations
	} else {
		updated.Annotations = make(map[string]string)
	}
	updated.Annotations[ControllerLastUpdatedAnnotationKey] = c.now()

	updated.Labels = ingress.Labels

	createdIngress, err := c.client.NetworkingV1().Ingresses(updated.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	c.recorder.Eventf(
		stackset,
		v1.EventTypeNormal,
		"UpdatedIngress",
		"Updated Ingress %s",
		ingress.Name)
	return createdIngress, nil
}

// AddUpdateStackSetRouteGroup reconciles the RouteGroup but never deletes it, it returns the existing/new RouteGroup
func (c *StackSetController) AddUpdateStackSetRouteGroup(ctx context.Context, stackset *zv1.StackSet, existing *rgv1.RouteGroup, ingress *networking.Ingress, rg *rgv1.RouteGroup) (*rgv1.RouteGroup, error) {
	// RouteGroup removed, handled outside
	if rg == nil {
		return existing, nil
	}

	// Create new RouteGroup
	if existing == nil {
		if rg.Annotations == nil {
			rg.Annotations = make(map[string]string)
		}
		rg.Annotations[ControllerLastUpdatedAnnotationKey] = c.now()

		createdRg, err := c.client.RouteGroupV1().RouteGroups(rg.Namespace).Create(ctx, rg, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
		c.recorder.Eventf(
			stackset,
			v1.EventTypeNormal,
			"CreatedRouteGroup",
			"Created RouteGroup %s",
			rg.Name)
		return createdRg, nil
	}

	lastUpdateValue, existingHaveUpdateTimeStamp := existing.Annotations[ControllerLastUpdatedAnnotationKey]
	if existingHaveUpdateTimeStamp {
		delete(existing.Annotations, ControllerLastUpdatedAnnotationKey)
	}

	// Check if we need to update the RouteGroup
	if existingHaveUpdateTimeStamp && equality.Semantic.DeepDerivative(rg.Spec, existing.Spec) &&
		equality.Semantic.DeepEqual(rg.Annotations, existing.Annotations) &&
		equality.Semantic.DeepEqual(rg.Labels, existing.Labels) {
		// add the annotation back after comparing
		existing.Annotations[ControllerLastUpdatedAnnotationKey] = lastUpdateValue
		return existing, nil
	}

	updated := existing.DeepCopy()
	updated.Spec = rg.Spec
	if rg.Annotations != nil {
		updated.Annotations = rg.Annotations
	} else {
		updated.Annotations = make(map[string]string)
	}
	updated.Annotations[ControllerLastUpdatedAnnotationKey] = c.now()

	updated.Labels = rg.Labels

	createdRg, err := c.client.RouteGroupV1().RouteGroups(updated.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	c.recorder.Eventf(
		stackset,
		v1.EventTypeNormal,
		"UpdatedRouteGroup",
		"Updated RouteGroup %s",
		rg.Name)
	return createdRg, nil
}

// RecordTrafficSwitch records an event detailing when switches in traffic to
// Stacks, only when there are changes to record.
func (c *StackSetController) RecordTrafficSwitch(ctx context.Context, ssc *core.StackSetContainer) error {
	trafficChanges := ssc.TrafficChanges()
	if len(trafficChanges) != 0 {
		var changeMessages []string
		for _, change := range trafficChanges {
			changeMessages = append(changeMessages, change.String())
		}

		c.recorder.Eventf(
			ssc.StackSet,
			v1.EventTypeNormal,
			"TrafficSwitched",
			"Switched traffic: %s",
			strings.Join(changeMessages, ", "))
	}

	return nil
}

func (c *StackSetController) ReconcileStackSetDesiredTraffic(ctx context.Context, existing *zv1.StackSet, generateUpdated func() []*zv1.DesiredTraffic) error {
	updatedTraffic := generateUpdated()

	if equality.Semantic.DeepEqual(existing.Spec.Traffic, updatedTraffic) {
		return nil
	}

	updated := existing.DeepCopy()
	updated.Spec.Traffic = updatedTraffic

	_, err := c.client.ZalandoV1().StackSets(updated.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		updated,
		v1.EventTypeNormal,
		"UpdatedStackSet",
		"Updated StackSet %s",
		updated.Name)
	return nil
}

func (c *StackSetController) ReconcileStackResources(ctx context.Context, ssc *core.StackSetContainer, sc *core.StackContainer) error {
	err := c.ReconcileStackIngress(ctx, sc.Stack, sc.Resources.Ingress, sc.GenerateIngress)
	if err != nil {
		return c.errorEventf(sc.Stack, "FailedManageIngress", err)
	}

	err = c.ReconcileStackIngress(
		ctx,
		sc.Stack,
		sc.Resources.IngressSegment,
		sc.GenerateIngressSegment,
	)
	if err != nil {
		return c.errorEventf(sc.Stack, "FailedManageIngressSegment", err)
	}

	if c.config.RouteGroupSupportEnabled {
		err = c.ReconcileStackRouteGroup(ctx, sc.Stack, sc.Resources.RouteGroup, sc.GenerateRouteGroup)
		if err != nil {
			return c.errorEventf(sc.Stack, "FailedManageRouteGroup", err)
		}

		err = c.ReconcileStackRouteGroup(
			ctx,
			sc.Stack,
			sc.Resources.RouteGroupSegment,
			sc.GenerateRouteGroupSegment,
		)
		if err != nil {
			return c.errorEventf(
				sc.Stack,
				"FailedManageRouteGroupSegment",
				err,
			)
		}
	}

	if c.config.ConfigMapSupportEnabled {
		err := c.ReconcileStackConfigMapRefs(ctx, sc.Stack, sc.UpdateObjectMeta)
		if err != nil {
			return c.errorEventf(sc.Stack, "FailedManageConfigMapRefs", err)
		}
	}

	if c.config.SecretSupportEnabled {
		err := c.ReconcileStackSecretRefs(ctx, sc.Stack, sc.UpdateObjectMeta)
		if err != nil {
			return c.errorEventf(sc.Stack, "FailedManageSecretRefs", err)
		}
	}

	if c.config.PcsSupportEnabled {
		err = c.ReconcileStackPlatformCredentialsSets(
			ctx,
			sc.Stack,
			sc.Resources.PlatformCredentialsSets,
			sc.GeneratePlatformCredentialsSet,
		)
		if err != nil {
			return c.errorEventf(sc.Stack, "FailedManagePlatformCredentialsSet", err)
		}
	}

	err = c.ReconcileStackDeployment(ctx, sc.Stack, sc.Resources.Deployment, sc.GenerateDeployment)
	if err != nil {
		return c.errorEventf(sc.Stack, "FailedManageDeployment", err)
	}

	hpaGenerator := sc.GenerateHPA
	err = c.ReconcileStackHPA(ctx, sc.Stack, sc.Resources.HPA, hpaGenerator)
	if err != nil {
		return c.errorEventf(sc.Stack, "FailedManageHPA", err)
	}

	err = c.ReconcileStackService(ctx, sc.Stack, sc.Resources.Service, sc.GenerateService)
	if err != nil {
		return c.errorEventf(sc.Stack, "FailedManageService", err)
	}

	return nil
}

// ReconcileStackSet reconciles all the things from a stackset
func (c *StackSetController) ReconcileStackSet(ctx context.Context, container *core.StackSetContainer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			c.metricsReporter.ReportPanic()
			c.stacksetLogger(container).Errorf("Encountered a panic while processing a stackset: %v\n%s", r, debug.Stack())
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	var errors []error

	// Create current stack, if needed. Proceed on errors.
	err = c.CreateCurrentStack(ctx, container)
	if err != nil {
		err = c.errorEventf(container.StackSet, "FailedCreateStack", err)
		c.stacksetLogger(container).Errorf("Unable to create stack: %v", err)
		errors = append(errors, err)
	}

	// Update statuses from external resources (ingresses, deployments, etc). Abort on errors.
	err = container.UpdateFromResources()
	if err != nil {
		c.recorder.Eventf(
			container.StackSet,
			v1.EventTypeWarning,
			"FailedUpdateFromResources",
			"Failed to update from resources: "+err.Error())
		return err
	}

	// Update the stacks with the currently selected traffic reconciler. Proceed on errors.
	err = container.ManageTraffic(time.Now())
	if err != nil {
		c.stacksetLogger(container).Errorf("Traffic reconciliation failed: %v", err)
		c.recorder.Eventf(
			container.StackSet,
			v1.EventTypeWarning,
			"TrafficNotSwitched",
			"Failed to switch traffic: "+err.Error())
		errors = append(errors, err)
	}

	// Mark stacks that should be removed
	container.MarkExpiredStacks()

	// Update traffic segments. Proceed on errors.
	segsInOrder, err := c.ReconcileTrafficSegments(ctx, container)
	if err != nil {
		err = c.errorEventf(
			container.StackSet,
			reasonFailedManageStackSet,
			err,
		)
		c.stacksetLogger(container).Errorf(
			"Unable to reconcile traffic segments: %v",
			err,
		)
		errors = append(errors, err)
	}

	// Reconcile stack resources. Proceed on errors.
	reconciledStacks := map[types.UID]bool{}
	for _, id := range segsInOrder {
		reconciledStacks[id] = true
		sc := container.StackContainers[id]
		err = c.ReconcileStackResources(ctx, container, sc)
		if err != nil {
			err = c.errorEventf(sc.Stack, "FailedManageStack", err)
			c.stackLogger(container, sc).Errorf(
				"Unable to reconcile stack resources: %v",
				err,
			)
			errors = append(errors, err)
		}
	}

	for k, sc := range container.StackContainers {
		if reconciledStacks[k] {
			continue
		}

		err = c.ReconcileStackResources(ctx, container, sc)
		if err != nil {
			err = c.errorEventf(sc.Stack, "FailedManageStack", err)
			c.stackLogger(container, sc).Errorf("Unable to reconcile stack resources: %v", err)
			errors = append(errors, err)
		}
	}

	// Reconcile stackset resources (update ingress and/or routegroups). Proceed on errors.
	err = c.RecordTrafficSwitch(ctx, container)
	if err != nil {
		err = c.errorEventf(container.StackSet, reasonFailedManageStackSet, err)
		c.stacksetLogger(container).Errorf("Unable to reconcile stackset resources: %v", err)
		errors = append(errors, err)
	}

	// Reconcile desired traffic in the stackset. Proceed on errors.
	err = c.ReconcileStackSetDesiredTraffic(ctx, container.StackSet, container.GenerateStackSetTraffic)
	if err != nil {
		err = c.errorEventf(container.StackSet, reasonFailedManageStackSet, err)
		c.stacksetLogger(container).Errorf("Unable to reconcile stackset traffic: %v", err)
		errors = append(errors, err)
	}

	// Delete old stacks. Proceed on errors.
	err = c.CleanupOldStacks(ctx, container)
	if err != nil {
		err = c.errorEventf(container.StackSet, reasonFailedManageStackSet, err)
		c.stacksetLogger(container).Errorf("Unable to delete old stacks: %v", err)
		errors = append(errors, err)
	}

	// Update statuses.
	err = c.ReconcileStatuses(ctx, container)
	if err != nil {
		c.recorder.Eventf(
			container.StackSet,
			v1.EventTypeWarning,
			"FailedUpdateStatuses",
			"Failed to update statuses: "+err.Error())
		return err
	}

	if len(errors) > 0 {
		return fmt.Errorf("encountered %d errors during reconciliation, see events/logs for details", len(errors))
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

func fixupStackSetTypeMeta(stackset *zv1.StackSet) {
	// set TypeMeta manually because of this bug:
	// https://github.com/kubernetes/client-go/issues/308
	stackset.APIVersion = core.APIVersion
	stackset.Kind = core.KindStackSet
}

func fixupStackTypeMeta(stack *zv1.Stack) {
	// set TypeMeta manually because of this bug:
	// https://github.com/kubernetes/client-go/issues/308
	stack.APIVersion = core.APIVersion
	stack.Kind = core.KindStack
}

// validateConfigurationResourcesNames returns an error if any ConfigurationResource
// name is not prefixed by Stack name.
func validateAllConfigurationResourcesNames(stack *zv1.Stack) error {
	for _, rsc := range stack.Spec.ConfigurationResources {
		if err := validateConfigurationResourceName(stack.Name, rsc.GetName()); err != nil {
			return err
		}
	}
	return nil
}

// validateConfigurationResourceName returns an error if specific resource
// name is not prefixed by Stack name.
func validateConfigurationResourceName(stack string, rsc string) error {
	if !strings.HasPrefix(rsc, stack) {
		return fmt.Errorf(configurationResourceNameError, rsc, stack)
	}
	return nil
}
