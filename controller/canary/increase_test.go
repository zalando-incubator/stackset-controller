package canary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zalando-incubator/stackset-controller/controller/entities"
)

func Test_Unchanged_GreenTraffic(t *testing.T) {
	trafficMap := TrafficMap{
		"A": entities.NewTrafficStatusSame(100),
		"B": entities.NewTrafficStatusSame(100),
		"C": entities.NewTrafficStatusSame(100),
	}

	assert.Equal(t, trafficMap, trafficMap)
}
