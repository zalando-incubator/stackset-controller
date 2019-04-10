package canary

import v1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"

type annotationChecker struct{}

const annotationCheckerKey = canaryAnnotationKeyPrefix + "expectations-not-fulfilled"

func (c annotationChecker) ExpectationsFulfilled(green, _ v1.Stack) bool {
	_, ok := green.Annotations[annotationCheckerKey]
	return !ok
}
