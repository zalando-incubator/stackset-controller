package main

// import (
// 	"testing"
// )

// func TestSyncAnnotationsPropagateToSegments(t *testing.T) {
// 	t.Parallel()

// 	stacksetName := "stackset-annotations-sync"
// 	specFactory := NewTestStacksetSpecFactory(
// 		stacksetName,
// 	).Ingress().RouteGroup()

// 	spec := specFactory.Create("v1")
// 	spec.Ingress.Annotations["a-random-annotation"] = "a-random-value"
// }