package main

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/controller"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/core"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type weightKind string

const (
	stacksetHeritageLabelKey    = "stackset"
	stacksetApplicationLabelKey = "application"
	stackVersionLabelKey        = "stack-version"

	weightKindDesired weightKind = "zalando.org/stack-traffic-weights"
	weightKindActual  weightKind = "zalando.org/backend-weights"
)

var (
	skipperPod = corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "skipper",
				Image: "ghcr.io/zalando/skipper:v0.15.33",
				Args: []string{
					"skipper",
					"-inline-routes",
					`* -> inlineContent("OK") -> <shunt>`,
					"-address=:80",
				},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 80,
						Protocol:      corev1.ProtocolTCP,
					},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128Mi"),
						corev1.ResourceCPU:    resource.MustParse("25m"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128Mi"),
						corev1.ResourceCPU:    resource.MustParse("25m"),
					},
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path:   "/",
							Port:   intstr.FromInt(80),
							Scheme: corev1.URISchemeHTTP,
						},
					},
					InitialDelaySeconds: 5,
					TimeoutSeconds:      5,
					PeriodSeconds:       3,
					SuccessThreshold:    3,
					FailureThreshold:    3,
				},
			},
		},
		TerminationGracePeriodSeconds: pint64(5),
	}
)

type awaiter struct {
	t           *testing.T
	description string
	timeout     time.Duration
	poll        func() (retry bool, err error)
}

func (a *awaiter) withTimeout(timeout time.Duration) *awaiter {
	a.timeout = timeout
	return a
}

func (a *awaiter) withPoll(poll func() (retry bool, err error)) *awaiter {
	a.poll = poll
	return a
}

func newAwaiter(t *testing.T, description string) *awaiter {
	return &awaiter{
		t:           t,
		description: description,
		timeout:     waitTimeout,
	}
}

func (a *awaiter) await() error {
	deadline := time.Now().Add(a.timeout)
	a.t.Logf("Waiting %v for %s...", a.timeout, a.description)
	for {
		retry, err := a.poll()
		if err != nil {
			if time.Now().After(deadline) {
				a.t.Logf("Wait deadline exceeded")
				return err
			}
			if !retry {
				a.t.Logf("Non-retryable error: %v", err)
				return err
			}
			a.t.Logf("%v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		a.t.Logf("Finished waiting for %s", a.description)
		return nil
	}
}

func resourceCreated(t *testing.T, kind string, name string, k8sInterface interface{}) *awaiter {
	get := reflect.ValueOf(k8sInterface).MethodByName("Get")
	return newAwaiter(t, fmt.Sprintf("creation of %s %s", kind, name)).withPoll(func() (bool, error) {
		result := get.Call([]reflect.Value{
			reflect.ValueOf(context.Background()),
			reflect.ValueOf(name),
			reflect.ValueOf(metav1.GetOptions{}),
		})
		err := result[1].Interface()
		if err != nil {
			return apiErrors.IsNotFound(err.(error)), err.(error)
		}
		return false, nil
	})
}

func resourceDeleted(t *testing.T, kind string, name string, k8sInterface interface{}) *awaiter {
	get := reflect.ValueOf(k8sInterface).MethodByName("Get")
	return newAwaiter(t, fmt.Sprintf("deletion of %s %s", kind, name)).withPoll(func() (bool, error) {
		result := get.Call([]reflect.Value{
			reflect.ValueOf(context.Background()),
			reflect.ValueOf(name),
			reflect.ValueOf(metav1.GetOptions{}),
		})
		err := result[1].Interface()
		if err != nil && apiErrors.IsNotFound(err.(error)) {
			return false, nil
		} else if err != nil {
			return false, err.(error)
		}
		return true, fmt.Errorf("%s %s still exists", kind, name)
	})
}

func removeZeroWeights(weights map[string]float64) {
	for k, v := range weights {
		if v == 0 {
			delete(weights, k)
		}
	}
}

// passed to trafficWeightsUpdatedIngress as a parameter
// func that takes zero inclusive actual traffic weights as a parameter and
// returns an error if they don't meet some criteria.
// will never be retried
type trafficAsserter func(map[string]float64) error

func trafficWeightsUpdatedIngress(t *testing.T, ingressName string, expectedWeights map[string]float64, asserter trafficAsserter) *awaiter {
	removeZeroWeights(expectedWeights)
	return newAwaiter(t, fmt.Sprintf("update of traffic weights in ingress %s", ingressName)).withPoll(func() (retry bool, err error) {
		predicates := map[string]string{}
		for stack := range expectedWeights {
			ingSeg, err := ingressInterface().Get(
				context.Background(),
				fmt.Sprintf("%s-traffic-segment", stack),
				metav1.GetOptions{},
			)

			if err != nil {
				return true, err
			}

			predicates[stack] = ingSeg.Annotations[core.IngressPredicateKey]
		}

		actualWeights := map[string]float64{}
		for version, predicate := range predicates {
			lower, upper, err := core.GetSegmentLimits(predicate)
			if err != nil {
				return false, err
			}
			actualWeights[version] = (upper - lower) * 100
		}

		if asserter != nil {
			err = asserter(actualWeights)
			if err != nil {
				return false, err
			}
		}

		removeZeroWeights(actualWeights)

		if !reflect.DeepEqual(actualWeights, expectedWeights) {
			return true, fmt.Errorf("%s: weights %v != expected %v", ingressName, actualWeights, expectedWeights)
		}
		return false, nil
	}).withTimeout(trafficSwitchWaitTimeout)
}

func trafficWeightsUpdatedStackset(t *testing.T, stacksetName string, kind weightKind, expectedWeights map[string]float64, asserter trafficAsserter) *awaiter {
	removeZeroWeights(expectedWeights)
	timeout := waitTimeout
	if kind == weightKindActual {
		timeout = trafficSwitchWaitTimeout
	}
	return newAwaiter(t, fmt.Sprintf("update of traffic weights in stackSet %s", stacksetName)).withPoll(func() (retry bool, err error) {
		stackset, err := stacksetInterface().Get(context.Background(), stacksetName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		actualWeights := getStacksetTrafficWeights(stackset, kind)

		if asserter != nil {
			err = asserter(actualWeights)
			if err != nil {
				return false, err
			}
		}

		removeZeroWeights(actualWeights)

		if !reflect.DeepEqual(actualWeights, expectedWeights) {
			return true, fmt.Errorf("%s: weights %v != expected %v", stacksetName, actualWeights, expectedWeights)
		}
		return false, nil
	}).withTimeout(timeout)
}

type expectedStackStatus struct {
	replicas             *int32
	readyReplicas        *int32
	updatedReplicas      *int32
	actualTrafficWeight  *float64
	desiredTrafficWeight *float64
}

func (expected expectedStackStatus) matches(stack *zv1.Stack) error {
	status := stack.Status
	if expected.replicas != nil && status.Replicas != *expected.replicas {
		return fmt.Errorf("%s: replicas %d != expected %d", stack.Name, status.Replicas, *expected.replicas)
	}
	if expected.updatedReplicas != nil && status.Replicas != *expected.updatedReplicas {
		return fmt.Errorf("%s: updatedReplicas %d != expected %d", stack.Name, status.UpdatedReplicas, *expected.updatedReplicas)
	}
	if expected.readyReplicas != nil && status.Replicas != *expected.readyReplicas {
		return fmt.Errorf("%s: readyReplicas %d != expected %d", stack.Name, status.ReadyReplicas, *expected.readyReplicas)
	}
	if expected.actualTrafficWeight != nil && status.ActualTrafficWeight != *expected.actualTrafficWeight {
		return fmt.Errorf("%s: actualTrafficWeight %f != expected %f", stack.Name, status.ActualTrafficWeight, *expected.actualTrafficWeight)
	}
	if expected.desiredTrafficWeight != nil && status.DesiredTrafficWeight != *expected.desiredTrafficWeight {
		return fmt.Errorf("%s: desiredTrafficWeight %f != expected %f", stack.Name, status.DesiredTrafficWeight, *expected.desiredTrafficWeight)
	}
	return nil
}

func stackStatusMatches(t *testing.T, stackName string, expectedStatus expectedStackStatus) *awaiter {
	return newAwaiter(t, fmt.Sprintf("stack %s to reach desired condition", stackName)).withPoll(func() (retry bool, err error) {
		stack, err := stacksetClient.ZalandoV1().Stacks(namespace).Get(context.Background(), stackName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return true, expectedStatus.matches(stack)
	})
}

type expectedStackSetStatus struct {
	observedStackVersion string
	actualTraffic        []*zv1.ActualTraffic
}

func (expected expectedStackSetStatus) matches(stackSet *zv1.StackSet) error {
	status := stackSet.Status
	if expected.observedStackVersion != status.ObservedStackVersion {
		return fmt.Errorf("%s: observedStackVersion %s != expected %s", stackSet.Name, status.ObservedStackVersion, expected.observedStackVersion)
	}

	if expected.actualTraffic != nil {
		h := make(map[string]*zv1.ActualTraffic)
		for i := range status.Traffic {
			t := status.Traffic[i]
			h[t.ServiceName] = t
		}
		exp := make(map[string]*zv1.ActualTraffic)
		for i := range expected.actualTraffic {
			t := expected.actualTraffic[i]
			exp[t.ServiceName] = t
		}
		if !reflect.DeepEqual(exp, h) {
			return fmt.Errorf("%s: actual traffic: %+v, expected: %+v, diff:\n%v", stackSet.Name, h, exp, cmp.Diff(h, exp))
		}
	}
	return nil
}

func stackSetStatusMatches(t *testing.T, stackSetName string, expectedStatus expectedStackSetStatus) *awaiter {
	return newAwaiter(t, fmt.Sprintf("stack %s to reach desired condition", stackSetName)).withPoll(func() (retry bool, err error) {
		stackSet, err := stacksetClient.ZalandoV1().StackSets(namespace).Get(context.Background(), stackSetName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return true, expectedStatus.matches(stackSet)

	})
}

func stackObjectMeta(name string, prescalingTimeout int) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{
		Name:        name,
		Namespace:   namespace,
		Annotations: map[string]string{},
		Labels:      map[string]string{"application": name},
	}
	if controllerId != "" {
		meta.Annotations[controller.StacksetControllerControllerAnnotationKey] = controllerId
	}
	if prescalingTimeout > 0 {
		meta.Annotations[controller.PrescaleStacksAnnotationKey] = "yes"
		meta.Annotations[controller.ResetHPAMinReplicasDelayAnnotationKey] = fmt.Sprintf("%dm", prescalingTimeout)
	}
	return meta
}

func createStackSet(stacksetName string, prescalingTimeout int, spec zv1.StackSetSpec) error {
	stackSet := &zv1.StackSet{
		ObjectMeta: stackObjectMeta(stacksetName, prescalingTimeout),
		Spec:       spec,
	}

	_, err := stacksetInterface().Create(context.Background(), stackSet, metav1.CreateOptions{})
	return err
}

func deleteStackset(stacksetName string) error {
	return stacksetInterface().Delete(
		context.Background(),
		stacksetName,
		metav1.DeleteOptions{},
	)
}

func stacksetExists(stacksetName string) bool {
	_, err := stacksetInterface().Get(context.Background(), stacksetName, metav1.GetOptions{})
	return err == nil
}

func stackExists(stacksetName, stackVersion string) bool {
	fullStackName := fmt.Sprintf("%s-%s", stacksetName, stackVersion)
	_, err := stackInterface().Get(context.Background(), fullStackName, metav1.GetOptions{})
	return err == nil
}

func updateStackSet(stacksetName string, spec zv1.StackSetSpec) error {
	for {
		stackSet, err := stacksetInterface().Get(
			context.Background(),
			stacksetName,
			metav1.GetOptions{},
		)
		if err != nil {
			return err
		}
		// Keep the desired traffic
		spec.Traffic = stackSet.Spec.Traffic
		stackSet.Spec = spec

		_, err = stacksetInterface().Update(
			context.Background(),
			stackSet,
			metav1.UpdateOptions{},
		)

		if apiErrors.IsConflict(err) {
			continue
		}
		return err
	}
}

func waitForStack(t *testing.T, stacksetName, stackVersion string) (*zv1.Stack, error) {
	stackName := fmt.Sprintf("%s-%s", stacksetName, stackVersion)
	err := resourceCreated(t, "stack", stackName, stackInterface()).await()
	if err != nil {
		return nil, err
	}
	return stackInterface().Get(context.Background(), stackName, metav1.GetOptions{})
}

func waitForDeployment(t *testing.T, name string) (*appsv1.Deployment, error) {
	err := resourceCreated(t, "deployment", name, deploymentInterface()).await()
	if err != nil {
		return nil, err
	}
	return deploymentInterface().Get(context.Background(), name, metav1.GetOptions{})
}

func waitForService(t *testing.T, name string) (*corev1.Service, error) {
	err := resourceCreated(t, "service", name, serviceInterface()).await()
	if err != nil {
		return nil, err
	}
	return serviceInterface().Get(context.Background(), name, metav1.GetOptions{})
}

func waitForHPA(t *testing.T, name string) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	err := resourceCreated(t, "hpa", name, hpaInterface()).await()
	if err != nil {
		return nil, err
	}
	return hpaInterface().Get(context.Background(), name, metav1.GetOptions{})
}

func waitForIngress(t *testing.T, name string) (*networkingv1.Ingress, error) {
	err := resourceCreated(t, "ingress", name, ingressInterface()).await()
	if err != nil {
		return nil, err
	}
	return ingressInterface().Get(context.Background(), name, metav1.GetOptions{})
}

func waitForIngressSegment(t *testing.T, name, version string) (
	*networkingv1.Ingress,
	error,
) {
	return waitForIngress(
		t,
		fmt.Sprintf(
			"%s-%s-traffic-segment",
			name,
			version,
		),
	)
}

func waitForRouteGroup(t *testing.T, name string) (*rgv1.RouteGroup, error) {
	err := resourceCreated(t, "routegroup", name, routegroupInterface()).await()
	if err != nil {
		return nil, err
	}
	return routegroupInterface().Get(context.Background(), name, metav1.GetOptions{})
}

func waitForRouteGroupSegment(t *testing.T, name, version string) (
	*rgv1.RouteGroup,
	error,
) {
	return waitForRouteGroup(
		t,
		fmt.Sprintf(
			"%s-%s-traffic-segment",
			name,
			version,
		),
	)
}

func waitForConfigMap(t *testing.T, configMapName string) (*corev1.ConfigMap, error) {
	err := resourceCreated(t, "configmap", configMapName, configMapInterface()).await()
	if err != nil {
		return nil, err
	}
	return configMapInterface().Get(context.Background(), configMapName, metav1.GetOptions{})
}

func waitForConfigMapOwnerReferences(t *testing.T, configMapName string, expectedOwnerReferences []metav1.OwnerReference) *awaiter {
	return newAwaiter(t, fmt.Sprintf("configmap %s to reach desired ownerReferences", configMapName)).withPoll(func() (retry bool, err error) {
		configMap, err := configMapInterface().Get(context.Background(), configMapName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if !reflect.DeepEqual(expectedOwnerReferences, configMap.OwnerReferences) {
			return true, fmt.Errorf("%s: actual ownerReferences: %+v, expected: %+v, diff:\n%v", configMapName, configMap.OwnerReferences, expectedOwnerReferences, cmp.Diff(configMap.OwnerReferences, expectedOwnerReferences))
		}
		return true, nil
	})
}

func waitForSecret(t *testing.T, secretName string) (*corev1.Secret, error) {
	err := resourceCreated(t, "secret", secretName, secretInterface()).await()
	if err != nil {
		return nil, err
	}
	return secretInterface().Get(context.Background(), secretName, metav1.GetOptions{})
}

func waitForSecretOwnerReferences(t *testing.T, secretName string, expectedOwnerReferences []metav1.OwnerReference) *awaiter {
	return newAwaiter(t, fmt.Sprintf("secret %s to reach desired ownerReferences", secretName)).withPoll(func() (retry bool, err error) {
		secret, err := secretInterface().Get(context.Background(), secretName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if !reflect.DeepEqual(expectedOwnerReferences, secret.OwnerReferences) {
			return true, fmt.Errorf("%s: actual ownerReferences: %+v, expected: %+v, diff:\n%v", secretName, secret.OwnerReferences, expectedOwnerReferences, cmp.Diff(secret.OwnerReferences, expectedOwnerReferences))
		}
		return true, nil
	})
}

func waitForPlatformCredentialsSet(t *testing.T, pcsName string) (*zv1.PlatformCredentialsSet, error) {
	err := resourceCreated(t, "platformCredentialsSet", pcsName, platformCredentialsSetInterface()).await()
	if err != nil {
		return nil, err
	}
	return platformCredentialsSetInterface().Get(context.Background(), pcsName, metav1.GetOptions{})
}

func getStacksetTrafficWeights(stackset *zv1.StackSet, kind weightKind) map[string]float64 {
	result := make(map[string]float64)

	switch kind {
	case weightKindActual:
		t := stackset.Status.Traffic
		for _, o := range t {
			result[o.ServiceName] = o.Weight
		}
	case weightKindDesired:
		t := stackset.Spec.Traffic
		for _, o := range t {
			result[o.StackName] = o.Weight
		}
	}
	return result
}

func setDesiredTrafficWeightsStackset(stacksetName string, weights map[string]float64) error {
	for {

		stackset, err := stacksetInterface().Get(context.Background(), stacksetName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		trafficSpec := make([]*zv1.DesiredTraffic, 0, len(weights))
		for k, v := range weights {
			trafficSpec = append(trafficSpec, &zv1.DesiredTraffic{
				StackName: k,
				Weight:    v,
			})
		}
		stackset.Spec.Traffic = trafficSpec
		_, err = stacksetInterface().Update(context.Background(), stackset, metav1.UpdateOptions{})
		if apiErrors.IsConflict(err) {
			continue
		}
		return err
	}
}

func createConfigMap(t *testing.T, name string) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: map[string]string{
			"key": "value",
		},
	}
	_, err := configMapInterface().Create(context.Background(), configMap, metav1.CreateOptions{})
	require.NoError(t, err)
}

func createSecret(t *testing.T, name string) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}
	_, err := secretInterface().Create(context.Background(), secret, metav1.CreateOptions{})
	require.NoError(t, err)
}

func pint32(i int32) *int32 {
	return &i
}

func pint64(i int64) *int64 {
	return &i
}

func pfloat64(i float64) *float64 {
	return &i
}
