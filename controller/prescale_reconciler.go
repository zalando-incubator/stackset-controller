package controller

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscaling "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	prescaleAnnotationKey        = "stacksetstacks.zalando.org/prescale-replicas"
	DefaultResetMinReplicasDelay = 10 * time.Minute
)

type PrescaleTrafficReconciler struct {
	ResetHPAMinReplicasTimeout time.Duration
}

type prescalingInfo struct {
	LastUpdated string `json:"lastUpdated"`
	Replicas    int    `json:"replicas"`
}

// ReconcileDeployment calculates the number replicas required when prescaling is active. If there is no associated
// HPA then the replicas of are also increased. Finally once traffic switching is complete the prescaling annotations
// are removed.
func (r *PrescaleTrafficReconciler) ReconcileDeployment(stacks map[types.UID]*StackContainer, stack *zv1.Stack, traffic map[string]TrafficStatus, deployment *appsv1.Deployment) error {
	// 1. Check if prescaling active
	// 2. If traffic does not need changing then return
	// 3. If prescaling not active calculate replicas
	// 4. If HPA not present set replicas

	prescalingInfoJson, prescalingActive := deployment.Annotations[prescaleAnnotationKey]
	var info prescalingInfo
	if prescalingActive {
		err := json.Unmarshal([]byte(prescalingInfoJson), &info)
		if err != nil {
			return fmt.Errorf("failed to deserialize prescaling informations: %v", err)
		}
	}

	if traffic != nil && traffic[stack.Name].DesiredWeight > 0 && traffic[stack.Name].ActualWeight < traffic[stack.Name].DesiredWeight {
		if !prescalingActive {
			for _, stackContainer := range stacks {
				if traffic[stackContainer.Stack.Name].ActualWeight > 0 {
					if stackContainer.Resources.Deployment != nil && stackContainer.Resources.Deployment.Spec.Replicas != nil {
						info.Replicas += int(*stackContainer.Resources.Deployment.Spec.Replicas)
					}
				}
			}
		}
		info.LastUpdated = time.Now().Format(time.RFC3339)

		updatedPrescalingJson, err := json.Marshal(info)
		if err != nil {
			return fmt.Errorf("failed to serialize prescaling information: %v", err)
		}
		deployment.Annotations[prescaleAnnotationKey] = string(updatedPrescalingJson)

		if stack.Spec.HorizontalPodAutoscaler == nil {
			replicas := int32(info.Replicas)
			deployment.Spec.Replicas = &replicas
		}
		return nil
	}

	if prescalingActive {
		lastUpdated, err := time.Parse(time.RFC3339, info.LastUpdated)
		if err != nil {
			return fmt.Errorf("failed to parse last updated timestamp: %v", err)
		}
		if time.Since(lastUpdated) > r.ResetHPAMinReplicasTimeout {
			delete(deployment.Annotations, prescaleAnnotationKey)
		}
	}
	return nil
}

// ReconcileHPA sets the MinReplicas to the prescale value defined in the
// annotation of the deployment. If no annotation is defined then the default
// minReplicas value is used from the Stack. This means that the HPA is allowed
// to scale down once the prescaling is done.
func (r *PrescaleTrafficReconciler) ReconcileHPA(stack *zv1.Stack, hpa *autoscaling.HorizontalPodAutoscaler, deployment *appsv1.Deployment) error {
	var info prescalingInfo
	prescalingInfoJson, prescalingActive := deployment.Annotations[prescaleAnnotationKey]

	if prescalingActive {
		err := json.Unmarshal([]byte(prescalingInfoJson), &info)
		if err != nil {
			return fmt.Errorf("failed to parse prescaling annotation: %v", err)
		}
		minReplicas := int32(math.Min(float64(info.Replicas), float64(stack.Spec.HorizontalPodAutoscaler.MaxReplicas)))
		hpa.Spec.MinReplicas = &minReplicas
		return nil
	}

	hpa.Spec.MinReplicas = stack.Spec.HorizontalPodAutoscaler.MinReplicas
	return nil
}

// getDeploymentPrescale parses and returns the prescale value if set in the
// deployment annotation.
func getDeploymentPrescale(deployment *appsv1.Deployment) (prescalingInfo, bool) {
	var info prescalingInfo
	prescaleReplicasJson, ok := deployment.Annotations[prescaleAnnotationKey]
	if !ok {
		return info, false
	}
	err := json.Unmarshal([]byte(prescaleReplicasJson), &info)
	if err != nil {
		return info, false
	}
	return info, true
}

// ReconcileIngress calculates the traffic distribution for the ingress. The
// implementation is optimized for prescaling stacks before directing traffic.
// It works like this:
//
// * If stack has a deployment with prescale annotation then it only gets
//   traffic if it has readyReplicas >= prescaleReplicas.
// * If stack is getting traffic but ReadyReplicas < prescaleReplicas, don't
//   remove traffic from it.
// * If no stacks are currently being prescaled fall back to the current
//   weights.
// * If no stacks are getting traffic fall back to desired weight without
//   checking health.
func (r *PrescaleTrafficReconciler) ReconcileIngress(stacks map[types.UID]*StackContainer, ingress *v1beta1.Ingress, traffic map[string]TrafficStatus) (map[string]float64, map[string]float64) {
	backendWeights := make(map[string]float64, len(stacks))
	currentWeights := make(map[string]float64, len(stacks))
	availableBackends := make(map[string]float64, len(stacks))
	for _, stack := range stacks {
		backendWeights[stack.Stack.Name] = traffic[stack.Stack.Name].DesiredWeight
		currentWeights[stack.Stack.Name] = traffic[stack.Stack.Name].ActualWeight

		deployment := stack.Resources.Deployment

		// prescale if stack is currently less than desired traffic
		if traffic[stack.Stack.Name].ActualWeight < traffic[stack.Stack.Name].DesiredWeight && deployment != nil {
			if pInfo, ok := getDeploymentPrescale(deployment); ok {
				var desired int32 = 1
				if deployment.Spec.Replicas != nil {
					desired = *deployment.Spec.Replicas
				}

				if desired >= int32(pInfo.Replicas) && deployment.Status.ReadyReplicas >= int32(pInfo.Replicas) {
					availableBackends[stack.Stack.Name] = traffic[stack.Stack.Name].DesiredWeight
				}
			}
			continue
		} else if traffic[stack.Stack.Name].ActualWeight > 0 && traffic[stack.Stack.Name].DesiredWeight > 0 {
			availableBackends[stack.Stack.Name] = traffic[stack.Stack.Name].DesiredWeight
		}
	}

	if !allZero(currentWeights) {
		normalizeWeights(currentWeights)
	}

	if !allZero(backendWeights) {
		normalizeWeights(backendWeights)
	}

	if len(availableBackends) == 0 {
		availableBackends = currentWeights
	}

	normalizeWeights(availableBackends)

	return availableBackends, backendWeights
}
