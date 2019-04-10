package canary

import (
	"fmt"

	"github.com/sirupsen/logrus"
	zv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
)

type Env struct {
	Client KubeClient
	Logger logrus.FieldLogger
}

func (e Env) WithLogger(logger logrus.FieldLogger) Env {
	return Env{
		Client: e.Client,
		Logger: logger,
	}
}

type KubeClient interface {
	CreateStack(set *zv1.StackSet, name string, version string, spec zv1.StackSpec) (*zv1.Stack, error)
	UpdateStack(set *zv1.StackSet, stack *zv1.Stack, reasonFmt string, reasonArgs ...interface{}) (*zv1.Stack, error)
	RemoveStack(set *zv1.StackSet, name, reasonFmt string, reasonArgs ...interface{}) error
	// MarkStackNotGreen always succeeds. Even if the underlying k8s call fails, there is no point
	// in returning an error since we are not doing anything with non-green stacks.
	MarkStackNotGreen(set *zv1.StackSet, stack *zv1.Stack, reason string)
}

func (e Env) SetAnnotation(
	set *zv1.StackSet,
	stack *zv1.Stack,
	key, value string,
) (*zv1.Stack, error) {
	stack.Annotations[key] = value
	msg := fmt.Sprintf("set annotation %q to %q", key, value)
	e.Logger.Debugf("stack %s/%s: %s", stack.Namespace, stack.Name, msg)
	return e.Client.UpdateStack(set, stack, msg)
}
