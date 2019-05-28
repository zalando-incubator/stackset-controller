package canary

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/zalando-incubator/stackset-controller/controller/canary/percent"
	"github.com/zalando-incubator/stackset-controller/controller/entities"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

// TrafficReconciler implements gradual deployments from Blue to Green via Baseline stacks.
type TrafficReconciler struct {
	// The reconciler which actually implements the traffic switch
	baseReconciler entities.TrafficReconciler
	checker        ExpectationChecker
	greenStates    map[string]tsState
	context        CanaryContext
}

func NewTrafficReconcilerReal(
	base entities.TrafficReconciler,
	client clientset.Interface,
	rec record.EventRecorder,
	logger logrus.FieldLogger,
) *TrafficReconciler {
	return &TrafficReconciler{
		baseReconciler: base,
		checker:        annotationChecker{},
		greenStates:    nil,
		context: CanaryContext{
			Client: NewRealKubeClient(client, rec, logger),
			Logger: logger,
		},
	}
}

// tsState is how ReconcileStacks updates the state read by ReconcileIngress.
type tsState struct {
	greenActualTraffic percent.Percent

	blueName          string
	blueActualTraffic percent.Percent

	baselineName          string
	baselineActualTraffic percent.Percent

	totalActualTraffic    percent.Percent
	desiredSwitchDuration time.Duration
	switchStartTime       time.Time
	ingressAction         reconcileIngressAction
}

type reconcileIngressAction int

const (
	actionDoNothing reconcileIngressAction = iota
	actionRollback
	actionIncrease
)

// ReconcileDeployment just delegates to the underlying baseReconciler.
func (r *TrafficReconciler) ReconcileDeployment(
	stacks map[types.UID]*entities.StackContainer,
	stack *zv1.Stack,
	traffic map[string]entities.TrafficStatus,
	deployment *appsv1.Deployment,
) error {
	return r.baseReconciler.ReconcileDeployment(stacks, stack, traffic, deployment)
}

// ReconcileHPA just delegates to the underlying baseReconciler.
func (r *TrafficReconciler) ReconcileHPA(
	stack *zv1.Stack,
	hpa *autoscaling.HorizontalPodAutoscaler,
	deployment *appsv1.Deployment,
) error {
	return r.baseReconciler.ReconcileHPA(stack, hpa, deployment)
}

// ReconcileIngress computes the new weights necessary for gradual traffic switching.
// Returns (actualWeights, desiredWeights)
func (r *TrafficReconciler) ReconcileIngress(
	stacks map[types.UID]*entities.StackContainer,
	ingress *v1beta1.Ingress,
	traffic map[string]entities.TrafficStatus,
) (map[string]float64, map[string]float64, error) {
	for greenName, greenState := range r.greenStates {
		var err error
		switch greenState.ingressAction {
		case actionRollback:
			rollbacker := FullBlueRollBack{
				blueName:           greenState.blueName,
				baselineName:       greenState.baselineName,
				greenName:          greenName,
				totalActualTraffic: greenState.totalActualTraffic,
			}
			traffic, err = rollbacker.Rollback(traffic)
			if err != nil {
				r.context.Logger.Errorf("failed to rollback: %s", err)
				return nil, nil, err
			}
		case actionIncrease:
			elapsedTime := time.Now().Sub(greenState.switchStartTime)

			var newTraffic TrafficMap
			newTraffic, err = increaseTraffic(
				traffic,
				greenState.blueName,
				greenState.baselineName,
				greenName,
				greenState.greenActualTraffic,
				greenState.totalActualTraffic,
				greenState.desiredSwitchDuration,
				elapsedTime,
			)
			if err == nil {
				traffic = newTraffic
			} else {
				r.context.Logger.Errorf("failed to increase traffic: %s", err)
				return nil, nil, err
			}
		}
	}

	return r.baseReconciler.ReconcileIngress(stacks, ingress, traffic)
}

// ReconcileStacks creates Baseline and manages Green lifecycle via annotations.
func (r *TrafficReconciler) ReconcileStacks(ssc *entities.StackSetContainer) error {
	logger := r.context.Logger.WithField(
		"stackset",
		fmt.Sprintf("%s/%s", ssc.StackSet.Namespace, ssc.StackSet.Name),
	)
	logger.Debug("ReconcileStacks started")
	newGreenStates := map[string]tsState{}
	for _, sc := range allGreenStacks(ssc.StackContainers) {
		logger := logger.WithField("green", sc.Name)

		d, errors := newTsInput(sc.Stack, ssc.StackContainers, ssc.Traffic)
		if errors != nil {
			for _, err := range errors {
				logger.Error(err)
			}
			continue
		}
		logger = logger.WithFields(logrus.Fields{
			"blue":                d.blue.Name,
			"desiredSwitchPeriod": d.desiredSwitchDuration,
		})

		if d.baseline == nil {
			if err := createBaseline(r.context.WithLogger(logger), *d, &ssc.StackSet); err != nil {
				logger.Errorf("failed to create baseline: %s", err)
			}
			continue // Traffic will be assigned at the next stackset-controller run.
		}
		logger = logger.WithField("baseline", d.baseline.Name)

		totalActualTraffic := totalActualTrafficFor([]canaryStack{d.green, d.blue, *d.baseline})
		if totalActualTraffic.Error() != nil {
			logger.Errorf(
				"failed to compute green+blue+baseline traffic: %s", totalActualTraffic.Error())
			continue
		}

		state := tsState{
			greenActualTraffic:    d.green.actualTraffic,
			blueName:              d.blue.Name,
			blueActualTraffic:     d.blue.actualTraffic,
			baselineName:          d.baseline.Name,
			baselineActualTraffic: d.baseline.actualTraffic,
			desiredSwitchDuration: d.desiredSwitchDuration,
			switchStartTime:       d.switchStartTime,
			totalActualTraffic:    totalActualTraffic,
			ingressAction:         actionDoNothing,
		}
		e := r.context.WithLogger(logger)
		if d.switchStartTime.IsZero() || d.green.actualTraffic.IsZero() {
			if !manageStateForNoTraffic(e, &state, *d, &ssc.StackSet) {
				continue
			}
		} else if r.checker.ExpectationsFulfilled(*d.green.Stack, *d.baseline.Stack) {
			logger.Infof("expectations still fulfilled, continuing")
			ok := manageStateForExpectationsFulfilled(e, &state, *d, &ssc.StackSet, totalActualTraffic)
			if !ok {
				continue
			}
		} else {
			state.ingressAction = actionRollback
			r.context.Client.MarkStackNotGreen(&ssc.StackSet, d.green.Stack, "rollback")
		}
		newGreenStates[d.green.Name] = state
	}
	r.greenStates = newGreenStates // Also cleans up the old states.
	return nil
}

// tsInput is a helper type for ReconcileStacks and its own helper functions. Please do not use it outside.
type tsInput struct {
	green                 canaryStack
	blue                  canaryStack
	baseline              *canaryStack
	desiredSwitchDuration time.Duration
	switchStartTime       time.Time
}

func newTsInput(green *zv1.Stack, allStacks map[types.UID]*entities.StackContainer, tm TrafficMap) (*tsInput, []error) {
	errors := []error{}
	g := canaryStack{
		Stack:      green,
		canaryKind: skGreen,
	}
	if err := g.readActualTraffic(tm); err != nil {
		errors = append(errors, err)
	}

	desiredSwitchDuration, err := desiredSwitchDurationFor(g)
	if err != nil {
		errors = append(errors, err)
	}

	switchStartTime, err := switchStartTimeFor(g)
	if err != nil {
		errors = append(errors, err)
	}

	blue, err := blueStackFor(g, allStacks)
	if err == nil {
		if err := blue.readActualTraffic(tm); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	baseline, err := baselineStackFor(g, allStacks)
	if err != nil {
		errors = append(errors, err)
	} else if baseline != nil {
		if err := baseline.readActualTraffic(tm); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return nil, errors
	}
	return &tsInput{
		green:                 g,
		blue:                  *blue,
		baseline:              baseline,
		desiredSwitchDuration: desiredSwitchDuration,
		switchStartTime:       switchStartTime,
	}, nil
}

func createBaseline(context CanaryContext, d tsInput, set *zv1.StackSet) error {
	baselineName := d.green.Name + "-baseline"
	context.Logger.Infof("creating baseline stack %q", baselineName)
	baselineSpec := zv1.StackSpec{ // Same infrastructure as green but same code as blue!
		Replicas:                d.green.Spec.Replicas,
		HorizontalPodAutoscaler: d.green.Spec.HorizontalPodAutoscaler.DeepCopy(),
		Service:                 d.green.Spec.Service.DeepCopy(),
		PodTemplate:             d.blue.Spec.PodTemplate,
		Autoscaler:              d.green.Spec.Autoscaler.DeepCopy(),
	}
	baseline, err := context.Client.CreateStack(set, baselineName, "default", baselineSpec)
	if err != nil {
		return err
	}
	_, err = context.SetAnnotation(set, baseline, baselineAnnotationKey, baselineName)
	if err != nil {
		e := context.Client.RemoveStack(set, baseline.Name, "failed to update green annotations")
		context.Logger.Errorf("failed to clean up aborted baseline stack: %s", e)
	}
	return err
}

func manageStateForNoTraffic(context CanaryContext, state *tsState, d tsInput, set *zv1.StackSet) bool {
	context.Logger.Infof("starting switching traffic to green")
	var err error
	state.switchStartTime, err = markStartTrafficSwitch(context, set, &d.green)
	if err != nil {
		context.Logger.Errorf("failed to update %s: %v", d.green, err)
		return false
	}
	state.ingressAction = actionIncrease
	return true
}

func manageStateForExpectationsFulfilled(
	context CanaryContext,
	state *tsState,
	d tsInput,
	set *zv1.StackSet,
	totalActualTraffic percent.Percent,
) bool {
	if cmp := d.green.actualTraffic.Cmp(totalActualTraffic); cmp >= 0 {
		// We are finished with the traffic switch.
		if cmp > 0 {
			context.Logger.Warnf(
				"traffic for %s (%s) is larger than targeted final traffic (%s)",
				d.green, d.green.actualTraffic, totalActualTraffic,
			)
		}
		return manageObsPeriod(context, d, set)
	}
	if d.baseline.actualTraffic.IsZero() {
		context.Logger.Info("waiting for latest baseline traffic increase to be applied")
		state.ingressAction = actionDoNothing
	} else {
		state.ingressAction = actionIncrease
	}
	return true
}

func manageObsPeriod(context CanaryContext, d tsInput, set *zv1.StackSet) bool {
	context.Logger.Infof("%s is in observation period", d.green)
	obsPeriod, obsStart, err := readObservationData(d.green)
	if err != nil {
		context.Logger.Error(err)
		return false
	}
	if obsPeriod == 0 {
		// Gradual deployment successfully finished without observation period!
		context.Client.MarkStackNotGreen(set, d.green.Stack, "reached target traffic, no observation period")
		return false // Everything is fine but we don't want to keep the state.
	}
	if obsStart.IsZero() {
		_, err := markStartObservation(context, set, &d.green)
		if err != nil {
			context.Logger.Errorf("failed to start observation period: %v", err)
			return false
		}
		return true
	}
	if obsStart.Add(obsPeriod).Before(time.Now()) {
		// Gradual deployment successfully finished after observation period!
		context.Client.MarkStackNotGreen(set, d.green.Stack, fmt.Sprintf(
			"reached target traffic, observation period %v over (started at %v)",
			obsPeriod, obsStart,
		))
		return false // Everything is fine but we don't want to keep the state.
	}
	return true
}

func markStartTrafficSwitch(context CanaryContext, set *zv1.StackSet, green *canaryStack) (time.Time, error) {
	return markStartOf(context, set, green, switchStartKey)
}

func markStartObservation(context CanaryContext, set *zv1.StackSet, green *canaryStack) (time.Time, error) {
	return markStartOf(context, set, green, finalStartKey)
}

func markStartOf(context CanaryContext, set *zv1.StackSet, green *canaryStack, key string) (time.Time, error) {
	context.Logger.Debugf("setting %s of %s to now", key, *green)
	t := time.Now()
	updatedStack, err := context.SetAnnotation(set, green.Stack, key, t.Format(time.RFC3339))
	if err == nil {
		green.Stack = updatedStack
	}
	return t, err
}

// allGreenStacks returns all provided StackContainer that contain a green stack.
func allGreenStacks(stackContainers map[types.UID]*entities.StackContainer) []canaryStack {
	greens := []canaryStack{}
	for _, sc := range stackContainers {
		if _, ok := sc.Stack.Annotations[greenAnnotationKey]; ok {
			greens = append(greens, canaryStack{Stack: &sc.Stack, canaryKind: skGreen})
		}
	}
	return greens
}

func desiredSwitchDurationFor(green canaryStack) (duration time.Duration, err error) {
	durationStr := green.Annotations[desiredSwitchDurationKey]
	duration, err = time.ParseDuration(durationStr)
	if err != nil {
		err = newBadDurationError(green, durationStr, desiredSwitchDurationKey)
	}
	return
}

func switchStartTimeFor(green canaryStack) (time.Time, error) {
	return readOptionalTimeAnnotation(green.Annotations, switchStartKey)
}

func blueStackFor(green canaryStack, scs map[types.UID]*entities.StackContainer) (*canaryStack, error) {
	blueName := green.Annotations[blueAnnotationKey]
	if blueName == "" {
		return nil, newNoBlueAnnotError(green)
	}
	for _, sc := range scs {
		if sc.Stack.Name == blueName {
			return &canaryStack{Stack: &sc.Stack, canaryKind: skBlue}, nil
		}
	}
	return nil, newStackNotFoundError(green, blueName, skBlue)
}

func baselineStackFor(green canaryStack, scs map[types.UID]*entities.StackContainer) (*canaryStack, error) {
	baselineName, ok := green.Annotations[baselineAnnotationKey]
	if !ok {
		return nil, nil // We need to create the baseline stack.
	}
	for _, sc := range scs {
		if sc.Stack.Name == baselineName {
			return &canaryStack{Stack: &sc.Stack, canaryKind: skBaseline}, nil
		}
	}
	return nil, newStackNotFoundError(green, baselineName, skBaseline)
}

func totalActualTrafficFor(stacks []canaryStack) percent.Percent {
	total := percent.NewFromInt(0)
	for _, s := range stacks {
		total = total.Add(s.actualTraffic)
	}
	return total
}

func readObservationData(green canaryStack) (time.Duration, time.Time, error) {
	durationStr := green.Annotations[desiredFinalDurationKey]
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return 0, time.Time{}, newBadDurationError(green, durationStr, desiredFinalDurationKey)
	}
	t, err := readOptionalTimeAnnotation(green.Annotations, finalStartKey)
	if err != nil {
		return 0, time.Time{}, newBadObservationStartError(green)
	}
	return duration, t, nil
}

// readOptionalTimeAnnotation reads the value corresponding to the key annotation.
// That value may not exist (which is not an error), but when it does it must be
// a RFC3339-formatted date (otherwise an error is returned).
func readOptionalTimeAnnotation(annotations map[string]string, key string) (time.Time, error) {
	str, ok := annotations[key]
	if !ok {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, str)
	if err != nil {
		return time.Time{}, fmt.Errorf("read %v: %v", key, err)
	}
	return t, nil
}
