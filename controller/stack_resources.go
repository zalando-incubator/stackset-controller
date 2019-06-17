package controller

import (
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/core"
	apps "k8s.io/api/apps/v1"
	"k8s.io/api/autoscaling/v2beta1"
	apiv1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
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

// syncObjectMeta copies metadata elements such as labels or annotations from source to target
func syncObjectMeta(target, source metav1.Object) {
	target.SetLabels(source.GetLabels())
	target.SetAnnotations(source.GetAnnotations())
}

func (c *StackSetController) ReconcileStackDeployment(stack *zv1.Stack, existing *apps.Deployment, generateUpdated func() *apps.Deployment) error {
	deployment := generateUpdated()

	// Create new deployment
	if existing == nil {
		_, err := c.client.AppsV1().Deployments(deployment.Namespace).Create(deployment)
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
	if core.IsResourceUpToDate(stack, existing.ObjectMeta) && deployment.Spec.Replicas == nil {
		return nil
	}

	updated := existing.DeepCopy()
	syncObjectMeta(updated, deployment)
	updated.Spec = deployment.Spec

	_, err := c.client.AppsV1().Deployments(updated.Namespace).Update(updated)
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

func (c *StackSetController) ReconcileStackHPA(stack *zv1.Stack, existing *v2beta1.HorizontalPodAutoscaler, generateUpdated func() (*v2beta1.HorizontalPodAutoscaler, error)) error {
	hpa, err := generateUpdated()
	if err != nil {
		return err
	}

	// HPA removed
	if hpa == nil {
		if existing != nil {
			err := c.client.AutoscalingV2beta1().HorizontalPodAutoscalers(existing.Namespace).Delete(existing.Name, &metav1.DeleteOptions{})
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
		_, err := c.client.AutoscalingV2beta1().HorizontalPodAutoscalers(hpa.Namespace).Create(hpa)
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
	if core.IsResourceUpToDate(stack, existing.ObjectMeta) && pint32Equal(existing.Spec.MinReplicas, hpa.Spec.MinReplicas) {
		return nil
	}

	updated := existing.DeepCopy()
	syncObjectMeta(updated, hpa)
	updated.Spec = hpa.Spec

	_, err = c.client.AutoscalingV2beta1().HorizontalPodAutoscalers(updated.Namespace).Update(updated)
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

func (c *StackSetController) ReconcileStackService(stack *zv1.Stack, existing *apiv1.Service, generateUpdated func() (*apiv1.Service, error)) error {
	service, err := generateUpdated()
	if err != nil {
		return err
	}

	// Create new service
	if existing == nil {
		_, err := c.client.CoreV1().Services(service.Namespace).Create(service)
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

	_, err = c.client.CoreV1().Services(updated.Namespace).Update(updated)
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

func (c *StackSetController) ReconcileStackIngress(stack *zv1.Stack, existing *extensions.Ingress, generateUpdated func() (*extensions.Ingress, error)) error {
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
		_, err := c.client.ExtensionsV1beta1().Ingresses(ingress.Namespace).Create(ingress)
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
	updated.Spec = ingress.Spec

	_, err = c.client.ExtensionsV1beta1().Ingresses(updated.Namespace).Update(updated)
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
