package canary

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/zalando-incubator/stackset-controller/controller/entities"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/stackset-controller/pkg/clientset"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

type RealKubeClient struct {
	client   clientset.Interface
	recorder record.EventRecorder
	log      logrus.FieldLogger
}

func NewRealKubeClient(
	client clientset.Interface,
	rec record.EventRecorder,
	logger logrus.FieldLogger,
) *RealKubeClient {
	return &RealKubeClient{
		client:   client,
		recorder: rec,
		log:      logger,
	}
}

func (c *RealKubeClient) CreateStack(
	set *zv1.StackSet,
	name string,
	version string,
	spec zv1.StackSpec,
) (*zv1.Stack, error) {
	owner := v1.OwnerReference{
		APIVersion: set.APIVersion,
		Kind:       set.Kind,
		Name:       set.Name,
		UID:        set.UID,
	}
	stack := entities.NewStack(name, set.Namespace, version, owner, spec)
	for k, v := range set.Labels {
		stack.Labels[k] = v
	}
	c.recorder.Eventf(
		set,
		apiv1.EventTypeNormal,
		"CreateStackSetStack",
		"Creating Stack '%s/%s' for StackSet",
		stack.Namespace, stack.Name,
	)
	return c.client.ZalandoV1().Stacks(stack.Namespace).Create(stack)
}

func (c *RealKubeClient) UpdateStack(
	set *zv1.StackSet,
	stack *zv1.Stack,
	reasonFmt string,
	reasonArgs ...interface{},
) (*zv1.Stack, error) {
	c.recorder.Eventf(
		set,
		apiv1.EventTypeNormal,
		"UpdateStackSetStack",
		"Updating Stack '%s/%s' for StackSet: %s",
		stack.Namespace, stack.Name, fmt.Sprintf(reasonFmt, reasonArgs...),
	)
	return c.client.ZalandoV1().Stacks(stack.Namespace).Update(stack)
}

func (c *RealKubeClient) RemoveStack(set *zv1.StackSet, name, reasonFmt string, reasonArgs ...interface{}) error {
	c.recorder.Eventf(
		set,
		apiv1.EventTypeNormal,
		"RemoveStackSetStack",
		"Deleting Stack '%s/%s' for StackSet: %s",
		set.Namespace, name, fmt.Sprintf(reasonFmt, reasonArgs...),
	)
	return c.client.ZalandoV1().Stacks(set.Namespace).Delete(name, v1.NewDeleteOptions(0))
}

func (c *RealKubeClient) MarkStackNotGreen(set *zv1.StackSet, stack *zv1.Stack, reason string) {
	// TODO: Add an annotation with the reason of unmarking.

	stack, err := c.client.ZalandoV1().Stacks(set.Namespace).Get(stack.Name, v1.GetOptions{})
	if err != nil {
		c.log.Errorf("failed to fetch the latest stack %s: %s", stack.Name, err)
	}

	delete(stack.Annotations, greenAnnotationKey)

	newStack, err := c.UpdateStack(set, stack, "MarkStackNotGreen: %s", reason)
	if err == nil {
		*stack = *newStack
	} else {
		c.log.Errorf("failed to remove is-green annotation: %s", err)
	}
}
