package canary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zalando-incubator/stackset-controller/controller/canary/percent"
	"github.com/zalando-incubator/stackset-controller/controller/entities"
)

func TestComputeNewTraffic(t *testing.T) {
	t.Run("empty traffic map", func(t *testing.T) {
		newTrafficMap, err := ComputeNewTraffic(
			TrafficMap{},
			"B",
			*percent.NewFromInt(100),
			StackNameSet{},
		)
		assert.Nil(t, newTrafficMap)
		assert.Error(t, err)
	})
	t.Run("unknown target stack", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": entities.NewTrafficStatusSame(100),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"B",
			*percent.NewFromInt(100),
			StackNameSet{},
		)
		assert.Nil(t, newTrafficMap)
		assert.Error(t, err)
	})
	t.Run("no modifiable stacks", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": entities.NewTrafficStatusSame(100),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"A",
			*percent.NewFromInt(100),
			StackNameSet{},
		)
		assert.Nil(t, newTrafficMap)
		assert.Error(t, err)
	})
	t.Run("unreachable percentage target", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": entities.NewTrafficStatusSame(75),
			"B": entities.NewTrafficStatusSame(20),
			"C": entities.NewTrafficStatusSame(5),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"B",
			*percent.NewFromInt(100),
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
				"A": entities.NewTrafficStatusSame(75),
				"B": entities.NewTrafficStatusSame(25),
			}
			newTrafficMap, err := ComputeNewTraffic(
				currentTrafficMap,
				"B",
				*percent.NewFromInt(100),
				StackNameSet{
					"A": struct{}{},
				},
			)
			assert.Nil(t, err)
			expectedTrafficMap := TrafficMap{
				"A": entities.NewTrafficStatus(75, 0),
				"B": entities.NewTrafficStatus(25, 100),
			}
			assert.Equal(t, expectedTrafficMap, newTrafficMap)
		})
		t.Run("with target stack in modifiable stacks", func(t *testing.T) {
			currentTrafficMap := TrafficMap{
				"A": entities.NewTrafficStatusSame(75),
				"B": entities.NewTrafficStatusSame(25),
			}
			newTrafficMap, err := ComputeNewTraffic(
				currentTrafficMap,
				"B",
				*percent.NewFromInt(100),
				StackNameSet{
					"A": struct{}{},
					"B": struct{}{},
				},
			)
			assert.Nil(t, err)
			expectedTrafficMap := TrafficMap{
				"A": entities.NewTrafficStatus(75, 0),
				"B": entities.NewTrafficStatus(25, 100),
			}
			assert.Equal(t, expectedTrafficMap, newTrafficMap)
		})
		t.Run("with non-modifiable stacks", func(t *testing.T) {
			currentTrafficMap := TrafficMap{
				"A": entities.NewTrafficStatusSame(75),
				"B": entities.NewTrafficStatusSame(20),
				"C": entities.NewTrafficStatusSame(5),
			}
			percent95 := percent.NewFromInt(95)
			newTrafficMap, err := ComputeNewTraffic(
				currentTrafficMap,
				"B",
				*percent95,
				StackNameSet{
					"A": struct{}{},
				},
			)
			assert.Nil(t, err)
			expectedTrafficMap := TrafficMap{
				"A": entities.NewTrafficStatus(75, 0),
				"B": entities.NewTrafficStatus(20, 95),
				"C": entities.NewTrafficStatusSame(5),
			}
			assert.Equal(t, expectedTrafficMap, newTrafficMap)
		})
	})
	t.Run("decreasing traffic", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": entities.NewTrafficStatusSame(75),
			"B": entities.NewTrafficStatusSame(20),
			"C": entities.NewTrafficStatusSame(5),
		}
		percent60_1 := percent.NewFromFloat(60.1)
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"A",
			*percent60_1,
			StackNameSet{
				"B": struct{}{},
			},
		)
		assert.Nil(t, err)
		expectedTrafficMap := TrafficMap{
			"A": entities.NewTrafficStatus(75, 60.1),
			"B": entities.NewTrafficStatus(20, 34.9),
			"C": entities.NewTrafficStatusSame(5),
		}
		assert.Equal(t, expectedTrafficMap, newTrafficMap)
	})
	t.Run("ignore modifiable stacks already at extremes", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": entities.NewTrafficStatusSame(0),
			"B": entities.NewTrafficStatusSame(100),
			"C": entities.NewTrafficStatusSame(0),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"A",
			*percent.NewFromInt(100),
			StackNameSet{
				"B": struct{}{},
				"C": struct{}{}, // not actually modifiable
			},
		)
		assert.Nil(t, err)
		expectedTrafficMap := TrafficMap{
			"A": entities.NewTrafficStatus(0, 100),
			"B": entities.NewTrafficStatus(100, 0),
			"C": entities.NewTrafficStatusSame(0),
		}
		assert.Equal(t, expectedTrafficMap, newTrafficMap)
	})
	t.Run("ignore modifiable stacks almost at extremes", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": entities.NewTrafficStatusSame(0),
			"B": entities.NewTrafficStatusSame(99),
			"C": entities.NewTrafficStatusSame(1),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"A",
			*percent.NewFromInt(100),
			StackNameSet{
				"B": struct{}{},
				"C": struct{}{}, // not actually modifiable
			},
		)
		assert.Nil(t, err)
		expectedTrafficMap := TrafficMap{
			"A": entities.NewTrafficStatus(0, 100),
			"B": entities.NewTrafficStatus(99, 0),
			"C": entities.NewTrafficStatus(1, 0),
		}
		assert.Equal(t, expectedTrafficMap, newTrafficMap)
	})
	t.Run("input traffic map has more than 100% traffic", func(t *testing.T) {
		currentTrafficMap := TrafficMap{
			"A": entities.NewTrafficStatusSame(50),
			"B": entities.NewTrafficStatusSame(0),
			"C": entities.NewTrafficStatusSame(51),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"B",
			*percent.NewFromInt(100),
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
			"A": entities.NewTrafficStatusSame(50),
			"B": entities.NewTrafficStatusSame(0),
			"C": entities.NewTrafficStatusSame(49),
		}
		newTrafficMap, err := ComputeNewTraffic(
			currentTrafficMap,
			"A",
			*percent.NewFromInt(0),
			StackNameSet{
				"B": struct{}{},
				"C": struct{}{},
			},
		)
		assert.Nil(t, newTrafficMap)
		assert.Error(t, err)
	})
}
