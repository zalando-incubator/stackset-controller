package keys

const (
	StackGenerationAnnotationKey              = "stackset-controller.zalando.org/stack-generation"
	StacksetControllerControllerAnnotationKey = "stackset-controller.zalando.org/controller"
	PrescaleStacksAnnotationKey               = "alpha.stackset-controller.zalando.org/prescale-stacks"
	ResetHPAMinReplicasDelayAnnotationKey     = "alpha.stackset-controller.zalando.org/reset-hpa-min-replicas-delay"
	StacksetGenerationAnnotationKey           = "stackset-controller.zalando.org/stackset-generation"

	StacksetHeritageLabelKey = "stackset"
	StackVersionLabelKey     = "stack-version"
)
