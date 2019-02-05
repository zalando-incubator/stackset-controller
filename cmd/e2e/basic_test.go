package main

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func newStacksetSpec(stacksetName, stackVersion string, hpa bool, ingress bool) zv1.StackSetSpec {
	var result = zv1.StackSetSpec{
		StackTemplate: zv1.StackTemplate{
			Spec: zv1.StackSpecTemplate{
				StackSpec: zv1.StackSpec{
					Replicas: pint32(1),
					PodTemplate: corev1.PodTemplateSpec{
						Spec: nginxPod,
					},
					Service: &zv1.StackServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port:       80,
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(80),
							},
						},
					},
				},
				Version: stackVersion,
			},
		},
	}
	if hpa {
		result.StackTemplate.Spec.HorizontalPodAutoscaler = &zv1.HorizontalPodAutoscaler{
			MaxReplicas: 3,
			Metrics: []autoscalingv2beta1.MetricSpec{
				{
					Type: autoscalingv2beta1.ResourceMetricSourceType,
					Resource: &autoscalingv2beta1.ResourceMetricSource{
						Name:                     corev1.ResourceCPU,
						TargetAverageUtilization: pint32(50),
					},
				},
			},
		}
	}

	if ingress {
		result.Ingress = &zv1.StackSetIngressSpec{
			Hosts:       []string{hostname(stacksetName)},
			BackendPort: intstr.FromInt(80),
		}
	}
	return result
}

func verifyStack(t *testing.T, stacksetName, currentVersion string, stacksetSpec zv1.StackSetSpec) {
	stackResourceLabels := map[string]string{stacksetHeritageLabelKey: stacksetName, stackVersionLabelKey: currentVersion}

	// Verify stack
	stack, err := waitForStack(t, stacksetName, currentVersion)
	require.NoError(t, err)
	require.EqualValues(t, stacksetSpec.StackTemplate.Spec.StackSpec, stack.Spec)
	require.EqualValues(t, stackResourceLabels, stack.Labels)

	// Verify deployment
	deployment, err := waitForDeployment(t, stack.Name)
	require.NoError(t, err)
	require.EqualValues(t, stackResourceLabels, deployment.Labels)
	require.EqualValues(t, deployment.Spec.Replicas, stack.Spec.Replicas)
	require.EqualValues(t, stackResourceLabels, deployment.Spec.Template.Labels)

	// Verify service
	service, err := waitForService(t, stack.Name)
	require.NoError(t, err)
	require.EqualValues(t, stackResourceLabels, service.Labels)
	require.EqualValues(t, stackResourceLabels, service.Spec.Selector)

	// Verify that the stack status is updated successfully
	err = stackStatusMatches(t, stack.Name, expectedStackStatus{
		replicas:        pint32(1),
		readyReplicas:   pint32(1),
		updatedReplicas: pint32(1),
	}).await()
	require.NoError(t, err)

	// Verify the HPA
	if stacksetSpec.StackTemplate.Spec.HorizontalPodAutoscaler != nil {
		hpa, err := waitForHPA(t, stack.Name)
		require.NoError(t, err)
		require.EqualValues(t, stackResourceLabels, hpa.Labels)
		require.EqualValues(t, stacksetSpec.StackTemplate.Spec.Replicas, hpa.Spec.MinReplicas)
		require.EqualValues(t, stacksetSpec.StackTemplate.Spec.HorizontalPodAutoscaler.MaxReplicas, hpa.Spec.MaxReplicas)
		expectedRef := autoscalingv1.CrossVersionObjectReference{
			Kind:       "Deployment",
			Name:       deployment.Name,
			APIVersion: "apps/v1",
		}
		require.EqualValues(t, expectedRef, hpa.Spec.ScaleTargetRef)
	}

	// Verify the ingress
	if stacksetSpec.Ingress != nil {
		// Per-stack ingress
		stackIngress, err := waitForIngress(t, stack.Name)
		require.NoError(t, err)
		require.EqualValues(t, stackResourceLabels, stackIngress.Labels)
		stackIngressRules := []v1beta1.IngressRule{{
			Host: fmt.Sprintf("%s.%s", stack.Name, clusterDomain),
			IngressRuleValue: v1beta1.IngressRuleValue{
				HTTP: &v1beta1.HTTPIngressRuleValue{
					Paths: []v1beta1.HTTPIngressPath{
						{
							Backend: v1beta1.IngressBackend{
								ServiceName: service.Name,
								ServicePort: intstr.FromInt(80),
							},
						},
					},
				},
			},
		}}
		require.EqualValues(t, stackIngressRules, stackIngress.Spec.Rules)
	}
}

func verifyStacksetIngress(t *testing.T, stacksetName string, stacksetSpec zv1.StackSetSpec, stackWeights map[string]float64) {
	stacksetResourceLabels := map[string]string{stacksetHeritageLabelKey: stacksetName}

	expectedWeights := make(map[string]float64)

	var expectedPaths []v1beta1.HTTPIngressPath
	for stack, weight := range stackWeights {
		serviceName := fmt.Sprintf("%s-%s", stacksetName, stack)
		expectedWeights[serviceName] = weight

		if weight == 0.0 {
			continue
		}

		expectedPaths = append(expectedPaths, v1beta1.HTTPIngressPath{
			Backend: v1beta1.IngressBackend{
				ServiceName: serviceName,
				ServicePort: intstr.FromInt(80),
			},
		})
		sort.Slice(expectedPaths, func(i, j int) bool {
			return strings.Compare(expectedPaths[i].Backend.ServiceName, expectedPaths[j].Backend.ServiceName) < 0
		})
	}

	globalIngress, err := waitForIngress(t, stacksetName)
	require.NoError(t, err)
	require.EqualValues(t, stacksetResourceLabels, globalIngress.Labels)
	globalIngressRules := []v1beta1.IngressRule{{
		Host: stacksetSpec.Ingress.Hosts[0],
		IngressRuleValue: v1beta1.IngressRuleValue{
			HTTP: &v1beta1.HTTPIngressRuleValue{
				Paths: expectedPaths,
			},
		},
	}}
	require.EqualValues(t, globalIngressRules, globalIngress.Spec.Rules)

	err = trafficWeightsUpdated(t, stacksetName, weightKindDesired, expectedWeights).await()
	require.NoError(t, err)
	err = trafficWeightsUpdated(t, stacksetName, weightKindActual, expectedWeights).await()
	require.NoError(t, err)
}

func testStacksetCreate(t *testing.T, testName string, hpa bool, ingress bool) {
	t.Parallel()

	stacksetName := fmt.Sprintf("stackset-create-%s", testName)
	stackVersion := "v1"
	stacksetSpec := newStacksetSpec(stacksetName, stackVersion, hpa, ingress)

	err := createStackSet(stacksetName, stacksetSpec)
	require.NoError(t, err)

	verifyStack(t, stacksetName, stackVersion, stacksetSpec)

	if ingress {
		verifyStacksetIngress(t, stacksetName, stacksetSpec, map[string]float64{stackVersion: 100})
	}
}

func testStacksetUpdate(t *testing.T, testName string, oldHpa, newHpa, oldIngress, newIngress bool) {
	t.Parallel()

	stacksetName := fmt.Sprintf("stackset-update-%s", testName)
	initialVersion := "v1"
	stacksetSpec := newStacksetSpec(stacksetName, initialVersion, oldHpa, oldIngress)

	err := createStackSet(stacksetName, stacksetSpec)
	require.NoError(t, err)
	verifyStack(t, stacksetName, initialVersion, stacksetSpec)

	if oldIngress {
		verifyStacksetIngress(t, stacksetName, stacksetSpec, map[string]float64{initialVersion: 100})
	}

	updatedVersion := "v2"
	updatedSpec := newStacksetSpec(stacksetName, updatedVersion, newHpa, newIngress)
	err = updateStackset(stacksetName, updatedSpec)
	require.NoError(t, err)
	verifyStack(t, stacksetName, updatedVersion, updatedSpec)

	if newIngress {
		verifyStacksetIngress(t, stacksetName, updatedSpec, map[string]float64{initialVersion: 100, updatedVersion: 0})
	} else if oldIngress {
		err = resourceDeleted(t, "ingress", fmt.Sprintf("%s-%s", stacksetName, initialVersion), ingressInterface()).await()
		require.NoError(t, err)

		err = resourceDeleted(t, "ingress", stacksetName, ingressInterface()).await()
		require.NoError(t, err)
	}
}

func TestStacksetCreateBasic(t *testing.T) {
	testStacksetCreate(t, "basic", false, false)
}

func TestStacksetCreateHPA(t *testing.T) {
	testStacksetCreate(t, "hpa", true, false)
}

func TestStacksetCreateIngress(t *testing.T) {
	testStacksetCreate(t, "ingress", false, true)
}

func TestStacksetUpdateBasic(t *testing.T) {
	testStacksetUpdate(t, "basic", false, false, false, false)
}

func TestStacksetUpdateAddHPA(t *testing.T) {
	testStacksetUpdate(t, "add-hpa", false, true, false, false)
}

func TestStacksetUpdateDeleteHPA(t *testing.T) {
	testStacksetUpdate(t, "delete-hpa", true, false, false, false)
}

func TestStacksetUpdateAddIngress(t *testing.T) {
	testStacksetUpdate(t, "add-ingress", false, false, false, true)
}

func TestStacksetUpdateDeleteIngress(t *testing.T) {
	testStacksetUpdate(t, "delete-ingress", false, false, true, false)
}
