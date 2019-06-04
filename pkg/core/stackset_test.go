package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestExpiredStacks(t *testing.T) {
	now := time.Now()

	stackContainer := func(name string, creationTime time.Time, noTrafficSince time.Time, desiredTraffic, actualTraffic float64) *StackContainer {
		return &StackContainer{
			Stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name:              name,
					CreationTimestamp: metav1.NewTime(creationTime),
				},
			},
			desiredTrafficWeight: desiredTraffic,
			actualTrafficWeight:  actualTraffic,
			noTrafficSince:       noTrafficSince,
		}
	}

	for _, tc := range []struct {
		name                string
		limit               int32
		scaledownTTLSeconds time.Duration
		ingress             bool
		stacks              []*StackContainer
		expected            map[string]bool
	}{
		{
			name:    "test GC oldest stack",
			limit:   1,
			ingress: true,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), now.Add(-1*time.Hour), 0, 0),
				stackContainer("stack2", now.Add(-2*time.Hour), now.Add(-1*time.Hour), 0, 0),
			},
			expected: map[string]bool{"stack2": true},
		},
		{
			name:    "test GC oldest stack (without ingress defined)",
			limit:   1,
			ingress: false,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), time.Time{}, 0, 0),
				stackContainer("stack2", now.Add(-2*time.Hour), time.Time{}, 0, 0),
			},
			expected: map[string]bool{"stack2": true},
		},
		{
			name:    "test don't GC stacks when all are getting traffic",
			limit:   1,
			ingress: true,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), now.Add(-1*time.Hour), 1, 0),
				stackContainer("stack2", now.Add(-2*time.Hour), now.Add(-2*time.Hour), 0, 1),
			},
			expected: nil,
		},
		{
			name:    "test don't GC stacks when there are less than limit",
			limit:   3,
			ingress: true,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), now.Add(-1*time.Hour), 0, 0),
				stackContainer("stack2", now.Add(-2*time.Hour), now.Add(-2*time.Hour), 0, 0),
			},
			expected: nil,
		},
		{
			name:    "test stacks with traffic don't count against limit",
			limit:   2,
			ingress: true,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), now.Add(-1*time.Hour), 1, 1),
				stackContainer("stack2", now.Add(-2*time.Hour), now.Add(-2*time.Hour), 0, 0),
				stackContainer("stack3", now.Add(-3*time.Hour), now.Add(-3*time.Hour), 0, 0),
			},
			expected: nil,
		},
		{
			name:                "not GC'ing a stack with no-traffic-since less than ScaledownTTLSeconds",
			limit:               1,
			ingress:             true,
			scaledownTTLSeconds: 300,
			stacks: []*StackContainer{
				stackContainer("stack1", now.Add(-1*time.Hour), now.Add(-200*time.Second), 0, 0),
				stackContainer("stack2", now.Add(-2*time.Hour), now.Add(-250*time.Second), 0, 0),
			},
			expected: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := StackSetContainer{
				StackSet: &zv1.StackSet{
					Spec: zv1.StackSetSpec{
						StackLifecycle: zv1.StackLifecycle{
							ScaledownTTLSeconds: nil,
							Limit:               nil,
						},
					},
				},
				StackContainers: map[types.UID]*StackContainer{},
			}
			c.StackSet.Spec.StackLifecycle.Limit = &tc.limit
			for _, stack := range tc.stacks {
				if tc.scaledownTTLSeconds == 0 {
					stack.scaledownTTL = defaultScaledownTTL
				} else {
					stack.scaledownTTL = time.Second * tc.scaledownTTLSeconds
				}
				if tc.ingress {
					stack.ingressSpec = &zv1.StackSetIngressSpec{}
				}
				c.StackContainers[types.UID(stack.Stack.Name)] = stack
			}

			err := c.MarkExpiredStacks()
			require.NoError(t, err)
			for _, stack := range tc.stacks {
				require.Equal(t, tc.expected[stack.Stack.Name], stack.PendingRemoval, "stack %s", stack.Stack.Name)
			}
		})
	}
}

func TestSanitizeServicePorts(t *testing.T) {
	service := &zv1.StackServiceSpec{
		Ports: []v1.ServicePort{
			{
				Protocol: "",
			},
		},
	}

	service = sanitizeServicePorts(service)
	require.Len(t, service.Ports, 1)
	require.Equal(t, v1.ProtocolTCP, service.Ports[0].Protocol)
}

func TestStackSetNewStack(t *testing.T) {
	for _, tc := range []struct {
		name              string
		stackset          *zv1.StackSet
		stacks            map[types.UID]*StackContainer
		expectedStack     *StackContainer
		expectedStackName string
	}{
		{
			name: "stack already exists",
			stackset: &zv1.StackSet{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: zv1.StackSetSpec{
					StackTemplate: zv1.StackTemplate{
						Spec: zv1.StackSpecTemplate{
							Version: "v1",
						},
					},
				},
			},
			stacks: map[types.UID]*StackContainer{
				"foo": {
					Stack: &zv1.Stack{
						ObjectMeta: metav1.ObjectMeta{Name: "foo-v1"},
					},
				},
			},
			expectedStack:     nil,
			expectedStackName: "",
		},
		{
			name: "stack already created",
			stackset: &zv1.StackSet{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: zv1.StackSetSpec{
					StackTemplate: zv1.StackTemplate{
						Spec: zv1.StackSpecTemplate{
							Version: "v1",
						},
					},
				},
				Status: zv1.StackSetStatus{
					ObservedStackVersion: "v1",
				},
			},
			stacks:            map[types.UID]*StackContainer{},
			expectedStack:     nil,
			expectedStackName: "",
		},
		{
			name: "stack needs to be created",
			stackset: &zv1.StackSet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "zalando.org/v1",
					Kind:       "StackSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					UID:       "1234-abc-2134",
					Labels:    map[string]string{"custom": "label"},
				},
				Spec: zv1.StackSetSpec{
					StackTemplate: zv1.StackTemplate{
						Spec: zv1.StackSpecTemplate{
							Version: "v1",
						},
					},
				},
			},
			stacks: map[types.UID]*StackContainer{},
			expectedStack: &StackContainer{
				Stack: &zv1.Stack{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-v1",
						Namespace: "bar",
						Labels: map[string]string{
							StacksetHeritageLabelKey: "foo",
							"custom":                 "label",
							StackVersionLabelKey:     "v1",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "zalando.org/v1",
								Kind:       "StackSet",
								Name:       "foo",
								UID:        "1234-abc-2134",
							},
						},
					},
				},
			},
			expectedStackName: "v1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stackset := &StackSetContainer{
				StackSet:        tc.stackset,
				StackContainers: tc.stacks,
			}
			newStack, newStackName := stackset.NewStack()
			require.EqualValues(t, tc.expectedStack, newStack)
			require.EqualValues(t, tc.expectedStackName, newStackName)
		})
	}
}

func dummyStacksetContainer() *StackSetContainer {
	return &StackSetContainer{
		StackSet: &zv1.StackSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
		},
		StackContainers: map[types.UID]*StackContainer{
			"foo-1": {
				Stack: &zv1.Stack{},
			},
			"foo-2": {
				Stack: &zv1.Stack{},
			},
		},
	}
}

func TestStackSetUpdateFromResources(t *testing.T) {
	minute := int64(60)

	for _, tc := range []struct {
		name                 string
		scaledownTTL         *int64
		ingress              *zv1.StackSetIngressSpec
		expectedScaledownTTL time.Duration
	}{
		{
			name:                 "no ingress, default scaledown TTL",
			expectedScaledownTTL: defaultScaledownTTL,
		},
		{
			name:                 "explicit scaledown TTL",
			scaledownTTL:         &minute,
			expectedScaledownTTL: 60 * time.Second,
		},
		{
			name: "ingress",
			ingress: &zv1.StackSetIngressSpec{
				Hosts: []string{"foo"},
			},
			expectedScaledownTTL: defaultScaledownTTL,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := dummyStacksetContainer()
			c.StackSet.Spec.StackLifecycle.ScaledownTTLSeconds = tc.scaledownTTL
			err := c.UpdateFromResources()
			require.NoError(t, err)

			for _, sc := range c.StackContainers {
				require.Equal(t, "zalando.org/v1", sc.Stack.APIVersion)
				require.Equal(t, "Stack", sc.Stack.Kind)

				require.Equal(t, c.StackSet.Name, sc.stacksetName)

				require.EqualValues(t, c.StackSet.Spec.Ingress, sc.ingressSpec)

				require.Equal(t, tc.expectedScaledownTTL, sc.scaledownTTL)
			}
		})
	}
}
