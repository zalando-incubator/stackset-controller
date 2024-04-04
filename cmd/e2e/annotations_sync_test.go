package main

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zalando-incubator/stackset-controller/controller"
	"github.com/zalando-incubator/stackset-controller/pkg/core"
)

func TestSyncAnnotationsPropagateToSegments(t *testing.T) {
	t.Parallel()

	for i, tc := range []struct {
		annotationsIng []map[string]string
		annotationsRg  []map[string]string
		expectedIng    []map[string]string
		expectedRg     []map[string]string
	}{
		{
			annotationsIng: []map[string]string{
				{},
				{
					"example.org/i-haz-synchronize": "keep-sync",
					"teapot.org/the-best":           "for-real",
					"example.org/no-sync-plz":       "ditto",
				},
			},
			annotationsRg: []map[string]string{
				{},
				{
					"example.org/i-haz-synchronize": "synchronize",
					"teapot.org/the-best":           "of-all-time",
					"example.org/no-sync-plz":       "nope",
				},
			},
			expectedIng: []map[string]string{
				{
					"example.org/i-haz-synchronize": "keep-sync",
					"teapot.org/the-best":           "for-real",
				},
				{
					"example.org/i-haz-synchronize": "keep-sync",
					"teapot.org/the-best":           "for-real",
					"example.org/no-sync-plz":       "ditto",
				},
			},
			expectedRg: []map[string]string{
				{
					"example.org/i-haz-synchronize": "synchronize",
					"teapot.org/the-best":           "of-all-time",
				},
				{
					"example.org/i-haz-synchronize": "synchronize",
					"teapot.org/the-best":           "of-all-time",
					"example.org/no-sync-plz":       "nope",
				},
			},
		},
		{
			annotationsIng: []map[string]string{
				{
					"example.org/i-haz-synchronize": "keep-sync",
					"teapot.org/the-best":           "for-real",
				},
				{
					"example.org/i-haz-synchronize": "keep-sync",
					"example.org/no-sync-plz":       "ditto",
				},
			},
			annotationsRg: []map[string]string{
				{
					"example.org/i-haz-synchronize": "synchronize",
					"teapot.org/the-best":           "of-all-time",
				},
				{
					"teapot.org/the-best":     "of-all-time",
					"example.org/no-sync-plz": "nope",
				},
			},
			expectedIng: []map[string]string{
				{
					"example.org/i-haz-synchronize": "keep-sync",
				},
				{
					"example.org/i-haz-synchronize": "keep-sync",
					"example.org/no-sync-plz":       "ditto",
				},
			},
			expectedRg: []map[string]string{
				{
					"teapot.org/the-best": "of-all-time",
				},
				{
					"teapot.org/the-best":     "of-all-time",
					"example.org/no-sync-plz": "nope",
				},
			},
		},
	} {
		stacksetName := fmt.Sprintf("stackset-annotations-sync-case-%d", i)
		specFactory := NewTestStacksetSpecFactory(
			stacksetName,
		).Ingress().RouteGroup()
		spec := specFactory.Create(t, "v0")

		for i, a := range tc.annotationsIng {
			version := fmt.Sprintf("v%d", i)
			spec.StackTemplate.Spec.Version = version
			spec.Ingress.Annotations = a
			spec.RouteGroup.Annotations = tc.annotationsRg[i]

			var err error
			if i == 0 {
				err = createStackSetWithAnnotations(
					stacksetName,
					1,
					spec,
					map[string]string{
						controller.TrafficSegmentsAnnotationKey: "true",
					},
				)
			} else {
				err = updateStackSetWithAnnotations(
					stacksetName,
					spec,
					map[string]string{
						controller.TrafficSegmentsAnnotationKey: "true",
					},
				)
			}
			require.NoError(t, err)

			_, err = waitForIngressSegment(t, stacksetName, version)
			require.NoError(t, err)

			_, err = waitForRouteGroupSegment(t, stacksetName, version)
			require.NoError(t, err)
		}

		// Wait some time for the annotations to be propagated
		time.Sleep(time.Second * 10)

		for i := 0; i < len(tc.annotationsIng); i++ {
			version := fmt.Sprintf("v%d", i)
			ingress, err := waitForIngressSegment(t, stacksetName, version)
			require.NoError(t, err)

			delete(
				ingress.Annotations,
				"stackset-controller.zalando.org/stack-generation",
			)
			delete(
				ingress.Annotations,
				core.IngressPredicateKey,
			)

			if !reflect.DeepEqual(tc.expectedIng[i], ingress.Annotations) {
				t.Errorf(
					"Expected ingress annotations in %q to be %v, got %v",
					version,
					tc.expectedIng[i],
					ingress.Annotations,
				)
			}

			routeGroup, err := waitForRouteGroupSegment(
				t,
				stacksetName,
				version,
			)
			require.NoError(t, err)

			delete(
				routeGroup.Annotations,
				"stackset-controller.zalando.org/stack-generation",
			)

			if !reflect.DeepEqual(tc.expectedRg[i], routeGroup.Annotations) {
				t.Errorf(
					"Expected routeGroup annotations in %q to be %v, got %v",
					version,
					tc.expectedRg[i],
					routeGroup.Annotations,
				)
			}
		}
	}
}
