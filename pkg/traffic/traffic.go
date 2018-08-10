package traffic

import (
	"encoding/json"
	"fmt"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando/v1"
	clientset "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	stacksetHeritageLabelKey         = "stackset"
	stackTrafficWeightsAnnotationKey = "zalando.org/stack-traffic-weights"
	backendWeightsAnnotationKey      = "zalando.org/backend-weights"
)

// Switcher is able to switch traffic between stacks.
type Switcher struct {
	kube      kubernetes.Interface
	appClient clientset.Interface
}

// NewSwitcher initializes a new traffic switcher.
func NewSwitcher(kube kubernetes.Interface, client clientset.Interface) *Switcher {
	return &Switcher{
		kube:      kube,
		appClient: client,
	}
}

// Switch changes traffic weight for a stack.
func (t *Switcher) Switch(stackset, stack, namespace string, weight float64) ([]StackTrafficWeight, error) {
	stacks, err := t.getStacks(stackset, namespace)
	if err != nil {
		return nil, err
	}

	normalized := normalizeWeights(stacks)
	newWeights, err := setWeightForStacks(normalized, stack, weight)
	if err != nil {
		return nil, err
	}

	changeNeeded := false
	stackWeights := make(map[string]float64, len(newWeights))
	for i, stack := range newWeights {
		if stack.Weight != stacks[i].Weight {
			changeNeeded = true
		}
		stackWeights[stack.Name] = stack.Weight
	}

	if changeNeeded {
		stackWeightsData, err := json.Marshal(&stackWeights)
		if err != nil {
			return nil, err
		}

		annotation := map[string]map[string]map[string]string{
			"metadata": map[string]map[string]string{
				"annotations": map[string]string{
					stackTrafficWeightsAnnotationKey: string(stackWeightsData),
				},
			},
		}

		annotationData, err := json.Marshal(&annotation)
		if err != nil {
			return nil, err
		}

		_, err = t.kube.ExtensionsV1beta1().Ingresses(namespace).Patch(stackset, types.StrategicMergePatchType, annotationData)
		if err != nil {
			return nil, err
		}
	}

	return newWeights, nil
}

type StackTrafficWeight struct {
	Name         string
	Weight       float64
	ActualWeight float64
}

// TrafficWeights returns a list of stacks with their current traffic weight.
func (t *Switcher) TrafficWeights(stackset, namespace string) ([]StackTrafficWeight, error) {
	stacks, err := t.getStacks(stackset, namespace)
	if err != nil {
		return nil, err
	}
	return normalizeWeights(stacks), nil
}

// getStacks returns the stacks of the stackset.
func (t *Switcher) getStacks(stackset, namespace string) ([]StackTrafficWeight, error) {
	heritageLabels := map[string]string{
		stacksetHeritageLabelKey: stackset,
	}
	opts := metav1.ListOptions{
		LabelSelector: labels.Set(heritageLabels).String(),
	}

	stacks, err := t.appClient.ZalandoV1().Stacks(namespace).List(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list stacks of stackset %s/%s: %v", namespace, stackset, err)
	}

	desired, actual, err := t.getIngressTraffic(stackset, namespace, stacks.Items)
	if err != nil {
		return nil, fmt.Errorf("failed to get Ingress traffic for StackSet %s/%s: %v", namespace, stackset, err)
	}

	stackWeights := make([]StackTrafficWeight, 0, len(stacks.Items))
	for _, stack := range stacks.Items {
		stackWeight := StackTrafficWeight{
			Name:         stack.Name,
			Weight:       desired[stack.Name],
			ActualWeight: actual[stack.Name],
		}

		stackWeights = append(stackWeights, stackWeight)
	}
	return stackWeights, nil
}

func (t *Switcher) getIngressTraffic(name, namespace string, stacks []zv1.Stack) (map[string]float64, map[string]float64, error) {
	if len(stacks) == 0 {
		return map[string]float64{}, map[string]float64{}, nil
	}

	ingress, err := t.kube.ExtensionsV1beta1().Ingresses(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	desiredTraffic := make(map[string]float64, len(stacks))
	if weights, ok := ingress.Annotations[stackTrafficWeightsAnnotationKey]; ok {
		err := json.Unmarshal([]byte(weights), &desiredTraffic)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get current desired Stack traffic weights: %v", err)
		}
	}

	actualTraffic := make(map[string]float64, len(stacks))
	if weights, ok := ingress.Annotations[backendWeightsAnnotationKey]; ok {
		err := json.Unmarshal([]byte(weights), &actualTraffic)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get current actual Stack traffic weights: %v", err)
		}
	}

	return desiredTraffic, actualTraffic, nil
}

// setWeightForStacks sets new traffic weight for the specified stack and adjusts
// the other stack weights relatively.
// It's assumed that the sum of weights over all stacks are 100.
func setWeightForStacks(stacks []StackTrafficWeight, stackName string, weight float64) ([]StackTrafficWeight, error) {
	newWeights := make([]StackTrafficWeight, len(stacks))
	currentWeight := float64(0)
	for i, stack := range stacks {
		if stack.Name == stackName {
			currentWeight = stack.Weight
			stack.Weight = weight
			newWeights[i] = stack
			break
		}
	}

	change := float64(0)

	if currentWeight < 100 {
		change = (100 - weight) / (100 - currentWeight)
	} else if weight < 100 {
		return nil, fmt.Errorf("'%s' is the only Stack getting traffic, Can't reduce it to %.1f%%", stackName, weight)
	}

	for i, stack := range stacks {
		if stack.Name != stackName {
			stack.Weight *= change
			newWeights[i] = stack
		}
	}

	return newWeights, nil
}

// allZero returns true if all weights defined in the map are 0.
func allZero(stacks []StackTrafficWeight) bool {
	for _, stack := range stacks {
		if stack.Weight > 0 {
			return false
		}
	}
	return true
}

// normalizeWeights normalizes the traffic weights specified on the stacks.
// If all weights are zero the total weight of 100 is distributed equally
// between all stacks.
// If not all weights are zero they are normalized to a sum of 100.
func normalizeWeights(stacks []StackTrafficWeight) []StackTrafficWeight {
	newWeights := make([]StackTrafficWeight, len(stacks))
	// if all weights are zero distribute them equally to all backends
	if allZero(stacks) && len(stacks) > 0 {
		eqWeight := 100 / float64(len(stacks))
		for i, stack := range stacks {
			stack.Weight = eqWeight
			newWeights[i] = stack
		}
		return newWeights
	}

	// if not all weights are zero, normalize them to a sum of 100
	sum := float64(0)
	for _, stack := range stacks {
		sum += stack.Weight
	}

	for i, stack := range stacks {
		stack.Weight = stack.Weight / sum * 100
		newWeights[i] = stack
	}

	return newWeights
}
