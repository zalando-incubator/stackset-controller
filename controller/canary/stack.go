package canary

import (
	"fmt"

	"github.com/zalando-incubator/stackset-controller/controller/canary/percent"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
)

type stackKind int

const (
	skGreen = iota + 1
	skBlue
	skBaseline
)

func (k stackKind) String() string {
	switch k {
	case skGreen:
		return "green"
	case skBaseline:
		return "baseline"
	case skBlue:
		return "blue"
	default:
		panic(fmt.Sprintf("unsupported stack kind %v", int(k)))
	}
}

type canaryStack struct {
	*zv1.Stack
	canaryKind    stackKind
	actualTraffic percent.Percent
}

func (cs *canaryStack) readActualTraffic(tm TrafficMap) error {
	if tm == nil {
		return nil
	}
	trafficStatus, ok := tm[cs.Name]
	if !ok {
		return newNoTrafficDataError(*cs)
	}
	trafficPercent := percent.NewFromFloat(trafficStatus.ActualWeight)
	err := trafficPercent.Error()
	if err == nil {
		cs.actualTraffic = *trafficPercent
	}
	return err
}

func (cs canaryStack) String() string {
	return newStackId(cs).String()
}
