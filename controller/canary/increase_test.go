package canary

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando-incubator/stackset-controller/controller/canary/percent"
	"github.com/zalando-incubator/stackset-controller/controller/entities"
)

// Helper test constructors for common TrafficStatus values
func NewTrafficStatusSame(weight float64) entities.TrafficStatus {
	return entities.TrafficStatus{
		ActualWeight:  weight,
		DesiredWeight: weight,
	}
}

func NewTrafficStatus(actualWeight, desiredWeight float64) entities.TrafficStatus {
	return entities.TrafficStatus{
		ActualWeight:  actualWeight,
		DesiredWeight: desiredWeight,
	}
}

func Test_Unchanged_GreenTraffic(t *testing.T) {
	trafficMap := TrafficMap{
		"A": NewTrafficStatusSame(100),
		"B": NewTrafficStatusSame(0),
		"C": NewTrafficStatusSame(0),
	}

	newTraffic, _ := increaseTraffic(
		trafficMap,
		"A",
		"B",
		"C",
		percent.NewFromInt(95),
		percent.NewFromInt(95),
		time.Millisecond,
		time.Millisecond,
	)

	assert.Equal(t, trafficMap, newTraffic)
}

func Test_Phase1(t *testing.T) {
	trafficMap := TrafficMap{
		"A": NewTrafficStatusSame(20),
		"B": NewTrafficStatusSame(40),
		"C": NewTrafficStatusSame(40),
	}

	expectedMap := TrafficMap{
		"A": NewTrafficStatus(20, 80),
		"B": NewTrafficStatus(40, 10),
		"C": NewTrafficStatus(40, 10),
	}

	newTraffic, _ := increaseTraffic(
		trafficMap,
		"A",
		"B",
		"C",
		percent.NewFromInt(40),
		percent.NewFromInt(100),
		10*60*1000*time.Millisecond,
		1*60*1000*time.Millisecond,
	)

	assert.Equal(t, expectedMap, newTraffic)
}

func Test_Phase2(t *testing.T) {
	trafficMap := TrafficMap{
		"A": NewTrafficStatusSame(0),
		"B": NewTrafficStatusSame(55),
		"C": NewTrafficStatusSame(45),
	}

	expectedMap := TrafficMap{
		"A": NewTrafficStatusSame(0),
		"B": NewTrafficStatus(55, 50),
		"C": NewTrafficStatus(45, 50),
	}

	newTraffic, _ := increaseTraffic(
		trafficMap,
		"A",
		"B",
		"C",
		percent.NewFromInt(45),
		percent.NewFromInt(100),
		2*60*1000*time.Millisecond,
		1*60*1000*time.Millisecond,
	)

	assert.Equal(t, expectedMap, newTraffic)
}
