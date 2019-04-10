package canary

import (
	"math"
	"time"

	"github.com/zalando-incubator/stackset-controller/controller/canary/percent"
)

func increaseTraffic(
	trafficMap TrafficMap,
	blueName, baselineName, greenName string,
	greenActualTraffic, totalActualTraffic percent.Percent,
	desiredSwitchDuration, elapsedTime time.Duration,
) (newTrafficMap TrafficMap, err error) {
	newGreenTraffic := percent.NewFromFloat(math.Min(
		100,
		math.Ceil(
			totalActualTraffic.AsFloat()*float64(elapsedTime)/float64(desiredSwitchDuration),
		),
	))

	if greenActualTraffic == *newGreenTraffic {
		return trafficMap, nil
	}

	modifiableStacks := StackNameSet{baselineName: struct{}{}, greenName: struct{}{}}
	blueActualTraffic := percent.NewFromFloat(trafficMap[blueName].ActualWeight)

	if blueActualTraffic.IsZero() { // Phase 2
		newTrafficMap, err = ComputeNewTraffic(
			trafficMap,
			greenName,
			*newGreenTraffic,
			modifiableStacks,
		)
	} else { // Phase 1
		newBlueTraffic := totalActualTraffic.SubCapped(newGreenTraffic.AddCapped(newGreenTraffic))
		newTrafficMap, err = ComputeNewTraffic(
			trafficMap,
			blueName,
			*newBlueTraffic,
			modifiableStacks,
		)
	}
	return
}
