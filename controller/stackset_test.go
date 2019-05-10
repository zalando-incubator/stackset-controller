package controller

import (
	"testing"
	"time"

	"github.com/zalando-incubator/stackset-controller/controller/entities"
	stacksetfake "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/fake"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/recorder"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetStacksToGC(tt *testing.T) {
	for _, tc := range []struct {
		name              string
		stackSetContainer entities.StackSetContainer
		expectedNum       int
	}{
		{
			name: "test GC oldest stack",
			stackSetContainer: entities.StackSetContainer{
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
				StackContainers: map[types.UID]*entities.StackContainer{
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
			stackSetContainer: entities.StackSetContainer{
				StackSet: zv1.StackSet{
					Spec: zv1.StackSetSpec{
						StackLifecycle: zv1.StackLifecycle{
							Limit: int32Ptr(1),
						},
					},
				},
				StackContainers: map[types.UID]*entities.StackContainer{
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
			stackSetContainer: entities.StackSetContainer{
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
				StackContainers: map[types.UID]*entities.StackContainer{
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
				Traffic: map[string]entities.TrafficStatus{
					"stack1": entities.TrafficStatus{ActualWeight: 50},
					"stack2": entities.TrafficStatus{ActualWeight: 50},
				},
			},
			expectedNum: 0,
		},
		{
			name: "test don't GC stacks when there are less than limit",
			stackSetContainer: entities.StackSetContainer{
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
				StackContainers: map[types.UID]*entities.StackContainer{
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
				Traffic: map[string]entities.TrafficStatus{
					"stack1": entities.TrafficStatus{ActualWeight: 0},
					"stack2": entities.TrafficStatus{ActualWeight: 0},
				},
			},
			expectedNum: 0,
		},
		{
			name: "test stacks with traffic don't count against limit",
			stackSetContainer: entities.StackSetContainer{
				StackSet: zv1.StackSet{
					Spec: zv1.StackSetSpec{
						Ingress: &zv1.StackSetIngressSpec{
							Hosts: []string{"app.example.org"},
						},
						StackLifecycle: zv1.StackLifecycle{
							Limit: int32Ptr(2),
						},
					},
				},
				StackContainers: map[types.UID]*entities.StackContainer{
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
					types.UID("uid3"): {
						Stack: zv1.Stack{
							ObjectMeta: metav1.ObjectMeta{
								Name:              "stack3",
								CreationTimestamp: metav1.NewTime(time.Now().Add(-3 * time.Hour)),
							},
							Status: zv1.StackStatus{
								NoTrafficSince: &metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
							},
						},
					},
				},
				Traffic: map[string]entities.TrafficStatus{
					"stack1": entities.TrafficStatus{ActualWeight: 100},
					"stack2": entities.TrafficStatus{ActualWeight: 0},
					"stack3": entities.TrafficStatus{ActualWeight: 0},
				},
			},
			expectedNum: 0,
		},
		{
			name: "test not GC'ing a stack with no-traffic-since less than ScaledownTTLSeconds",
			stackSetContainer: entities.StackSetContainer{
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
				StackContainers: map[types.UID]*entities.StackContainer{
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
	sc := entities.StackSetContainer{
		StackContainers: map[types.UID]*entities.StackContainer{
			types.UID("uid"): {
				Stack: zv1.Stack{},
			},
		},
	}
	assert.Len(t, sc.Stacks(), 1)
}

func TestScaledownTTL(t *testing.T) {
	scaledownTTL := defaultScaledownTTLSeconds
	sc := entities.StackSetContainer{
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

func generateStackSet(stacksetName, namespace, version string, minReplicas, maxReplicas int) zv1.StackSet {
	return zv1.StackSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       stacksetName,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: zv1.StackSetSpec{
			StackTemplate: zv1.StackTemplate{
				Spec: zv1.StackSpecTemplate{
					Version: version,
					StackSpec: zv1.StackSpec{
						Autoscaler: &zv1.Autoscaler{
							MinReplicas: &[]int32{int32(minReplicas)}[0],
							MaxReplicas: int32(maxReplicas),
						},
					},
				},
			},
		},
	}
}

func generateStack(name, namespace string) zv1.Stack {
	return zv1.Stack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func TestStackSetController_ReconcileStackCreate(t *testing.T) {
	kubeClient := fake.NewSimpleClientset()
	ssClient := stacksetfake.NewSimpleClientset()
	fakeClient := clientset.NewClientset(kubeClient, ssClient)
	controller := NewStackSetController(fakeClient, "test-controller", time.Second)
	ss := generateStackSet("stack", "test", "01", 10, 20)
	ssc := entities.StackSetContainer{StackSet: ss}
	err := controller.ReconcileStack(ssc)
	assert.NoError(t, err, "error reconciling stack")
	stackName := generateStackName(ss, "01")
	stack, err := fakeClient.ZalandoV1().Stacks("test").Get(stackName, metav1.GetOptions{})
	assert.NoError(t, err, "failed to get created stack")
	assert.NotNil(t, stack, "created stack cannot be referenced")
	assert.EqualValues(t, 10, *stack.Spec.Autoscaler.MinReplicas)
	assert.EqualValues(t, 20, stack.Spec.Autoscaler.MaxReplicas)
}
