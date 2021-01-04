/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	zalandoorgv1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeStacks implements StackInterface
type FakeStacks struct {
	Fake *FakeZalandoV1
	ns   string
}

var stacksResource = schema.GroupVersionResource{Group: "zalando.org", Version: "v1", Resource: "stacks"}

var stacksKind = schema.GroupVersionKind{Group: "zalando.org", Version: "v1", Kind: "Stack"}

// Get takes name of the stack, and returns the corresponding stack object, and an error if there is any.
func (c *FakeStacks) Get(ctx context.Context, name string, options v1.GetOptions) (result *zalandoorgv1.Stack, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(stacksResource, c.ns, name), &zalandoorgv1.Stack{})

	if obj == nil {
		return nil, err
	}
	return obj.(*zalandoorgv1.Stack), err
}

// List takes label and field selectors, and returns the list of Stacks that match those selectors.
func (c *FakeStacks) List(ctx context.Context, opts v1.ListOptions) (result *zalandoorgv1.StackList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(stacksResource, stacksKind, c.ns, opts), &zalandoorgv1.StackList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &zalandoorgv1.StackList{ListMeta: obj.(*zalandoorgv1.StackList).ListMeta}
	for _, item := range obj.(*zalandoorgv1.StackList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested stacks.
func (c *FakeStacks) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(stacksResource, c.ns, opts))

}

// Create takes the representation of a stack and creates it.  Returns the server's representation of the stack, and an error, if there is any.
func (c *FakeStacks) Create(ctx context.Context, stack *zalandoorgv1.Stack, opts v1.CreateOptions) (result *zalandoorgv1.Stack, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(stacksResource, c.ns, stack), &zalandoorgv1.Stack{})

	if obj == nil {
		return nil, err
	}
	return obj.(*zalandoorgv1.Stack), err
}

// Update takes the representation of a stack and updates it. Returns the server's representation of the stack, and an error, if there is any.
func (c *FakeStacks) Update(ctx context.Context, stack *zalandoorgv1.Stack, opts v1.UpdateOptions) (result *zalandoorgv1.Stack, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(stacksResource, c.ns, stack), &zalandoorgv1.Stack{})

	if obj == nil {
		return nil, err
	}
	return obj.(*zalandoorgv1.Stack), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeStacks) UpdateStatus(ctx context.Context, stack *zalandoorgv1.Stack, opts v1.UpdateOptions) (*zalandoorgv1.Stack, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(stacksResource, "status", c.ns, stack), &zalandoorgv1.Stack{})

	if obj == nil {
		return nil, err
	}
	return obj.(*zalandoorgv1.Stack), err
}

// Delete takes name of the stack and deletes it. Returns an error if one occurs.
func (c *FakeStacks) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(stacksResource, c.ns, name), &zalandoorgv1.Stack{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeStacks) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(stacksResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &zalandoorgv1.StackList{})
	return err
}

// Patch applies the patch and returns the patched stack.
func (c *FakeStacks) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *zalandoorgv1.Stack, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(stacksResource, c.ns, name, pt, data, subresources...), &zalandoorgv1.Stack{})

	if obj == nil {
		return nil, err
	}
	return obj.(*zalandoorgv1.Stack), err
}
