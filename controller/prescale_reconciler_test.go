package controller

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestPrescaleReconcilerReconcileDeployment(tt *testing.T) {
	for _, ti := range []struct {
		msg                 string
		deployment          *appsv1.Deployment
		stacks              map[types.UID]*StackContainer
		stack               *zv1.Stack
		traffic             map[string]TrafficStatus
		err                 error
		expectedReplicas    int32
		expectedAnnotations map[string]string
	}{
		{
			msg: "should not prescale deployment replicas if there is an HPA defined",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
					Annotations: map[string]string{
						prescaleAnnotationKey: "10",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &[]int32{3}[0],
				},
			},
			stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
				},
				Spec: zv1.StackSpec{
					HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
						MinReplicas: &[]int32{3}[0],
						MaxReplicas: 20,
					},
				},
			},
			stacks: map[types.UID]*StackContainer{
				types.UID("1"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{5}[0],
							},
						},
					},
				},
				types.UID("2"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{10}[0],
							},
						},
					},
				},
				types.UID("3"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
								Annotations: map[string]string{
									prescaleAnnotationKey: "10",
								},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{3}[0],
							},
						},
					},
				},
			},
			traffic: map[string]TrafficStatus{
				"svc-1": TrafficStatus{
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-2": TrafficStatus{
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-3": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 100.0,
				},
			},
			expectedReplicas: 3,
			expectedAnnotations: map[string]string{
				prescaleAnnotationKey: "10",
			},
		},
		{
			msg: "should prescale deployment if no HPA is defined",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
					Annotations: map[string]string{
						prescaleAnnotationKey: "10",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &[]int32{3}[0],
				},
			},
			stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
				},
				Spec: zv1.StackSpec{
					// HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
					// 	MinReplicas: &[]int32{3}[0],
					// 	MaxReplicas: 20,
					// },
				},
			},
			stacks: map[types.UID]*StackContainer{
				types.UID("1"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{5}[0],
							},
						},
					},
				},
				types.UID("2"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{10}[0],
							},
						},
					},
				},
				types.UID("3"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
								Annotations: map[string]string{
									prescaleAnnotationKey: "10",
								},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{3}[0],
							},
						},
					},
				},
			},
			traffic: map[string]TrafficStatus{
				"svc-1": TrafficStatus{
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-2": TrafficStatus{
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-3": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 100.0,
				},
			},
			expectedReplicas: 10,
			expectedAnnotations: map[string]string{
				prescaleAnnotationKey: "10",
			},
		},
		{
			msg: "remove prescale annotation if already getting traffic.",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
					Annotations: map[string]string{
						prescaleAnnotationKey: "10",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &[]int32{3}[0],
				},
			},
			stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
				},
				Spec: zv1.StackSpec{},
			},
			traffic: map[string]TrafficStatus{
				"svc-1": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-2": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-3": TrafficStatus{
					ActualWeight:  100.0,
					DesiredWeight: 100.0,
				},
			},
			expectedReplicas:    3,
			expectedAnnotations: map[string]string{},
		},
		{
			msg: "prescale stack if desired is > 0",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "svc-3",
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &[]int32{3}[0],
				},
			},
			stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
				},
				Spec: zv1.StackSpec{
					// HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
					// 	MinReplicas: &[]int32{3}[0],
					// 	MaxReplicas: 20,
					// },
				},
			},
			stacks: map[types.UID]*StackContainer{
				types.UID("1"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{5}[0],
							},
						},
					},
				},
				types.UID("2"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{10}[0],
							},
						},
					},
				},
				types.UID("3"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-3",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{3}[0],
							},
						},
					},
				},
			},
			traffic: map[string]TrafficStatus{
				"svc-1": TrafficStatus{
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-2": TrafficStatus{
					ActualWeight:  50.0,
					DesiredWeight: 50.0,
				},
				"svc-3": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 50.0,
				},
			},
			expectedReplicas: 15,
			expectedAnnotations: map[string]string{
				prescaleAnnotationKey: "15",
			},
		},
		{
			msg: "prescale stack if desired is > 0 (with HPA)",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "svc-3",
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &[]int32{3}[0],
				},
			},
			stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
				},
				Spec: zv1.StackSpec{
					// HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
					// 	MinReplicas: &[]int32{3}[0],
					// 	MaxReplicas: 20,
					// },
				},
			},
			stacks: map[types.UID]*StackContainer{
				types.UID("1"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{5}[0],
							},
						},
						HPA: &autoscaling.HorizontalPodAutoscaler{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{10}[0],
							},
						},
					},
				},
				types.UID("3"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-3",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{3}[0],
							},
						},
					},
				},
			},
			traffic: map[string]TrafficStatus{
				"svc-1": TrafficStatus{
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-2": TrafficStatus{
					ActualWeight:  50.0,
					DesiredWeight: 50.0,
				},
				"svc-3": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 50.0,
				},
			},
			expectedReplicas: 15,
			expectedAnnotations: map[string]string{
				prescaleAnnotationKey: "15",
			},
		},
		{
			msg: "prescale stack if desired is > actual (with HPA), cap prescale at Max HPA replicas",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "svc-3",
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &[]int32{3}[0],
				},
			},
			stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
				},
				Spec: zv1.StackSpec{
					HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
						MinReplicas: &[]int32{3}[0],
						MaxReplicas: 10,
					},
				},
			},
			stacks: map[types.UID]*StackContainer{
				types.UID("1"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{5}[0],
							},
						},
						HPA: &autoscaling.HorizontalPodAutoscaler{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{10}[0],
							},
						},
					},
				},
				types.UID("3"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-3",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{3}[0],
							},
						},
					},
				},
			},
			traffic: map[string]TrafficStatus{
				"svc-1": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-2": TrafficStatus{
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-3": TrafficStatus{
					ActualWeight:  50.0,
					DesiredWeight: 100.0,
				},
			},
			expectedReplicas: 3,
			expectedAnnotations: map[string]string{
				prescaleAnnotationKey: "10",
			},
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			trafficReconciler := PrescaleTrafficReconciler{}
			err := trafficReconciler.ReconcileDeployment(ti.stacks, ti.stack, ti.traffic, ti.deployment)
			if ti.err != nil {
				require.Error(t, err)
			} else {
				require.Equal(t, ti.expectedReplicas, *ti.deployment.Spec.Replicas)
				require.Equal(t, ti.expectedAnnotations, ti.deployment.Annotations)
			}
		})
	}
}

func TestPrescaleReconcilerReconcileHPA(tt *testing.T) {
	for _, ti := range []struct {
		msg                       string
		hpa                       *autoscaling.HorizontalPodAutoscaler
		deployment                *appsv1.Deployment
		stack                     *zv1.Stack
		expectedMinReplicas       int32
		expectedMaxReplicas       int32
		expectedHPAAnnotationKeys map[string]struct{}
		err                       error
	}{
		{
			msg: "minReplicas should match prescale replicas",
			hpa: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						prescaleAnnotationKey: "10",
					},
				},
			},
			stack: &zv1.Stack{
				Spec: zv1.StackSpec{
					HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
						MinReplicas: &[]int32{3}[0],
						MaxReplicas: 20,
					},
				},
			},
			expectedMaxReplicas: 20,
			expectedMinReplicas: 10,
			expectedHPAAnnotationKeys: map[string]struct{}{
				resetHPAMinReplicasSinceKey: struct{}{},
			},
		},
		{
			msg: "Invalid prescale replicas should return error",
			hpa: &autoscaling.HorizontalPodAutoscaler{},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						prescaleAnnotationKey: "abc",
					},
				},
			},
			stack: &zv1.Stack{
				Spec: zv1.StackSpec{
					HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
						MinReplicas: &[]int32{10}[0],
						MaxReplicas: 20,
					},
				},
			},
			err: errors.New("failure"),
		},
		{
			msg: "stack without prescale annotation should have default MinReplicas.",
			hpa: &autoscaling.HorizontalPodAutoscaler{},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			stack: &zv1.Stack{
				Spec: zv1.StackSpec{
					HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
						MinReplicas: &[]int32{3}[0],
						MaxReplicas: 20,
					},
				},
			},
			expectedMaxReplicas: 20,
			expectedMinReplicas: 3,
		},
		{
			msg: "re-use HPA minReplicas if the ResetHPAMinReplicasTimeout has not expired",
			hpa: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						resetHPAMinReplicasSinceKey: time.Now().Format(time.RFC3339),
					},
				},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &[]int32{20}[0],
					MaxReplicas: 20,
				},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			stack: &zv1.Stack{
				Spec: zv1.StackSpec{
					HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
						MinReplicas: &[]int32{3}[0],
						MaxReplicas: 20,
					},
				},
			},
			expectedMaxReplicas: 20,
			expectedMinReplicas: 20,
			expectedHPAAnnotationKeys: map[string]struct{}{
				resetHPAMinReplicasSinceKey: struct{}{},
			},
		},
		{
			msg: "invalid date format for 'min-replicas-prescale-since' should be an error.",
			hpa: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						resetHPAMinReplicasSinceKey: "invalid date",
					},
				},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			stack: &zv1.Stack{
				Spec: zv1.StackSpec{
					HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
						MinReplicas: &[]int32{3}[0],
						MaxReplicas: 20,
					},
				},
			},
			err: errors.New("error"),
		},
		{
			msg: "Don't re-use HPA minReplicas if the ResetHPAMinReplicasTimeout has expired",
			hpa: &autoscaling.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						resetHPAMinReplicasSinceKey: time.Now().Add(-(20 * time.Minute)).Format(time.RFC3339),
					},
				},
				Spec: autoscaling.HorizontalPodAutoscalerSpec{
					MinReplicas: &[]int32{20}[0],
					MaxReplicas: 20,
				},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			stack: &zv1.Stack{
				Spec: zv1.StackSpec{
					HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
						MinReplicas: &[]int32{3}[0],
						MaxReplicas: 20,
					},
				},
			},
			expectedMaxReplicas: 20,
			expectedMinReplicas: 3,
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			trafficReconciler := PrescaleTrafficReconciler{
				ResetHPAMinReplicasTimeout: 10 * time.Minute,
			}
			err := trafficReconciler.ReconcileHPA(ti.stack, ti.hpa, ti.deployment)
			if ti.err != nil {
				require.Error(t, err)
			} else {
				require.Equal(t, ti.expectedMinReplicas, *ti.hpa.Spec.MinReplicas)
				require.Equal(t, ti.expectedMaxReplicas, ti.hpa.Spec.MaxReplicas)
				require.Len(t, ti.hpa.Annotations, len(ti.expectedHPAAnnotationKeys))
				for k := range ti.expectedHPAAnnotationKeys {
					require.Contains(t, ti.hpa.Annotations, k)
				}
			}
		})
	}
}

func TestGetDeploymentPrescale(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				prescaleAnnotationKey: "10",
			},
		},
	}

	prescale, ok := getDeploymentPrescale(deployment)
	require.True(t, ok)
	require.Equal(t, int32(10), prescale)

	deployment.Annotations = map[string]string{}
	_, ok = getDeploymentPrescale(deployment)
	require.False(t, ok)

	deployment.Annotations = map[string]string{prescaleAnnotationKey: "abc"}
	_, ok = getDeploymentPrescale(deployment)
	require.False(t, ok)
}

func TestReconcileIngressTraffic(tt *testing.T) {
	for _, ti := range []struct {
		msg                      string
		stacks                   map[types.UID]*StackContainer
		ingress                  *v1beta1.Ingress
		traffic                  map[string]TrafficStatus
		expectedAvailableWeights map[string]float64
		expectedAllWeights       map[string]float64
	}{
		{
			msg: "stacks without prescale annotation should not get desired traffic if it was already 0",
			stacks: map[types.UID]*StackContainer{
				types.UID("1"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("3"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
								Annotations: map[string]string{
									"": "",
								},
							},
						},
					},
				},
			},
			traffic: map[string]TrafficStatus{
				"svc-1": TrafficStatus{
					ActualWeight:  100.0,
					DesiredWeight: 0.0,
				},
				"svc-2": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-3": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 100.0,
				},
			},
			expectedAvailableWeights: map[string]float64{
				"svc-1": 100.0,
				"svc-2": 0.0,
				"svc-3": 0.0,
			},
			expectedAllWeights: map[string]float64{
				"svc-1": 0.0,
				"svc-2": 0.0,
				"svc-3": 100.0,
			},
		},
		{
			msg: "Prescaled stack should get desired traffic",
			stacks: map[types.UID]*StackContainer{
				types.UID("1"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("3"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
								Annotations: map[string]string{
									prescaleAnnotationKey: "10",
								},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{10}[0],
							},
							Status: appsv1.DeploymentStatus{
								ReadyReplicas: 10,
							},
						},
					},
				},
			},
			traffic: map[string]TrafficStatus{
				"svc-1": TrafficStatus{
					ActualWeight:  100.0,
					DesiredWeight: 0.0,
				},
				"svc-2": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-3": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 100.0,
				},
			},
			expectedAvailableWeights: map[string]float64{
				"svc-3": 100.0,
			},
			expectedAllWeights: map[string]float64{
				"svc-1": 0.0,
				"svc-2": 0.0,
				"svc-3": 100.0,
			},
		},
		{
			msg: "Prescaled stack should get not desired traffic if not ready",
			stacks: map[types.UID]*StackContainer{
				types.UID("1"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("3"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
								Annotations: map[string]string{
									prescaleAnnotationKey: "10",
								},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{10}[0],
							},
							Status: appsv1.DeploymentStatus{
								ReadyReplicas: 9, // 9/10 ready
							},
						},
					},
				},
			},
			traffic: map[string]TrafficStatus{
				"svc-1": TrafficStatus{
					ActualWeight:  100.0,
					DesiredWeight: 0.0,
				},
				"svc-2": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-3": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 100.0,
				},
			},
			expectedAvailableWeights: map[string]float64{
				"svc-1": 100.0,
				"svc-2": 0.0,
				"svc-3": 0.0,
			},
			expectedAllWeights: map[string]float64{
				"svc-1": 0.0,
				"svc-2": 0.0,
				"svc-3": 100.0,
			},
		},
		{
			msg: "Prescaled stack with actual traffic should not loose traffic if not all replicas are ready",
			stacks: map[types.UID]*StackContainer{
				types.UID("1"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("3"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
								Annotations: map[string]string{
									prescaleAnnotationKey: "10",
								},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{10}[0],
							},
							Status: appsv1.DeploymentStatus{
								ReadyReplicas: 9, // 9/10 ready
							},
						},
					},
				},
			},
			traffic: map[string]TrafficStatus{
				"svc-1": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-2": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-3": TrafficStatus{
					ActualWeight:  100.0,
					DesiredWeight: 100.0,
				},
			},
			expectedAvailableWeights: map[string]float64{
				"svc-3": 100.0,
			},
			expectedAllWeights: map[string]float64{
				"svc-1": 0.0,
				"svc-2": 0.0,
				"svc-3": 100.0,
			},
		},
		{
			msg: "test two prescaled stacks one is ready and one is not",
			stacks: map[types.UID]*StackContainer{
				types.UID("1"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-2",
								Annotations: map[string]string{
									prescaleAnnotationKey: "10",
								},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{10}[0],
							},
							Status: appsv1.DeploymentStatus{
								ReadyReplicas: 10,
							},
						},
					},
				},
				types.UID("3"): &StackContainer{
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
								Annotations: map[string]string{
									prescaleAnnotationKey: "10",
								},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{10}[0],
							},
							Status: appsv1.DeploymentStatus{
								ReadyReplicas: 9, // 9/10 ready
							},
						},
					},
				},
			},
			traffic: map[string]TrafficStatus{
				"svc-1": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-2": TrafficStatus{
					ActualWeight:  100.0,
					DesiredWeight: 50.0,
				},
				"svc-3": TrafficStatus{
					ActualWeight:  0.0,
					DesiredWeight: 50.0,
				},
			},
			expectedAvailableWeights: map[string]float64{
				// "svc-1": 0.0,
				"svc-2": 100.0,
				// "svc-3": 0.0,
			},
			expectedAllWeights: map[string]float64{
				"svc-1": 0.0,
				"svc-2": 50.0,
				"svc-3": 50.0,
			},
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			trafficReconciler := PrescaleTrafficReconciler{}
			availableWeights, allWeights := trafficReconciler.ReconcileIngress(ti.stacks, ti.ingress, ti.traffic)
			require.Equal(t, ti.expectedAvailableWeights, availableWeights)
			require.Equal(t, ti.expectedAllWeights, allWeights)
		})
	}
}
