package canary

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando-incubator/stackset-controller/controller/canary/percent"
	"github.com/zalando-incubator/stackset-controller/controller/entities"
)

func Test_Unchanged_GreenTraffic(t *testing.T) {
	trafficMap := TrafficMap{
		"A": entities.NewTrafficStatusSame(100),
		"B": entities.NewTrafficStatusSame(0),
		"C": entities.NewTrafficStatusSame(0),
	}

	newTraffic, _ := increaseTraffic(
		trafficMap,
		"A",
		"B",
		"C",
		*percent.NewFromInt(95),
		*percent.NewFromInt(95),
		time.Millisecond,
		time.Millisecond,
	)

	assert.Equal(t, trafficMap, newTraffic)
}

func Test_Phase1(t *testing.T) {
	trafficMap := TrafficMap{
		"A": entities.NewTrafficStatusSame(0),
		"B": entities.NewTrafficStatusSame(55),
		"C": entities.NewTrafficStatusSame(45),
	}

	expectedMap := TrafficMap{
		"A": entities.NewTrafficStatusSame(0),
		"B": entities.NewTrafficStatus(55, 50),
		"C": entities.NewTrafficStatus(45, 50),
	}

	newTraffic, _ := increaseTraffic(
		trafficMap,
		"A",
		"B",
		"C",
		*percent.NewFromInt(45),
		*percent.NewFromInt(100),
		2*60*1000*time.Millisecond,
		1*60*1000*time.Millisecond,
	)

	assert.Equal(t, expectedMap, newTraffic)
}
