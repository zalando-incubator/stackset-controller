package main

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type TestStacksetSpecFactory struct {
	stacksetName   string
	hpa            bool
	ingress        bool
	limit          int32
	scaleDownTTL   int64
	replicas       int32
	hpaMaxReplicas int32
	hpaMinReplicas int32
	autoscaler     bool
	metrics        []zv1.AutoscalerMetrics
}

func NewTestStacksetSpecFactory(stacksetName string) *TestStacksetSpecFactory {
	return &TestStacksetSpecFactory{
		stacksetName:   stacksetName,
		hpa:            false,
		ingress:        false,
		limit:          4,
		scaleDownTTL:   10,
		replicas:       1,
		hpaMinReplicas: 1,
		hpaMaxReplicas: 3,
	}
}

func (f *TestStacksetSpecFactory) HPA(minReplicas, maxReplicas int32) *TestStacksetSpecFactory {
	f.hpa = true
	f.hpaMinReplicas = minReplicas
	f.hpaMaxReplicas = maxReplicas
	return f
}

func (f *TestStacksetSpecFactory) Ingress() *TestStacksetSpecFactory {
	f.ingress = true
	return f
}

func (f *TestStacksetSpecFactory) StackGC(limit int32, ttl int64) *TestStacksetSpecFactory {
	f.limit = limit
	f.scaleDownTTL = ttl
	return f
}

func (f *TestStacksetSpecFactory) Replicas(replicas int32) *TestStacksetSpecFactory {
	f.replicas = replicas
	return f
}

func (f *TestStacksetSpecFactory) Create(stackVersion string) zv1.StackSetSpec {
	var result = zv1.StackSetSpec{
		StackLifecycle: zv1.StackLifecycle{
			Limit:               pint32(f.limit),
			ScaledownTTLSeconds: &f.scaleDownTTL,
		},
		StackTemplate: zv1.StackTemplate{
			Spec: zv1.StackSpecTemplate{
				StackSpec: zv1.StackSpec{
					Replicas: pint32(f.replicas),
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
	if f.hpa {
		result.StackTemplate.Spec.HorizontalPodAutoscaler = &zv1.HorizontalPodAutoscaler{
			MaxReplicas: f.hpaMaxReplicas,
			MinReplicas: pint32(f.hpaMinReplicas),
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name:                     corev1.ResourceCPU,
						TargetAverageUtilization: pint32(50),
					},
				},
			},
		}
	}

	if f.autoscaler {
		result.StackTemplate.Spec.Autoscaler = &zv1.Autoscaler{
			MaxReplicas: f.hpaMaxReplicas,
			MinReplicas: pint32(f.hpaMinReplicas),
			Metrics:     f.metrics,
		}
	}

	if f.ingress {
		result.Ingress = &zv1.StackSetIngressSpec{
			Hosts:       []string{hostname(f.stacksetName)},
			BackendPort: intstr.FromInt(80),
		}
	}
	return result
}

func (f *TestStacksetSpecFactory) Autoscaler(minReplicas, maxReplicas int32, metrics []zv1.AutoscalerMetrics) *TestStacksetSpecFactory {
	f.autoscaler = true
	f.hpaMinReplicas = minReplicas
	f.hpaMaxReplicas = maxReplicas
	f.metrics = metrics
	return f
}

func replicas(value *int32) int32 {
	if value == nil {
		return -1
	}
	return *value
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
	require.EqualValues(t, replicas(deployment.Spec.Replicas), replicas(stack.Spec.Replicas))
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
		require.EqualValues(t, replicas(stacksetSpec.StackTemplate.Spec.Replicas), replicas(hpa.Spec.MinReplicas))
		require.EqualValues(t, stacksetSpec.StackTemplate.Spec.HorizontalPodAutoscaler.MaxReplicas, hpa.Spec.MaxReplicas)
		expectedRef := autoscalingv2.CrossVersionObjectReference{
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

func verifyStackSetStatus(t *testing.T, stacksetName, version string) {
	// Verify that the stack status is updated successfully
	err := stackSetStatusMatches(t, stacksetName, expectedStackSetStatus{
		observedStackVersion: version,
	}).await()
	require.NoError(t, err)
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
	stacksetSpecFactory := NewTestStacksetSpecFactory(stacksetName)
	if hpa {
		stacksetSpecFactory.HPA(1, 3)
	}
	if ingress {
		stacksetSpecFactory.Ingress()
	}
	stacksetSpec := stacksetSpecFactory.Create(stackVersion)
	err := createStackSet(stacksetName, 0, stacksetSpec)
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
	stacksetSpecFactory := NewTestStacksetSpecFactory(stacksetName)
	if oldHpa {
		stacksetSpecFactory.HPA(1, 3)
	}
	if oldIngress {
		stacksetSpecFactory.Ingress()
	}
	stacksetSpec := stacksetSpecFactory.Create(initialVersion)

	err := createStackSet(stacksetName, 0, stacksetSpec)
	require.NoError(t, err)
	verifyStack(t, stacksetName, initialVersion, stacksetSpec)

	if oldIngress {
		verifyStacksetIngress(t, stacksetName, stacksetSpec, map[string]float64{initialVersion: 100})
	}

	verifyStackSetStatus(t, stacksetName, initialVersion)

	stacksetSpecFactory = NewTestStacksetSpecFactory(stacksetName)
	updatedVersion := "v2"
	if newHpa {
		stacksetSpecFactory.HPA(1, 3)
	}
	if newIngress {
		stacksetSpecFactory.Ingress()
	}
	updatedSpec := stacksetSpecFactory.Create(updatedVersion)
	err = updateStackset(stacksetName, updatedSpec)
	require.NoError(t, err)
	verifyStack(t, stacksetName, updatedVersion, updatedSpec)
	verifyStackSetStatus(t, stacksetName, updatedVersion)

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
