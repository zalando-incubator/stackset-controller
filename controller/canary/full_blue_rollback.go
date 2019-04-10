package canary

import "github.com/zalando-incubator/stackset-controller/controller/canary/percent"

// FullBlueRollBack is a Rollbacker that switches all the visible traffic onto the Blue stack.
type FullBlueRollBack struct {
	blueName           string
	baselineName       string
	greenName          string
	totalActualTraffic percent.Percent
}

func (rb FullBlueRollBack) Rollback(trafficMap TrafficMap) (TrafficMap, error) {
	modifiableStacks := StackNameSet{rb.baselineName: struct{}{}, rb.greenName: struct{}{}}
	return ComputeNewTraffic(trafficMap, rb.blueName, rb.totalActualTraffic, modifiableStacks)
}
