package canary

const (
	canaryAnnotationKeyPrefix = "stackset-controller.zalando.org/canary."

	StacksetAnnotationKey = canaryAnnotationKeyPrefix + "gradual-deployments-enabled"

	// greenAnnotationKey is the Green stack annotation that identifies a stack as Green.
	// All other annotations may be kept after a successful gradual traffic switch
	// (for informational purposes) and so cannot be reliably used to select
	// currently Green stacks that deserve attention.
	greenAnnotationKey = canaryAnnotationKeyPrefix + "is-green"

	// baselineAnnotationKey is the Green stack annotation recording
	// the corresponding Baseline stack's ID.
	// Empty or absent if Baseline doesn't exist yet.
	baselineAnnotationKey = canaryAnnotationKeyPrefix + "baseline"

	// blueAnnotationKey is the Green stack annotation recording
	// the corresponding Blue stack's ID. Required.
	blueAnnotationKey = canaryAnnotationKeyPrefix + "blue"

	// desiredSwitchDurationKey is the Green stack annotation that records the user-specified
	// duration after which Green should be receiving 100% of traffic. Required.
	desiredSwitchDurationKey = canaryAnnotationKeyPrefix + "desired-switch-duration"

	// switchStartKey is the Green stack annotation that records when the Green stack started
	// being assigned more than 0% of traffic (the "start of the traffic switching").
	// Empty or absent if the traffic switch has not started.
	switchStartKey = canaryAnnotationKeyPrefix + "switch-start"

	// desiredFinalDurationKey is the Green stack annotation that records the user-specified
	// duration between Green receiving 100% of traffic and Green no longer being monitored.
	// In other words, it is the duration for which a completely-switched Green stack can
	// still be disabled by a rollback to Baseline. Required.
	desiredFinalDurationKey = canaryAnnotationKeyPrefix + "desired-final-observation-duration"

	// finalStartKey is the Green stack annotation that records when the Green stack
	// started its final observation period, i.e. the period during which it is assigned
	// 100% of traffic but is still monitored and could still be rolled back to Baseline
	// if the user expectations were no longer satisfied.
	// Empty or absent if the final observation period has not started.
	finalStartKey = canaryAnnotationKeyPrefix + "final-observation-start"
)
