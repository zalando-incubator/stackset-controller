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
					"v1": testStack("foo-v1").ready(3).traffic(50, 50).prescaling(3, time.Now()).stack(),
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

func TestTrafficSwitchSimpleNotReady(t *testing.T) {
	for _, tc := range []struct {
		name               string
		stack              *StackContainer
		deploymentUpdated  bool
		deploymentReplicas int32
		updatedReplicas    int32
		readyReplicas      int32
	}{
		{
			name:               "deployment not updated yet",
			deploymentUpdated:  false,
			deploymentReplicas: 3,
			updatedReplicas:    3,
			readyReplicas:      3,
		},
		{
			name:               "not enough updated replicas",
			deploymentUpdated:  true,
			deploymentReplicas: 3,
			updatedReplicas:    2,
			readyReplicas:      3,
		},
		{
			name:               "not enough ready replicas",
			deploymentUpdated:  true,
			deploymentReplicas: 3,
			updatedReplicas:    3,
			readyReplicas:      2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{},
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					"v1": testStack("foo-v1").traffic(70, 30).deployment(tc.deploymentUpdated, tc.deploymentReplicas, tc.updatedReplicas, tc.readyReplicas).stack(),
					"v2": testStack("foo-v2").traffic(30, 70).ready(3).stack(),
				},
				TrafficReconciler: SimpleTrafficReconciler{},
			}
			err := c.ManageTraffic(time.Now())
			expected := &trafficSwitchError{
				reason: "stacks foo-v1 not ready",
			}
			require.Equal(t, expected, err)
		})
	}
}

func TestTrafficSwitchSimple(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		stacks                 map[types.UID]*StackContainer
		expectedDesiredWeights map[string]float64
		expectedActualWeights  map[string]float64
		expectedError          string
	}{
		{
			name: "traffic is switched if all the stacks are ready",
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
			name: "traffic weights are normalised",
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
			expectedError: "stacks foo-v2, foo-v3 not ready",
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
			expectedError: "stacks foo-v2 not ready",
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
			expectedError: "stacks foo-v2 not ready",
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
			expectedError: "stacks foo-v1, foo-v3 not ready",
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
			expectedError: "stacks foo-v1, foo-v3 not ready",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{},
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
	for _, tc := range []struct {
		name                   string
		stacks                 map[types.UID]*StackContainer
		expectedDesiredWeights map[string]float64
		expectedActualWeights  map[string]float64
		expectedError          string
	}{
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
			expectedError: "stacks foo-v2, foo-v3 not ready",
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
			expectedError: "stacks foo-v2 not ready",
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
			expectedError: "stacks foo-v2 not ready",
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
			expectedError: "stacks foo-v1, foo-v3 not ready",
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
			expectedError: "stacks foo-v1, foo-v3 not ready",
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
