package controller

import (
	"github.com/stretchr/testify/assert"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"

	fakeController "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/fake"
	ScController "github.com/zalando-incubator/stackset-controller/pkg/clientset"
	fakeK8s "k8s.io/client-go/kubernetes/fake"
)

func getFakeController() *StackSetController {
	fStacksetClient := fakeController.NewSimpleClientset()
	fk8sClient := fakeK8s.NewSimpleClientset()
	fSSClientSet := ScController.NewClientset(fk8sClient, fStacksetClient)

	return NewStackSetController(fSSClientSet, "test-controller", 0)
}

// Test an Ingress is created if specified in the StackSet
func TestIngressCreation(t *testing.T) {
	controller := getFakeController()
	sc := StackSetContainer{
		StackContainers: map[types.UID]*StackContainer{
			"test": {},
		},
		StackSet: zv1.StackSet{
			Spec: zv1.StackSetSpec{
				Ingress: &zv1.StackSetIngressSpec{},
			},
		},
	}

	err := controller.ReconcileIngress(sc)
	assert.NoError(t, err)
	ingressList, err := controller.client.ExtensionsV1beta1().Ingresses("").List(v1.ListOptions{})
	assert.NoError(t, err)
	ingressLength := len(ingressList.Items)
	assert.Equal(t, 1, ingressLength)
}

// Test Ingress gets deleted if not defined in StackSet Spec
func TestIngressDeletion(t *testing.T) {
	controller := getFakeController()
	stackIngress, err := controller.client.ExtensionsV1beta1().Ingresses("").Create(&v1beta1.Ingress{})
	assert.NoError(t, err)

	ingressList, err := controller.client.ExtensionsV1beta1().Ingresses("").List(v1.ListOptions{})
	ingressCount := len(ingressList.Items)
	assert.Equal(t, 1, ingressCount)

	sc := StackSetContainer{
		Ingress: stackIngress,
	}

	err = controller.ReconcileIngress(sc)
	ingressList, err = controller.client.ExtensionsV1beta1().Ingresses("").List(v1.ListOptions{})
	assert.NoError(t, err)

	ingressCount = len(ingressList.Items)
	assert.Equal(t, 0, ingressCount)

}

// Test that if an Ingress Spec is updated, the Ingress will also be updated
func TestIngressUpdate(t *testing.T) {
	controller := getFakeController()
	stackIngress, err := controller.client.ExtensionsV1beta1().Ingresses("default").Create(&v1beta1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name: "example",
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					Host: "not-example.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Backend: v1beta1.IngressBackend{
										ServicePort: intstr.FromInt(8080),
									},
								},
							},
						},
					},
				},
			},
		},
	})

	assert.Equal(t, "not-example.com", stackIngress.Spec.Rules[0].Host)
	assert.Equal(t, 8080, stackIngress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.ServicePort.IntValue())

	sc := StackSetContainer{
		StackContainers: map[types.UID]*StackContainer{
			"test": {},
		},
		StackSet: zv1.StackSet{
			ObjectMeta: v1.ObjectMeta{
				Name: "example",
				Namespace: "default",
			},
			Spec: zv1.StackSetSpec{
				Ingress: &zv1.StackSetIngressSpec{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "default",
					},
					BackendPort: intstr.FromInt(80),
					Hosts: []string{"example.com"},
					Path: "example",
				},
			},
		},
		Ingress: stackIngress,
	}

	err = controller.ReconcileIngress(sc)
	assert.NoError(t, err)

	ingressList, err := controller.client.ExtensionsV1beta1().Ingresses("default").List(v1.ListOptions{})
	assert.NoError(t, err)
	ingressCount := len(ingressList.Items)
	assert.Equal(t, 1, ingressCount)
	assert.Equal(t, "example.com", ingressList.Items[0].Spec.Rules[0].Host)
	assert.Equal(t, 80, ingressList.Items[0].Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.ServicePort.IntValue())
}
