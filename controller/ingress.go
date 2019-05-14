package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/go-cmp/cmp"
	log "github.com/sirupsen/logrus"
	"github.com/zalando-incubator/stackset-controller/controller/entities"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kube_record "k8s.io/client-go/tools/record"
)

const (
	stackTrafficWeightsAnnotationKey = "zalando.org/stack-traffic-weights"
	backendWeightsAnnotationKey      = "zalando.org/backend-weights"
)

var (
	errNoPaths = errors.New("invalid ingress, no paths defined")
)

// ingressReconciler is able to bring Ingresses of a StackSet to the desired
// state.
type ingressReconciler struct {
	logger   *log.Entry
	client   clientset.Interface
	recorder kube_record.EventRecorder
}

// ReconcileIngress brings Ingresses of a StackSet to the desired state.
func (c *StackSetController) ReconcileIngress(sc entities.StackSetContainer) error {
	ir := c.newIngressReconciler(sc)
	return ir.reconcile(sc)
}

func (c *StackSetController) newIngressReconciler(sc entities.StackSetContainer) *ingressReconciler {
	return &ingressReconciler{
		logger: c.logger.WithFields(
			log.Fields{
				"controller": "ingress",
				"stackset":   sc.StackSet.Name,
				"namespace":  sc.StackSet.Namespace,
			},
		),
		client:   c.client,
		recorder: c.recorder,
	}
}

func (c *ingressReconciler) reconcile(sc entities.StackSetContainer) error {
	stacks := sc.Stacks()

	// cleanup Ingress if ingress is disabled.
	if sc.StackSet.Spec.Ingress == nil {
		if sc.Ingress != nil {
			err := c.client.ExtensionsV1beta1().Ingresses(sc.Ingress.Namespace).Delete(sc.Ingress.Name, nil)
			if err != nil {
				c.recorder.Eventf(&sc.StackSet,
					apiv1.EventTypeWarning,
					"DeleteIngress",
					"Failed to delete Ingress %s/%s for StackSet %s/%s: %v",
					sc.Ingress.Namespace,
					sc.Ingress.Name,
					sc.StackSet.Namespace,
					sc.StackSet.Name,
					err,
				)
				return err
			}
			c.recorder.Eventf(&sc.StackSet,
				apiv1.EventTypeNormal,
				"DeletedIngress",
				"Deleted obsolete Ingress %s/%s for StackSet %s/%s",
				sc.Ingress.Namespace,
				sc.Ingress.Name,
				sc.StackSet.Namespace,
				sc.StackSet.Name,
			)

			// cleanup any per stack ingresses.
			for _, stack := range stacks {
				err := c.gcStackIngress(stack)
				if err != nil {
					continue
				}
			}
		}
		return nil
	}

	ingress, err := c.ingressForStackSet(sc, sc.Ingress)
	if err != nil {
		if err == errNoPaths {
			return nil
		}
		c.recorder.Eventf(&sc.StackSet,
			apiv1.EventTypeWarning,
			"GenerateIngress",
			"Failed to generate Ingress for StackSet %s/%s: %v",
			sc.StackSet.Namespace,
			sc.StackSet.Name,
			err,
		)
		return err
	}

	err = trafficSwitchingAnnotationsForIngress(sc, ingress)

	if sc.Ingress == nil {
		_, err := c.client.ExtensionsV1beta1().Ingresses(ingress.Namespace).Create(ingress)
		if err != nil {
			return err
		}
		c.recorder.Eventf(&sc.StackSet,
			apiv1.EventTypeNormal,
			"CreatedIngress",
			"Created Ingress %s/%s with %d service backend(s).",
			ingress.Namespace,
			ingress.Name,
			len(stacks),
		)
	} else {
		sc.Ingress.Status = v1beta1.IngressStatus{}
		if !equality.Semantic.DeepEqual(sc.Ingress, ingress) {
			c.logger.Debugf("Ingress %s/%s changed: %s", ingress.Namespace, ingress.Name, cmp.Diff(sc.Ingress, ingress))
			_, err := c.client.ExtensionsV1beta1().Ingresses(ingress.Namespace).Update(ingress)
			if err != nil {
				return err
			}
			c.recorder.Eventf(&sc.StackSet,
				apiv1.EventTypeNormal,
				"UpdatedIngress",
				"Updated Ingress %s/%s with %d service backend(s).",
				ingress.Namespace,
				ingress.Name,
				len(stacks),
			)
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
	for _, stack := range stacks {
		err := c.stackIngress(sc.StackSet, stack)
		if err != nil {
			continue
		}
	}

	return nil
}

func (c *ingressReconciler) stackIngress(stackset zv1.StackSet, stack zv1.Stack) error {
	ingress, err := c.ingressForStack(&stackset, &stack)
	if err != nil {
		c.recorder.Eventf(&stack,
			apiv1.EventTypeWarning,
			"GenerateIngress",
			"Failed generate Ingress for Stack %s/%s: %s", stack.Namespace, stack.Name, err)
		return err
	}

	ing, err := c.client.ExtensionsV1beta1().Ingresses(ingress.Namespace).Get(ingress.Name, metav1.GetOptions{})
	if err != nil {
		if !apiErrors.IsNotFound(err) {
			c.recorder.Eventf(&stack,
				apiv1.EventTypeWarning,
				"GetIngress",
				"Failed to get Ingress %s/%s: %s", ingress.Namespace, ingress.Name, err)
			return err
		}
		ing = nil
	}

	if ing == nil {
		_, err := c.client.ExtensionsV1beta1().Ingresses(ingress.Namespace).Create(ingress)
		if err != nil {
			c.recorder.Eventf(&stack,
				apiv1.EventTypeWarning,
				"Failed to create Ingress %s/%s: %s", ingress.Namespace, ingress.Name, err)
			return err
		}
		c.recorder.Eventf(&stack,
			apiv1.EventTypeNormal,
			"CreatedIngress",
			"Created Ingress %s/%s", ingress.Namespace, ingress.Name)
	} else {
		// check if ingress is already owned by a different resource.
		if !isOwnedReference(stack.TypeMeta, stack.ObjectMeta, ing.ObjectMeta) {
			c.recorder.Eventf(&stack,
				apiv1.EventTypeWarning,
				"IngressDifferentOwner",
				"Ingress %s/%s already has a different owner: %v", ing.Namespace, ing.Name, ing.ObjectMeta.OwnerReferences)
			return err
		}

		// add objectMeta from existing ingress
		ingress.SelfLink = ing.SelfLink
		ingress.UID = ing.UID
		ingress.Generation = ing.Generation
		ingress.CreationTimestamp = ing.CreationTimestamp
		ingress.ResourceVersion = ing.ResourceVersion
		ing.Status = v1beta1.IngressStatus{}

		if !equality.Semantic.DeepEqual(ing, ingress) {
			c.logger.Debugf("Ingress %s/%s changed: %s", ingress.Namespace, ingress.Name, cmp.Diff(ing, ingress))
			_, err := c.client.ExtensionsV1beta1().Ingresses(ingress.Namespace).Update(ingress)
			if err != nil {
				c.recorder.Eventf(&stack,
					apiv1.EventTypeWarning,
					"UpdateIngress",
					"Failed to update Ingress %s/%s: %v", ingress.Namespace, ingress.Name, err)
				return err
			}
			c.recorder.Eventf(&stack,
				apiv1.EventTypeNormal,
				"UpdatedIngress",
				"Updated Ingress %s/%s.", ingress.Namespace, ingress.Name)
		}
	}

	return nil
}

func (c *ingressReconciler) gcStackIngress(stack zv1.Stack) error {
	ing, err := c.client.ExtensionsV1beta1().Ingresses(stack.Namespace).Get(stack.Name, metav1.GetOptions{})
	if err != nil {
		if !apiErrors.IsNotFound(err) {
			c.recorder.Eventf(&stack,
				apiv1.EventTypeWarning,
				"GetIngress",
				"Failed to get Ingress %s/%s: %v", stack.Namespace, stack.Name, err)
			return err
		}
		return nil
	}

	// check if ingress is already owned by a different resource.
	if !isOwnedReference(stack.TypeMeta, stack.ObjectMeta, ing.ObjectMeta) {
		c.recorder.Eventf(&stack,
			apiv1.EventTypeWarning,
			"IngressDifferentOwner",
			"Ingress %s/%s already has a different owner: %v", ing.Namespace, ing.Name, ing.ObjectMeta.OwnerReferences)
		return err
	}

	err = c.client.ExtensionsV1beta1().Ingresses(ing.Namespace).Delete(ing.Name, nil)
	if err != nil {
		c.recorder.Eventf(&stack,
			apiv1.EventTypeWarning,
			"DeleteIngress",
			"Failed to delete Ingress %s/%s: %v", ing.Namespace, ing.Name, err)
		return err
	}
	c.recorder.Eventf(&stack,
		apiv1.EventTypeNormal,
		"DeletedIngress",
		"Deleted obsolete Ingress %s/%s.", ing.Namespace, ing.Name)

	return nil
}

// ingressForStack generates an ingress object based on a stack.
func (c *ingressReconciler) ingressForStack(stackset *zv1.StackSet, stack *zv1.Stack) (*v1beta1.Ingress, error) {
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
			c.recorder.Eventf(stack,
				apiv1.EventTypeWarning,
				"CreateDomainName",
				"Failed to create domain name: %s", err)
			return nil, err
		}
		r.Host = newHost
		ingress.Spec.Rules = append(ingress.Spec.Rules, r)
	}

	return ingress, nil
}

// ingressForStackSet
func (c *ingressReconciler) ingressForStackSet(ssc entities.StackSetContainer, origIngress *v1beta1.Ingress) (*v1beta1.Ingress, error) {
	stackset := &ssc.StackSet
	heritageLabels := map[string]string{
		entities.StacksetHeritageLabelKey: stackset.Name,
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

	if ingress.Annotations == nil {
		ingress.Annotations = map[string]string{}
	}

	return ingress, nil
}

func trafficSwitchingAnnotationsForIngress(ssc entities.StackSetContainer, ingress *v1beta1.Ingress) error {
	stackset := &ssc.StackSet
	availableWeights, allWeights := ssc.TrafficReconciler.ReconcileIngress(ssc.StackContainers, ingress, ssc.Traffic)

	rule := v1beta1.IngressRule{
		IngressRuleValue: v1beta1.IngressRuleValue{
			HTTP: &v1beta1.HTTPIngressRuleValue{
				Paths: make([]v1beta1.HTTPIngressPath, 0),
			},
		},
	}

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

	if len(rule.IngressRuleValue.HTTP.Paths) == 0 {
		return errNoPaths
	}

	// sort backends by name to have a consistent generated ingress
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
		return err
	}

	allWeightsData, err := json.Marshal(&allWeights)
	if err != nil {
		return err
	}

	ingress.Annotations[backendWeightsAnnotationKey] = string(availableWeightsData)
	ingress.Annotations[stackTrafficWeightsAnnotationKey] = string(allWeightsData)

	return nil
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
