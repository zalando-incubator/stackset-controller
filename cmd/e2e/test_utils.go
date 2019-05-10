package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/zalando-incubator/stackset-controller/controller/entities"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type weightKind string

const (
	defaultWaitTimeout = 30 * time.Second

	stacksetHeritageLabelKey = "stackset"
	stackVersionLabelKey     = "stack-version"

	weightKindDesired weightKind = "zalando.org/stack-traffic-weights"
	weightKindActual  weightKind = "zalando.org/backend-weights"
)

var (
	nginxPod = corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "nginx",
				Image: "nginx",
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 80,
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
					Handler: corev1.Handler{
						HTTPGet: &corev1.HTTPGetAction{
							Path:   "/",
							Port:   intstr.FromInt(80),
							Scheme: corev1.URISchemeHTTP,
						},
					},
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       5,
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
		timeout:     defaultWaitTimeout,
	}
}

func (a *awaiter) await() error {
	deadline := time.Now().Add(a.timeout)
	a.t.Logf("Waiting for %s...", a.description)
	for {
		retry, err := a.poll()
		if err != nil {
			if retry && time.Now().Before(deadline) {
				a.t.Logf("%v", err)
				time.Sleep(1 * time.Second)
				continue
			}
			return err
		}
		a.t.Logf("Finished waiting for %s", a.description)
		return nil
	}
}

func resourceCreated(t *testing.T, kind string, name string, k8sInterface interface{}) *awaiter {
	get := reflect.ValueOf(k8sInterface).MethodByName("Get")
	return newAwaiter(t, fmt.Sprintf("creation of %s %s", kind, name)).withPoll(func() (bool, error) {
		result := get.Call([]reflect.Value{
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

func trafficWeightsUpdated(t *testing.T, ingressName string, kind weightKind, expectedWeights map[string]float64) *awaiter {
	removeZeroWeights(expectedWeights)
	return newAwaiter(t, fmt.Sprintf("update of traffic weights in ingress %s", ingressName)).withPoll(func() (retry bool, err error) {
		ingress, err := ingressInterface().Get(ingressName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		actualWeights := ingressTrafficWeights(ingress, kind)
		removeZeroWeights(actualWeights)
		if !reflect.DeepEqual(actualWeights, expectedWeights) {
			return true, fmt.Errorf("%s: weights %v != expected %v", ingressName, actualWeights, expectedWeights)
		}
		return false, nil
	})
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
		stack, err := stacksetClient.ZalandoV1().Stacks(namespace).Get(stackName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return true, expectedStatus.matches(stack)
	})
}

type expectedStackSetStatus struct {
	observedStackVersion string
}

func (expected expectedStackSetStatus) matches(stackSet *zv1.StackSet) error {
	status := stackSet.Status
	if expected.observedStackVersion != status.ObservedStackVersion {
		return fmt.Errorf("%s: observedStackVersion %s != expected %s", stackSet.Name, status.ObservedStackVersion, expected.observedStackVersion)
	}
	return nil
}

func stackSetStatusMatches(t *testing.T, stackSetName string, expectedStatus expectedStackSetStatus) *awaiter {
	return newAwaiter(t, fmt.Sprintf("stack %s to reach desired condition", stackSetName)).withPoll(func() (retry bool, err error) {
		stackSet, err := stacksetClient.ZalandoV1().StackSets(namespace).Get(stackSetName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return true, expectedStatus.matches(stackSet)

	})
}

func stackObjectMeta(name string, prescalingTimeout int) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Annotations: map[string]string{
			entities.StacksetControllerControllerAnnotationKey: controllerId,
		},
	}
	if prescalingTimeout > 0 {
		meta.Annotations[entities.PrescaleStacksAnnotationKey] = "yes"
		meta.Annotations[entities.ResetHPAMinReplicasDelayAnnotationKey] = fmt.Sprintf("%dm", prescalingTimeout)
	}
	return meta
}

func createStackSet(stacksetName string, prescalingTimeout int, spec zv1.StackSetSpec) error {
	stackSet := &zv1.StackSet{
		ObjectMeta: stackObjectMeta(stacksetName, prescalingTimeout),
		Spec:       spec,
	}
	_, err := stacksetInterface().Create(stackSet)
	return err
}

func stacksetExists(stacksetName string) bool {
	_, err := stacksetInterface().Get(stacksetName, metav1.GetOptions{})
	if err != nil {
		return false
	}
	return true
}

func stackExists(stacksetName, stackVersion string) bool {
	fullStackName := fmt.Sprintf("%s-%s", stacksetName, stackVersion)
	_, err := stackInterface().Get(fullStackName, metav1.GetOptions{})
	if err != nil {
		return false
	}
	return true
}

func updateStackset(stacksetName string, spec zv1.StackSetSpec) error {
	for {
		ss, err := stacksetInterface().Get(stacksetName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		ss.Spec = spec
		_, err = stacksetInterface().Update(ss)
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
	return stackInterface().Get(stackName, metav1.GetOptions{})
}

func waitForDeployment(t *testing.T, name string) (*appsv1.Deployment, error) {
	err := resourceCreated(t, "deployment", name, deploymentInterface()).await()
	if err != nil {
		return nil, err
	}
	return deploymentInterface().Get(name, metav1.GetOptions{})
}

func waitForService(t *testing.T, name string) (*corev1.Service, error) {
	err := resourceCreated(t, "service", name, serviceInterface()).await()
	if err != nil {
		return nil, err
	}
	return serviceInterface().Get(name, metav1.GetOptions{})
}

func waitForHPA(t *testing.T, name string) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	err := resourceCreated(t, "hpa", name, hpaInterface()).await()
	if err != nil {
		return nil, err
	}
	return hpaInterface().Get(name, metav1.GetOptions{})
}

func waitForIngress(t *testing.T, name string) (*extensionsv1beta1.Ingress, error) {
	err := resourceCreated(t, "ingress", name, ingressInterface()).await()
	if err != nil {
		return nil, err
	}
	return ingressInterface().Get(name, metav1.GetOptions{})
}

func ingressTrafficWeights(ingress *extensionsv1beta1.Ingress, kind weightKind) map[string]float64 {
	weights := ingress.Annotations[string(kind)]
	if weights == "" {
		return nil
	}

	var result map[string]float64
	err := json.Unmarshal([]byte(weights), &result)
	if err != nil {
		return nil
	}
	return result
}

func setDesiredTrafficWeights(ingressName string, weights map[string]float64) error {
	for {
		ingress, err := ingressInterface().Get(ingressName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		serializedWeights, err := json.Marshal(weights)
		if err != nil {
			return err
		}
		ingress.Annotations[string(weightKindDesired)] = string(serializedWeights)
		_, err = ingressInterface().Update(ingress)
		if apiErrors.IsConflict(err) {
			continue
		}
		return err
	}
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
