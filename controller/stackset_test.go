package controller

import (
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/recorder"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetStacksToGC(tt *testing.T) {
	for _, tc := range []struct {
		name              string
		stackSetContainer StackSetContainer
		expectedNum       int
	}{
		{
			name: "test GC oldest stack",
			stackSetContainer: StackSetContainer{
				StackSet: zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{
							Hosts: []string{"app.example.org"},
						},
						StackLifecycle: zv1.StackLifecycle{
							Limit: int32Ptr(1),
						},
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					types.UID("uid"): {
						Stack: zv1.Stack{
							ObjectMeta: metav1.ObjectMeta{
								Name:              "stack1",
								CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
							},
							Status: zv1.StackStatus{
								NoTrafficSince: &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
							},
						},
					},
					types.UID("uid2"): {
						Stack: zv1.Stack{
							ObjectMeta: metav1.ObjectMeta{
								Name:              "stack2",
								CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
							},
							Status: zv1.StackStatus{
								NoTrafficSince: &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
							},
						},
					},
				},
			},
			expectedNum: 1,
		},
		{
			name: "test GC oldest stack (without ingress defined)",
			stackSetContainer: StackSetContainer{
				StackSet: zv1.StackSet{
					Spec: zv1.StackSetSpec{
						StackLifecycle: zv1.StackLifecycle{
							Limit: int32Ptr(1),
						},
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					types.UID("uid"): {
						Stack: zv1.Stack{
							ObjectMeta: metav1.ObjectMeta{
								Name:              "stack1",
								CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
							},
						},
					},
					types.UID("uid2"): {
						Stack: zv1.Stack{
							ObjectMeta: metav1.ObjectMeta{
								Name:              "stack2",
								CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
							},
						},
					},
				},
			},
			expectedNum: 1,
		},
		{
			name: "test don't GC stacks when all are getting traffic",
			stackSetContainer: StackSetContainer{
				StackSet: zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{
							Hosts: []string{"app.example.org"},
						},
						StackLifecycle: zv1.StackLifecycle{
							Limit: int32Ptr(1),
						},
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					types.UID("uid"): {
						Stack: zv1.Stack{
							ObjectMeta: metav1.ObjectMeta{
								Name:              "stack1",
								CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
							},
							Status: zv1.StackStatus{
								NoTrafficSince: &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
							},
						},
					},
					types.UID("uid2"): {
						Stack: zv1.Stack{
							ObjectMeta: metav1.ObjectMeta{
								Name:              "stack2",
								CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
							},
							Status: zv1.StackStatus{
								NoTrafficSince: &metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
							},
						},
					},
				},
				Traffic: map[string]TrafficStatus{
					"stack1": TrafficStatus{ActualWeight: 50},
					"stack2": TrafficStatus{ActualWeight: 50},
				},
			},
			expectedNum: 0,
		},
		{
			name: "test don't GC stacks when there are less than limit",
			stackSetContainer: StackSetContainer{
				StackSet: zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{
							Hosts: []string{"app.example.org"},
						},
						StackLifecycle: zv1.StackLifecycle{
							Limit: int32Ptr(3),
						},
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					types.UID("uid"): {
						Stack: zv1.Stack{
							ObjectMeta: metav1.ObjectMeta{
								Name:              "stack1",
								CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
							},
							Status: zv1.StackStatus{
								NoTrafficSince: &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
							},
						},
					},
					types.UID("uid2"): {
						Stack: zv1.Stack{
							ObjectMeta: metav1.ObjectMeta{
								Name:              "stack2",
								CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
							},
							Status: zv1.StackStatus{
								NoTrafficSince: &metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
							},
						},
					},
				},
				Traffic: map[string]TrafficStatus{
					"stack1": TrafficStatus{ActualWeight: 0},
					"stack2": TrafficStatus{ActualWeight: 0},
				},
			},
			expectedNum: 0,
		},
		{
			name: "test not GC'ing a stack with no-traffic-since less than ScaledownTTLSeconds",
			stackSetContainer: StackSetContainer{
				StackSet: zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{
							Hosts: []string{"app.example.org"},
						},
						StackLifecycle: zv1.StackLifecycle{
							Limit:               int32Ptr(1),
							ScaledownTTLSeconds: int64Ptr(300),
						},
					},
				},
				StackContainers: map[types.UID]*StackContainer{
					types.UID("uid"): {
						Stack: zv1.Stack{
							ObjectMeta: metav1.ObjectMeta{
								Name:              "stack1",
								CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
							},
							Status: zv1.StackStatus{
								NoTrafficSince: &metav1.Time{Time: time.Now().Add(-200 * time.Second)},
							},
						},
					},
					types.UID("uid2"): {
						Stack: zv1.Stack{
							ObjectMeta: metav1.ObjectMeta{
								Name:              "stack2",
								CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
							},
							Status: zv1.StackStatus{
								NoTrafficSince: &metav1.Time{Time: time.Now().Add(-250 * time.Second)},
							},
						},
					},
				},
			},
			expectedNum: 0,
		},
	} {
		tt.Run(tc.name, func(t *testing.T) {
			c := &StackSetController{
				logger: log.WithFields(
					log.Fields{
						"test": "yes",
					},
				),
				recorder: recorder.CreateEventRecorder(fake.NewSimpleClientset()),
			}

			stacks := c.getStacksToGC(tc.stackSetContainer)
			assert.Len(t, stacks, tc.expectedNum)
		})
	}
}

func TestSetStackSetDefaults(t *testing.T) {
	stackset := zv1.StackSet{
		Spec: zv1.StackSetSpec{
			Ingress: &zv1.StackSetIngressSpec{
				BackendPort: intstr.FromInt(0),
			},
		},
	}

	setStackSetDefaults(&stackset)
	assert.NotNil(t, stackset.Spec.StackLifecycle.ScaledownTTLSeconds)
	assert.Equal(t, defaultScaledownTTLSeconds, *stackset.Spec.StackLifecycle.ScaledownTTLSeconds)
}

func TestStacks(t *testing.T) {
	sc := StackSetContainer{
		StackContainers: map[types.UID]*StackContainer{
			types.UID("uid"): {
				Stack: zv1.Stack{},
			},
		},
	}
	assert.Len(t, sc.Stacks(), 1)
}

func TestScaledownTTL(t *testing.T) {
	scaledownTTL := defaultScaledownTTLSeconds
	sc := StackSetContainer{
		StackSet: zv1.StackSet{
			Spec: zv1.StackSetSpec{
				StackLifecycle: zv1.StackLifecycle{
					ScaledownTTLSeconds: &scaledownTTL,
				},
			},
		},
	}

	assert.Equal(t, time.Duration(defaultScaledownTTLSeconds)*time.Second, sc.ScaledownTTL())

	sc.StackSet.Spec.StackLifecycle.ScaledownTTLSeconds = nil
	assert.Equal(t, time.Duration(0), sc.ScaledownTTL())
}

func TestGetOwnerUID(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		OwnerReferences: []metav1.OwnerReference{
			{
				UID: types.UID("x"),
			},
		},
	}

	uid, ok := getOwnerUID(objectMeta)
	assert.Equal(t, types.UID("x"), uid)
	assert.True(t, ok)

	uid, ok = getOwnerUID(metav1.ObjectMeta{})
	assert.Equal(t, types.UID(""), uid)
	assert.False(t, ok)
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
	assert.Len(t, service.Ports, 1)
	assert.Equal(t, v1.ProtocolTCP, service.Ports[0].Protocol)
}

func TestMergeLabels(t *testing.T) {
	labels1 := map[string]string{
		"foo": "bar",
	}

	labels2 := map[string]string{
		"foo": "baz",
	}

	merged := mergeLabels(labels1, labels2)
	assert.Equal(t, labels2, merged)

	labels3 := map[string]string{
		"bar": "foo",
	}

	merged = mergeLabels(labels1, labels3)
	assert.Equal(t, map[string]string{"foo": "bar", "bar": "foo"}, merged)
}

func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}
