package core

import (
	"encoding/json"
	"testing"

	rgv1 "github.com/szuecs/routegroup-client/apis/zalando.org/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestComputeTrafficSegments(t *testing.T) {
	for _, tc := range []struct {
		actualTrafficWeights map[types.UID]float64
		ingressSegments      map[types.UID]string
		routeGroupSegments   map[types.UID]string
		expected             []map[types.UID]map[string]string
		expectErr            bool
	}{
		{
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
			},
			actualTrafficWeights: map[types.UID]float64{
				"v1": 100.0,
			},
			expected: []map[types.UID]map[string]string{
				{"v1": {"ingress": "TrafficSegment(0.00, 1.00)"}},
			},
			expectErr: false,
		},
		{
			ingressSegments: map[types.UID]string{
				"v1": "",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.0)",
			},
			actualTrafficWeights: map[types.UID]float64{
				"v1": 100.0,
			},
			expected: []map[types.UID]map[string]string{
				{"v1": {"routegroup": "TrafficSegment(0.00, 1.00)"}},
			},
			expectErr: false,
		},
		{
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.0)",
			},
			actualTrafficWeights: map[types.UID]float64{
				"v1": 100.0,
			},
			expected: []map[types.UID]map[string]string{
				{
					"v1": {
						"ingress":    "TrafficSegment(0.00, 1.00)",
						"routegroup": "TrafficSegment(0.00, 1.00)",
					},
				},
			},
			expectErr: false,
		},
		{
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 1.0)",
				"v2": "TrafficSegment(0.0, 0.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
			},
			actualTrafficWeights: map[types.UID]float64{
				"v1": 0.0,
				"v2": 100.0,
			},
			expected: []map[types.UID]map[string]string{
				{"v2": {"ingress": "TrafficSegment(0.00, 1.00)"}},
				{"v1": {"ingress": "TrafficSegment(0.00, 0.00)"}},
			},
			expectErr: false,
		},
		{
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 1.0)",
				"v2": "",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
				"v2": "TrafficSegment(0.0, 0.0)",
			},
			actualTrafficWeights: map[types.UID]float64{
				"v1": 0.0,
				"v2": 100.0,
			},
			expected: []map[types.UID]map[string]string{
				{"v2": {"routegroup": "TrafficSegment(0.00, 1.00)"}},
				{"v1": {"ingress": "TrafficSegment(0.00, 0.00)"}},
			},
			expectErr: false,
		},
		{
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.4)",
				"v2": "TrafficSegment(0.4, 0.6)",
				"v3": "TrafficSegment(0.6, 1.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
				"v2": "",
				"v3": "",
			},
			actualTrafficWeights: map[types.UID]float64{
				"v1": 30.0,
				"v2": 40.0,
				"v3": 30.0,
			},
			expected: []map[types.UID]map[string]string{
				{"v2": {"ingress": "TrafficSegment(0.30, 0.70)"}},
				{"v1": {"ingress": "TrafficSegment(0.00, 0.30)"}},
				{"v3": {"ingress": "TrafficSegment(0.70, 1.00)"}},
			},
			expectErr: false,
		},
		{
			ingressSegments: map[types.UID]string{
				"v1": "TrafficSegment(0.0, 0.3)",
				"v2": "TrafficSegment(0.3, 0.7)",
				"v3": "TrafficSegment(0.7, 1.0)",
			},
			routeGroupSegments: map[types.UID]string{
				"v1": "",
				"v2": "",
				"v3": "",
			},
			actualTrafficWeights: map[types.UID]float64{
				"v1": 40.0,
				"v2": 20.0,
				"v3": 40.0,
			},
			expected: []map[types.UID]map[string]string{
				{"v1": {"ingress": "TrafficSegment(0.00, 0.40)"}},
				{"v3": {"ingress": "TrafficSegment(0.60, 1.00)"}},
				{"v2": {"ingress": "TrafficSegment(0.40, 0.60)"}},
			},
			expectErr: false,
		},
	} {
		expectedJSON, err := json.MarshalIndent(tc.expected, "", "  ")
		if err != nil {
			// shouldn't happen
			t.Errorf("Failed marshalling expected result")
			break
		}
		expectedPretty := string(expectedJSON)

		stackContainers := map[types.UID]*StackContainer{}
		for k, v := range tc.actualTrafficWeights {
			stackContainers[k] = &StackContainer{
				actualTrafficWeight: v,
				Resources:           StackResources{},
			}

			if tc.ingressSegments[k] != "" {
				stackContainers[k].Resources.IngressSegment = &v1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							IngressPredicateKey: tc.ingressSegments[k],
						},
					},
				}
			}

			if tc.routeGroupSegments[k] != "" {
				stackContainers[k].Resources.RouteGroupSegment = &rgv1.RouteGroup{
					Spec: rgv1.RouteGroupSpec{
						Routes: []rgv1.RouteGroupRouteSpec{
							{Predicates: []string{tc.routeGroupSegments[k]}},
						},
					},
				}
			}
		}

		ssc := &StackSetContainer{
			StackContainers: stackContainers,
		}

		res, err := ssc.ComputeTrafficSegments()
		if (err != nil) != tc.expectErr {
			t.Errorf(
				"expected error: %s. got error: %s",
				yesNo[tc.expectErr],
				yesNo[err != nil],
			)
			continue
		}

		if tc.expectErr {
			continue
		}

		if len(res) != len(tc.expected) {
			t.Errorf(
				"wrong number of segments (%d), expected\n%s",
				len(res),
				expectedPretty,
			)
			continue
		}

		for i, v := range res {
			segments, ok := tc.expected[i][v.id]
			if !ok {
				t.Errorf(
					"id mismatch at index %d(%q), expected\n%s",
					i,
					v.id,
					expectedPretty,
				)
				break
			}

			ingressSegment, ok := segments["ingress"]
			if !ok && v.IngressSegment != nil {
				t.Errorf(
					"non nil IngressSegment at index %d, expected\n%s",
					i,
					expectedPretty,
				)
				break
			}

			if ok {
				if v.IngressSegment == nil {
					t.Errorf(
						"nil IngressSegment at index %d, expected\n%s",
						i,
						expectedPretty,
					)
					break
				}

				if v.IngressSegment.Annotations[IngressPredicateKey] !=
					ingressSegment {

					t.Errorf(
						"IngressSegment mismatch %q at index %d, expected\n%s",
						v.IngressSegment.Annotations[IngressPredicateKey],
						i,
						expectedPretty,
					)
					break
				}
			}

			rgSegment, ok := segments["routegroup"]
			if !ok && v.RouteGroupSegment != nil {
				t.Errorf(
					"non nil RouteGroupSegment at index %d, expected\n%s",
					i,
					expectedPretty,
				)
				break
			}

			if ok {
				if v.RouteGroupSegment == nil {
					t.Errorf(
						"nil RouteGroupSegment at index %d, expected\n%s",
						i,
						expectedPretty,
					)
					break
				}

				for _, pred := range v.RouteGroupSegment.Spec.Routes[0].Predicates {
					if pred == rgSegment {
						continue
					}
				}

				t.Errorf(
					"RouteGroupSegment not found at index %d, expected\n%s",
					i,
					expectedPretty,
				)
			}
		}
	}
}
