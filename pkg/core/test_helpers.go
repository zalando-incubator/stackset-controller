package core

import (
	"time"

	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	testDefaultCreationTime       = time.Now().Add(-time.Hour)
	testPort                int32 = 8080
	intStrTestPort                = intstr.FromInt(int(testPort))
)

type testStackFactory struct {
	container *StackContainer
}

func testStack(name string) *testStackFactory {
	return &testStackFactory{
		container: &StackContainer{
			backendPort: &intStrTestPort,
			Stack: &zv1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name:              name,
					CreationTimestamp: metav1.Time{Time: testDefaultCreationTime},
				},
			},
			Resources: StackResources{
				Service: &v1.Service{
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{
							{
								Port: int32(testPort),
							},
						},
					},
				},
			},
		},
	}
}

func (f *testStackFactory) ready(replicas int32) *testStackFactory {
	return f.partiallyReady(replicas, replicas)
}

func (f *testStackFactory) partiallyReady(readyReplicas, replicas int32) *testStackFactory {
	f.container.resourcesUpdated = true
	f.container.deploymentReplicas = replicas
	f.container.updatedReplicas = readyReplicas
	f.container.readyReplicas = readyReplicas
	return f
}

func (f *testStackFactory) deployment(resourcesUpdated bool, deploymentReplicas, updatedReplicas, readyReplicas int32) *testStackFactory {
	f.container.resourcesUpdated = resourcesUpdated
	f.container.deploymentReplicas = deploymentReplicas
	f.container.updatedReplicas = updatedReplicas
	f.container.readyReplicas = readyReplicas
	return f
}

func (f *testStackFactory) traffic(desiredTrafficWeight, actualTrafficWeight float64) *testStackFactory {
	f.container.desiredTrafficWeight = desiredTrafficWeight
	f.container.actualTrafficWeight = actualTrafficWeight
	f.container.currentActualTrafficWeight = actualTrafficWeight
	return f
}

func (f *testStackFactory) currentActualTrafficWeight(weight float64) *testStackFactory {
	f.container.currentActualTrafficWeight = weight
	return f
}

func (f *testStackFactory) maxReplicas(replicas int32) *testStackFactory {
	f.container.Stack.Spec.StackSpec.HorizontalPodAutoscaler = &zv1.HorizontalPodAutoscaler{
		MaxReplicas: replicas,
	}
	return f
}

func (f *testStackFactory) createdAt(creationTime time.Time) *testStackFactory {
	f.container.Stack.CreationTimestamp = metav1.Time{Time: creationTime}
	return f
}

func (f *testStackFactory) noTrafficSince(since time.Time) *testStackFactory {
	f.container.noTrafficSince = since
	return f
}

func (f *testStackFactory) pendingRemoval() *testStackFactory {
	f.container.PendingRemoval = true
	return f
}

func (f *testStackFactory) prescaling(replicas int32, desiredTrafficWeight float64, lastTrafficIncrease time.Time) *testStackFactory {
	f.container.prescalingActive = true
	f.container.prescalingReplicas = replicas
	f.container.prescalingDesiredTrafficWeight = desiredTrafficWeight
	f.container.prescalingLastTrafficIncrease = lastTrafficIncrease
	return f
}

func (f *testStackFactory) stack() *StackContainer {
	return f.container
}
