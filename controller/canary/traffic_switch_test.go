package canary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zalando-incubator/stackset-controller/controller/canary/percent"
)

func TestComputeNewTraffic(t *testing.T) {
	t.Run("empty traffic map", func(t *testing.T) {
		newTrafficMap, err := ComputeNewTraffic(
			TrafficMap{},
			"B",
			percent.NewFromInt(100),
			StackNameSet{},
		)
		assert.Nil(t, newTrafficMap)
		assert.Error(t, err)
	})
	t.Run("unknown target stack", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": NewTrafficStatusSame(100),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"B",
			percent.NewFromInt(100),
			StackNameSet{},
		)
		assert.Nil(t, newTrafficMap)
		assert.Error(t, err)
	})
	t.Run("no modifiable stacks", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": NewTrafficStatusSame(100),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"A",
			percent.NewFromInt(100),
			StackNameSet{},
		)
		assert.Nil(t, newTrafficMap)
		assert.Error(t, err)
	})
	t.Run("unreachable percentage target", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": NewTrafficStatusSame(75),
			"B": NewTrafficStatusSame(20),
			"C": NewTrafficStatusSame(5),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"B",
			percent.NewFromInt(100),
			StackNameSet{
				"A": struct{}{},
			},
		)
		assert.Nil(t, newTrafficMap)
		assert.Error(t, err)
	})
	t.Run("increasing traffic", func(t *testing.T) {
		t.Run("with target stack not in modifiable stacks", func(t *testing.T) {
			currentTrafficMap := TrafficMap{
				"A": NewTrafficStatusSame(75),
				"B": NewTrafficStatusSame(25),
			}
			newTrafficMap, err := ComputeNewTraffic(
				currentTrafficMap,
				"B",
				percent.NewFromInt(100),
				StackNameSet{
					"A": struct{}{},
				},
			)
			assert.Nil(t, err)
			expectedTrafficMap := TrafficMap{
				"A": NewTrafficStatus(75, 0),
				"B": NewTrafficStatus(25, 100),
			}
			assert.Equal(t, expectedTrafficMap, newTrafficMap)
		})
		t.Run("with target stack in modifiable stacks", func(t *testing.T) {
			currentTrafficMap := TrafficMap{
				"A": NewTrafficStatusSame(75),
				"B": NewTrafficStatusSame(25),
			}
			newTrafficMap, err := ComputeNewTraffic(
				currentTrafficMap,
				"B",
				percent.NewFromInt(100),
				StackNameSet{
					"A": struct{}{},
					"B": struct{}{},
				},
			)
			assert.Nil(t, err)
			expectedTrafficMap := TrafficMap{
				"A": NewTrafficStatus(75, 0),
				"B": NewTrafficStatus(25, 100),
			}
			assert.Equal(t, expectedTrafficMap, newTrafficMap)
		})
		t.Run("with non-modifiable stacks", func(t *testing.T) {
			currentTrafficMap := TrafficMap{
				"A": NewTrafficStatusSame(75),
				"B": NewTrafficStatusSame(20),
				"C": NewTrafficStatusSame(5),
			}
			percent95 := percent.NewFromInt(95)
			newTrafficMap, err := ComputeNewTraffic(
				currentTrafficMap,
				"B",
				percent95,
				StackNameSet{
					"A": struct{}{},
				},
			)
			assert.Nil(t, err)
			expectedTrafficMap := TrafficMap{
				"A": NewTrafficStatus(75, 0),
				"B": NewTrafficStatus(20, 95),
				"C": NewTrafficStatusSame(5),
			}
			assert.Equal(t, expectedTrafficMap, newTrafficMap)
		})
	})
	t.Run("decreasing traffic", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": NewTrafficStatusSame(75),
			"B": NewTrafficStatusSame(20),
			"C": NewTrafficStatusSame(5),
		}
		percent60_1 := percent.NewFromFloat(60.1)
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"A",
			percent60_1,
			StackNameSet{
				"B": struct{}{},
			},
		)
		assert.Nil(t, err)
		expectedTrafficMap := TrafficMap{
			"A": NewTrafficStatus(75, 60.1),
			"B": NewTrafficStatus(20, 34.9),
			"C": NewTrafficStatusSame(5),
		}
		assert.Equal(t, expectedTrafficMap, newTrafficMap)
	})
	t.Run("ignore modifiable stacks already at extremes", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": NewTrafficStatusSame(0),
			"B": NewTrafficStatusSame(100),
			"C": NewTrafficStatusSame(0),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"A",
			percent.NewFromInt(100),
			StackNameSet{
				"B": struct{}{},
				"C": struct{}{}, // not actually modifiable
			},
		)
		assert.Nil(t, err)
		expectedTrafficMap := TrafficMap{
			"A": NewTrafficStatus(0, 100),
			"B": NewTrafficStatus(100, 0),
			"C": NewTrafficStatusSame(0),
		}
		assert.Equal(t, expectedTrafficMap, newTrafficMap)
	})
	t.Run("ignore modifiable stacks almost at extremes", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": NewTrafficStatusSame(0),
			"B": NewTrafficStatusSame(99),
			"C": NewTrafficStatusSame(1),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"A",
			percent.NewFromInt(100),
			StackNameSet{
				"B": struct{}{},
				"C": struct{}{}, // not actually modifiable
			},
		)
		assert.Nil(t, err)
		expectedTrafficMap := TrafficMap{
			"A": NewTrafficStatus(0, 100),
			"B": NewTrafficStatus(99, 0),
			"C": NewTrafficStatus(1, 0),
		}
		assert.Equal(t, expectedTrafficMap, newTrafficMap)
	})
	t.Run("input traffic map has more than 100% traffic", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": NewTrafficStatusSame(50),
			"B": NewTrafficStatusSame(0),
			"C": NewTrafficStatusSame(51),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"B",
			percent.NewFromInt(100),
			StackNameSet{
				"A": struct{}{},
				"C": struct{}{},
			},
		)
		assert.Nil(t, newTrafficMap)
		assert.Error(t, err)
	})
	t.Run("input traffic map has less than 100% traffic", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": NewTrafficStatusSame(50),
			"B": NewTrafficStatusSame(0),
			"C": NewTrafficStatusSame(49),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"A",
			percent.NewFromInt(0),
			StackNameSet{
				"B": struct{}{},
				"C": struct{}{},
			},
		)
		assert.Nil(t, newTrafficMap)
		assert.Error(t, err)
	})
}
