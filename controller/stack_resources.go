package controller

import (
	"github.com/zalando-incubator/stackset-controller/pkg/core"
	apiv1 "k8s.io/api/core/v1"
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

func (c *StackSetController) ReconcileStackDeployment(sc *core.StackContainer) error {
	deployment := sc.GenerateDeployment()
	existing := sc.Resources.Deployment

	// Create new deployment
	if existing == nil {
		_, err := c.client.AppsV1().Deployments(deployment.Namespace).Create(deployment)
		if err != nil {
			return err
		}
		c.recorder.Eventf(
			sc.Stack,
			apiv1.EventTypeNormal,
			"CreatedDeployment",
			"Created Deployment %s",
			deployment.Name)
		return nil
	}

	// Check if we need to update the deployment
	if core.IsResourceUpToDate(sc.Stack, existing.ObjectMeta) && deployment.Spec.Replicas == nil {
		return nil
	}

	updated := existing.DeepCopy()
	updated.Spec = deployment.Spec

	_, err := c.client.AppsV1().Deployments(updated.Namespace).Update(updated)
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		sc.Stack,
		apiv1.EventTypeNormal,
		"UpdatedDeployment",
		"Updated Deployment %s",
		deployment.Name)
	return nil
}

func (c *StackSetController) ReconcileStackHPA(sc *core.StackContainer) error {
	hpa, err := sc.GenerateHPA()
	if err != nil {
		return err
	}
	existing := sc.Resources.HPA

	// HPA removed
	if hpa == nil {
		if existing != nil {
			err := c.client.AutoscalingV2beta1().HorizontalPodAutoscalers(sc.Resources.HPA.Namespace).Delete(sc.Resources.HPA.Name, &metav1.DeleteOptions{})
			if err != nil {
				return err
			}
			c.recorder.Eventf(
				sc.Stack,
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
			sc.Stack,
			apiv1.EventTypeNormal,
			"CreatedHPA",
			"Created HPA %s",
			hpa.Name)
		return nil
	}

	// Check if we need to update the HPA
	if core.IsResourceUpToDate(sc.Stack, existing.ObjectMeta) && pint32Equal(existing.Spec.MinReplicas, hpa.Spec.MinReplicas) && existing.Spec.MaxReplicas == hpa.Spec.MaxReplicas {
		return nil
	}

	updated := existing.DeepCopy()
	updated.Spec = hpa.Spec

	_, err = c.client.AutoscalingV2beta1().HorizontalPodAutoscalers(updated.Namespace).Update(updated)
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		sc.Stack,
		apiv1.EventTypeNormal,
		"UpdatedHPA",
		"Updated HPA %s",
		hpa.Name)
	return nil
}

func (c *StackSetController) ReconcileStackService(sc *core.StackContainer) error {
	service, err := sc.GenerateService()
	if err != nil {
		return err
	}
	existing := sc.Resources.Service

	// Create new service
	if existing == nil {
		_, err := c.client.CoreV1().Services(service.Namespace).Create(service)
		if err != nil {
			return err
		}
		c.recorder.Eventf(
			sc.Stack,
			apiv1.EventTypeNormal,
			"CreatedService",
			"Created Service %s",
			service.Name)
		return nil
	}

	// Check if we need to update the service
	if core.IsResourceUpToDate(sc.Stack, existing.ObjectMeta) {
		return nil
	}

	updated := existing.DeepCopy()
	updated.Spec = service.Spec
	updated.Spec.ClusterIP = existing.Spec.ClusterIP // ClusterIP is immutable

	_, err = c.client.CoreV1().Services(updated.Namespace).Update(updated)
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		sc.Stack,
		apiv1.EventTypeNormal,
		"UpdatedService",
		"Updated Service %s",
		service.Name)
	return nil
}

func (c *StackSetController) ReconcileStackIngress(sc *core.StackContainer) error {
	ingress, err := sc.GenerateIngress()
	if err != nil {
		return err
	}
	existing := sc.Resources.Ingress

	// Ingress removed
	if ingress == nil {
		if existing != nil {
			err := c.client.ExtensionsV1beta1().Ingresses(sc.Resources.Ingress.Namespace).Delete(sc.Resources.Ingress.Name, &metav1.DeleteOptions{})
			if err != nil {
				return err
			}
			c.recorder.Eventf(
				sc.Stack,
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
			sc.Stack,
			apiv1.EventTypeNormal,
			"CreatedIngress",
			"Created Ingress %s",
			ingress.Name)
		return nil
	}

	// Check if we need to update the Ingress
	if core.IsResourceUpToDate(sc.Stack, existing.ObjectMeta) {
		return nil
	}

	updated := existing.DeepCopy()
	updated.Spec = ingress.Spec

	_, err = c.client.ExtensionsV1beta1().Ingresses(updated.Namespace).Update(updated)
	if err != nil {
		return err
	}
	c.recorder.Eventf(
		sc.Stack,
		apiv1.EventTypeNormal,
		"UpdatedIngress",
		"Updated Ingress %s",
		ingress.Name)
	return nil
}
