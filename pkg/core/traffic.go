package core

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

const (
	segmentString = "TrafficSegment(%.2f, %.2f)"
)

type (
	TrafficReconciler interface {
		// Handle the traffic switching and/or scaling logic.
		Reconcile(
			stacks map[string]*StackContainer,
			currentTimestamp time.Time,
		) error
	}

	// trafficSegment holds segment information for a stack, specified by its UID.
	trafficSegment struct {
		id         types.UID
		lowerLimit float64
		upperLimit float64
	}

	// segmentList holds a sortable set of TrafficSegments.
	segmentList []trafficSegment
)

var (
	segmentRe = regexp.MustCompile(
		`TrafficSegment\((?P<Low>.*?), (?P<High>.*?)\)`,
	)
)

// newTrafficSegment returns a new TrafficSegment based on the specified stack
// container.
func newTrafficSegment(id types.UID, sc *StackContainer) (
	*trafficSegment,
	error,
) {
	res := &trafficSegment{
		id:         id,
		lowerLimit: 0.0,
		upperLimit: 0.0,
	}

	if sc.ingressSpec != nil {
		if sc.Resources.IngressSegment != nil {
			predicates := sc.Resources.IngressSegment.Annotations[IngressPredicateKey]
			lowerLimit, upperLimit, err := GetSegmentLimits(predicates)
			if err != nil {
				return nil, err
			}
			res.lowerLimit, res.upperLimit = lowerLimit, upperLimit
		}
	}

	if sc.routeGroupSpec != nil {
		if sc.Resources.RouteGroupSegment != nil {
			lowerLimit, upperLimit, err := GetSegmentLimits(
				sc.Resources.RouteGroupSegment.Spec.Routes[0].Predicates...,
			)
			if err != nil {
				return nil, err
			}

			if sc.ingressSpec != nil &&
				(res.lowerLimit != lowerLimit || res.upperLimit != upperLimit) {

				return nil, errors.New(
					"mismatch in routegroup and ingress segment values",
				)
			}
			res.lowerLimit, res.upperLimit = lowerLimit, upperLimit
		}
	}

	return res, nil
}

// weight returns the corresponding weight of the segment, in a decimal
// fraction.
func (t *trafficSegment) weight() float64 {
	return t.upperLimit - t.lowerLimit
}

// setLimits sets the lower and upper limit of the traffic segment to the
// specified values.
//
// Returns an error if any of the limits is a negative value, or if the upper
// limit's value lower than the lower limit.
func (t *trafficSegment) setLimits(lower, upper float64) error {
	switch {
	case lower < 0.0 || upper < 0.0:
		return fmt.Errorf(
			"limits cannot have a negative value (%f, %f)",
			lower,
			upper,
		)

	case upper < lower:
		return fmt.Errorf(
			"lower must be smaller or equal to upper (%f, %f)",
			lower,
			upper,
		)

	case upper == lower:
		t.lowerLimit, t.upperLimit = 0.0, 0.0

	default:
		t.lowerLimit, t.upperLimit = lower, upper
	}

	return nil
}

// Len returns the slice length, as required by the sort Interface.
func (l segmentList) Len() int {
	return len(l)
}

// Less reports wheter segment i comes before segment j
func (l segmentList) Less(i, j int) bool {
	switch {
	case l[i].lowerLimit != l[j].lowerLimit:
		return l[i].lowerLimit < l[j].lowerLimit
	case l[i].upperLimit != l[j].upperLimit:
		return l[i].upperLimit < l[j].upperLimit
	default:
		return false
	}
}

// Swap swaps the segments with indexes i and j
func (l segmentList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

// GetSegmentLimits returns the lower and upper limit of the TrafficSegment
// predicate.
//
// Returns an error if it fails to parse.
func GetSegmentLimits(predicates ...string) (float64, float64, error) {
	for _, p := range predicates {
		segmentParams := segmentRe.FindStringSubmatch(p)
		if len(segmentParams) != 3 {
			continue
		}

		lowerLimit, err := strconv.ParseFloat(segmentParams[1], 64)
		if err != nil || lowerLimit < 0.0 {
			return -1.0, -1.0, fmt.Errorf(
				"error parsing TrafficSegment %q",
				p,
			)
		}

		upperLimit, err := strconv.ParseFloat(segmentParams[2], 64)
		if err != nil || upperLimit < lowerLimit {
			return -1.0, -1.0, fmt.Errorf(
				"error parsing TrafficSegment %q",
				p,
			)
		}

		return lowerLimit, upperLimit, nil
	}

	return -1.0, -1.0, fmt.Errorf("TrafficSegment not found")
}

// allZero returns true if all weights defined in the map are 0.
func allZero(weights map[string]float64) bool {
	for _, weight := range weights {
		if weight > 0 {
			return false
		}
	}
	return true
}

// normalizeMinReadyPercent normalizes minimum percentage of Ready pods.
// If value is under or equal to 0, or over or equal to 100, set it to 1.0
// If value is between 0-100, it's then set as decimal
func normalizeMinReadyPercent(minReadyPercent int) float64 {
	if minReadyPercent >= 100 || minReadyPercent <= 0 {
		return 1.0
	}
	return float64(minReadyPercent) / 100
}

// normalizeWeights normalizes a map of backend weights.
// If all weights are zero the total weight of 100 is distributed equally
// between all backends.
// If not all weights are zero they are normalized to a sum of 100.
// Note this modifies the passed map inplace instead of returning a modified
// copy.
func normalizeWeights(backendWeights map[string]float64) {
	// if all weights are zero distribute them equally to all backends
	if allZero(backendWeights) && len(backendWeights) > 0 {
		eqWeight := 100 / float64(len(backendWeights))
		for backend := range backendWeights {
			backendWeights[backend] = eqWeight
		}
		return
	}

	// if not all weights are zero, normalize them to a sum of 100
	sum := float64(0)
	for _, weight := range backendWeights {
		sum += weight
	}

	for backend, weight := range backendWeights {
		backendWeights[backend] = weight / sum * 100
	}
}

// roundWeights rounds all the weights to whole numbers while ensuring they
// still add up to 100.
//
// Example:
//
// The weights:
//
//	[33.33, 33.33, 33.33]
//
// will be rounded to:
//
//	[34, 33, 33]
//
// The function assumes that the weights are already normalized to a sum of
// 100.
// It's using the "Largest Remainder Method" for rounding:
// https://en.wikipedia.org/wiki/Largest_remainder_method
func roundWeights(weights map[string]float64) {
	type backendWeight struct {
		Backend string
		Weight  float64
	}

	var weightList []backendWeight
	sum := 0
	// floor all weights of the map
	// sum the rounded weights
	// copy weights map to a slice to sort it later
	for backend, weight := range weights {
		roundedWeight := math.Floor(weight)
		weights[backend] = roundedWeight
		sum += int(roundedWeight)
		weightList = append(weightList, backendWeight{
			Backend: backend,
			Weight:  weight,
		})
	}
	// sort weights by:
	// 1. biggest fraction
	// 2. biggest integer
	// 3. backend name - lexicographical
	sort.Slice(weightList, func(i, j int) bool {
		ii, fi := math.Modf(weightList[i].Weight)
		ij, fj := math.Modf(weightList[j].Weight)
		if fi > fj {
			return true
		}

		if fi == fj && ii > ij {
			return true
		}

		return fi == fj && ii == ij && weightList[i].Backend < weightList[j].Backend
	})
	// check the remaining weight and distribute
	diff := 100 - sum
	for _, backend := range weightList[:diff] {
		weights[backend.Backend]++
	}
}

// ManageTraffic handles the traffic reconciler logic
func (ssc *StackSetContainer) ManageTraffic(currentTimestamp time.Time) error {
	// No ingress -> no traffic management required
	if ssc.StackSet.Spec.Ingress == nil && ssc.StackSet.Spec.RouteGroup == nil && ssc.StackSet.Spec.ExternalIngress == nil {
		for _, sc := range ssc.StackContainers {
			sc.desiredTrafficWeight = 0
			sc.actualTrafficWeight = 0
			sc.noTrafficSince = time.Time{}
			sc.prescalingActive = false
			sc.prescalingReplicas = 0
			sc.prescalingLastTrafficIncrease = time.Time{}
		}
		return nil
	}

	stacks := make(map[string]*StackContainer)
	for _, stack := range ssc.StackContainers {
		stacks[stack.Name()] = stack
	}

	// Collect the desired weights
	desiredWeights := make(map[string]float64)
	actualWeights := make(map[string]float64)
	for stackName, stack := range stacks {
		desiredWeights[stackName] = stack.desiredTrafficWeight
		actualWeights[stackName] = stack.actualTrafficWeight
	}

	// Normalize the weights and ensure that at least one stack gets traffic. This is done for both desired
	// and actual weights, because otherwise we might end up in a situation where the desired weights are
	// automagically fixed before reconciling traffic, but the reconciler still has the old actual weights
	// that for example don't add up to 100.
	for _, weights := range []map[string]float64{desiredWeights, actualWeights} {
		// No traffic at all; select a fallback stack and send all traffic there
		if allZero(weights) {
			fallbackStack := findFallbackStack(stacks)
			if fallbackStack == nil {
				return errNoStacks
			}
			weights[fallbackStack.Name()] = 100
		} else {
			normalizeWeights(weights)
			roundWeights(weights)
		}
	}

	minReadyPercent := normalizeMinReadyPercent(ssc.StackSet.Spec.MinReadyPercent)
	for stackName, stack := range stacks {
		stack.desiredTrafficWeight = desiredWeights[stackName]
		stack.actualTrafficWeight = actualWeights[stackName]
		stack.minReadyPercent = minReadyPercent
	}

	// Run the traffic reconciler which will update the actual weights according to the desired weights. The resulting
	// weights **must** be normalised.
	err := ssc.TrafficReconciler.Reconcile(stacks, currentTimestamp)

	// Update the actual weights from the reconciled ones
	if err == nil {
		actualWeights = make(map[string]float64)
		for stackName, stack := range stacks {
			actualWeights[stackName] = stack.actualTrafficWeight
		}
	}

	// If none of the stacks are getting traffic, just fallback to desired
	if allZero(actualWeights) {
		actualWeights = desiredWeights
	}
	for stackName, stack := range stacks {
		stack.actualTrafficWeight = actualWeights[stackName]
	}

	// update NoTrafficSince
	for _, stack := range ssc.StackContainers {
		if stack.HasTraffic() {
			stack.noTrafficSince = time.Time{}
		} else if stack.noTrafficSince.IsZero() {
			stack.noTrafficSince = currentTimestamp
		}
	}
	return err
}

// ComputeTrafficSegments returns the stack segments necessary to fulfill the
// actual traffic configured in the main StackSet.
//
// Returns an ordered list of traffic segments, to ensure no gaps in traffic
// assignment.
func (ssc *StackSetContainer) ComputeTrafficSegments() ([]types.UID, error) {
	var segments segmentList
	weightDiffs := map[types.UID]float64{}
	changes := []trafficSegment{}
	unchanged := []trafficSegment{}
	existingStacks := map[types.UID]bool{}
	newWeights := map[types.UID]float64{}

	// Gather actual active weights and currently active segments.
	for uid, sc := range ssc.StackContainers {
		// active stacks
		if sc.actualTrafficWeight > 0 {
			newWeights[uid] = sc.actualTrafficWeight / 100.0
		}

		trafficSegment, err := newTrafficSegment(uid, sc)
		if err != nil {
			return nil, err
		}

		// Consider only active segments
		if trafficSegment.weight() != 0 {
			segments = append(segments, *trafficSegment)
			existingStacks[trafficSegment.id] = true
		}
	}

	sort.Sort(segments)

	// Construct new traffic segments based on actual traffic weights
	index := 0.0
	for _, s := range segments {
		w := newWeights[s.id]
		wBefore, lBefore, uBefore := s.weight(), s.lowerLimit, s.upperLimit
		err := s.setLimits(index, index+w)
		if err != nil {
			return nil, err
		}

		weightDiffs[s.id] = s.weight() - wBefore
		// Don't add segments that didn't change
		if lBefore != s.lowerLimit || uBefore != s.upperLimit {
			changes = append(changes, s)
		} else {
			unchanged = append(unchanged, s)
		}
		index += w
	}

	// Add new stacks, previously with no traffic
	for id, w := range newWeights {
		if !existingStacks[id] {
			s, err := newTrafficSegment(id, ssc.StackContainers[id])
			if err != nil {
				return nil, err
			}
			err = s.setLimits(index, index+w)
			if err != nil {
				return nil, err
			}

			weightDiffs[id] = s.weight()
			changes = append(changes, *s)
			index += w
		}
	}

	// Sorts descending by weight diff, to make sure we apply growing segments
	// first.
	sort.SliceStable(changes, func(i, j int) bool {
		if weightDiffs[changes[i].id] > weightDiffs[changes[j].id] {
			return true
		}

		return changes[i].id < changes[j].id
	})

	ordered := []types.UID{}
	for _, s := range changes {
		ordered = append(ordered, s.id)
		ssc.StackContainers[s.id].segmentLowerLimit = s.lowerLimit
		ssc.StackContainers[s.id].segmentUpperLimit = s.upperLimit
	}

	// This ensures that at the time of ingress reconciliation, the limits are
	// consistent with the segment collected by the controller.
	for _, s := range unchanged {
		ssc.StackContainers[s.id].segmentLowerLimit = s.lowerLimit
		ssc.StackContainers[s.id].segmentUpperLimit = s.upperLimit
	}

	return ordered, nil
}

// fallbackStack returns a stack that should be the target of traffic if none of the existing stacks get anything
func findFallbackStack(stacks map[string]*StackContainer) *StackContainer {
	var recentlyUsed *StackContainer
	var earliest *StackContainer

	for _, stack := range stacks {
		if earliest == nil || stack.Stack.CreationTimestamp.Before(&earliest.Stack.CreationTimestamp) {
			earliest = stack
		}
		if !stack.noTrafficSince.IsZero() && (recentlyUsed == nil || stack.noTrafficSince.After(recentlyUsed.noTrafficSince)) {
			recentlyUsed = stack
		}
	}

	if recentlyUsed != nil {
		return recentlyUsed
	}
	return earliest
}
