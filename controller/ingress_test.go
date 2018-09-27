package controller

import (
	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando/v1"
	fakeController "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/fake"
	scController "github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	fakeK8s "k8s.io/client-go/kubernetes/fake"
	"testing"
)

func getFakeController() *StackSetController {
	fStacksetClient := fakeController.NewSimpleClientset()
	fk8sClient := fakeK8s.NewSimpleClientset()
	fSSClientSet := scController.NewClientset(fk8sClient, fStacksetClient)

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
	require.NoError(t, err)
	ingressList, err := controller.client.ExtensionsV1beta1().Ingresses("").List(v1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, ingressList.Items, 1)
}

// Test Ingress gets deleted if not defined in StackSet Spec
func TestIngressDeletion(t *testing.T) {
	controller := getFakeController()
	stackIngress, err := controller.client.ExtensionsV1beta1().Ingresses("").Create(&v1beta1.Ingress{})
	require.NoError(t, err)

	ingressList, err := controller.client.ExtensionsV1beta1().Ingresses("").List(v1.ListOptions{})
	ingressCount := len(ingressList.Items)
	require.Equal(t, 1, ingressCount)

	sc := StackSetContainer{
		Ingress: stackIngress,
	}

	err = controller.ReconcileIngress(sc)
	ingressList, err = controller.client.ExtensionsV1beta1().Ingresses("").List(v1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, ingressList.Items, 0)

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

	require.Equal(t, "not-example.com", stackIngress.Spec.Rules[0].Host)
	require.Equal(t, 8080, stackIngress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.ServicePort.IntValue())

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
	require.NoError(t, err)

	ingressList, err := controller.client.ExtensionsV1beta1().Ingresses("default").List(v1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, ingressList.Items, 1)
	require.Equal(t, "example.com", ingressList.Items[0].Spec.Rules[0].Host)
	require.Equal(t, 80, ingressList.Items[0].Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.ServicePort.IntValue())
}
