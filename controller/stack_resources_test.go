package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	apps "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	testStackSet = zv1.StackSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
			UID:       "123",
		},
	}
	baseTestStack = zv1.Stack{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "foo-v1",
			Namespace:       testStackSet.Namespace,
			UID:             "456",
			Generation:      1,
			OwnerReferences: stacksetOwned(testStackSet).OwnerReferences,
		},
		Status: zv1.StackStatus{
			ActualTrafficWeight: 42.0,
		},
	}
	updatedTestStack = *baseTestStack.DeepCopy()

	baseTestStackOwned    = stackOwned(baseTestStack)
	updatedTestStackOwned = stackOwned(baseTestStack)
)

func init() {
	baseTestStackOwned.Annotations = map[string]string{"stackset-controller.zalando.org/stack-generation": "1"}

	updatedTestStack.Generation = 2
	updatedTestStackOwned.Annotations = map[string]string{"stackset-controller.zalando.org/stack-generation": "2"}
}

func TestReconcileStackDeployment(t *testing.T) {
	exampleReplicas := int32(3)
	updatedReplicas := int32(4)

	examplePodTemplateSpec := v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "foo",
					Image: "ghcr.io/zalando/skipper:latest",
				},
			},
		},
	}
	updatedPodTemplateSpec := v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "bar",
					Image: "ghcr.io/zalando/skipper:latest",
				},
			},
		},
	}

	for _, tc := range []struct {
		name     string
		stack    zv1.Stack
		existing *apps.Deployment
		updated  *apps.Deployment
		expected *apps.Deployment
	}{
		{
			name:  "deployment is created if it doesn't exist",
			stack: baseTestStack,
			updated: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
			expected: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
		},
		{
			name:  "deployment is updated if the stack version changes",
			stack: updatedTestStack,
			existing: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
			updated: &apps.Deployment{
				ObjectMeta: updatedTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: updatedPodTemplateSpec,
				},
			},
			expected: &apps.Deployment{
				ObjectMeta: updatedTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: updatedPodTemplateSpec,
				},
			},
		},
		{
			name:  "deployment is updated if the replica count is set",
			stack: baseTestStack,
			existing: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
			updated: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &updatedReplicas,
					Template: examplePodTemplateSpec,
				},
			},
			expected: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &updatedReplicas,
					Template: examplePodTemplateSpec,
				},
			},
		},
		{
			name:  "deployment is not updated if the stack version remains the same and replica count is unchanged",
			stack: baseTestStack,
			existing: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
			updated: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: updatedPodTemplateSpec,
				},
			},
			expected: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Template: examplePodTemplateSpec,
				},
			},
		},
		{
			name:  "spec.selector is preserved",
			stack: baseTestStack,
			existing: &apps.Deployment{
				ObjectMeta: baseTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &exampleReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: examplePodTemplateSpec,
				},
			},
			updated: &apps.Deployment{
				ObjectMeta: updatedTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &updatedReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"updated": "selector"},
					},
					Template: updatedPodTemplateSpec,
				},
			},
			expected: &apps.Deployment{
				ObjectMeta: updatedTestStackOwned,
				Spec: apps.DeploymentSpec{
					Replicas: &updatedReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"foo": "bar"},
					},
					Template: updatedPodTemplateSpec,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnvironment()

			err := env.CreateStacksets(context.Background(), []zv1.StackSet{testStackSet})
			require.NoError(t, err)

			err = env.CreateStacks(context.Background(), []zv1.Stack{tc.stack})
			require.NoError(t, err)

			if tc.existing != nil {
				err = env.CreateDeployments(context.Background(), []apps.Deployment{*tc.existing})
				require.NoError(t, err)
			}

			err = env.controller.ReconcileStackDeployment(context.Background(), &tc.stack, tc.existing, func() *apps.Deployment {
				return tc.updated
			})
			require.NoError(t, err)

			updated, err := env.client.AppsV1().Deployments(tc.stack.Namespace).Get(context.Background(), tc.stack.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, tc.expected, updated)
		})
	}
}

func TestReconcileStackService(t *testing.T) {
	examplePorts := []v1.ServicePort{
		{
			Name:       "foo",
			Protocol:   v1.ProtocolTCP,
			Port:       8080,
			TargetPort: intstr.FromInt(80),
		},
	}
	exampleUpdatedPorts := []v1.ServicePort{
		{
			Name:       "bar",
			Protocol:   v1.ProtocolTCP,
			Port:       9090,
			TargetPort: intstr.FromInt(90),
		},
	}
	exampleClusterIP := "10.3.0.1"

	for _, tc := range []struct {
		name     string
		stack    zv1.Stack
		existing *v1.Service
		updated  *v1.Service
		expected *v1.Service
	}{
		{
			name:  "service is created if it doesn't exist",
			stack: baseTestStack,
			updated: &v1.Service{
				ObjectMeta: baseTestStackOwned,
				Spec: v1.ServiceSpec{
					Ports: examplePorts,
				},
			},
			expected: &v1.Service{
				ObjectMeta: baseTestStackOwned,
				Spec: v1.ServiceSpec{
					Ports: examplePorts,
				},
			},
		},
		{
			name:  "service is updated if the stack changes, ClusterIP is preserved",
			stack: updatedTestStack,
			existing: &v1.Service{
				ObjectMeta: baseTestStackOwned,
				Spec: v1.ServiceSpec{
					Ports:     examplePorts,
					ClusterIP: exampleClusterIP,
				},
			},
			updated: &v1.Service{
				ObjectMeta: updatedTestStackOwned,
				Spec: v1.ServiceSpec{
					Ports: exampleUpdatedPorts,
				},
			},
			expected: &v1.Service{
				ObjectMeta: updatedTestStackOwned,
				Spec: v1.ServiceSpec{
					Ports:     exampleUpdatedPorts,
					ClusterIP: exampleClusterIP,
				},
			},
		},
		{
			name:  "service is not updated if the stack version remains the same",
			stack: baseTestStack,
			existing: &v1.Service{
				ObjectMeta: baseTestStackOwned,
				Spec: v1.ServiceSpec{
					Ports:     examplePorts,
					ClusterIP: exampleClusterIP,
				},
			},
			updated: &v1.Service{
				ObjectMeta: baseTestStackOwned,
				Spec: v1.ServiceSpec{
					Ports: exampleUpdatedPorts,
				},
			},
			expected: &v1.Service{
				ObjectMeta: baseTestStackOwned,
				Spec: v1.ServiceSpec{
					Ports:     examplePorts,
					ClusterIP: exampleClusterIP,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnvironment()

			err := env.CreateStacksets(context.Background(), []zv1.StackSet{testStackSet})
			require.NoError(t, err)

			err = env.CreateStacks(context.Background(), []zv1.Stack{tc.stack})
			require.NoError(t, err)

			if tc.existing != nil {
				err = env.CreateServices(context.Background(), []v1.Service{*tc.existing})
				require.NoError(t, err)
			}

			err = env.controller.ReconcileStackService(context.Background(), &tc.stack, tc.existing, func() (*v1.Service, error) {
				return tc.updated, nil
			})
			require.NoError(t, err)

			updated, err := env.client.CoreV1().Services(tc.stack.Namespace).Get(context.Background(), tc.stack.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, tc.expected, updated)
		})
	}
}

func TestReconcileStackHPA(t *testing.T) {
	exampleResource := resource.MustParse("10m")
	exampleMetrics := []autoscaling.MetricSpec{
		{
			Type: "cpu",
			Resource: &autoscaling.ResourceMetricSource{
				Name: "cpu",
				Target: autoscaling.MetricTarget{
					Type:         autoscaling.AverageValueMetricType,
					AverageValue: &exampleResource,
				},
			},
		},
	}

	exampleUpdatedResource := resource.MustParse("20m")
	exampleUpdatedMetrics := []autoscaling.MetricSpec{
		{
			Type: "cpu",
			Resource: &autoscaling.ResourceMetricSource{
				Name: "cpu",
				Target: autoscaling.MetricTarget{
					Type:         autoscaling.AverageValueMetricType,
					AverageValue: &exampleUpdatedResource,
				},
			},
		},
	}

	exampleMinReplicas := int32(3)
	exampleUpdatedMinReplicas := int32(5)

	exampleBehavior := autoscaling.HorizontalPodAutoscalerBehavior{
		ScaleDown: &autoscaling.HPAScalingRules{
			Policies: []autoscaling.HPAScalingPolicy{
				{
					Type:          autoscaling.PercentScalingPolicy,
					Value:         10,
					PeriodSeconds: 60,
				},
			},
		},
	}

	exampleExternalRPSMetric := []autoscaling.MetricSpec{
		{
			Type: autoscaling.ExternalMetricSourceType,
			External: &autoscaling.ExternalMetricSource{
				Metric: autoscaling.MetricIdentifier{
					Name: "foo",
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"type": "requests-per-second",
						},
					},
				},
				Target: autoscaling.MetricTarget{
					Type:         autoscaling.MetricTargetType("AverageValue"),
					AverageValue: resource.NewQuantity(10, resource.DecimalSI),
				},
			},
		},
	}

	externalRPSHPAObjMeta := baseTestStackOwned.DeepCopy()
	for k, v := range map[string]string{
		"metric-config.external.foo.requests-per-second/hostnames": "just.testing.com",
		"metric-config.external.foo.requests-per-second/weight":    "42",
	} {
		externalRPSHPAObjMeta.Annotations[k] = v
	}
	externalRPSHPAObjMetaUpdated := baseTestStackOwned.DeepCopy()
	for k, v := range map[string]string{
		"metric-config.external.foo.requests-per-second/hostnames": "just.testing.com",
		"metric-config.external.foo.requests-per-second/weight":    "100",
	} {
		externalRPSHPAObjMetaUpdated.Annotations[k] = v
	}
	externalRPSHPAStack := baseTestStack.DeepCopy()
	externalRPSHPAStack.Status.ActualTrafficWeight = 100

	for _, tc := range []struct {
		name     string
		stack    zv1.Stack
		existing *autoscaling.HorizontalPodAutoscaler
		updated  *autoscaling.HorizontalPodAutoscaler
		expected *autoscaling.HorizontalPodAutoscaler
	}{
		{
			name:  "HPA is created if it doesn't exist",
			stack: baseTestStack,
			updated: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
				},
			},
			expected: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
				},
			},
		},
		{
			name:  "HPA is removed if it's no longer needed",
			stack: baseTestStack,
			existing: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
				},
			},
			updated:  nil,
			expected: nil,
		},
		{
			name:  "HPA is updated if stack version changes",
			stack: updatedTestStack,
			existing: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
				},
			},
			updated: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: updatedTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 7,
					Metrics:     exampleUpdatedMetrics,
				},
			},
			expected: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: updatedTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 7,
					Metrics:     exampleUpdatedMetrics,
				},
			},
		},
		{
			name:  "HPA is updated if stack weight changes",
			stack: *externalRPSHPAStack,
			existing: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: *externalRPSHPAObjMeta,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleExternalRPSMetric,
				},
			},
			updated: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: *externalRPSHPAObjMetaUpdated,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleExternalRPSMetric,
				},
			},
			expected: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: *externalRPSHPAObjMetaUpdated,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleExternalRPSMetric,
				},
			},
		},
		{
			name:  "HPA is updated if min. replicas is changed",
			stack: updatedTestStack,
			existing: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
				},
			},
			updated: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleUpdatedMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
				},
			},
			expected: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleUpdatedMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
				},
			},
		},
		{
			name:  "HPA is updated if behavior is changed",
			stack: updatedTestStack,
			existing: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
				},
			},
			updated: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
					Behavior:    &exampleBehavior,
				},
			},
			expected: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
					Behavior:    &exampleBehavior,
				},
			},
		},
		{
			name:  "HPA is not updated if the stack version remains the same and min. replicas are unchanged",
			stack: baseTestStack,
			existing: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
				},
			},
			updated: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleUpdatedMetrics,
				},
			},
			expected: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: baseTestStackOwned,
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &exampleMinReplicas,
					MaxReplicas: 5,
					Metrics:     exampleMetrics,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnvironment()

			err := env.CreateStacksets(context.Background(), []zv1.StackSet{testStackSet})
			require.NoError(t, err)

			err = env.CreateStacks(context.Background(), []zv1.Stack{tc.stack})
			require.NoError(t, err)

			if tc.existing != nil {
				err = env.CreateHPAs(context.Background(), []autoscaling.HorizontalPodAutoscaler{*tc.existing})
				require.NoError(t, err)
			}

			err = env.controller.ReconcileStackHPA(context.Background(), &tc.stack, tc.existing, func() (*autoscaling.HorizontalPodAutoscaler, error) {
				return tc.updated, nil
			})
			require.NoError(t, err)

			updated, err := env.client.AutoscalingV2().HorizontalPodAutoscalers(tc.stack.Namespace).Get(context.Background(), tc.stack.Name, metav1.GetOptions{})
			if tc.expected != nil {
				require.NoError(t, err)
				require.Equal(t, tc.expected, updated)
			} else {
				require.True(t, errors.IsNotFound(err))
			}
		})
	}
}

func TestReconcileStackIngress(t *testing.T) {
	exampleRules := []networking.IngressRule{
		{
			Host: "example.org",
			IngressRuleValue: networking.IngressRuleValue{
				HTTP: &networking.HTTPIngressRuleValue{
					Paths: []networking.HTTPIngressPath{
						{
							Path: "/",
							Backend: networking.IngressBackend{
								Service: &networking.IngressServiceBackend{
									Name: "foo",
									Port: networking.ServiceBackendPort{
										Number: 80,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	exampleUpdatedRules := []networking.IngressRule{
		{
			Host: "example.com",
			IngressRuleValue: networking.IngressRuleValue{
				HTTP: &networking.HTTPIngressRuleValue{
					Paths: []networking.HTTPIngressPath{
						{
							Path: "/",
							Backend: networking.IngressBackend{
								Service: &networking.IngressServiceBackend{
									Name: "bar",
									Port: networking.ServiceBackendPort{
										Number: 8181,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range []struct {
		name     string
		stack    zv1.Stack
		existing *networking.Ingress
		updated  *networking.Ingress
		expected *networking.Ingress
	}{
		{
			name:  "ingress is created if it doesn't exist",
			stack: baseTestStack,
			updated: &networking.Ingress{
				ObjectMeta: baseTestStackOwned,
				Spec: networking.IngressSpec{
					Rules: exampleRules,
				},
			},
			expected: &networking.Ingress{
				ObjectMeta: baseTestStackOwned,
				Spec: networking.IngressSpec{
					Rules: exampleRules,
				},
			},
		},
		{
			name:  "ingress is removed if it is no longer needed",
			stack: baseTestStack,
			existing: &networking.Ingress{
				ObjectMeta: baseTestStackOwned,
				Spec: networking.IngressSpec{
					Rules: exampleRules,
				},
			},
			updated:  nil,
			expected: nil,
		},
		{
			name:  "ingress is updated if the stack changes",
			stack: updatedTestStack,
			existing: &networking.Ingress{
				ObjectMeta: baseTestStackOwned,
				Spec: networking.IngressSpec{
					Rules: exampleRules,
				},
			},
			updated: &networking.Ingress{
				ObjectMeta: updatedTestStackOwned,
				Spec: networking.IngressSpec{
					Rules: exampleUpdatedRules,
				},
			},
			expected: &networking.Ingress{
				ObjectMeta: updatedTestStackOwned,
				Spec: networking.IngressSpec{
					Rules: exampleUpdatedRules,
				},
			},
		},
		{
			name:  "ingress is not updated if the stack version remains the same",
			stack: baseTestStack,
			existing: &networking.Ingress{
				ObjectMeta: baseTestStackOwned,
				Spec: networking.IngressSpec{
					Rules: exampleRules,
				},
			},
			updated: &networking.Ingress{
				ObjectMeta: baseTestStackOwned,
				Spec: networking.IngressSpec{
					Rules: exampleUpdatedRules,
				},
			},
			expected: &networking.Ingress{
				ObjectMeta: baseTestStackOwned,
				Spec: networking.IngressSpec{
					Rules: exampleRules,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnvironment()

			err := env.CreateStacksets(context.Background(), []zv1.StackSet{testStackSet})
			require.NoError(t, err)

			err = env.CreateStacks(context.Background(), []zv1.Stack{tc.stack})
			require.NoError(t, err)

			if tc.existing != nil {
				err = env.CreateIngresses(context.Background(), []networking.Ingress{*tc.existing})
				require.NoError(t, err)
			}

			err = env.controller.ReconcileStackIngress(context.Background(), &tc.stack, tc.existing, func() (*networking.Ingress, error) {
				return tc.updated, nil
			})
			require.NoError(t, err)

			updated, err := env.client.NetworkingV1().Ingresses(tc.stack.Namespace).Get(context.Background(), tc.stack.Name, metav1.GetOptions{})
			if tc.expected != nil {
				require.NoError(t, err)
				require.Equal(t, tc.expected, updated)
			} else {
				require.True(t, errors.IsNotFound(err))
			}
		})
	}
}

func TestReconcileStackRouteGroup(t *testing.T) {
	exampleSpec := rgv1.RouteGroupSpec{
		Hosts: []string{"example.org"},
		Backends: []rgv1.RouteGroupBackend{
			{
				Name:        "foo",
				Type:        rgv1.ServiceRouteGroupBackend,
				ServiceName: "foo",
				ServicePort: 80,
			},
		},
		DefaultBackends: []rgv1.RouteGroupBackendReference{
			{
				BackendName: "foo",
				Weight:      100,
			},
		},
		Routes: []rgv1.RouteGroupRouteSpec{
			{
				PathSubtree: "/",
			},
		},
	}

	exampleUpdatedSpec := rgv1.RouteGroupSpec{
		Hosts: []string{"example.org"},
		Backends: []rgv1.RouteGroupBackend{
			{
				Name:        "foo",
				Type:        rgv1.ServiceRouteGroupBackend,
				ServiceName: "foo",
				ServicePort: 80,
			},
			{
				Name:    "remote",
				Type:    rgv1.NetworkRouteGroupBackend,
				Address: "https://zalando.de",
			},
		},
		DefaultBackends: []rgv1.RouteGroupBackendReference{
			{
				BackendName: "foo",
				Weight:      100,
			},
		},
		Routes: []rgv1.RouteGroupRouteSpec{
			{
				PathSubtree: "/",
			},
			{
				PathSubtree: "/redirect",
				Backends: []rgv1.RouteGroupBackendReference{
					{
						BackendName: "remote",
						Weight:      100,
					},
				},
			},
		},
	}

	for _, tc := range []struct {
		name     string
		stack    zv1.Stack
		existing *rgv1.RouteGroup
		updated  *rgv1.RouteGroup
		expected *rgv1.RouteGroup
	}{
		{
			name:  "routegroup is created if it doesn't exist",
			stack: baseTestStack,
			updated: &rgv1.RouteGroup{
				ObjectMeta: baseTestStackOwned,
				Spec:       exampleSpec,
			},
			expected: &rgv1.RouteGroup{
				ObjectMeta: baseTestStackOwned,
				Spec:       exampleSpec,
			},
		},
		{
			name:  "routegroup is removed if it is no longer needed",
			stack: baseTestStack,
			existing: &rgv1.RouteGroup{
				ObjectMeta: baseTestStackOwned,
				Spec:       exampleSpec,
			},
			updated:  nil,
			expected: nil,
		},
		{
			name:  "routegroup is updated if the stack changes",
			stack: updatedTestStack,
			existing: &rgv1.RouteGroup{
				ObjectMeta: baseTestStackOwned,
				Spec:       exampleSpec,
			},
			updated: &rgv1.RouteGroup{
				ObjectMeta: updatedTestStackOwned,
				Spec:       exampleUpdatedSpec,
			},
			expected: &rgv1.RouteGroup{
				ObjectMeta: updatedTestStackOwned,
				Spec:       exampleUpdatedSpec,
			},
		},
		{
			name:  "routegroup is not updated if the stack version remains the same",
			stack: baseTestStack,
			existing: &rgv1.RouteGroup{
				ObjectMeta: baseTestStackOwned,
				Spec:       exampleSpec,
			},
			updated: &rgv1.RouteGroup{
				ObjectMeta: baseTestStackOwned,
				Spec:       exampleSpec,
			},
			expected: &rgv1.RouteGroup{
				ObjectMeta: baseTestStackOwned,
				Spec:       exampleSpec,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnvironment()

			err := env.CreateStacksets(context.Background(), []zv1.StackSet{testStackSet})
			require.NoError(t, err)

			err = env.CreateStacks(context.Background(), []zv1.Stack{tc.stack})
			require.NoError(t, err)

			if tc.existing != nil {
				err = env.CreateRouteGroups(context.Background(), []rgv1.RouteGroup{*tc.existing})
				require.NoError(t, err)
			}

			err = env.controller.ReconcileStackRouteGroup(context.Background(), &tc.stack, tc.existing, func() (*rgv1.RouteGroup, error) {
				return tc.updated, nil
			})
			require.NoError(t, err)

			updated, err := env.client.RouteGroupV1().RouteGroups(tc.stack.Namespace).Get(context.Background(), tc.stack.Name, metav1.GetOptions{})
			if tc.expected != nil {
				require.NoError(t, err)
				require.Equal(t, tc.expected, updated)
			} else {
				require.True(t, errors.IsNotFound(err))
			}
		})
	}
}

func TestReconcileStackConfigMap(t *testing.T) {
	testConfigMapStack := baseTestStack
	testConfigMapStack.Spec = zv1.StackSpecInternal{
		StackSpec: zv1.StackSpec{
			ConfigurationResources: []zv1.ConfigurationResourcesSpec{
				{
					ConfigMapRef: v1.LocalObjectReference{
						Name: "foo-v1-test-configmap",
					},
				},
			},
		},
	}

	multipleConfigMapsStack := baseTestStack
	multipleConfigMapsStack.Spec = zv1.StackSpecInternal{
		StackSpec: zv1.StackSpec{
			ConfigurationResources: []zv1.ConfigurationResourcesSpec{
				{
					ConfigMapRef: v1.LocalObjectReference{
						Name: "foo-v1-first-configmap",
					},
				},
				{
					ConfigMapRef: v1.LocalObjectReference{
						Name: "foo-v1-scnd-configmap",
					},
				},
			},
		},
	}

	baseData := map[string]string{
		"testK": "testV",
	}

	differentData := map[string]string{
		"testK-00": "testV-00",
	}

	testConfigMap := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "foo-v1-test-configmap",
			Namespace:       baseTestStackOwned.Namespace,
			OwnerReferences: baseTestStackOwned.OwnerReferences,
		},
		Data: baseData,
	}

	firstConfigMap := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "foo-v1-first-configmap",
			Namespace:       baseTestStackOwned.Namespace,
			OwnerReferences: baseTestStackOwned.OwnerReferences,
		},
		Data: baseData,
	}

	scndConfigMap := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "foo-v1-scnd-configmap",
			Namespace:       baseTestStackOwned.Namespace,
			OwnerReferences: baseTestStackOwned.OwnerReferences,
		},
		Data: differentData,
	}

	immutable := true
	notImmutable := false

	for _, tc := range []struct {
		name     string
		stack    zv1.Stack
		existing []*v1.ConfigMap
		template []*v1.ConfigMap
		expected map[string]*v1.ConfigMap
	}{
		{
			name:     "configmap ownerReference is added to referenced configmap",
			stack:    testConfigMapStack,
			existing: nil,
			template: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1-test-configmap",
						Namespace: testConfigMapStack.Namespace,
					},
					Data: baseData,
				},
			},
			expected: map[string]*v1.ConfigMap{
				"foo-v1-test-configmap": &testConfigMap,
			},
		},
		{
			name:  "stack already has configmap version",
			stack: testConfigMapStack,
			existing: []*v1.ConfigMap{
				&testConfigMap,
			},
			template: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1-test-configmap",
						Namespace: testConfigMapStack.Namespace,
					},
					Data: baseData,
				},
			},
			expected: nil,
		},
		{
			name:  "configmap name does not follow expected pattern",
			stack: testConfigMapStack,
			existing: []*v1.ConfigMap{
				&testConfigMap,
			},
			template: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-configmap",
						Namespace: testConfigMapStack.Namespace,
					},
					Data: baseData,
				},
			},
			expected: nil,
		},
		{
			name:     "stack with multiple configmap resources",
			stack:    multipleConfigMapsStack,
			existing: nil,
			template: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1-first-configmap",
						Namespace: multipleConfigMapsStack.Namespace,
					},
					Data: baseData,
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1-scnd-configmap",
						Namespace: multipleConfigMapsStack.Namespace,
					},
					Data: differentData,
				},
			},
			expected: map[string]*v1.ConfigMap{
				"foo-v1-first-configmap": &firstConfigMap,
				"foo-v1-scnd-configmap":  &scndConfigMap,
			},
		},
		{
			name:     "configmap version doesnt have immutability changed",
			stack:    multipleConfigMapsStack,
			existing: nil,
			template: []*v1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1-first-configmap",
						Namespace: multipleConfigMapsStack.Namespace,
					},
					Data:      baseData,
					Immutable: &immutable,
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1-scnd-configmap",
						Namespace: multipleConfigMapsStack.Namespace,
					},
					Data:      differentData,
					Immutable: &notImmutable,
				},
			},
			expected: map[string]*v1.ConfigMap{
				"foo-v1-first-configmap": {
					ObjectMeta: firstConfigMap.ObjectMeta,
					Data:       baseData,
					Immutable:  &immutable,
				},
				"foo-v1-scnd-configmap": {
					ObjectMeta: scndConfigMap.ObjectMeta,
					Data:       differentData,
					Immutable:  &notImmutable,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnvironment()

			err := env.CreateStacksets(context.Background(), []zv1.StackSet{testStackSet})
			require.NoError(t, err)

			err = env.CreateStacks(context.Background(), []zv1.Stack{tc.stack})
			require.NoError(t, err)

			// Create case's existing resources
			if tc.existing != nil {
				for _, existing := range tc.existing {
					err = env.CreateConfigMaps(context.Background(), []v1.ConfigMap{*existing})
					require.NoError(t, err)
				}
			}

			// Create case's template resources
			if tc.template != nil {
				for _, template := range tc.template {
					err = env.CreateConfigMaps(context.Background(), []v1.ConfigMap{*template})
					for _, existing := range tc.existing {
						if existing.Name == template.Name {
							require.Error(t, err)
						} else {
							require.NoError(t, err)
						}
					}
				}
			}

			err = env.controller.ReconcileStackConfigMap(
				context.Background(), &tc.stack, tc.existing, func(tmp *metav1.ObjectMeta) *metav1.ObjectMeta {
					return &tc.expected[tmp.Name].ObjectMeta
				})
			require.NoError(t, err)

			// Versioned ConfigMaps exist as expected
			for _, expected := range tc.expected {
				versioned, err := env.client.CoreV1().ConfigMaps(tc.stack.Namespace).Get(
					context.Background(), expected.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, expected, versioned)
				require.Equal(t, versioned.OwnerReferences, baseTestStackOwned.OwnerReferences)
			}
		})
	}
}

func TestReconcileStackSecret(t *testing.T) {
	testSecretStack := baseTestStack
	testSecretStack.Spec = zv1.StackSpecInternal{
		StackSpec: zv1.StackSpec{
			ConfigurationResources: []zv1.ConfigurationResourcesSpec{
				{
					SecretRef: v1.LocalObjectReference{
						Name: "foo-v1-test-secret",
					},
				},
			},
		},
	}

	testMixedConfigStack := baseTestStack
	testMixedConfigStack.Spec = zv1.StackSpecInternal{
		StackSpec: zv1.StackSpec{
			ConfigurationResources: []zv1.ConfigurationResourcesSpec{
				{
					SecretRef: v1.LocalObjectReference{
						Name: "foo-v1-test-secret",
					},
				},
				{
					ConfigMapRef: v1.LocalObjectReference{
						Name: "foo-v1-test-configmap",
					},
				},
			},
		},
	}

	multipleSecretsStack := baseTestStack
	multipleSecretsStack.Spec = zv1.StackSpecInternal{
		StackSpec: zv1.StackSpec{
			ConfigurationResources: []zv1.ConfigurationResourcesSpec{
				{
					SecretRef: v1.LocalObjectReference{
						Name: "foo-v1-first-secret",
					},
				},
				{
					SecretRef: v1.LocalObjectReference{
						Name: "foo-v1-scnd-secret",
					},
				},
			},
		},
	}

	baseData := map[string][]byte{
		"testK": []byte("value-00"),
	}

	differentData := map[string][]byte{
		"testK": []byte("value-01"),
	}

	testSecret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "foo-v1-test-secret",
			Namespace:       baseTestStackOwned.Namespace,
			OwnerReferences: baseTestStackOwned.OwnerReferences,
		},
		Data: baseData,
	}

	firstSecret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "foo-v1-first-secret",
			Namespace:       baseTestStackOwned.Namespace,
			OwnerReferences: baseTestStackOwned.OwnerReferences,
		},
		Data: baseData,
	}

	scndSecret := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "foo-v1-scnd-secret",
			Namespace:       baseTestStackOwned.Namespace,
			OwnerReferences: baseTestStackOwned.OwnerReferences,
		},
		Data: differentData,
	}

	for _, tc := range []struct {
		name     string
		stack    zv1.Stack
		existing []*v1.Secret
		template []*v1.Secret
		expected map[string]*v1.Secret
	}{
		{
			name:     "secret ownerReference is added to referenced secret",
			stack:    testSecretStack,
			existing: nil,
			template: []*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1-test-secret",
						Namespace: testSecretStack.Namespace,
					},
					Data: baseData,
				},
			},
			expected: map[string]*v1.Secret{
				"foo-v1-test-secret": &testSecret,
			},
		},
		{
			name:  "stack already has secret version",
			stack: testSecretStack,
			existing: []*v1.Secret{
				&testSecret,
			},
			template: []*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1-test-secret",
						Namespace: testSecretStack.Namespace,
					},
					Data: baseData,
				},
			},
			expected: nil,
		},
		{
			name:  "secret name does not follow expected pattern",
			stack: testSecretStack,
			existing: []*v1.Secret{
				&testSecret,
			},
			template: []*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: testSecretStack.Namespace,
					},
					Data: baseData,
				},
			},
			expected: nil,
		},
		{
			name:     "stack with multiple secret resources",
			stack:    multipleSecretsStack,
			existing: nil,
			template: []*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1-first-secret",
						Namespace: multipleSecretsStack.Namespace,
					},
					Data: baseData,
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1-scnd-secret",
						Namespace: multipleSecretsStack.Namespace,
					},
					Data: differentData,
				},
			},
			expected: map[string]*v1.Secret{
				"foo-v1-first-secret": &firstSecret,
				"foo-v1-scnd-secret":  &scndSecret,
			},
		},
		{
			name:     "manage secrets out of mixed configurationResources",
			stack:    testMixedConfigStack,
			existing: nil,
			template: []*v1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1-test-secret",
						Namespace: testSecretStack.Namespace,
					},
					Data: baseData,
				},
			},
			expected: map[string]*v1.Secret{
				"foo-v1-test-secret": &testSecret,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := NewTestEnvironment()

			err := env.CreateStacksets(context.Background(), []zv1.StackSet{testStackSet})
			require.NoError(t, err)

			err = env.CreateStacks(context.Background(), []zv1.Stack{tc.stack})
			require.NoError(t, err)

			// Create case's existing resources
			if tc.existing != nil {
				for _, existing := range tc.existing {
					err = env.CreateSecrets(context.Background(), []v1.Secret{*existing})
					require.NoError(t, err)
				}
			}

			// Create case's template resources
			if tc.template != nil {
				for _, template := range tc.template {
					err = env.CreateSecrets(context.Background(), []v1.Secret{*template})
					for _, existing := range tc.existing {
						if existing.Name == template.Name {
							require.Error(t, err)
						} else {
							require.NoError(t, err)
						}
					}
				}
			}

			err = env.controller.ReconcileStackSecret(
				context.Background(), &tc.stack, tc.existing, func(tmp *metav1.ObjectMeta) *metav1.ObjectMeta {
					return &tc.expected[tmp.Name].ObjectMeta
				})
			require.NoError(t, err)

			// Versioned Secrets exist as expected
			for _, expected := range tc.expected {
				versioned, err := env.client.CoreV1().Secrets(tc.stack.Namespace).Get(
					context.Background(), expected.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, expected, versioned)
				require.Equal(t, versioned.OwnerReferences, baseTestStackOwned.OwnerReferences)
			}
		})
	}
}
