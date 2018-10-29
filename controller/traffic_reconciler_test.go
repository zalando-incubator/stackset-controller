package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

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
				"svc-1": 0.0,
				"svc-2": 0.0,
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
				"svc-1": 0.0,
				"svc-2": 100.0,
				"svc-3": 0.0,
			},
			expectedAllWeights: map[string]float64{
				"svc-1": 0.0,
				"svc-2": 50.0,
				"svc-3": 50.0,
			},
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			trafficReconciler := &PrescaleTrafficReconciler{}
			availableWeights, allWeights := trafficReconciler.ReconcileIngress(ti.stacks, ti.ingress, ti.traffic)
			require.Equal(t, ti.expectedAvailableWeights, availableWeights)
			require.Equal(t, ti.expectedAllWeights, allWeights)
		})
	}
}
