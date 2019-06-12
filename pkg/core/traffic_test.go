package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	hourAgo        = time.Now().Add(-time.Hour)
	fiveMinutesAgo = time.Now().Add(-5 * time.Minute)
)

func stackContainer(name string, desiredTrafficWeight, actualTrafficWeight float64, ready bool, createdAt time.Time, noTrafficSince time.Time) *StackContainer {
	result := &StackContainer{
		Stack: &zv1.Stack{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				CreationTimestamp: metav1.Time{Time: createdAt},
			},
		},
		actualTrafficWeight:  actualTrafficWeight,
		desiredTrafficWeight: desiredTrafficWeight,
		noTrafficSince:       noTrafficSince,
		deploymentUpdated:    true,
		deploymentReplicas:   1,
		updatedReplicas:      1,
		readyReplicas:        0,
	}
	if ready {
		result.readyReplicas = 1
	}
	return result
}

func TestTrafficSwitchNoIngress(t *testing.T) {
	for reconcilerName, reconciler := range map[string]TrafficReconciler{
		"simple": SimpleTrafficReconciler{},
		"prescale": PrescalingTrafficReconciler{
			ResetHPAMinReplicasTimeout: time.Minute,
		},
	} {
		t.Run(reconcilerName, func(t *testing.T) {
			prescaledStack := stackContainer("foo-v1", 50, 50, true, hourAgo, time.Time{})
			prescaledStack.prescalingActive = false
			prescaledStack.prescalingReplicas = 3
			prescaledStack.prescalingLastTrafficIncrease = time.Now()

			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{},
				},
				StackContainers: map[types.UID]*StackContainer{
					"v1": prescaledStack,
					"v2": stackContainer("foo-v2", 50, 50, true, hourAgo, time.Time{}),
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
			initialStack := stackContainer("foo-v1", 70, 30, true, hourAgo, time.Time{})
			initialStack.deploymentUpdated = tc.deploymentUpdated
			initialStack.deploymentReplicas = tc.deploymentReplicas
			initialStack.updatedReplicas = tc.updatedReplicas
			initialStack.readyReplicas = tc.readyReplicas

			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{},
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					"v1": initialStack,
					"v2": stackContainer("foo-v2", 30, 70, true, hourAgo, time.Time{}),
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
				"foo-v1": stackContainer("foo-v1", 25, 70, true, hourAgo, time.Time{}),
				"foo-v2": stackContainer("foo-v2", 50, 30, true, hourAgo, time.Time{}),
				"foo-v3": stackContainer("foo-v3", 25, 0, true, hourAgo, time.Time{}),
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
				"foo-v1": stackContainer("foo-v1", 25, 70, false, hourAgo, time.Time{}),
				"foo-v2": stackContainer("foo-v2", 50, 30, true, hourAgo, time.Time{}),
				"foo-v3": stackContainer("foo-v3", 25, 0, true, hourAgo, time.Time{}),
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
				"foo-v1": stackContainer("foo-v1", 1, 7, true, hourAgo, time.Time{}),
				"foo-v2": stackContainer("foo-v2", 2, 3, true, hourAgo, time.Time{}),
				"foo-v3": stackContainer("foo-v3", 1, 0, true, hourAgo, time.Time{}),
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
				"foo-v1": stackContainer("foo-v1", 1, 7, false, hourAgo, time.Time{}),
				"foo-v2": stackContainer("foo-v2", 2, 3, false, hourAgo, time.Time{}),
				"foo-v3": stackContainer("foo-v3", 1, 0, false, hourAgo, time.Time{}),
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
				"foo-v1": stackContainer("foo-v1", 0, 10, false, hourAgo, time.Time{}),
				"foo-v2": stackContainer("foo-v2", 0, 30, false, hourAgo, fiveMinutesAgo),
				"foo-v3": stackContainer("foo-v3", 0, 60, false, hourAgo, hourAgo),
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
				"foo-v1": stackContainer("foo-v1", 0, 10, false, fiveMinutesAgo, time.Time{}),
				"foo-v2": stackContainer("foo-v2", 0, 30, false, hourAgo, time.Time{}),
				"foo-v3": stackContainer("foo-v3", 0, 60, false, fiveMinutesAgo, time.Time{}),
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
				"foo-v1": stackContainer("foo-v1", 25, 0, false, hourAgo, time.Time{}),
				"foo-v2": stackContainer("foo-v2", 50, 0, false, hourAgo, fiveMinutesAgo),
				"foo-v3": stackContainer("foo-v3", 25, 0, false, hourAgo, hourAgo),
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
			name: "if there are no stacks traffic, and no stacks had traffic recently, send traffic to the earliest created stack",
			stacks: map[types.UID]*StackContainer{
				"foo-v1": stackContainer("foo-v1", 25, 0, false, fiveMinutesAgo, time.Time{}),
				"foo-v2": stackContainer("foo-v2", 50, 0, false, hourAgo, time.Time{}),
				"foo-v3": stackContainer("foo-v3", 25, 0, false, fiveMinutesAgo, time.Time{}),
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
					"foo-v1": stackContainer("foo-v1", 90, 0, false, hourAgo, time.Time{}),
					"foo-v2": stackContainer("foo-v2", 0, 90, false, hourAgo, time.Time{}),
					"foo-v3": stackContainer("foo-v3", 10, 10, false, hourAgo, time.Time{}),
					"foo-v4": stackContainer("foo-v4", 0, 0, false, hourAgo, time.Time{}),
					"foo-v5": stackContainer("foo-v5", 0, 0, false, hourAgo, fiveMinutesAgo),
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
