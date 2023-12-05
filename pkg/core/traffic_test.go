package core

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	hourAgo        = time.Now().Add(-time.Hour)
	fiveMinutesAgo = time.Now().Add(-5 * time.Minute)

	yesNo = map[bool]string{true: "yes", false: "no"}
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
				TrafficReconciler: reconciler,
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
				TrafficReconciler: SimpleTrafficReconciler{},
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
				StackContainers:   tc.stacks,
				TrafficReconciler: SimpleTrafficReconciler{},
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
				TrafficReconciler: reconciler,
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

func TestNewTrafficSegment(t *testing.T) {
	for _, tc := range []struct {
		stackContainer     *StackContainer
		expectedLowerLimit float64
		expectedUpperLimit float64
		expectIngress      bool
		expectRouteGroup   bool
		expectErr          bool
	}{
		{
			stackContainer: &StackContainer{
				ingressSpec: &zv1.StackSetIngressSpec{},
				Resources: StackResources{
					IngressSegment: &v1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								IngressPredicateKey: "TrafficSegment(0.0, 0.1)",
							},
						},
					},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.1,
			expectIngress:      true,
			expectRouteGroup:   false,
			expectErr:          false,
		},
		{
			stackContainer: &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-v1",
					},
				},
				backendPort: &intstr.IntOrString{
					IntVal: 8080,
				},
				ingressSpec: &zv1.StackSetIngressSpec{
					Hosts: []string{"foo.example.com"},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.0,
			expectIngress:      true,
			expectRouteGroup:   false,
			expectErr:          false,
		},
		{
			stackContainer: &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-v1",
					},
				},
				ingressSpec: &zv1.StackSetIngressSpec{
					Hosts: []string{"foo.example.com"},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.0,
			expectIngress:      false,
			expectRouteGroup:   false,
			expectErr:          true,
		},
		{
			stackContainer: &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-v1",
					},
				},
				backendPort: &intstr.IntOrString{
					IntVal: 8080,
				},
				routeGroupSpec: &zv1.RouteGroupSpec{
					Hosts: []string{"foo.example.com"},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.0,
			expectIngress:      false,
			expectRouteGroup:   true,
			expectErr:          false,
		},
		{
			stackContainer: &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-v1",
					},
				},
				routeGroupSpec: &zv1.RouteGroupSpec{
					Hosts: []string{"foo.example.com"},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.0,
			expectIngress:      false,
			expectRouteGroup:   false,
			expectErr:          true,
		},
		{
			stackContainer: &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-v1",
					},
				},
				backendPort: &intstr.IntOrString{
					IntVal: 8080,
				},
				routeGroupSpec: &zv1.RouteGroupSpec{
					Hosts: []string{"foo.example.com"},
				},
				ingressSpec: &zv1.StackSetIngressSpec{
					Hosts: []string{"foo.example.com"},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.0,
			expectIngress:      true,
			expectRouteGroup:   true,
			expectErr:          false,
		},
		{
			stackContainer: &StackContainer{
				ingressSpec: &zv1.StackSetIngressSpec{},
				Resources: StackResources{
					IngressSegment: &v1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								IngressPredicateKey: "TraficSegment(0.0, 0.1)",
							},
						},
					},
				},
			},
			expectedLowerLimit: -1.0,
			expectedUpperLimit: -1.0,
			expectIngress:      false,
			expectRouteGroup:   false,
			expectErr:          true,
		},
		{
			stackContainer: &StackContainer{
				ingressSpec: &zv1.StackSetIngressSpec{},
				Resources: StackResources{
					IngressSegment: &v1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								IngressPredicateKey: "TrafficSegment(0.1, 0.0)",
							},
						},
					},
				},
			},
			expectedLowerLimit: -1.0,
			expectedUpperLimit: -1.0,
			expectIngress:      false,
			expectRouteGroup:   false,
			expectErr:          true,
		},
		{
			stackContainer: &StackContainer{
				ingressSpec: &zv1.StackSetIngressSpec{},
				Resources: StackResources{
					IngressSegment: &v1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								IngressPredicateKey: "TrafficSegment(-0.8, -0.6)",
							},
						},
					},
				},
			},
			expectedLowerLimit: -1.0,
			expectedUpperLimit: -1.0,
			expectIngress:      false,
			expectRouteGroup:   false,
			expectErr:          true,
		},
		{
			stackContainer: &StackContainer{
				ingressSpec: &zv1.StackSetIngressSpec{},
				Resources: StackResources{
					IngressSegment: &v1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								IngressPredicateKey: "TrafficSegment(0.0, 0.1) && Method(\"GET\")",
							},
						},
					},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.1,
			expectIngress:      true,
			expectRouteGroup:   false,
			expectErr:          false,
		},
		{
			stackContainer: &StackContainer{
				ingressSpec: &zv1.StackSetIngressSpec{},
				Resources: StackResources{
					IngressSegment: &v1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								IngressPredicateKey: "Method(\"GET\") && TrafficSegment(0.0, 0.1)",
							},
						},
					},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.1,
			expectIngress:      true,
			expectRouteGroup:   false,
			expectErr:          false,
		},
		{
			stackContainer: &StackContainer{
				routeGroupSpec: &zv1.RouteGroupSpec{},
				Resources: StackResources{
					RouteGroupSegment: &rgv1.RouteGroup{
						Spec: rgv1.RouteGroupSpec{
							Routes: []rgv1.RouteGroupRouteSpec{
								{
									Predicates: []string{
										"TrafficSegment(0.0, 0.1)",
									},
								},
							},
						},
					},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.1,
			expectIngress:      false,
			expectRouteGroup:   true,
			expectErr:          false,
		},
		{
			stackContainer: &StackContainer{
				routeGroupSpec: &zv1.RouteGroupSpec{},
				Resources: StackResources{
					RouteGroupSegment: &rgv1.RouteGroup{
						Spec: rgv1.RouteGroupSpec{
							Routes: []rgv1.RouteGroupRouteSpec{
								{Predicates: []string{}},
							},
						},
					},
				},
			},
			expectedLowerLimit: -1.0,
			expectedUpperLimit: -1.0,
			expectIngress:      false,
			expectRouteGroup:   false,
			expectErr:          true,
		},
		{
			stackContainer: &StackContainer{
				routeGroupSpec: &zv1.RouteGroupSpec{},
				Resources: StackResources{
					RouteGroupSegment: &rgv1.RouteGroup{
						Spec: rgv1.RouteGroupSpec{
							Routes: []rgv1.RouteGroupRouteSpec{
								{
									Predicates: []string{
										"TrafficSegment(0.0, 0.1)",
										"Method(\"GET\")",
									},
								},
							},
						},
					},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.1,
			expectIngress:      false,
			expectRouteGroup:   true,
			expectErr:          false,
		},
		{
			stackContainer: &StackContainer{
				routeGroupSpec: &zv1.RouteGroupSpec{},
				Resources: StackResources{
					RouteGroupSegment: &rgv1.RouteGroup{
						Spec: rgv1.RouteGroupSpec{
							Routes: []rgv1.RouteGroupRouteSpec{
								{
									Predicates: []string{
										"Method(\"GET\")",
										"TrafficSegment(0.0, 0.1)",
									},
								},
							},
						},
					},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.1,
			expectIngress:      false,
			expectRouteGroup:   true,
			expectErr:          false,
		},
		{
			stackContainer: &StackContainer{
				routeGroupSpec: &zv1.RouteGroupSpec{},
				Resources: StackResources{
					RouteGroupSegment: &rgv1.RouteGroup{
						Spec: rgv1.RouteGroupSpec{
							Routes: []rgv1.RouteGroupRouteSpec{
								{
									Predicates: []string{
										"Method(\"GET\")",
										"Path(\"/hello\")",
									},
								},
							},
						},
					},
				},
			},
			expectedLowerLimit: -1.0,
			expectedUpperLimit: -1.10,
			expectIngress:      false,
			expectRouteGroup:   false,
			expectErr:          true,
		},
		{
			stackContainer: &StackContainer{
				ingressSpec:    &zv1.StackSetIngressSpec{},
				routeGroupSpec: &zv1.RouteGroupSpec{},
				Resources: StackResources{
					IngressSegment: &v1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								IngressPredicateKey: "TrafficSegment(0.0, 0.1) && Method(\"GET\")",
							},
						},
					},
					RouteGroupSegment: &rgv1.RouteGroup{
						Spec: rgv1.RouteGroupSpec{
							Routes: []rgv1.RouteGroupRouteSpec{
								{
									Predicates: []string{
										"TrafficSegment(0.0, 0.1)",
										"Method(\"GET\")",
									},
								},
							},
						},
					},
				},
			},
			expectedLowerLimit: 0.0,
			expectedUpperLimit: 0.1,
			expectIngress:      true,
			expectRouteGroup:   true,
			expectErr:          false,
		},
		{
			stackContainer: &StackContainer{
				ingressSpec:    &zv1.StackSetIngressSpec{},
				routeGroupSpec: &zv1.RouteGroupSpec{},
				Resources: StackResources{
					IngressSegment: &v1.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								IngressPredicateKey: "TrafficSegment(0.0, 0.2) && Method(\"GET\")",
							},
						},
					},
					RouteGroupSegment: &rgv1.RouteGroup{
						Spec: rgv1.RouteGroupSpec{
							Routes: []rgv1.RouteGroupRouteSpec{
								{
									Predicates: []string{
										"TrafficSegment(0.0, 0.1)",
										"Method(\"GET\")",
									},
								},
							},
						},
					},
				},
			},
			expectedLowerLimit: -1.0,
			expectedUpperLimit: -1.0,
			expectIngress:      false,
			expectRouteGroup:   false,
			expectErr:          true,
		},
	} {
		res, err := NewTrafficSegment("id", tc.stackContainer)
		if (err != nil) != tc.expectErr {
			t.Errorf(
				"expected error: %s. got error: %s",
				yesNo[tc.expectErr],
				yesNo[err != nil],
			)
			continue
		}

		if tc.expectErr {
			continue
		}

		if res.lowerLimit != tc.expectedLowerLimit ||
			res.upperLimit != tc.expectedUpperLimit {

			t.Errorf(
				"expected (%f,%f), got (%f,%f)",
				tc.expectedLowerLimit,
				tc.expectedUpperLimit,
				res.lowerLimit,
				res.upperLimit,
			)
			continue
		}

		if (res.IngressSegment != nil) != tc.expectIngress {
			t.Errorf(
				"expected Ingress segment: %s. got Ingress segment: %s",
				yesNo[tc.expectIngress],
				yesNo[res.IngressSegment != nil],
			)
			continue
		}

		if (res.RouteGroupSegment != nil) != tc.expectRouteGroup {
			t.Errorf(
				"expected RouteGroup Segment: %s. got RouteGroup: %s",
				yesNo[tc.expectRouteGroup],
				yesNo[res.RouteGroupSegment != nil],
			)
		}
	}
}

func TestSetLimits(t *testing.T) {
	for _, tc := range []struct {
		ingressSegment    string
		routeGroupSegment string
		lower             float64
		upper             float64
		expected          map[string]string
		expectErr         bool
	}{
		{
			ingressSegment:    "TrafficSegment(0.4, 0.6)",
			routeGroupSegment: "",
			lower:             0.2,
			upper:             0.8,
			expected: map[string]string{
				"ingress": "TrafficSegment(0.20, 0.80)",
			},
			expectErr: false,
		},
		{
			ingressSegment:    "TrafficSegment(0.4, 0.6)",
			routeGroupSegment: "",
			lower:             0.2,
			upper:             0.2,
			expected: map[string]string{
				"ingress": "TrafficSegment(0.00, 0.00)",
			},
			expectErr: false,
		},
		{
			ingressSegment:    "TrafficSegment(0.1, 0.2) && Method(\"GET\")",
			routeGroupSegment: "",
			lower:             0.2,
			upper:             0.8,
			expected: map[string]string{
				"ingress": "TrafficSegment(0.20, 0.80) && Method(\"GET\")",
			},
			expectErr: false,
		},
		{
			ingressSegment:    "",
			routeGroupSegment: "TrafficSegment(0.4, 0.6)",
			lower:             0.2,
			upper:             0.8,
			expected: map[string]string{
				"routegroup": "TrafficSegment(0.20, 0.80)",
			},
			expectErr: false,
		},
		{
			ingressSegment:    "TrafficSegment(0.4, 0.6)",
			routeGroupSegment: "TrafficSegment(0.4, 0.6)",
			lower:             0.2,
			upper:             0.8,
			expected: map[string]string{
				"ingress":    "TrafficSegment(0.20, 0.80)",
				"routegroup": "TrafficSegment(0.20, 0.80)",
			},
			expectErr: false,
		},
		{
			ingressSegment:    "TrafficSegment(0.4, 0.6)",
			routeGroupSegment: "",
			lower:             0.3,
			upper:             0.2,
			expectErr:         true,
		},
		{
			ingressSegment:    "TrafficSegment(0.4, 0.6)",
			routeGroupSegment: "",
			lower:             -0.3,
			upper:             0.2,
			expectErr:         true,
		},
		{
			ingressSegment:    "TrafficSegment(0.4, 0.6)",
			routeGroupSegment: "",
			lower:             -0.3,
			upper:             -0.2,
			expectErr:         true,
		},
	} {
		expectedJSON, _ := json.MarshalIndent(tc.expected, "", "  ")
		expectedPretty := string(expectedJSON)

		container := &StackContainer{}

		if tc.ingressSegment != "" {
			container.Resources.IngressSegment = &v1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						IngressPredicateKey: tc.ingressSegment,
					},
				},
			}
			container.backendPort = &intstr.IntOrString{
				IntVal: 8080,
			}
			container.ingressSpec = &zv1.StackSetIngressSpec{
				Hosts: []string{"foo.example.com"},
			}
		}

		if tc.routeGroupSegment != "" {
			container.Resources.RouteGroupSegment = &rgv1.RouteGroup{
				Spec: rgv1.RouteGroupSpec{
					Routes: []rgv1.RouteGroupRouteSpec{
						{Predicates: []string{tc.routeGroupSegment}},
					},
				},
			}
			container.backendPort = &intstr.IntOrString{
				IntVal: 8080,
			}
			container.routeGroupSpec = &zv1.RouteGroupSpec{
				Hosts: []string{"foo.example.com"},
			}
		}

		segment, _ := NewTrafficSegment("v1", container)
		err := segment.setLimits(tc.lower, tc.upper)

		if (err != nil) != tc.expectErr {
			t.Errorf(
				"expected error: %s. got error: %s",
				yesNo[tc.expectErr],
				yesNo[err != nil],
			)
			continue
		}

		if tc.expectErr {
			continue
		}

		if tc.lower != tc.upper {
			if segment.lowerLimit != tc.lower || segment.upperLimit != tc.upper {
				t.Errorf(
					"Limits mismatch (%f,%f), expected\n%s",
					segment.lowerLimit,
					segment.upperLimit,
					expectedPretty,
				)
				break
			}
		} else {
			if segment.lowerLimit != 0.0 || segment.upperLimit != 0.0 {
				t.Errorf(
					"Active segment (%f,%f), expected\n%s",
					segment.lowerLimit,
					segment.upperLimit,
					expectedPretty,
				)
				break
			}
		}

		if tc.ingressSegment == "" {
			if segment.IngressSegment != nil {
				t.Errorf(
					"non nil IngressSegment, expected\n%s",
					expectedPretty,
				)
				break
			}

		} else {
			if segment.IngressSegment == nil {
				t.Errorf(
					"nil IngressSegment, expected\n%s",
					expectedPretty,
				)
				break
			}

			if segment.IngressSegment.Annotations[IngressPredicateKey] !=
				tc.expected["ingress"] {

				t.Errorf(
					"IngressSegment mismatch %q, expected\n%s",
					segment.IngressSegment.Annotations[IngressPredicateKey],
					expectedPretty,
				)
				break
			}
		}

		if tc.routeGroupSegment == "" {
			if segment.RouteGroupSegment != nil {
				t.Errorf(
					"non nil RouteGroupSegment, expected\n%s",
					expectedPretty,
				)
				break
			}

		} else {
			found := false
			for _, pred := range segment.RouteGroupSegment.Spec.Routes[0].Predicates {
				if pred == tc.expected["routegroup"] {
					found = true
					break
				}
			}

			if !found {
				t.Errorf(
					"RouteGroupSegment not found in %v, expected\n%s",
					segment.RouteGroupSegment.Spec.Routes[0].Predicates,
					expectedPretty,
				)
			}
		}
	}
}

func TestSegmentSorting(t *testing.T) {
	for _, tc := range []struct {
		input    segmentList
		expected segmentList
	}{
		{
			input: segmentList{
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.1},
				TrafficSegment{lowerLimit: 0.1, upperLimit: 0.3},
				TrafficSegment{lowerLimit: 0.3, upperLimit: 1.0},
			},
			expected: segmentList{
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.1},
				TrafficSegment{lowerLimit: 0.1, upperLimit: 0.3},
				TrafficSegment{lowerLimit: 0.3, upperLimit: 1.0},
			},
		},
		{
			input: segmentList{
				TrafficSegment{lowerLimit: 0.3, upperLimit: 1.0},
				TrafficSegment{lowerLimit: 0.1, upperLimit: 0.3},
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.1},
			},
			expected: segmentList{
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.1},
				TrafficSegment{lowerLimit: 0.1, upperLimit: 0.3},
				TrafficSegment{lowerLimit: 0.3, upperLimit: 1.0},
			},
		},
		{
			input: segmentList{
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.0},
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.0},
			},
			expected: segmentList{
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.0},
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.0},
			},
		},
		{
			input: segmentList{
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.1},
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.0},
				TrafficSegment{lowerLimit: 0.1, upperLimit: 1.0},
			},
			expected: segmentList{
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.0},
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.1},
				TrafficSegment{lowerLimit: 0.1, upperLimit: 1.0},
			},
		},
		{
			input: segmentList{
				TrafficSegment{lowerLimit: 0.1, upperLimit: 1.0},
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.2},
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.1},
			},
			expected: segmentList{
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.1},
				TrafficSegment{lowerLimit: 0.0, upperLimit: 0.2},
				TrafficSegment{lowerLimit: 0.1, upperLimit: 1.0},
			},
		},
	} {
		sort.Sort(tc.input)

		for i, v := range tc.input {
			if v.lowerLimit != tc.expected[i].lowerLimit {
				t.Errorf(
					"lowerLimit %f, expected %f",
					v.lowerLimit,
					tc.expected[i].lowerLimit,
				)
				break
			}

			if v.upperLimit != tc.expected[i].upperLimit {
				t.Errorf(
					"upperLimit %f, expected %f",
					v.upperLimit,
					tc.expected[i].upperLimit,
				)
				break
			}
		}
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

func TestComputeTrafficSegments(t *testing.T) {
	for _, tc := range []struct {
		actualTrafficWeights map[types.UID]float64
		ingressSegments      map[types.UID]string
		routeGroupSegments   map[types.UID]string
		expected             []map[types.UID]map[string]string
		expectErr            bool
	}{
		{
			actualTrafficWeights: map[types.UID]float64{
				"v1": 100.0,
			},
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
			},
			expected: []map[types.UID]map[string]string{
				{"v1": {"ingress": "TrafficSegment(0.00, 1.00)"}},
			},
			expectErr: false,
		},
		{
			actualTrafficWeights: map[types.UID]float64{
				"v1": 100.0,
			},
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 1.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
			},
			expected:  []map[types.UID]map[string]string{},
			expectErr: false,
		},
		{
			actualTrafficWeights: map[types.UID]float64{
				"v1": 20.0,
				"v2": 30.0,
				"v3": 50.0,
			},
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.2)",
				"v2": "TrafficSegment(0.2, 0.5)",
				"v3": "TrafficSegment(0.5, 1.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
				"v2": "",
				"v3": "",
			},
			expected:  []map[types.UID]map[string]string{},
			expectErr: false,
		},
		{
			actualTrafficWeights: map[types.UID]float64{
				"v1": 100.0,
			},
			ingressSegments: map[types.UID]string{
				"v1": "",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.0)",
			},
			expected: []map[types.UID]map[string]string{
				{"v1": {"routegroup": "TrafficSegment(0.00, 1.00)"}},
			},
			expectErr: false,
		},
		{
			actualTrafficWeights: map[types.UID]float64{
				"v1": 100.0,
			},
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.0)",
			},
			expected: []map[types.UID]map[string]string{
				{
					"v1": {
						"ingress":    "TrafficSegment(0.00, 1.00)",
						"routegroup": "TrafficSegment(0.00, 1.00)",
					},
				},
			},
			expectErr: false,
		},
		{
			actualTrafficWeights: map[types.UID]float64{
				"v1": 0.0,
				"v2": 100.0,
			},
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 1.0)",
				"v2": "TrafficSegment(0.0, 0.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
				"v2": "",
			},
			expected: []map[types.UID]map[string]string{
				{"v2": {"ingress": "TrafficSegment(0.00, 1.00)"}},
				{"v1": {"ingress": "TrafficSegment(0.00, 0.00)"}},
			},
			expectErr: false,
		},
		{
			actualTrafficWeights: map[types.UID]float64{
				"v1": 0.0,
				"v2": 100.0,
			},
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 1.0)",
				"v2": "",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
				"v2": "TrafficSegment(0.0, 0.0)",
			},
			expected: []map[types.UID]map[string]string{
				{"v2": {"routegroup": "TrafficSegment(0.00, 1.00)"}},
				{"v1": {"ingress": "TrafficSegment(0.00, 0.00)"}},
			},
			expectErr: false,
		},
		{
			actualTrafficWeights: map[types.UID]float64{
				"v1": 30.0,
				"v2": 40.0,
				"v3": 30.0,
			},
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.4)",
				"v2": "TrafficSegment(0.4, 0.6)",
				"v3": "TrafficSegment(0.6, 1.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
				"v2": "",
				"v3": "",
			},
			expected: []map[types.UID]map[string]string{
				{"v2": {"ingress": "TrafficSegment(0.30, 0.70)"}},
				{"v3": {"ingress": "TrafficSegment(0.70, 1.00)"}},
				{"v1": {"ingress": "TrafficSegment(0.00, 0.30)"}},
			},
			expectErr: false,
		},
		{
			actualTrafficWeights: map[types.UID]float64{
				"v1": 40.0,
				"v2": 20.0,
				"v3": 40.0,
			},
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.3)",
				"v2": "TrafficSegment(0.3, 0.7)",
				"v3": "TrafficSegment(0.7, 1.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
				"v2": "",
				"v3": "",
			},
			expected: []map[types.UID]map[string]string{
				{"v1": {"ingress": "TrafficSegment(0.00, 0.40)"}},
				{"v3": {"ingress": "TrafficSegment(0.60, 1.00)"}},
				{"v2": {"ingress": "TrafficSegment(0.40, 0.60)"}},
			},
			expectErr: false,
		},
		{
			actualTrafficWeights: map[types.UID]float64{
				"v1": 40.0,
				"v2": 20.0,
				"v3": 40.0,
			},
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.5)",
				"v2": "TrafficSegment(0.5, 0.7)",
				"v3": "TrafficSegment(0.7, 1.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
				"v2": "",
				"v3": "",
			},
			expected: []map[types.UID]map[string]string{
				{"v3": {"ingress": "TrafficSegment(0.60, 1.00)"}},
				{"v2": {"ingress": "TrafficSegment(0.40, 0.60)"}},
				{"v1": {"ingress": "TrafficSegment(0.00, 0.40)"}},
			},
			expectErr: false,
		},
	} {
		expectedJSON, err := json.MarshalIndent(tc.expected, "", "  ")
		if err != nil {
			// shouldn't happen
			t.Errorf("Failed marshalling expected result")
			break
		}
		expectedPretty := string(expectedJSON)

		stackContainers := map[types.UID]*StackContainer{}
		for k, v := range tc.actualTrafficWeights {
			stackContainers[k] = &StackContainer{
				actualTrafficWeight: v,
				Resources:           StackResources{},
			}

			if tc.ingressSegments[k] != "" {
				stackContainers[k].Resources.IngressSegment = &v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							IngressPredicateKey: tc.ingressSegments[k],
						},
					},
				}
				stackContainers[k].backendPort = &intstr.IntOrString{
					IntVal: 8080,
				}
				stackContainers[k].ingressSpec = &zv1.StackSetIngressSpec{
					Hosts: []string{"foo.example.com"},
				}
			}

			if tc.routeGroupSegments[k] != "" {
				stackContainers[k].Resources.RouteGroupSegment = &rgv1.RouteGroup{
					Spec: rgv1.RouteGroupSpec{
						Routes: []rgv1.RouteGroupRouteSpec{
							{Predicates: []string{tc.routeGroupSegments[k]}},
						},
					},
				}
				stackContainers[k].backendPort = &intstr.IntOrString{
					IntVal: 8080,
				}
				stackContainers[k].routeGroupSpec = &zv1.RouteGroupSpec{
					Hosts: []string{"foo.example.com"},
				}
			}
		}

		ssc := &StackSetContainer{
			StackContainers: stackContainers,
		}

		res, err := ssc.ComputeTrafficSegments()
		if (err != nil) != tc.expectErr {
			t.Errorf(
				"expected error: %s. got error: %s",
				yesNo[tc.expectErr],
				yesNo[err != nil],
			)
			continue
		}

		if tc.expectErr {
			continue
		}

		if len(res) != len(tc.expected) {
			t.Errorf(
				"wrong number of segments (%d), expected\n%s",
				len(res),
				expectedPretty,
			)
			continue
		}

		for i, v := range res {
			segments, ok := tc.expected[i][v.id]
			if !ok {
				t.Errorf(
					"id mismatch at index %d(%q), expected\n%s",
					i,
					v.id,
					expectedPretty,
				)
				break
			}

			ingressSegment, ok := segments["ingress"]
			if !ok && v.IngressSegment != nil {
				t.Errorf(
					"non nil IngressSegment at index %d, expected\n%s",
					i,
					expectedPretty,
				)
				break
			}

			if ok {
				if v.IngressSegment == nil {
					t.Errorf(
						"nil IngressSegment at index %d, expected\n%s",
						i,
						expectedPretty,
					)
					break
				}

				if v.IngressSegment.Annotations[IngressPredicateKey] !=
					ingressSegment {

					t.Errorf(
						"IngressSegment mismatch %q at index %d, expected\n%s",
						v.IngressSegment.Annotations[IngressPredicateKey],
						i,
						expectedPretty,
					)
					break
				}
			}

			rgSegment, ok := segments["routegroup"]
			if !ok && v.RouteGroupSegment != nil {
				t.Errorf(
					"non nil RouteGroupSegment at index %d, expected\n%s",
					i,
					expectedPretty,
				)
				break
			}

			if ok {
				if v.RouteGroupSegment == nil {
					t.Errorf(
						"nil RouteGroupSegment at index %d, expected\n%s",
						i,
						expectedPretty,
					)
					break
				}

				found := false
				for _, pred := range v.RouteGroupSegment.Spec.Routes[0].Predicates {
					if pred == rgSegment {
						found = true
						break
					}
				}

				if !found {
					t.Errorf(
						"RouteGroupSegment not found at index %d, expected\n%s",
						i,
						expectedPretty,
					)
				}
			}
		}
	}
}
