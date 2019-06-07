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
	hourAgo = time.Now().Add(-time.Hour)
)

func stackContainer(name string, desiredTrafficWeight, actualTrafficWeight float64, noTrafficSince time.Time) *StackContainer {
	return &StackContainer{
		Stack: &zv1.Stack{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		},
		actualTrafficWeight:  actualTrafficWeight,
		desiredTrafficWeight: desiredTrafficWeight,
		noTrafficSince:       noTrafficSince,
	}
}

func TestTrafficSwitchNoIngress(t *testing.T) {
	for reconcilerName, reconciler := range map[string]TrafficReconciler{
		"simple": SimpleTrafficReconciler{},
		"prescale": PrescalingTrafficReconciler{
			ResetHPAMinReplicasTimeout: time.Minute,
		},
	} {
		t.Run(reconcilerName, func(t *testing.T) {
			prescaledStack := stackContainer("foo-v1", 0.5, 0.5, hourAgo)
			prescaledStack.prescalingActive = false
			prescaledStack.prescalingReplicas = 3
			prescaledStack.prescalingLastTrafficIncrease = time.Now()

			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{},
				},
				StackContainers: map[types.UID]*StackContainer{
					"v1": prescaledStack,
					"v2": stackContainer("foo-v2", 0.5, 0.5, time.Time{}),
				},
				TrafficReconciler: reconciler,
			}
			err := c.ManageTraffic()
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
