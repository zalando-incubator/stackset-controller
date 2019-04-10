package canary

import (
	"fmt"
)

type stackId struct {
	name      string
	namespace string
	kind      stackKind
}

func newStackId(stack canaryStack) stackId {
	return stackId{
		name:      stack.Name,
		namespace: stack.Namespace,
		kind:      stack.canaryKind,
	}
}

func (s stackId) String() string {
	qualifiedName := fmt.Sprintf("%s/%s", s.namespace, s.name)
	return fmt.Sprintf("%s stack %q", s.kind, qualifiedName)
}

type noBlueAnnotError struct {
	stack stackId
}

func (e noBlueAnnotError) Error() string {
	return fmt.Sprintf(
		"missing annotation %q in %s",
		blueAnnotationKey, e.stack,
	)
}

func newNoBlueAnnotError(green canaryStack) noBlueAnnotError {
	return noBlueAnnotError{newStackId(green)}
}

type stackNotFoundError struct {
	green stackId
	lost  stackId
}

func (e stackNotFoundError) Error() string {
	return fmt.Sprintf("%s not found for %s", e.lost, e.green)
}

func newStackNotFoundError(green canaryStack, name string, kind stackKind) stackNotFoundError {
	return stackNotFoundError{
		green: newStackId(green),
		lost: stackId{
			name:      name,
			namespace: green.Namespace,
			kind:      kind,
		},
	}
}

type noTrafficDataError struct {
	stack stackId
}

func (e noTrafficDataError) Error() string {
	return fmt.Sprintf("no traffic data in ingress for %s", e.stack)
}

func newNoTrafficDataError(stack canaryStack) noTrafficDataError {
	return noTrafficDataError{newStackId(stack)}
}

type badDurationError struct {
	green    stackId
	duration string
	key      string
}

func (e badDurationError) Error() string {
	return fmt.Sprintf(
		"bad %q period specified for %s: %q",
		e.key, e.green, e.duration,
	)
}

func newBadDurationError(green canaryStack, duration, key string) badDurationError {
	return badDurationError{
		green:    newStackId(green),
		duration: duration,
		key:      key,
	}
}

type badObservationStartError struct {
	green stackId
	start string
}

func (e badObservationStartError) Error() string {
	return fmt.Sprintf(
		"bad observation start timestamp %q specified for %s: %q",
		finalStartKey, e.green, e.start,
	)
}

func newBadObservationStartError(green canaryStack) badObservationStartError {
	return badObservationStartError{
		green: newStackId(green),
	}
}
