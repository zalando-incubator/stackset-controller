package entities

import "k8s.io/apimachinery/pkg/types"

import (
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
)

type StackStatus struct {
	Stack     zv1.Stack
	Available bool
}

func GetStackStatuses(stacks map[types.UID]*StackContainer) []StackStatus {
	statuses := make([]StackStatus, 0, len(stacks))
	for _, stack := range stacks {
		status := StackStatus{
			Stack: stack.Stack,
		}

		// check that service has at least one endpoint, otherwise it
		// should not get traffic.
		endpoints := stack.Resources.Endpoints
		if endpoints == nil {
			status.Available = false
		} else {
			readyEndpoints := 0
			for _, subset := range endpoints.Subsets {
				readyEndpoints += len(subset.Addresses)
			}

			status.Available = readyEndpoints > 0
		}

		statuses = append(statuses, status)
	}

	return statuses
}
