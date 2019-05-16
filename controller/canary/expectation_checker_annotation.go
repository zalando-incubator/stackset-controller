package canary

import v1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"

// annotationChecker is a very basic ExpectationChecker.
// It considers expectations fulfilled if and only if the green stack does not have
// the annotationCheckerKey annotation, which is not removed.
// Handy for manual experiments, but do not use in production:
// it probably is not be desirable to continue a gradual deployment when the service responsible
// for adding the annotationCheckerKey annotation is down.
type annotationChecker struct{}

const annotationCheckerKey = canaryAnnotationKeyPrefix + "expectations-not-fulfilled"

func (c annotationChecker) ExpectationsFulfilled(green, _ v1.Stack) bool {
	_, ok := green.Annotations[annotationCheckerKey]
	return !ok
}
