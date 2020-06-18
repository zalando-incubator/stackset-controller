package core

import (
	"fmt"
	"math"
	"time"

	log "github.com/sirupsen/logrus"
)

// TODO: Higher level abstraction
type GradualMetric interface {
	Check() (bool, error)
	StepWeight() int32
	Threshold() int32
	Interval() time.Duration
}

type TrueMetric struct{}

func (t TrueMetric) Check() (bool, error) {
	fmt.Println("running check")
	return true, nil
}

func (t TrueMetric) StepWeight() int32 {
	return 10
}

func (t TrueMetric) Threshold() int32 {
	return 3
}

func (t TrueMetric) Interval() time.Duration {
	return 1 * time.Minute
}

// GradualRolloutReconciler is a reconciler that gradually switches
// desired traffic for stacks being gradually rolled out.
type GradualRolloutReconciler struct{}

func (r GradualRolloutReconciler) Reconcile(stacks map[string]*StackContainer, currentTimestamp time.Time) error {
	// Gradual rollout stacks if needed
	for _, stack := range stacks {
		// gradual rollout is only enabled for stacks with a gradual
		// metric defined
		if stack.gradualMetric == nil {
			continue
		}

		// If traffic needs to be increased
		// if !stack.gradualRolloutFailed && stack.gradualDesiredTrafficWeight > stack.desiredTrafficWeight {
		if float64(stack.gradualDesiredTrafficWeight) > stack.desiredTrafficWeight {
			// If we never switched any traffic do that first
			// before we can observe metrics.
			if stack.desiredTrafficWeight <= 0 {
				stack.desiredTrafficWeight = math.Min(float64(stack.gradualDesiredTrafficWeight), float64(stack.gradualMetric.StepWeight()))
				continue
			}

			// wait for desired and actual weight to match before
			// trying to move forward
			if stack.desiredTrafficWeight > stack.actualTrafficWeight {
				continue
			}

			// first time we observe desired == actual mark the
			// last traffic increase.
			if stack.desiredTrafficWeight == stack.actualTrafficWeight {
				if stack.gradualLastTrafficIncrease.IsZero() {
					stack.gradualLastTrafficIncrease = currentTimestamp
				}
			}

			// check if we need to check the metrics
			// TODO: do we need lastTrafficIncrease?
			interval := stack.gradualMetric.Interval()
			fmt.Println("since last Traffic", time.Since(stack.gradualLastTrafficIncrease))
			fmt.Println("since last Metric", time.Since(stack.gradualLastMetricCheck))
			if time.Since(stack.gradualLastTrafficIncrease) >= interval && time.Since(stack.gradualLastMetricCheck) >= interval {
				// check metrics
				success, err := stack.gradualMetric.Check()
				if err != nil {
					// TODO: how to handle this?
					return err
				}

				stack.gradualLastMetricCheck = currentTimestamp

				if success {
					// mark the stack for traffic increase
					stack.desiredTrafficWeight = math.Min(float64(stack.gradualDesiredTrafficWeight), stack.desiredTrafficWeight+float64(stack.gradualMetric.StepWeight()))
					stack.gradualLastTrafficIncrease = time.Time{}
					// TODO: send event to stack
				} else {
					// TODO: total or in row?
					stack.gradualMetricFailureChecks++
					if stack.gradualMetricFailureChecks >= stack.gradualMetric.Threshold() {
						// TODO: send event to stack
						log.Errorf(
							"Gradual Rollout of stack %s/%s failed after %d checks. Rolling back %0.1f -> %0.1f",
							stack.Namespace(),
							stack.Name(),
							stack.gradualMetricFailureChecks,
							stack.desiredTrafficWeight,
							0.0,
						)
						// indicating rollback needed
						// stack.gradualRolloutFailed = true
						stack.desiredTrafficWeight = 0
						stack.gradualDesiredTrafficWeight = 0
						stack.gradualLastTrafficIncrease = time.Time{} // TODO: needed?
					}
				}
			}
		}
	}

	nonGradualWeights := make(map[string]float64, len(stacks))
	totalNonGradualWeight := 0.0
	totalWeight := 0.0
	for stackName, stack := range stacks {
		fmt.Println("before", stack.Name(), stack.desiredTrafficWeight)
		totalWeight += stack.desiredTrafficWeight

		if stack.desiredTrafficWeight >= float64(stack.gradualDesiredTrafficWeight) && stack.actualTrafficWeight >= stack.desiredTrafficWeight {
			totalNonGradualWeight += stack.desiredTrafficWeight
			nonGradualWeights[stackName] = stack.desiredTrafficWeight
			continue
		}
	}

	fmt.Println("total weight", totalWeight)
	fmt.Println("total non gradaual weight", totalNonGradualWeight)

	if totalNonGradualWeight > 0 {
		for stackName, weight := range nonGradualWeights {
			nonGradualWeights[stackName] = weight / totalNonGradualWeight
		}

		nonGradualWeight := 100 - (totalWeight - totalNonGradualWeight)

		for stackName, stack := range stacks {
			if weight, ok := nonGradualWeights[stackName]; ok {
				stack.desiredTrafficWeight = nonGradualWeight * weight
			}
			fmt.Println("after", stack.Name(), stack.desiredTrafficWeight)
		}
	}

	return nil
}
