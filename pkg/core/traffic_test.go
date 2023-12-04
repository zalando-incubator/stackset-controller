package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	hourAgo        = time.Now().Add(-time.Hour)
	fiveMinutesAgo = time.Now().Add(-5 * time.Minute)
)

func TestTrafficSwitchNoIngress(t *testing.T) {
	for reconcilerName, reconciler := range map[string]TrafficReconciler{
		"simple": SimpleTrafficReconciler{},
		"prescale": PrescalingTrafficReconciler{
			ResetHPAMinReplicasTimeout: time.Minute,
		},
	} {
		t.Run(reconcilerName, func(t *testing.T) {
			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{},
				},
				StackContainers: map[types.UID]*StackContainer{
					"v1": testStack("foo-v1").ready(3).traffic(50, 50).prescaling(3, 10, time.Now()).stack(),
					"v2": testStack("foo-v2").ready(3).traffic(50, 50).stack(),
				},
				TrafficReconciler:        reconciler,
				GradualRolloutReconciler: &GradualRolloutReconciler{},
			}
			err := c.ManageTraffic(time.Now())
			require.NoError(t, err)
			for _, sc := range c.StackContainers {
				require.EqualValues(t, 0, sc.desiredTrafficWeight)
				require.EqualValues(t, 0, sc.actualTrafficWeight)
				require.Equal(t, time.Time{}, sc.noTrafficSince)
				require.Equal(t, false, sc.prescalingActive)
				require.EqualValues(t, 0, sc.prescalingReplicas)
				require.Equal(t, time.Time{}, sc.prescalingLastTrafficIncrease)
			}
		})
	}
}

func TestTrafficSwitchSimpleReadyByPercentage(t *testing.T) {
	for _, tc := range []struct {
		name               string
		stack              *StackContainer
		resourcesUpdated   bool
		deploymentReplicas int32
		minReadyPercent    int
		updatedReplicas    int32
		readyReplicas      int32
		expectedError      string
	}{
		{
			name:               "min percent reachead",
			resourcesUpdated:   true,
			deploymentReplicas: 100,
			minReadyPercent:    85,
			updatedReplicas:    85,
			readyReplicas:      85,
		},
		{
			name:               "min percent not reachead on updates",
			resourcesUpdated:   true,
			deploymentReplicas: 100,
			minReadyPercent:    85,
			updatedReplicas:    84,
			readyReplicas:      85,
			expectedError:      "stacks not ready: foo-v1",
		},
		{
			name:               "min percent not reachead on readiness",
			resourcesUpdated:   true,
			deploymentReplicas: 100,
			minReadyPercent:    85,
			updatedReplicas:    85,
			readyReplicas:      84,
			expectedError:      "stacks not ready: foo-v1",
		},
		{
			name:               "min percent reachead, decimal result",
			resourcesUpdated:   true,
			deploymentReplicas: 3,
			minReadyPercent:    50,
			updatedReplicas:    2,
			readyReplicas:      2,
		},
		{
			name:               "min percent under 0",
			resourcesUpdated:   true,
			deploymentReplicas: 3,
			minReadyPercent:    -10, // normalized to 100
			updatedReplicas:    3,
			readyReplicas:      3,
		},
		{
			name:               "min percent at 0",
			resourcesUpdated:   true,
			deploymentReplicas: 3,
			minReadyPercent:    0, // normalized to 100
			updatedReplicas:    3,
			readyReplicas:      3,
		},
		{
			name:               "min percent over 100",
			resourcesUpdated:   true,
			deploymentReplicas: 3,
			minReadyPercent:    200, // normalized to 100
			updatedReplicas:    3,
			readyReplicas:      3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress:         &zv1.StackSetIngressSpec{},
						MinReadyPercent: tc.minReadyPercent,
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					"v1": testStack("foo-v1").traffic(70, 30).deployment(tc.resourcesUpdated, tc.deploymentReplicas, tc.updatedReplicas, tc.readyReplicas).stack(),
					"v2": testStack("foo-v2").traffic(30, 70).ready(3).stack(),
				},
				TrafficReconciler: SimpleTrafficReconciler{},
			}
			err := c.ManageTraffic(time.Now())
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTrafficSwitchSimpleNotReady(t *testing.T) {
	for _, tc := range []struct {
		name               string
		stack              *StackContainer
		resourcesUpdated   bool
		deploymentReplicas int32
		minReadyPercent    int
		updatedReplicas    int32
		readyReplicas      int32
	}{
		{
			name:               "deployment not updated yet",
			resourcesUpdated:   false,
			deploymentReplicas: 3,
			updatedReplicas:    3,
			readyReplicas:      3,
		},
		{
			name:               "not enough updated replicas",
			resourcesUpdated:   true,
			deploymentReplicas: 3,
			updatedReplicas:    2,
			readyReplicas:      3,
		},
		{
			name:               "not enough ready replicas",
			resourcesUpdated:   true,
			deploymentReplicas: 3,
			updatedReplicas:    3,
			readyReplicas:      2,
		},
		{
			name:               "deployment scaled down",
			resourcesUpdated:   true,
			deploymentReplicas: 0,
			updatedReplicas:    0,
			readyReplicas:      0,
		},
		{
			name:               "min percent not reachead",
			resourcesUpdated:   true,
			deploymentReplicas: 100,
			minReadyPercent:    85,
			updatedReplicas:    100,
			readyReplicas:      84,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress:         &zv1.StackSetIngressSpec{},
						MinReadyPercent: tc.minReadyPercent,
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					"v1": testStack("foo-v1").traffic(70, 30).deployment(tc.resourcesUpdated, tc.deploymentReplicas, tc.updatedReplicas, tc.readyReplicas).stack(),
					"v2": testStack("foo-v2").traffic(30, 70).ready(3).stack(),
				},
				TrafficReconciler:        SimpleTrafficReconciler{},
				GradualRolloutReconciler: &GradualRolloutReconciler{},
			}
			err := c.ManageTraffic(time.Now())
			require.Error(t, err)
			require.Equal(t, "stacks not ready: foo-v1", err.Error())
		})
	}
}

func TestTrafficSwitchSimple(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		minReadyPercent        int
		stacks                 map[types.UID]*StackContainer
		expectedDesiredWeights map[string]float64
		expectedActualWeights  map[string]float64
		expectedError          string
	}{
		{
			name:            "traffic is switched if all the stacks are ready",
			minReadyPercent: 100,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 70).ready(1).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 30).ready(1).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).ready(1).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
		},
		{
			name:            "traffic is switched if all the stacks are ready",
			minReadyPercent: 0,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 70).ready(1).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 30).ready(1).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).ready(1).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
		},
		{
			name:            "traffic is switched if 85% of the stacks are ready",
			minReadyPercent: 85,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 70).ready(100).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 30).ready(100).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).partiallyReady(85, 100).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
		},
		{
			name:            "traffic is not switched if less than 85% the stacks are ready",
			minReadyPercent: 85,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 70).ready(100).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 30).ready(100).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).partiallyReady(84, 100).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 70,
				"foo-v2": 30,
				"foo-v3": 0,
			},
			expectedError: "stacks not ready: foo-v3",
		},
		{
			name:            "traffic is switched if 85% of the stacks are ready, decimal result",
			minReadyPercent: 85,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 70).ready(3).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 30).ready(3).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).partiallyReady(2, 3).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 70,
				"foo-v2": 30,
				"foo-v3": 0,
			},
			expectedError: "stacks not ready: foo-v3",
		},
		{
			name:            "traffic is switched if 50% of the stacks are ready, decimal result",
			minReadyPercent: 50,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 70).ready(3).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 30).ready(3).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).partiallyReady(2, 3).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
		},
		{
			name:            "traffic is switched even if some of the stacks are not ready, if their traffic weight is reduced",
			minReadyPercent: 100,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 70).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 30).ready(1).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).ready(1).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
		},
		{
			name:            "if there are no stacks with traffic, send traffic to the stack that had it most recently",
			minReadyPercent: 100,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 0).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 0).noTrafficSince(fiveMinutesAgo).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).noTrafficSince(hourAgo).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 0,
				"foo-v2": 100,
				"foo-v3": 0,
			},
			expectedError: "stacks not ready: foo-v1, foo-v3",
		},
		{
			name:            "traffic weights are normalised",
			minReadyPercent: 100,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(1, 7).ready(1).stack(),
				"foo-v2": testStack("foo-v2").traffic(2, 3).ready(1).stack(),
				"foo-v3": testStack("foo-v3").traffic(1, 0).ready(1).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
		},
		{
			name:            "traffic weights are normalised even if actual traffic switching fails",
			minReadyPercent: 100,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(1, 7).stack(),
				"foo-v2": testStack("foo-v2").traffic(2, 3).stack(),
				"foo-v3": testStack("foo-v3").traffic(1, 0).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 70,
				"foo-v2": 30,
				"foo-v3": 0,
			},
			expectedError: "stacks not ready: foo-v2, foo-v3",
		},
		{
			name:            "if there are no stacks with a desired traffic weight, send traffic to the stack that had it most recently",
			minReadyPercent: 100,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(0, 10).stack(),
				"foo-v2": testStack("foo-v2").traffic(0, 30).noTrafficSince(fiveMinutesAgo).stack(),
				"foo-v3": testStack("foo-v3").traffic(0, 60).noTrafficSince(hourAgo).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 0,
				"foo-v2": 100,
				"foo-v3": 0,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 10,
				"foo-v2": 30,
				"foo-v3": 60,
			},
			expectedError: "stacks not ready: foo-v2",
		},
		{
			name:            "if there are no stacks with a desired traffic weight, and no stacks had traffic recently, send traffic to the earliest created stack",
			minReadyPercent: 100,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(0, 10).createdAt(fiveMinutesAgo).stack(),
				"foo-v2": testStack("foo-v2").traffic(0, 30).stack(),
				"foo-v3": testStack("foo-v3").traffic(0, 60).createdAt(fiveMinutesAgo).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 0,
				"foo-v2": 100,
				"foo-v3": 0,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 10,
				"foo-v2": 30,
				"foo-v3": 60,
			},
			expectedError: "stacks not ready: foo-v2",
		},
		{
			name:            "if there are no stacks with traffic, and no stacks had traffic recently, send traffic to the earliest created stack",
			minReadyPercent: 100,
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 0).createdAt(fiveMinutesAgo).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 0).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).createdAt(fiveMinutesAgo).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 0,
				"foo-v2": 100,
				"foo-v3": 0,
			},
			expectedError: "stacks not ready: foo-v1, foo-v3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress:         &zv1.StackSetIngressSpec{},
						MinReadyPercent: tc.minReadyPercent,
					},
				},
				StackContainers:          tc.stacks,
				TrafficReconciler:        SimpleTrafficReconciler{},
				GradualRolloutReconciler: &GradualRolloutReconciler{},
			}

			err := c.ManageTraffic(time.Now())
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err.Error())
			} else {
				require.NoError(t, err)
			}

			for name, weight := range tc.expectedDesiredWeights {
				require.Equal(t, weight, c.StackContainers[types.UID(name)].desiredTrafficWeight, "desired weight, stack %s", name)
			}
			for name, weight := range tc.expectedActualWeights {
				require.Equal(t, weight, c.StackContainers[types.UID(name)].actualTrafficWeight, "actual weight, stack %s", name)
			}
		})
	}
}

func TestTrafficSwitchPrescaling(t *testing.T) {
	now := time.Now()
	minuteAgo := now.Add(-time.Minute)

	type expectedPrescale struct {
		replicas             int32
		desiredTrafficWeight float64
		lastTrafficIncrease  time.Time
	}

	for _, tc := range []struct {
		name                    string
		stacks                  map[types.UID]*StackContainer
		expectedPrescaledStacks map[string]expectedPrescale
		expectedDesiredWeights  map[string]float64
		expectedActualWeights   map[string]float64
		expectedError           string
	}{
		{
			name: "stacks are prescaled first and traffic is not switched",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 50).ready(2).stack(),
				"foo-v2": testStack("foo-v2").traffic(25, 40).deployment(false, 3, 2, 2).stack(),
				"foo-v3": testStack("foo-v3").traffic(40, 10).ready(1).stack(),
				"foo-v4": testStack("foo-v4").traffic(10, 0).stack(),
			},
			// 2+3+1/100 = 0.06 replicas per 1% of traffoc
			expectedPrescaledStacks: map[string]expectedPrescale{
				"foo-v3": {3, 40, now}, // 2.4 replicas rounded up
				"foo-v4": {1, 10, now}, // 0.6 replicas rounded up
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 40,
				"foo-v4": 10,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 50,
				"foo-v2": 40,
				"foo-v3": 10,
			},
			expectedError: "stacks not ready: foo-v3, foo-v4",
		},
		{
			name: "already prescaled stacks also contribute to the replica count (prescalingReplicas and prescalingDesiredTargetWeight)",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 50).ready(2).stack(),
				"foo-v2": testStack("foo-v2").traffic(25, 40).deployment(false, 3, 2, 2).stack(),
				"foo-v3": testStack("foo-v3").traffic(40, 9).ready(1).stack(),
				"foo-v4": testStack("foo-v4").traffic(10, 1).deployment(false, 3, 2, 2).prescaling(3, 10, minuteAgo).stack(),
			},
			// (2+3+1+3)/(50+40+9+10) ≈ 0.083 replicas per 1% of traffic
			expectedPrescaledStacks: map[string]expectedPrescale{
				"foo-v3": {4, 40, now}, // 3.3 replicas rounded up
				"foo-v4": {3, 10, now}, // already prescaled
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 40,
				"foo-v4": 10,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 50,
				"foo-v2": 40,
				"foo-v3": 9,
				"foo-v4": 1,
			},
			expectedError: "stacks not ready: foo-v3, foo-v4",
		},
		{
			name: "already prescaled stacks also contribute to the replica count (deploymentReplicas and actualTrafficWeight)",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 50).ready(2).stack(),
				"foo-v2": testStack("foo-v2").traffic(25, 40).deployment(false, 3, 2, 2).stack(),
				"foo-v3": testStack("foo-v3").traffic(40, 9).ready(1).stack(),
				"foo-v4": testStack("foo-v4").traffic(10, 1).deployment(false, 10, 2, 2).prescaling(3, 10, minuteAgo).stack(),
			},
			// (2+3+1+10)/(50+40+9+1) = 0.16 replicas per 1% of traffic
			expectedPrescaledStacks: map[string]expectedPrescale{
				"foo-v3": {7, 40, now}, // 6.4 replicas rounded up
				"foo-v4": {3, 10, now}, // already prescaled
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 40,
				"foo-v4": 10,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 50,
				"foo-v2": 40,
				"foo-v3": 9,
				"foo-v4": 1,
			},
			expectedError: "stacks not ready: foo-v3, foo-v4",
		},
		{
			name: "already prescaled stacks are rescaled if the desired traffic weight is increased",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 50).ready(5).stack(),
				"foo-v2": testStack("foo-v2").traffic(25, 40).deployment(false, 4, 2, 2).stack(),
				"foo-v3": testStack("foo-v3").traffic(10, 10).ready(2).stack(),
				"foo-v4": testStack("foo-v4").traffic(40, 0).deployment(false, 1, 2, 2).prescaling(1, 10, minuteAgo).stack(),
			},
			// (5+4+2+1)/(50+40+10+10) ≈ 0.11 replicas per 1% of traffic
			expectedPrescaledStacks: map[string]expectedPrescale{
				"foo-v4": {5, 40, now}, // 4.3 replicas rounded up
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 10,
				"foo-v4": 40,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 50,
				"foo-v2": 40,
				"foo-v3": 10,
				"foo-v4": 0,
			},
			expectedError: "stacks not ready: foo-v4",
		},
		{
			name: "traffic is not switched until stacks reach the desired number of replicas",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 50).ready(2).stack(),
				"foo-v2": testStack("foo-v2").traffic(25, 40).ready(4).stack(),
				"foo-v3": testStack("foo-v3").traffic(50, 10).prescaling(4, 50, minuteAgo).ready(1).stack(),
			},
			expectedPrescaledStacks: map[string]expectedPrescale{
				"foo-v3": {4, 50, now},
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 50,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 50,
				"foo-v2": 40,
				"foo-v3": 10,
			},
			expectedError: "stacks not ready: foo-v3",
		},
		{
			name: "when stacks are prescaled, max. replicas is capped by the HPA",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 50).ready(20).stack(),
				"foo-v2": testStack("foo-v2").traffic(25, 40).ready(30).stack(),
				"foo-v3": testStack("foo-v3").traffic(50, 10).ready(1).maxReplicas(4).stack(),
			},
			expectedPrescaledStacks: map[string]expectedPrescale{
				"foo-v3": {4, 50, now},
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 50,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 50,
				"foo-v2": 40,
				"foo-v3": 10,
			},
			expectedError: "stacks not ready: foo-v3",
		},
		{
			name: "traffic is switched after prescaling is finished",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 50).ready(2).stack(),
				"foo-v2": testStack("foo-v2").traffic(25, 40).ready(4).stack(),
				"foo-v3": testStack("foo-v3").traffic(50, 10).prescaling(4, 50, minuteAgo).ready(4).stack(),
			},
			expectedPrescaledStacks: map[string]expectedPrescale{
				"foo-v3": {4, 50, now},
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 50,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 50,
			},
		},
		{
			name: "traffic is switched after prescaling is finished",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 50).ready(2).stack(),
				"foo-v2": testStack("foo-v2").traffic(25, 40).ready(4).stack(),
				"foo-v3": testStack("foo-v3").traffic(50, 10).prescaling(4, 50, minuteAgo).ready(4).stack(),
			},
			expectedPrescaledStacks: map[string]expectedPrescale{
				"foo-v3": {4, 50, now},
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 50,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 50,
			},
		},
		{
			name: "once traffic is switched, prescaling timestamp is no longer updated",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 25).ready(2).stack(),
				"foo-v2": testStack("foo-v2").traffic(25, 25).ready(4).stack(),
				"foo-v3": testStack("foo-v3").traffic(50, 50).prescaling(4, 50, minuteAgo).ready(4).stack(),
			},
			expectedPrescaledStacks: map[string]expectedPrescale{
				"foo-v3": {4, 50, minuteAgo},
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 50,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 50,
			},
		},
		{
			name: "prescaling is disabled once the cooldown expires",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 25).ready(2).stack(),
				"foo-v2": testStack("foo-v2").traffic(25, 25).ready(4).stack(),
				"foo-v3": testStack("foo-v3").traffic(50, 50).prescaling(4, 50, now.Add(-5*time.Minute)).ready(4).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 50,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 25,
				"foo-v3": 50,
			},
		},
		{
			name: "traffic is switched even if some of the stacks are not ready, if their traffic weight is reduced",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 70).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 30).ready(1).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).ready(1).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
		},
		{
			name: "traffic weights are normalised even if actual traffic switching fails",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(1, 7).stack(),
				"foo-v2": testStack("foo-v2").traffic(2, 3).stack(),
				"foo-v3": testStack("foo-v3").traffic(1, 0).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 70,
				"foo-v2": 30,
				"foo-v3": 0,
			},
			expectedError: "stacks not ready: foo-v2, foo-v3",
		},
		{
			name: "if there are no stacks with a desired traffic weight, send traffic to the stack that had it most recently",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(0, 10).stack(),
				"foo-v2": testStack("foo-v2").traffic(0, 30).noTrafficSince(fiveMinutesAgo).stack(),
				"foo-v3": testStack("foo-v3").traffic(0, 60).noTrafficSince(hourAgo).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 0,
				"foo-v2": 100,
				"foo-v3": 0,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 10,
				"foo-v2": 30,
				"foo-v3": 60,
			},
			expectedError: "stacks not ready: foo-v2",
		},
		{
			name: "if there are no stacks with a desired traffic weight, and no stacks had traffic recently, send traffic to the earliest created stack",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(0, 10).createdAt(fiveMinutesAgo).stack(),
				"foo-v2": testStack("foo-v2").traffic(0, 30).stack(),
				"foo-v3": testStack("foo-v3").traffic(0, 60).createdAt(fiveMinutesAgo).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 0,
				"foo-v2": 100,
				"foo-v3": 0,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 10,
				"foo-v2": 30,
				"foo-v3": 60,
			},
			expectedError: "stacks not ready: foo-v2",
		},
		{
			name: "if there are no stacks with traffic, send traffic to the stack that had it most recently",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 0).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 0).noTrafficSince(fiveMinutesAgo).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).noTrafficSince(hourAgo).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 0,
				"foo-v2": 100,
				"foo-v3": 0,
			},
			expectedError: "stacks not ready: foo-v1, foo-v3",
		},
		{
			name: "if there are no stacks with traffic, and no stacks had traffic recently, send traffic to the earliest created stack",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": testStack("foo-v1").traffic(25, 0).createdAt(fiveMinutesAgo).stack(),
				"foo-v2": testStack("foo-v2").traffic(50, 0).stack(),
				"foo-v3": testStack("foo-v3").traffic(25, 0).createdAt(fiveMinutesAgo).stack(),
			},
			expectedDesiredWeights: map[string]float64{
				"foo-v1": 25,
				"foo-v2": 50,
				"foo-v3": 25,
			},
			expectedActualWeights: map[string]float64{
				"foo-v1": 0,
				"foo-v2": 100,
				"foo-v3": 0,
			},
			expectedError: "stacks not ready: foo-v1, foo-v3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{},
					},
				},
				StackContainers: tc.stacks,
				TrafficReconciler: PrescalingTrafficReconciler{
					ResetHPAMinReplicasTimeout: 5 * time.Minute,
				},
				GradualRolloutReconciler: &GradualRolloutReconciler{},
			}

			err := c.ManageTraffic(now)
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err.Error())
			} else {
				require.NoError(t, err)
			}

			if tc.expectedPrescaledStacks != nil {
				prescaledStacks := map[string]expectedPrescale{}
				for name := range tc.expectedPrescaledStacks {
					stack := c.StackContainers[types.UID(name)]
					if !stack.prescalingActive {
						continue
					}
					prescaledStacks[name] = expectedPrescale{
						replicas:             stack.prescalingReplicas,
						desiredTrafficWeight: stack.prescalingDesiredTrafficWeight,
						lastTrafficIncrease:  stack.prescalingLastTrafficIncrease,
					}
				}
				require.Equal(t, tc.expectedPrescaledStacks, prescaledStacks)
			}

			if tc.expectedDesiredWeights != nil {
				desiredWeights := map[string]float64{}
				for name := range tc.expectedDesiredWeights {
					desiredWeights[name] = c.StackContainers[types.UID(name)].desiredTrafficWeight
				}
				require.Equal(t, tc.expectedDesiredWeights, desiredWeights)
			}

			if tc.expectedActualWeights != nil {
				actualWeights := map[string]float64{}
				for name := range tc.expectedActualWeights {
					actualWeights[name] = c.StackContainers[types.UID(name)].actualTrafficWeight
				}
				require.Equal(t, tc.expectedActualWeights, actualWeights)
			}
		})
	}
}

func TestTrafficSwitchNoTrafficSince(t *testing.T) {
	for reconcilerName, reconciler := range map[string]TrafficReconciler{
		"simple": SimpleTrafficReconciler{},
		"prescale": PrescalingTrafficReconciler{
			ResetHPAMinReplicasTimeout: time.Minute,
		},
	} {
		t.Run(reconcilerName, func(t *testing.T) {
			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{},
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					"foo-v1": testStack("foo-v1").traffic(90, 0).stack(),
					"foo-v2": testStack("foo-v2").traffic(0, 90).stack(),
					"foo-v3": testStack("foo-v3").traffic(10, 10).stack(),
					"foo-v4": testStack("foo-v4").traffic(0, 0).stack(),
					"foo-v5": testStack("foo-v5").traffic(0, 0).noTrafficSince(fiveMinutesAgo).stack(),
				},
				TrafficReconciler:        reconciler,
				GradualRolloutReconciler: &GradualRolloutReconciler{},
			}

			switchTimestamp := time.Now()
			err := c.ManageTraffic(switchTimestamp)
			require.Error(t, err)

			require.Equal(t, time.Time{}, c.StackContainers["foo-v1"].noTrafficSince, "stacks with desired but no actual traffic must not have noTrafficSince")
			require.Equal(t, time.Time{}, c.StackContainers["foo-v2"].noTrafficSince, "stacks with actual but no desired traffic must not have noTrafficSince")
			require.Equal(t, time.Time{}, c.StackContainers["foo-v3"].noTrafficSince, "stacks with both desired and actual traffic must not have noTrafficSince")
			require.Equal(t, switchTimestamp, c.StackContainers["foo-v4"].noTrafficSince, "stacks with no traffic must have the value noTrafficSince preserved")
			require.Equal(t, fiveMinutesAgo, c.StackContainers["foo-v5"].noTrafficSince, "stacks with no traffic and empty noTrafficSince should have it populated")
		})
	}
}

func TestTrafficChanges(t *testing.T) {
	c := StackSetContainer{
		StackSet: &zv1.StackSet{
			Spec: zv1.StackSetSpec{
				Ingress: &zv1.StackSetIngressSpec{},
			},
		},
		StackContainers: map[types.UID]*StackContainer{
			"foo-v1": testStack("foo-v1").traffic(100, 50).currentActualTrafficWeight(50).stack(),
			"foo-v2": testStack("foo-v2").traffic(0, 40).currentActualTrafficWeight(20).stack(),
			"foo-v3": testStack("foo-v3").traffic(0, 10).currentActualTrafficWeight(30).stack(),
			"foo-v4": testStack("foo-v4").traffic(0, 0).currentActualTrafficWeight(0).stack(),
		},
	}

	expected := []TrafficChange{
		{
			StackName:        "foo-v2",
			OldTrafficWeight: 20,
			NewTrafficWeight: 40,
		},
		{
			StackName:        "foo-v3",
			OldTrafficWeight: 30,
			NewTrafficWeight: 10,
		},
	}
	require.Equal(t, expected, c.TrafficChanges())
}

func TestRoundWeights(t *testing.T) {
	for _, tc := range []struct {
		name     string
		weights  map[string]float64
		expected map[string]float64
	}{
		{
			name: "no decimal, no change",
			weights: map[string]float64{
				"v1": 50.0,
				"v2": 50.0,
			},
			expected: map[string]float64{
				"v1": 50.0,
				"v2": 50.0,
			},
		},
		{
			name: "biggest weight should get most of the remaining weight after rounding down",
			weights: map[string]float64{
				"v1": 49.5,
				"v2": 50.5,
			},
			expected: map[string]float64{
				"v1": 49.0,
				"v2": 51.0,
			},
		},
		{
			name: "biggest fraction should get most of the remaining weight after rounding down",
			weights: map[string]float64{
				"v1": 72.3,
				"v2": 27.7,
			},
			expected: map[string]float64{
				"v1": 72.0,
				"v2": 28.0,
			},
		},
		{
			name: "first sorted name (lexicographical) should get most of the remaining weight when weights are equal after rounding down",
			weights: map[string]float64{
				"v1": 100.0 / 3,
				"v2": 100.0 / 3,
				"v3": 100.0 / 3,
			},
			expected: map[string]float64{
				"v1": 34.0,
				"v2": 33.0,
				"v3": 33.0,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			roundWeights(tc.weights)
			require.Equal(t, tc.expected, tc.weights)
		})
	}
}
