package entities

import "math"

// TrafficStatus represents the traffic status of an Ingress. ActualWeight is
// the actual traffic a particular backend is getting and DesiredWeight is the
// user specified value that it should try to achieve.
type TrafficStatus struct {
	ActualWeight  float64
	DesiredWeight float64
}

// Weight returns the max of ActualWeight and DesiredWeight.
func (t TrafficStatus) Weight() float64 {
	return math.Max(t.ActualWeight, t.DesiredWeight)
}

// Helper constructors for common TrafficStatus values
func NewTrafficStatusSame(weight float64) TrafficStatus {
	return TrafficStatus{
		ActualWeight:  weight,
		DesiredWeight: weight,
	}
}

func NewTrafficStatus(actualWeight, desiredWeight float64) TrafficStatus {
	return TrafficStatus{
		ActualWeight:  actualWeight,
		DesiredWeight: desiredWeight,
	}
}
