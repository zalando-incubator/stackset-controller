package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zalando-incubator/stackset-controller/controller/entities"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestPrescaleReconcilerReconcileDeployment(tt *testing.T) {
	for _, ti := range []struct {
		msg                string
		deployment         *appsv1.Deployment
		stacks             map[types.UID]*entities.StackContainer
		stack              *zv1.Stack
		traffic            map[string]entities.TrafficStatus
		err                error
		expectedReplicas   int32
		calculatedReplicas int
		prescalingActive   bool
		timestampUpdated   bool
	}{
		{
			msg: "should not prescale deployment replicas if there is an HPA defined",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
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
				Status: zv1.StackStatus{
					Prescaling: zv1.PrescalingStatus{
						Active:              true,
						Replicas:            10,
						LastTrafficIncrease: &metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
					},
				},
			},
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
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
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: entities.StackResources{
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
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{3}[0],
							},
						},
					},
				},
			},
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-3": {
					ActualWeight:  0.0,
					DesiredWeight: 100.0,
				},
			},
			expectedReplicas:   3,
			calculatedReplicas: 10,
			prescalingActive:   true,
			timestampUpdated:   true,
		},
		{
			msg: "should prescale deployment if no HPA is defined",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
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
				Status: zv1.StackStatus{
					Prescaling: zv1.PrescalingStatus{
						Active:              true,
						Replicas:            10,
						LastTrafficIncrease: &metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
					},
				},
			},
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
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
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: entities.StackResources{
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
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{3}[0],
							},
						},
					},
				},
			},
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-3": {
					ActualWeight:  0.0,
					DesiredWeight: 100.0,
				},
			},
			expectedReplicas:   10,
			calculatedReplicas: 10,
			prescalingActive:   true,
			timestampUpdated:   true,
		},
		{
			msg: "disable prescaling if prescaling timeout has elapsed and getting traffic.",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "svc-3",
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
				Status: zv1.StackStatus{
					Prescaling: zv1.PrescalingStatus{
						Active:              true,
						Replicas:            10,
						LastTrafficIncrease: &metav1.Time{Time: time.Now().Add(-2 * DefaultResetMinReplicasDelay)},
					},
				},
			},
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-3": {
					ActualWeight:  100.0,
					DesiredWeight: 100.0,
				},
			},
			expectedReplicas:   3,
			calculatedReplicas: 0,
			prescalingActive:   false,
			timestampUpdated:   false,
		},
		{
			msg: "prescale stack if desired traffic is > 0 (without HPA)",
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
				Spec: zv1.StackSpec{},
			},
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
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
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: entities.StackResources{
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
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: entities.StackResources{
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
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  50.0,
					DesiredWeight: 50.0,
				},
				"svc-3": {
					ActualWeight:  0.0,
					DesiredWeight: 50.0,
				},
			},
			expectedReplicas:   15,
			prescalingActive:   true,
			calculatedReplicas: 15,
			timestampUpdated:   true,
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
					HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
						MinReplicas: &[]int32{3}[0],
						MaxReplicas: 20,
					},
				},
				Status: zv1.StackStatus{
					Prescaling: zv1.PrescalingStatus{
						Active:              false,
						Replicas:            0,
						LastTrafficIncrease: &metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					},
				},
			},
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
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
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: entities.StackResources{
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
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: entities.StackResources{
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
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  50.0,
					DesiredWeight: 50.0,
				},
				"svc-3": {
					ActualWeight:  0.0,
					DesiredWeight: 50.0,
				},
			},
			expectedReplicas:   3,
			prescalingActive:   true,
			calculatedReplicas: 15,
			timestampUpdated:   true,
		},
		{
			msg: "prescale stack  correctly when other stacks have no replicas",
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
						MinReplicas: &[]int32{1}[0],
						MaxReplicas: 20,
					},
					Replicas: pint32(3),
				},
				Status: zv1.StackStatus{
					Prescaling: zv1.PrescalingStatus{
						Active:              false,
						Replicas:            0,
						LastTrafficIncrease: &metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					},
				},
			},
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{0}[0],
							},
						},
					},
				},
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{0}[0],
							},
						},
					},
				},
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: entities.StackResources{
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
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  50.0,
					DesiredWeight: 50.0,
				},
				"svc-3": {
					ActualWeight:  0.0,
					DesiredWeight: 50.0,
				},
			},
			expectedReplicas:   3,
			prescalingActive:   true,
			calculatedReplicas: 3,
			timestampUpdated:   true,
		},
		{
			msg: "prescale stack and limit to maximum allowed by HPA",
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
						MinReplicas: &[]int32{1}[0],
						MaxReplicas: 3,
					},
					Replicas: pint32(3),
				},
				Status: zv1.StackStatus{
					Prescaling: zv1.PrescalingStatus{
						Active:              false,
						Replicas:            0,
						LastTrafficIncrease: &metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					},
				},
			},
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{3}[0],
							},
						},
					},
				},
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{3}[0],
							},
						},
					},
				},
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: entities.StackResources{
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
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  50.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  50.0,
					DesiredWeight: 50.0,
				},
				"svc-3": {
					ActualWeight:  0.0,
					DesiredWeight: 50.0,
				},
			},
			expectedReplicas:   3,
			prescalingActive:   true,
			calculatedReplicas: 3,
			timestampUpdated:   true,
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			trafficReconciler := PrescaleTrafficReconciler{ResetHPAMinReplicasTimeout: DefaultResetMinReplicasDelay}
			err := trafficReconciler.ReconcileDeployment(ti.stacks, ti.stack, ti.traffic, ti.deployment)
			if ti.err != nil {
				require.Error(t, err)
			} else {
				require.Equal(t, ti.expectedReplicas, *ti.deployment.Spec.Replicas)
				require.Equal(t, ti.prescalingActive, ti.stack.Status.Prescaling.Active, "expected prescaling to be %v", ti.prescalingActive)
				if ti.prescalingActive {
					require.EqualValues(t, ti.calculatedReplicas, ti.stack.Status.Prescaling.Replicas)
				}
				if ti.timestampUpdated {
					require.InDelta(t, time.Now().Unix(), ti.stack.Status.Prescaling.LastTrafficIncrease.Unix(), float64(time.Second*10), "last updated is older than 10 seconds")
				}
			}
		})
	}
}

func TestPrescaleReconcilerReconcileHPA(tt *testing.T) {
	for _, ti := range []struct {
		msg                 string
		hpa                 *autoscaling.HorizontalPodAutoscaler
		deployment          *appsv1.Deployment
		stack               *zv1.Stack
		expectedMinReplicas int32
		expectedMaxReplicas int32
		err                 error
	}{
		{
			msg: "minReplicas should match prescale replicas",
			hpa: &autoscaling.HorizontalPodAutoscaler{},
			stack: &zv1.Stack{
				Spec: zv1.StackSpec{
					HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
						MinReplicas: &[]int32{3}[0],
						MaxReplicas: 20,
					},
				},
				Status: zv1.StackStatus{
					Prescaling: zv1.PrescalingStatus{Active: true, Replicas: 10},
				},
			},
			expectedMaxReplicas: 20,
			expectedMinReplicas: 10,
		},
		{
			msg: "stack without prescale annotation should have default MinReplicas.",
			hpa: &autoscaling.HorizontalPodAutoscaler{},
			stack: &zv1.Stack{
				Spec: zv1.StackSpec{
					HorizontalPodAutoscaler: &zv1.HorizontalPodAutoscaler{
						MinReplicas: &[]int32{3}[0],
						MaxReplicas: 20,
					},
				},
				Status: zv1.StackStatus{
					Prescaling: zv1.PrescalingStatus{Active: false, Replicas: 10},
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
			}
		})
	}
}

func TestReconcileIngressTraffic(tt *testing.T) {
	for _, ti := range []struct {
		msg                      string
		stacks                   map[types.UID]*entities.StackContainer
		ingress                  *v1beta1.Ingress
		traffic                  map[string]entities.TrafficStatus
		expectedAvailableWeights map[string]float64
		expectedAllWeights       map[string]float64
	}{
		{
			msg: "stacks without active prescaling status should not get desired traffic if it was already 0",
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
					},
					Resources: entities.StackResources{
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
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  100.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-3": {
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
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
						Status: zv1.StackStatus{
							Prescaling: zv1.PrescalingStatus{Active: true, Replicas: 10},
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
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
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  100.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-3": {
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
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
						Status: zv1.StackStatus{
							Prescaling: zv1.PrescalingStatus{Active: true, Replicas: 10},
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
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
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  100.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-3": {
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
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-2",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
						Status: zv1.StackStatus{
							Prescaling: zv1.PrescalingStatus{Active: true, Replicas: 10},
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
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
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-3": {
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
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
						Status: zv1.StackStatus{
							Prescaling: zv1.PrescalingStatus{Active: true, Replicas: 10},
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-2",
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
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
						Status: zv1.StackStatus{
							Prescaling: zv1.PrescalingStatus{Active: true, Replicas: 10},
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
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
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  100.0,
					DesiredWeight: 50.0,
				},
				"svc-3": {
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
		{
			msg: "test two prescaled stacks one is ready and one is not with both receiving traffic",
			stacks: map[types.UID]*entities.StackContainer{
				types.UID("1"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-1",
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name:        "svc-1",
								Annotations: map[string]string{},
							},
						},
					},
				},
				types.UID("2"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-2",
						},
						Status: zv1.StackStatus{
							Prescaling: zv1.PrescalingStatus{Active: true, Replicas: 10},
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-2",
							},
							Spec: appsv1.DeploymentSpec{
								Replicas: &[]int32{10}[0],
							},
							Status: appsv1.DeploymentStatus{
								ReadyReplicas: 1,
							},
						},
					},
				},
				types.UID("3"): {
					Stack: zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{
							Name: "svc-3",
						},
						Status: zv1.StackStatus{
							Prescaling: zv1.PrescalingStatus{Active: true, Replicas: 10},
						},
					},
					Resources: entities.StackResources{
						Deployment: &appsv1.Deployment{
							ObjectMeta: metav1.ObjectMeta{
								Name: "svc-3",
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
			traffic: map[string]entities.TrafficStatus{
				"svc-1": {
					ActualWeight:  0.0,
					DesiredWeight: 0.0,
				},
				"svc-2": {
					ActualWeight:  50.0,
					DesiredWeight: 30.0,
				},
				"svc-3": {
					ActualWeight:  50.0,
					DesiredWeight: 70.0,
				},
			},
			expectedAvailableWeights: map[string]float64{
				"svc-1": 0.0,
				"svc-2": 50.0,
				"svc-3": 50.0,
			},
			expectedAllWeights: map[string]float64{
				"svc-1": 0.0,
				"svc-2": 30.0,
				"svc-3": 70.0,
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
