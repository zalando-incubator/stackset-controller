package controller

import (
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando/v1"
	fakeController "github.com/zalando-incubator/stackset-controller/pkg/client/clientset/versioned/fake"
	scController "github.com/zalando-incubator/stackset-controller/pkg/clientset"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

var ingressTests = []struct {
	msg string
	in  StackSetContainer
	out int
}{
	{
		msg: "Test an Ingress is created if specified in the StackSet",
		in: StackSetContainer{
			StackContainers: map[types.UID]*StackContainer{
				"test": {},
			},
			StackSet: zv1.StackSet{
				ObjectMeta: v1.ObjectMeta{
					Name:      "example",
					Namespace: "default",
				},
				Spec: zv1.StackSetSpec{
					Ingress: &zv1.StackSetIngressSpec{
						ObjectMeta: v1.ObjectMeta{
							Namespace: "default",
						},
						BackendPort: intstr.FromInt(80),
						Hosts:       []string{"example.com"},
						Path:        "example",
					},
				},
			},
		},
		out: 1,
	},
	{ // TODO: Test that per stack ingresses were also cleaned up
		msg: "Test Ingress gets deleted if not defined in StackSet Spec",
		in: StackSetContainer{
			Ingress: &v1beta1.Ingress{},
		},
		out: 0,
	},
	{
		msg: "Test that if an Ingress Spec is updated, the Ingress will also be updated",
		in: StackSetContainer{
			StackContainers: map[types.UID]*StackContainer{
				"test": {},
			},
			StackSet: zv1.StackSet{
				ObjectMeta: v1.ObjectMeta{
					Name:      "example",
					Namespace: "default",
				},
				Spec: zv1.StackSetSpec{
					Ingress: &zv1.StackSetIngressSpec{
						ObjectMeta: v1.ObjectMeta{
							Namespace: "default",
						},
						BackendPort: intstr.FromInt(80),
						Hosts:       []string{"example.com"},
						Path:        "example",
					},
				},
			},
			Ingress: &v1beta1.Ingress{
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
			},
		},
		out: 1,
	},
}

func TestReconcileIngress(t *testing.T) {
	for _, tc := range ingressTests {
		t.Run(tc.msg, func(t *testing.T) {
			controller := getFakeController()
			// If the StackSetContainer has an ingress create & update it as setup
			if tc.in.Ingress != nil {
				stackIngress, err := controller.client.ExtensionsV1beta1().Ingresses(tc.in.StackSet.Namespace).Create(tc.in.Ingress)
				require.NoError(t, err)
				tc.in.Ingress = stackIngress
			}

			err := controller.ReconcileIngress(tc.in)

			require.NoError(t, err)
			ingressList, err := controller.client.ExtensionsV1beta1().Ingresses(tc.in.StackSet.Namespace).List(v1.ListOptions{})
			require.NoError(t, err)
			require.Len(t, ingressList.Items, tc.out)
			// Only run these tests if we wanted an ingress to be created and it was
			if tc.out > 0 && len(ingressList.Items) > 0 {
				require.Equal(t, tc.in.StackSet.Spec.Ingress.Hosts[0], ingressList.Items[0].Spec.Rules[0].Host)
				require.Equal(t, tc.in.StackSet.Spec.Ingress.BackendPort, ingressList.Items[0].Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.ServicePort)
			}
		})
	}
}

func (c *StackSetController) NewIngressReconiler(sc StackSetContainer) *ingressReconciler {
	return &ingressReconciler{
		logger: c.logger.WithFields(
			log.Fields{
				"controller": "ingress",
				"stackset":   sc.StackSet.Name,
				"namespace":  sc.StackSet.Namespace,
			},
		),
		client:   c.client,
		recorder: c.recorder,
	}
}


func TestGcStackIngress(t *testing.T) {
	var gcTests = []struct {
		msg string
		in  StackSetContainer
		createIngress bool
	}{
		{
			msg: "If the ingress doesn't exist, nothing happens",
			in:  StackSetContainer{
				StackContainers: map[types.UID]*StackContainer{
					"test": {},
				},
			},
		},
		{
			msg: "If the ingress is owned by another resource it doesn't get cleaned up",
			in: StackSetContainer{
				StackContainers: map[types.UID]*StackContainer{
					"test": {
						Stack: zv1.Stack{
							TypeMeta: v1.TypeMeta{
								APIVersion: "v1",
								Kind: "test",
							},
							ObjectMeta: v1.ObjectMeta{
								Name: "example",
								UID: types.UID("1234"),
							},
						},
					},
				},

			},
			createIngress: true,

		},
	}
	for _, tc := range gcTests {
		controller := getFakeController()
		stack := tc.in.Stacks()[0]
		if tc.createIngress {
			_, err := controller.client.ExtensionsV1beta1().Ingresses(stack.Namespace).Create(&v1beta1.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Name: stack.Name,
				},
			})
			require.NoError(t, err)
		}

		t.Run(tc.msg, func(t *testing.T) {
			ingressReconciler := controller.NewIngressReconiler(StackSetContainer{})

			err := ingressReconciler.gcStackIngress(stack)

			require.NoError(t, err)
			if tc.createIngress {
				_, err := controller.client.ExtensionsV1beta1().Ingresses(stack.Namespace).Get(stack.Name, metav1.GetOptions{})
				require.NoError(t, err)
			}

		})
	}
}
