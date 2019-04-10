package canary

import (
	"fmt"
	"math"
	"sort"

	"github.com/zalando-incubator/stackset-controller/controller/canary/percent"
	"github.com/zalando-incubator/stackset-controller/controller/entities"
)

type TrafficMap = map[string]entities.TrafficStatus

type StackNameSet = map[string]struct{}

// ComputeNewTraffic returns a copy of currentTrafficMap where targetStackName has desiredStackTraffic percentage of
// the total traffic (or an error if this is impossible) by only modifying the traffic of other stacks listed in
// modifiableStacks.
func ComputeNewTraffic(
	currentTrafficMap TrafficMap,
	targetStackName string,
	desiredStackTraffic percent.Percent,
	modifiableStacks StackNameSet,
) (TrafficMap, error) {
	if _, ok := currentTrafficMap[targetStackName]; !ok {
		return nil, fmt.Errorf("target stack %q not found in current traffic map", targetStackName)
	}
	if len(modifiableStacks) == 0 {
		return nil, fmt.Errorf("no modifiable stacks provided apart from the target stack")
	}
	if err := desiredStackTraffic.Error(); err != nil {
		return nil, fmt.Errorf("cannot compute new traffic map for invalid target traffic: %v", err)
	}

	var err error
	currentTrafficMap, err = unfloatTrafficMap(currentTrafficMap)
	if err != nil {
		return nil, err
	}

	modifiableStacksSlice := make([]string, 0, len(modifiableStacks))
	for k := range modifiableStacks {
		if k != targetStackName {
			modifiableStacksSlice = append(modifiableStacksSlice, k)
		}
	}
	trafficDiffByStack, err := computeStackDiffs(
		modifiableStacksSlice,
		currentTrafficMap,
		targetStackName,
		desiredStackTraffic.AsFloat(),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot compute new traffic map: %v", err)
	}

	newTrafficMap := make(TrafficMap, len(currentTrafficMap))
	for stackName, currentTraffic := range currentTrafficMap {
		var newDesiredWeight float64

		if stackName == targetStackName {
			newDesiredWeight = desiredStackTraffic.AsFloat()
		} else if _, ok := modifiableStacks[stackName]; ok {
			newDesiredWeight = currentTraffic.DesiredWeight + trafficDiffByStack[stackName]
		} else {
			newDesiredWeight = currentTraffic.DesiredWeight
		}

		newTrafficMap[stackName] = entities.TrafficStatus{
			ActualWeight:  currentTraffic.ActualWeight,
			DesiredWeight: newDesiredWeight,
		}
	}
	return newTrafficMap, nil
}

// computeStackDiffs returns, for each modifiable stack, the traffic diff that should be applied
// to it to reach desiredTraffic on targetStack by taking traffic from modifiableStacks in the
// most uniform way possible.
// modifiableStacks will be sorted in place.
func computeStackDiffs(
	modifiableStacks []string,
	tm TrafficMap,
	targetStack string,
	desiredTraffic float64,
) (map[string]float64, error) {
	// Sort modifiableStacks so that the stacks with the smallest leeway (i.e. that will most
	// probably not be able to accept the uniform, "distributed" diff) come first.
	// This way, we can adapt the distributed diff to take into account what could not be put on
	// the first stacks, and continue trying to apply this modified distributed diff to the next
	// stacks. Without sorting, we would have to loop over modifiableStacks and change stack diffs
	// multiple times until satisfied.
	trafficFunc := func(i int) float64 { return tm[modifiableStacks[i]].DesiredWeight }
	var possibleDiffFunc func(int) float64
	diff := tm[targetStack].DesiredWeight - desiredTraffic
	if diff > 0 { // we add traffic to modifiable stacks (diff > 0)
		sort.Slice(modifiableStacks, func(i, j int) bool { return trafficFunc(i) > trafficFunc(j) })
		possibleDiffFunc = func(i int) float64 { return 100 - trafficFunc(i) }
	} else { // we remove traffic from modifiable stacks (diff < 0)
		sort.Slice(modifiableStacks, func(i, j int) bool { return trafficFunc(i) < trafficFunc(j) })
		possibleDiffFunc = func(i int) float64 { return -trafficFunc(i) }
	}

	trafficDiffByStack := make(map[string]float64)
	// amount of traffic we'd like to add/remove uniformly in modifiableStacks
	distributedDiff := diff / float64(len(modifiableStacks))
	for i, stackName := range modifiableStacks {
		possibleDiff := possibleDiffFunc(i)
		if math.Abs(possibleDiff) < math.Abs(distributedDiff) { // they have the same sign so we can
			rest := distributedDiff - possibleDiff
			nbRemainingStacks := len(modifiableStacks) - i - 1
			if nbRemainingStacks == 0 {
				return nil, fmt.Errorf(
					"not enough leeway in modifiable stacks to give %f%% to the target stack %q; missing %v%%",
					desiredTraffic, targetStack, rest,
				)
			}
			trafficDiffByStack[stackName] = possibleDiff
			distributedDiff += rest / float64(nbRemainingStacks)
		} else {
			trafficDiffByStack[stackName] = distributedDiff
		}
	}
	return trafficDiffByStack, nil
}

// unfloatTrafficMap "simplifies" all DesiredWeight float values by converting them to Percent then back to float.
// The conversion to Percent ensures that they are in range and rounds them to one decimal place. This avoids errors
// where a traffic float is almost equal to zero (like 1e-16) but not quite.
// The tm argument is not modified. Also ensures that the sum of all traffic is equal to 100%.
func unfloatTrafficMap(tm TrafficMap) (TrafficMap, error) {
	total := percent.NewFromInt(0)
	res := make(TrafficMap, len(tm))
	for stack, ts := range tm {
		pc := percent.NewFromFloat(ts.DesiredWeight)
		if pc.Error() != nil {
			return nil, pc.Error()
		}

		total = total.Add(pc)
		if total.Error() != nil {
			return nil, fmt.Errorf("invalid desired traffic in map: %s", total.Error())
		}

		res[stack] = entities.TrafficStatus{
			ActualWeight:  ts.ActualWeight,
			DesiredWeight: pc.AsFloat(),
		}
	}
	if total.Cmp(pc100) != 0 {
		return nil, fmt.Errorf("invalid input traffic map: total traffic %s is less than %s", total, pc100)
	}
	return res, nil
}

var pc100 = percent.NewFromInt(100)
