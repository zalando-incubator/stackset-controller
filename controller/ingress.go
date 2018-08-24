package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	log "github.com/sirupsen/logrus"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando/v1"
	clientset "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

const (
	stackTrafficWeightsAnnotationKey = "zalando.org/stack-traffic-weights"
	backendWeightsAnnotationKey      = "zalando.org/backend-weights"
)

// IngressController is a controller that can manage ingresses for an
// stackset.
type IngressController struct {
	logger    *log.Entry
	kube      kubernetes.Interface
	appClient clientset.Interface
	stackset  zv1.StackSet
	done      chan<- struct{}
	interval  time.Duration
}

// NewIngressController initializes a new IngressController.
func NewIngressController(client kubernetes.Interface, appClient clientset.Interface, stackset zv1.StackSet, done chan<- struct{}, interval time.Duration) *IngressController {
	return &IngressController{
		logger: log.WithFields(
			log.Fields{
				"controller": "ingress",
				"stackset":   stackset.Name,
				"namespace":  stackset.Namespace,
			},
		),
		kube:      client,
		appClient: appClient,
		stackset:  stackset,
		done:      done,
		interval:  interval,
	}
}

// Run runs the main loop of the IngressController.
func (c *IngressController) Run(ctx context.Context) {
	for {
		err := c.runOnce()
		if err != nil {
			c.logger.Error(err)
		}

		select {
		case <-time.After(c.interval):
		case <-ctx.Done():
			c.logger.Info("Terminating Ingress Controller.")
			c.done <- struct{}{}
			return
		}
	}
}

func (c *IngressController) runOnce() error {
	ing, err := getIngress(c.kube, &c.stackset)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	heritageLabels := map[string]string{
		stacksetHeritageLabelKey: c.stackset.Name,
	}
	opts := metav1.ListOptions{
		LabelSelector: labels.Set(heritageLabels).String(),
	}

	stacks, err := c.appClient.ZalandoV1().Stacks(c.stackset.Namespace).List(opts)
	if err != nil {
		return fmt.Errorf("failed to list StackSet stacks of StackSet %s/%s: %v", c.stackset.Namespace, c.stackset.Name, err)
	}

	// cleanup Ingress if ingress is disabled.
	if c.stackset.Spec.Ingress == nil {
		if ing != nil {
			c.logger.Infof(
				"Deleting obsolete Ingress %s/%s for StackSet %s/%s",
				ing.Namespace,
				ing.Name,
				c.stackset.Namespace,
				c.stackset.Name,
			)
			err := c.kube.ExtensionsV1beta1().Ingresses(ing.Namespace).Delete(ing.Name, nil)
			if err != nil {
				return fmt.Errorf(
					"failed to delete Ingress %s/%s for StackSet %s/%s: %v",
					ing.Namespace,
					ing.Name,
					c.stackset.Namespace,
					c.stackset.Name,
					err,
				)
			}

			// cleanup any per stack ingresses.
			for _, stack := range stacks.Items {
				err := c.gcStackIngress(stack)
				if err != nil {
					log.Error(err)
					continue
				}
			}
		}
		return nil
	}

	stackStatuses, err := c.getStackStatuses(stacks.Items)
	if err != nil {
		return fmt.Errorf("failed to get Stack statuses for StackSet %s/%s: %v", c.stackset.Namespace, c.stackset.Name, err)
	}

	ingress, err := ingressForStackSet(&c.stackset, ing, stackStatuses)
	if err != nil {
		return fmt.Errorf("failed to generate Ingress for StackSet %s/%s: %v", c.stackset.Namespace, c.stackset.Name, err)
	}

	if ing == nil {
		c.logger.Infof("Creating Ingress %s/%s with %d service backend(s).", ingress.Namespace, ingress.Name, len(stacks.Items))
		_, err := c.kube.ExtensionsV1beta1().Ingresses(ingress.Namespace).Create(ingress)
		if err != nil {
			return err
		}
	} else {
		ing.Status = v1beta1.IngressStatus{}
		if !reflect.DeepEqual(ing, ingress) {
			c.logger.Debugf("Ingress %s/%s changed: %s", ingress.Namespace, ingress.Name, cmp.Diff(ing, ingress))
			c.logger.Infof("Updating Ingress %s/%s with %d service backend(s).", ingress.Namespace, ingress.Name, len(stacks.Items))
			_, err := c.kube.ExtensionsV1beta1().Ingresses(ingress.Namespace).Update(ingress)
			if err != nil {
				return err
			}
		}
	}

	// create per stack ingress resources in order to have per stack
	// hostnames. The ingress created will be owned by the stack and thus
	// will automatically get deleted when the corresponding stack is
	// deleted.
	// Because of how the traffic switching works in skipper we can't just
	// have a single ingress with all the hostnames, as the traffic would
	// apply to all host rules, even though we don't want traffic switching
	// for the per stack hostnames. For this reason we must create extra
	// ingresses per stack.
	for _, stack := range stacks.Items {
		// don't manage ingress for terminating stack.
		if stack.DeletionTimestamp != nil && stack.DeletionTimestamp.Time.Before(time.Now().UTC()) {
			continue
		}

		err := c.stackIngress(stack)
		if err != nil {
			log.Error(err)
			continue
		}
	}

	return nil
}

type stackStatus struct {
	Stack     zv1.Stack
	Available bool
}

func (c *IngressController) getStackStatuses(stacks []zv1.Stack) ([]stackStatus, error) {
	statuses := make([]stackStatus, 0, len(stacks))
	for _, stack := range stacks {
		status := stackStatus{
			Stack: stack,
		}

		// check that service has at least one endpoint, otherwise it
		// should not get traffic.
		endpoints, err := c.kube.CoreV1().Endpoints(stack.Namespace).Get(stack.Name, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf(
					"failed to get Endpoints for stack %s/%s: %v",
					stack.Namespace,
					stack.Name,
					err,
				)
			}
			status.Available = false
		} else {
			readyEndpoints := 0
			for _, subset := range endpoints.Subsets {
				readyEndpoints += len(subset.Addresses)
			}

			status.Available = readyEndpoints > 0
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

func (c *IngressController) stackIngress(stack zv1.Stack) error {
	ingress, err := ingressForStack(&c.stackset, &stack)
	if err != nil {
		return fmt.Errorf("failed generate Ingress for Stack %s/%s: %s", stack.Namespace, stack.Name, err)
	}

	ing, err := c.kube.ExtensionsV1beta1().Ingresses(ingress.Namespace).Get(ingress.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get Ingress %s/%s: %s", ingress.Namespace, ingress.Name, err)
		}
		ing = nil
	}

	if ing == nil {
		c.logger.Infof("Creating Ingress %s/%s", ingress.Namespace, ingress.Name)
		_, err := c.kube.ExtensionsV1beta1().Ingresses(ingress.Namespace).Create(ingress)
		if err != nil {
			return fmt.Errorf("failed to create Ingress %s/%s: %s", ingress.Namespace, ingress.Name, err)
		}
	} else {
		// check if ingress is already owned by a different resource.
		if !isOwnedReference(stack.TypeMeta, stack.ObjectMeta, ing.ObjectMeta) {
			return fmt.Errorf("Ingress %s/%s already has a different owner: %v", ing.Namespace, ing.Name, ing.ObjectMeta.OwnerReferences)
		}

		// add objectMeta from existing ingress
		ingress.SelfLink = ing.SelfLink
		ingress.UID = ing.UID
		ingress.Generation = ing.Generation
		ingress.CreationTimestamp = ing.CreationTimestamp
		ingress.ResourceVersion = ing.ResourceVersion
		ing.Status = v1beta1.IngressStatus{}

		if !reflect.DeepEqual(ing, ingress) {
			c.logger.Debugf("Ingress %s/%s changed: %s", ingress.Namespace, ingress.Name, cmp.Diff(ing, ingress))
			c.logger.Infof("Updating Ingress %s/%s.", ingress.Namespace, ingress.Name)
			_, err := c.kube.ExtensionsV1beta1().Ingresses(ingress.Namespace).Update(ingress)
			if err != nil {
				return fmt.Errorf("failed to update Ingress %s/%s: %v", ingress.Namespace, ingress.Name, err)
			}
		}
	}

	return nil
}

func (c *IngressController) gcStackIngress(stack zv1.Stack) error {
	ing, err := c.kube.ExtensionsV1beta1().Ingresses(stack.Namespace).Get(stack.Name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get Ingress %s/%s: %s", stack.Namespace, stack.Name, err)
		}
		return nil
	}

	// check if ingress is already owned by a different resource.
	if !isOwnedReference(stack.TypeMeta, stack.ObjectMeta, ing.ObjectMeta) {
		return fmt.Errorf("Ingress %s/%s already has a different owner: %v", ing.Namespace, ing.Name, ing.ObjectMeta.OwnerReferences)
	}

	c.logger.Infof("Deleting obsolete Ingress %s/%s.", ing.Namespace, ing.Name)
	err = c.kube.ExtensionsV1beta1().Ingresses(ing.Namespace).Delete(ing.Name, nil)
	if err != nil {
		return fmt.Errorf("failed to delete Ingress %s/%s: %v", ing.Namespace, ing.Name, err)
	}

	return nil
}

// ingressForStack generates an ingress object based on a stack.
func ingressForStack(stackset *zv1.StackSet, stack *zv1.Stack) (*v1beta1.Ingress, error) {
	ingress := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stack.Name,
			Namespace: stack.Namespace,
			Labels:    stack.Labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: stack.APIVersion,
					Kind:       stack.Kind,
					Name:       stack.Name,
					UID:        stack.UID,
				},
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: make([]v1beta1.IngressRule, 0),
		},
	}

	if stackset.Spec.Ingress.Annotations != nil {
		ingress.Annotations = map[string]string{}
	}

	// insert annotations
	for k, v := range stackset.Spec.Ingress.Annotations {
		ingress.Annotations[k] = v
	}

	rule := v1beta1.IngressRule{
		IngressRuleValue: v1beta1.IngressRuleValue{
			HTTP: &v1beta1.HTTPIngressRuleValue{
				Paths: make([]v1beta1.HTTPIngressPath, 0),
			},
		},
	}

	path := v1beta1.HTTPIngressPath{
		Path: stackset.Spec.Ingress.Path,
		Backend: v1beta1.IngressBackend{
			ServiceName: stack.Name,
			ServicePort: stackset.Spec.Ingress.BackendPort,
		},
	}
	rule.IngressRuleValue.HTTP.Paths = append(rule.IngressRuleValue.HTTP.Paths, path)

	// create rule per hostname
	for _, host := range stackset.Spec.Ingress.Hosts {
		r := rule
		newHost, err := createSubdomain(host, stack.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to create domain name: %s", err)
		}
		r.Host = newHost
		ingress.Spec.Rules = append(ingress.Spec.Rules, r)
	}

	return ingress, nil
}

// ingressForStackSet
func ingressForStackSet(stackset *zv1.StackSet, origIngress *v1beta1.Ingress, stackStatuses []stackStatus) (*v1beta1.Ingress, error) {
	heritageLabels := map[string]string{
		stacksetHeritageLabelKey: stackset.Name,
	}

	labels := mergeLabels(
		heritageLabels,
		stackset.Labels,
	)

	ingress := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        stackset.Name,
			Namespace:   stackset.Namespace,
			Labels:      labels,
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: stackset.APIVersion,
					Kind:       stackset.Kind,
					Name:       stackset.Name,
					UID:        stackset.UID,
				},
			},
		},
		Spec: v1beta1.IngressSpec{
			Rules: make([]v1beta1.IngressRule, 0),
		},
	}

	// insert annotations
	for k, v := range stackset.Spec.Ingress.Annotations {
		ingress.Annotations[k] = v
	}

	// set ObjectMeta from the exisiting ingress resource
	// this is done to ensure we only update the resource if something
	// changes.
	if origIngress != nil {
		ingress.SelfLink = origIngress.SelfLink
		ingress.UID = origIngress.UID
		ingress.Generation = origIngress.Generation
		ingress.CreationTimestamp = origIngress.CreationTimestamp
		ingress.ResourceVersion = origIngress.ResourceVersion
	}

	rule := v1beta1.IngressRule{
		IngressRuleValue: v1beta1.IngressRuleValue{
			HTTP: &v1beta1.HTTPIngressRuleValue{
				Paths: make([]v1beta1.HTTPIngressPath, 0),
			},
		},
	}

	// get current stack traffic weights stored on ingress.
	currentWeights := make(map[string]float64, len(stackStatuses))
	if origIngress != nil {
		if weights, ok := origIngress.Annotations[stackTrafficWeightsAnnotationKey]; ok {
			err := json.Unmarshal([]byte(weights), &currentWeights)
			if err != nil {
				return nil, fmt.Errorf("failed to get current Stack traffic weights: %v", err)
			}
		}
	}

	availableWeights, allWeights := computeBackendWeights(stackStatuses, currentWeights)

	for backend, traffic := range availableWeights {
		if traffic > 0 {
			path := v1beta1.HTTPIngressPath{
				Path: stackset.Spec.Ingress.Path,
				Backend: v1beta1.IngressBackend{
					ServiceName: backend,
					ServicePort: stackset.Spec.Ingress.BackendPort,
				},
			}
			rule.IngressRuleValue.HTTP.Paths = append(rule.IngressRuleValue.HTTP.Paths, path)
		}
	}

	// sort backends by name to have a consitent generated ingress
	// resource.
	sort.Slice(rule.IngressRuleValue.HTTP.Paths, func(i, j int) bool {
		return rule.IngressRuleValue.HTTP.Paths[i].Backend.ServiceName < rule.IngressRuleValue.HTTP.Paths[j].Backend.ServiceName
	})

	// create rule per hostname
	for _, host := range stackset.Spec.Ingress.Hosts {
		r := rule
		r.Host = host
		ingress.Spec.Rules = append(ingress.Spec.Rules, r)
	}

	availableWeightsData, err := json.Marshal(&availableWeights)
	if err != nil {
		return nil, err
	}

	allWeightsData, err := json.Marshal(&allWeights)
	if err != nil {
		return nil, err
	}

	if ingress.Annotations == nil {
		ingress.Annotations = map[string]string{}
	}

	ingress.Annotations[backendWeightsAnnotationKey] = string(availableWeightsData)
	ingress.Annotations[stackTrafficWeightsAnnotationKey] = string(allWeightsData)

	return ingress, nil
}

// allZero returns true if all weights defined in the map are 0.
func allZero(weights map[string]float64) bool {
	for _, weight := range weights {
		if weight > 0 {
			return false
		}
	}
	return true
}

// normalizeWeights normalizes a map of backend weights.
// If all weights are zero the total weight of 100 is distributed equally
// between all backends.
// If not all weights are zero they are normalized to a sum of 100.
// Note this modifies the passed map inplace instead of returning a modified
// copy.
func normalizeWeights(backendWeights map[string]float64) {
	// if all weights are zero distribute them equally to all backends
	if allZero(backendWeights) && len(backendWeights) > 0 {
		eqWeight := 100 / float64(len(backendWeights))
		for backend := range backendWeights {
			backendWeights[backend] = eqWeight
		}
		return
	}

	// if not all weights are zero, normalize them to a sum of 100
	sum := float64(0)
	for _, weight := range backendWeights {
		sum += weight
	}

	for backend, weight := range backendWeights {
		backendWeights[backend] = weight / sum * 100
	}
}

func computeBackendWeights(stacks []stackStatus, traffic map[string]float64) (map[string]float64, map[string]float64) {
	backendWeights := make(map[string]float64, len(stacks))
	availableBackends := make(map[string]float64, len(stacks))
	for _, stack := range stacks {
		backendWeights[stack.Stack.Name] = traffic[stack.Stack.Name]

		if stack.Available {
			availableBackends[stack.Stack.Name] = traffic[stack.Stack.Name]
		}
	}

	// TODO: validate this logic
	if !allZero(backendWeights) {
		normalizeWeights(backendWeights)
	}

	if len(availableBackends) == 0 {
		availableBackends = backendWeights
	}

	// TODO: think of case were all are zero and the service/deployment is
	// deleted.
	normalizeWeights(availableBackends)

	return availableBackends, backendWeights
}

func getIngress(client kubernetes.Interface, stackset *zv1.StackSet) (*v1beta1.Ingress, error) {
	// check for existing ingress object
	ing, err := client.ExtensionsV1beta1().Ingresses(stackset.Namespace).Get(stackset.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// check if ingress is owned by the stackset resource
	if !isOwnedReference(stackset.TypeMeta, stackset.ObjectMeta, ing.ObjectMeta) {
		return nil, fmt.Errorf(
			"found Ingress '%s/%s' not managed by the StackSet %s/%s",
			ing.Namespace,
			ing.Name,
			stackset.Namespace,
			stackset.Name,
		)
	}

	return ing, nil
}

// isOwnedReference returns true of the dependent object is owned by the owner
// object.
func isOwnedReference(ownerTypeMeta metav1.TypeMeta, ownerObjectMeta, dependent metav1.ObjectMeta) bool {
	for _, ref := range dependent.OwnerReferences {
		if ref.APIVersion == ownerTypeMeta.APIVersion &&
			ref.Kind == ownerTypeMeta.Kind &&
			ref.UID == ownerObjectMeta.UID &&
			ref.Name == ownerObjectMeta.Name {
			return true
		}
	}
	return false
}

// createSubdomain creates a subdomain giving an existing domain by replacing
// the first section of the domain. E.g. given the domain: my-app.example.org
// and the subdomain part my-new-app the resulting domain will be
// my-new-app.example.org.
func createSubdomain(domain, subdomain string) (string, error) {
	names := strings.SplitN(domain, ".", 2)
	if len(names) != 2 {
		return "", fmt.Errorf("unexpected domain format: %s", domain)
	}

	names[0] = subdomain

	return strings.Join(names, "."), nil
}
