package canary

import (
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
)

// ExpectationChecker defines the interface between stackset-controller
// and the component in charge of deciding whether a gradual traffic switching
// should continue.
type ExpectationChecker interface {
	// ExpectationsFulfilled returns whether stack green still fulfills user expectations
	// compared to baseline.
	ExpectationsFulfilled(green, baseline zv1.Stack) bool
}
