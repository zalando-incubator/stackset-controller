package controller

import (
	"context"
	"fmt"

	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"github.com/zalando-incubator/stackset-controller/pkg/core"
	apps "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	apiv1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func pint32Equal(p1, p2 *int32) bool {
	if p1 == nil && p2 == nil {
		return true
	}
	if p1 != nil && p2 != nil {
		return *p1 == *p2
	}
	return false
}

// syncObjectMeta copies metadata elements such as labels or annotations from source to target
func syncObjectMeta(target, source metav1.Object) {
	target.SetLabels(source.GetLabels())
	target.SetAnnotations(source.GetAnnotations())
}

// isOwned checks if the resource is owned and returns the UID of the owner.
func isOwned(ownerReferences []metav1.OwnerReference) (bool, types.UID) {
	for _, ownerRef := range ownerReferences {
		return true, ownerRef.UID
	}

	return false, ""
}

func (c *StackSetController) ReconcileStackDeployment(ctx context.Context, stack *zv1.Stack, existing *apps.Deployment, generateUpdated func() *apps.Deployment) error {
	deployment := generateUpdated()

	// Create new deployment
	if existing == nil {
		_, err := c.client.AppsV1().Deployments(deployment.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		c.recorder.Eventf(
			stack,
			apiv1.EventTypeNormal,
			"CreatedDeployment",
			"Created Deployment %s",
			deployment.Name)
		return nil
	}

	// Check if we need to update the deployment
	if core.IsResourceUpToDate(stack, existing.ObjectMeta) && pint32Equal(existing.Spec.Replicas, deployment.Spec.Replicas) {
		return nil
	}

	updated := existing.DeepCopy()
	syncObjectMeta(updated, deployment)
	updated.Spec = deployment.Spec
	updated.Spec.Selector = existing.Spec.Selector

	_, err := c.client.AppsV1().Deployments(updated.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		stack,
		apiv1.EventTypeNormal,
		"UpdatedDeployment",
		"Updated Deployment %s",
		deployment.Name)
	return nil
}

func (c *StackSetController) ReconcileStackHPA(ctx context.Context, stack *zv1.Stack, existing *v2.HorizontalPodAutoscaler, generateUpdated func() (*v2.HorizontalPodAutoscaler, error)) error {
	hpa, err := generateUpdated()
	if err != nil {
		return err
	}

	// HPA removed
	if hpa == nil {
		if existing != nil {
			err := c.client.AutoscalingV2().HorizontalPodAutoscalers(existing.Namespace).Delete(ctx, existing.Name, metav1.DeleteOptions{})
			if err != nil {
				return err
			}
			c.recorder.Eventf(
				stack,
				apiv1.EventTypeNormal,
				"DeletedHPA",
				"Deleted HPA %s",
				existing.Namespace)
		}
		return nil
	}

	// Create new HPA
	if existing == nil {
		_, err := c.client.AutoscalingV2().HorizontalPodAutoscalers(hpa.Namespace).Create(ctx, hpa, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		c.recorder.Eventf(
			stack,
			apiv1.EventTypeNormal,
			"CreatedHPA",
			"Created HPA %s",
			hpa.Name)
		return nil
	}

	// Check if we need to update the HPA
	if core.IsResourceUpToDate(stack, existing.ObjectMeta) &&
		pint32Equal(existing.Spec.MinReplicas, hpa.Spec.MinReplicas) &&
		core.AreAnnotationsUpToDate(hpa.ObjectMeta, existing.ObjectMeta) {
		return nil
	}

	updated := existing.DeepCopy()
	syncObjectMeta(updated, hpa)
	updated.Spec = hpa.Spec

	_, err = c.client.AutoscalingV2().HorizontalPodAutoscalers(updated.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		stack,
		apiv1.EventTypeNormal,
		"UpdatedHPA",
		"Updated HPA %s",
		hpa.Name)
	return nil
}

func (c *StackSetController) ReconcileStackService(ctx context.Context, stack *zv1.Stack, existing *apiv1.Service, generateUpdated func() (*apiv1.Service, error)) error {
	service, err := generateUpdated()
	if err != nil {
		return err
	}

	// Create new service
	if existing == nil {
		_, err := c.client.CoreV1().Services(service.Namespace).Create(ctx, service, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		c.recorder.Eventf(
			stack,
			apiv1.EventTypeNormal,
			"CreatedService",
			"Created Service %s",
			service.Name)
		return nil
	}

	// Check if we need to update the service
	if core.IsResourceUpToDate(stack, existing.ObjectMeta) {
		return nil
	}

	updated := existing.DeepCopy()
	syncObjectMeta(updated, service)
	updated.Spec = service.Spec
	updated.Spec.ClusterIP = existing.Spec.ClusterIP // ClusterIP is immutable

	_, err = c.client.CoreV1().Services(updated.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		stack,
		apiv1.EventTypeNormal,
		"UpdatedService",
		"Updated Service %s",
		service.Name)
	return nil
}

func (c *StackSetController) ReconcileStackIngress(ctx context.Context, stack *zv1.Stack, existing *networking.Ingress, generateUpdated func() (*networking.Ingress, error)) error {
	ingress, err := generateUpdated()
	if err != nil {
		return err
	}

	// Ingress removed
	if ingress == nil {
		if existing != nil {
			err := c.client.NetworkingV1().Ingresses(existing.Namespace).Delete(ctx, existing.Name, metav1.DeleteOptions{})
			if err != nil {
				return err
			}
			c.recorder.Eventf(
				stack,
				apiv1.EventTypeNormal,
				"DeletedIngress",
				"Deleted Ingress %s",
				existing.Namespace)
		}
		return nil
	}

	// Create new Ingress
	if existing == nil {
		_, err := c.client.NetworkingV1().Ingresses(ingress.Namespace).Create(ctx, ingress, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		c.recorder.Eventf(
			stack,
			apiv1.EventTypeNormal,
			"CreatedIngress",
			"Created Ingress %s",
			ingress.Name)
		return nil
	}

	// Check if we need to update the Ingress
	if core.IsResourceUpToDate(stack, existing.ObjectMeta) &&
		core.AreAnnotationsUpToDate(ingress.ObjectMeta, existing.ObjectMeta) {

		return nil
	}

	updated := existing.DeepCopy()
	syncObjectMeta(updated, ingress)
	updated.Spec = ingress.Spec

	_, err = c.client.NetworkingV1().Ingresses(updated.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		stack,
		apiv1.EventTypeNormal,
		"UpdatedIngress",
		"Updated Ingress %s",
		ingress.Name)
	return nil
}

func (c *StackSetController) ReconcileStackRouteGroup(ctx context.Context, stack *zv1.Stack, existing *rgv1.RouteGroup, generateUpdated func() (*rgv1.RouteGroup, error)) error {
	routegroup, err := generateUpdated()
	if err != nil {
		return err
	}

	// RouteGroup removed
	if routegroup == nil {
		if existing != nil {
			err := c.client.RouteGroupV1().RouteGroups(existing.Namespace).Delete(ctx, existing.Name, metav1.DeleteOptions{})
			if err != nil {
				return err
			}
			c.recorder.Eventf(
				stack,
				apiv1.EventTypeNormal,
				"DeletedRouteGroup",
				"Deleted RouteGroup %s",
				existing.Namespace)
		}
		return nil
	}

	// Create new RouteGroup
	if existing == nil {
		_, err := c.client.RouteGroupV1().RouteGroups(routegroup.Namespace).Create(ctx, routegroup, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		c.recorder.Eventf(
			stack,
			apiv1.EventTypeNormal,
			"CreatedRouteGroup",
			"Created RouteGroup %s",
			routegroup.Name)
		return nil
	}

	// Check if we need to update the RouteGroup
	if core.IsResourceUpToDate(stack, existing.ObjectMeta) &&
		equality.Semantic.DeepEqual(routegroup.Spec, existing.Spec) &&
		core.AreAnnotationsUpToDate(
			routegroup.ObjectMeta,
			existing.ObjectMeta,
		) {

		return nil
	}

	updated := existing.DeepCopy()
	syncObjectMeta(updated, routegroup)
	updated.Spec = routegroup.Spec

	_, err = c.client.RouteGroupV1().RouteGroups(updated.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		stack,
		apiv1.EventTypeNormal,
		"UpdatedRouteGroup",
		"Updated RouteGroup %s",
		routegroup.Name)
	return nil
}

// ReconcileStackConfigMapRefs will update the named user-provided ConfigMaps to be
// attached to the Stack by ownerReferences, when a list of Configuration
// Resources are defined on the Stack template.
//
// The provided ConfigMap name must be prefixed by the Stack name.
// eg: Stack: myapp-v1 ConfigMap: myapp-v1-my-config
//
// User update of running versioned ConfigMaps is not encouraged but is allowed
// on consideration of emergency needs. Similarly, addition of ConfigMaps to
// running resources is also allowed, so the method checks for changes on the
// ConfigurationResources to ensure all listed ConfigMaps are properly linked
// to the Stack.
func (c *StackSetController) ReconcileStackConfigMapRefs(
	ctx context.Context,
	stack *zv1.Stack,
	updateObjMeta func(*metav1.ObjectMeta) *metav1.ObjectMeta,
) error {
	for _, rsc := range stack.Spec.ConfigurationResources {
		if !rsc.IsConfigMapRef() {
			continue
		}

		if err := validateConfigurationResourceName(stack.Name, rsc.GetName()); err != nil {
			return err
		}

		if err := c.ReconcileStackConfigMapRef(ctx, stack, rsc, updateObjMeta); err != nil {
			return err
		}
	}

	return nil
}

func (c *StackSetController) ReconcileStackConfigMapRef(
	ctx context.Context,
	stack *zv1.Stack,
	rsc zv1.ConfigurationResourcesSpec,
	updateObjMeta func(*metav1.ObjectMeta) *metav1.ObjectMeta,
) error {
	configMap, err := c.client.CoreV1().ConfigMaps(stack.Namespace).
		Get(ctx, rsc.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Check if the ConfigMap is already owned by us or another resource.
	isOwned, owner := isOwned(configMap.OwnerReferences)
	if isOwned {
		// If the ConfigMap is already owned by us, we don't need to do anything.
		if owner == stack.UID {
			return nil
		}

		// If the ConfigMap is owned by another resource, we should not update it.
		return fmt.Errorf("ConfigMap already owned by other resource. "+
			"ConfigMap: %s, Stack: %s", rsc.GetName(), stack.Name)
	}

	objectMeta := updateObjMeta(&configMap.ObjectMeta)
	configMap.ObjectMeta = *objectMeta

	_, err = c.client.CoreV1().ConfigMaps(configMap.Namespace).
		Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	c.recorder.Eventf(
		stack,
		apiv1.EventTypeNormal,
		"UpdatedConfigMap",
		"Updated ConfigMap %s",
		configMap.Name,
	)

	return nil
}

// ReconcileStackSecretRefs will update the named user-provided Secrets to be
// attached to the Stack by ownerReferences, when a list of Configuration
// Resources are defined on the Stack template.
//
// The provided Secret name must be prefixed by the Stack name.
// eg: Stack: myapp-v1 Secret: myapp-v1-my-secret
//
// User update of running versioned Secrets is not encouraged but is allowed
// on consideration of emergency needs. Similarly, addition of Secrets to
// running resources is also allowed, so the method checks for changes on the
// ConfigurationResources to ensure all listed Secrets are properly linked
// to the Stack.
func (c *StackSetController) ReconcileStackSecretRefs(
	ctx context.Context,
	stack *zv1.Stack,
	updateObjMeta func(*metav1.ObjectMeta) *metav1.ObjectMeta,
) error {
	for _, rsc := range stack.Spec.ConfigurationResources {
		if !rsc.IsSecretRef() {
			continue
		}

		if err := validateConfigurationResourceName(stack.Name, rsc.GetName()); err != nil {
			return err
		}

		if err := c.ReconcileStackSecretRef(ctx, stack, rsc, updateObjMeta); err != nil {
			return err
		}
	}

	return nil
}

func (c *StackSetController) ReconcileStackSecretRef(ctx context.Context,
	stack *zv1.Stack,
	rsc zv1.ConfigurationResourcesSpec,
	updateObjMeta func(*metav1.ObjectMeta) *metav1.ObjectMeta,
) error {
	secret, err := c.client.CoreV1().Secrets(stack.Namespace).
		Get(ctx, rsc.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Check if the Secret is already owned by us or another resource.
	isOwned, owner := isOwned(secret.OwnerReferences)
	if isOwned {
		// If the Secret is already owned by us, we don't need to do anything.
		if owner == stack.UID {
			return nil
		}

		// If the Secret is owned by another resource, we should not update it.
		return fmt.Errorf("Secret already owned by other resource. "+
			"Secret: %s, Stack: %s", rsc.GetName(), stack.Name)
	}

	objectMeta := updateObjMeta(&secret.ObjectMeta)
	secret.ObjectMeta = *objectMeta

	_, err = c.client.CoreV1().Secrets(secret.Namespace).
		Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	c.recorder.Eventf(
		stack,
		apiv1.EventTypeNormal,
		"UpdatedSecret",
		"Updated Secret %s",
		secret.Name,
	)

	return nil
}

func (c *StackSetController) ReconcileStackConfigMaps(ctx context.Context, stack *zv1.Stack, existingConfigMaps []*apiv1.ConfigMap, generateUpdated func(ctx context.Context, client clientset.Interface) ([]*apiv1.ConfigMap, error),
) error {
	desiredConfigMaps, err := generateUpdated(ctx, c.client)
	if err != nil {
		return err
	}

	for _, desiredConfigMap := range desiredConfigMaps {
		existing := findConfigMap(existingConfigMaps, desiredConfigMap.Name)
		if err := c.ReconcileStackConfigMap(ctx, stack, existing, desiredConfigMap); err != nil {
			return err
		}
	}

	for _, existingConfigMap := range existingConfigMaps {
		desiredConfigMap := findConfigMap(desiredConfigMaps, existingConfigMap.Name)
		if desiredConfigMap == nil {
			if err := c.ReconcileStackConfigMap(ctx, stack, existingConfigMap, desiredConfigMap); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *StackSetController) ReconcileStackConfigMap(ctx context.Context, stack *zv1.Stack, existingConfigMap *apiv1.ConfigMap, desiredConfigMap *apiv1.ConfigMap,
) error {
	// ConfigMap removed
	if desiredConfigMap == nil {
		if existingConfigMap != nil {
			err := c.client.CoreV1().ConfigMaps(existingConfigMap.Namespace).Delete(ctx, existingConfigMap.Name, metav1.DeleteOptions{})
			if err != nil {
				return err
			}
			c.recorder.Eventf(
				stack,
				apiv1.EventTypeNormal,
				"DeletedConfigMap",
				"Deleted ConfigMap %s",
				existingConfigMap.Namespace)
		}
		return nil
	}

	// Create new ConfigMap
	if existingConfigMap == nil {
		_, err := c.client.CoreV1().ConfigMaps(desiredConfigMap.Namespace).Create(ctx, desiredConfigMap, metav1.CreateOptions{})
		if err == nil {
			c.recorder.Eventf(
				stack,
				apiv1.EventTypeNormal,
				"CreatedConfigMap",
				"Created ConfigMap %s",
				desiredConfigMap.Name)

			return nil
		}

		// TODO: check error
		// configmap already exists but doesn't have _our_owner yet.
		existingConfigMap, err = c.client.CoreV1().ConfigMaps(desiredConfigMap.Namespace).Get(ctx, desiredConfigMap.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
	}

	// Check if we need to update the ConfigMap
	if core.IsResourceUpToDate(stack, existingConfigMap.ObjectMeta) {
		return nil
	}

	updated := existingConfigMap.DeepCopy()
	syncObjectMeta(updated, desiredConfigMap)
	updated.Data = desiredConfigMap.Data

	_, err := c.client.CoreV1().ConfigMaps(updated.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		stack,
		apiv1.EventTypeNormal,
		"UpdatedConfigMap",
		"Updated ConfigMap %s",
		desiredConfigMap.Name)
	return nil
}

func findConfigMap(configMaps []*apiv1.ConfigMap, name string) *apiv1.ConfigMap {
	for _, configMap := range configMaps {
		if configMap.Name == name {
			return configMap
		}
	}
	return nil
}
