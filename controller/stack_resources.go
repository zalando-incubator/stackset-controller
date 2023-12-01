package controller

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/core"
	apps "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	apiv1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// There are HPA metrics that depend on annotations to work properly,
// e.g. External RPS metric, this verification provides a way to verify
// all relevant annotations are actually up to date.
func areHPAAnnotationsUpToDate(updated, existing *v2.HorizontalPodAutoscaler) bool {
	if len(updated.Annotations) != len(existing.Annotations) {
		return false
	}

	for k, v := range updated.Annotations {
		if k == "stackset-controller.zalando.org/stack-generation" {
			continue
		}

		existingValue, ok := existing.Annotations[k]
		if ok && existingValue == v {
			continue
		}

		return false
	}

	return true
}

// syncObjectMeta copies metadata elements such as labels or annotations from source to target
func syncObjectMeta(target, source metav1.Object) {
	target.SetLabels(source.GetLabels())
	target.SetAnnotations(source.GetAnnotations())
}

// equalResourceList compares existing Resources with list of
// ConfigurationResources to be created for Stack
func equalResourceList(
	existing []*apiv1.ConfigMap,
	defined []zv1.ConfigurationResourcesSpec,
) bool {
	var existingName []string
	for _, e := range existing {
		existingName = append(existingName, e.Name)
	}
	var crName []string
	for _, cr := range defined {
		crName = append(crName, cr.ConfigMapRef.Name)
	}

	sort.Strings(existingName)
	sort.Strings(crName)

	return reflect.DeepEqual(existingName, crName)
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
		areHPAAnnotationsUpToDate(hpa, existing) {
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
	if core.IsResourceUpToDate(stack, existing.ObjectMeta) {
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
	if core.IsResourceUpToDate(stack, existing.ObjectMeta) {
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

// ReconcileStackConfigMap will update the named user-provided ConfigMap to be
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
func (c *StackSetController) ReconcileStackConfigMap(
	ctx context.Context,
	stack *zv1.Stack,
	existing []*apiv1.ConfigMap,
	updateObjMeta func(*metav1.ObjectMeta) *metav1.ObjectMeta,
) error {
	if stack.Spec.ConfigurationResources == nil {
		return nil
	}

	if equalResourceList(existing, stack.Spec.ConfigurationResources) {
		return nil
	}

	for _, rsc := range stack.Spec.ConfigurationResources {
		rscName := rsc.ConfigMapRef.Name

		// ensure that ConfigurationResources are prefixed by Stack name.
		if err := validateConfigurationResourceNames(stack); err != nil {
			return err
		}

		configMap, err := c.client.CoreV1().ConfigMaps(stack.Namespace).
			Get(ctx, rscName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		if configMap.OwnerReferences != nil {
			for _, owner := range configMap.OwnerReferences {
				if owner.UID != stack.UID {
					return fmt.Errorf("ConfigMap already owned by other resource. "+
						"ConfigMap: %s, Stack: %s", rscName, stack.Name)
				}
			}
			continue
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
	}
	return nil
}
