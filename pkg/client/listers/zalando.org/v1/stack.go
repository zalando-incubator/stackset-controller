/*
Copyright 2022 The Kubernetes Authors.

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

// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	v1 "github.com/zalando-incubator/stackset-controller/pkg/apis/zalando.org/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// StackLister helps list Stacks.
// All objects returned here must be treated as read-only.
type StackLister interface {
	// List lists all Stacks in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.Stack, err error)
	// Stacks returns an object that can list and get Stacks.
	Stacks(namespace string) StackNamespaceLister
	StackListerExpansion
}

// stackLister implements the StackLister interface.
type stackLister struct {
	indexer cache.Indexer
}

// NewStackLister returns a new StackLister.
func NewStackLister(indexer cache.Indexer) StackLister {
	return &stackLister{indexer: indexer}
}

// List lists all Stacks in the indexer.
func (s *stackLister) List(selector labels.Selector) (ret []*v1.Stack, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.Stack))
	})
	return ret, err
}

// Stacks returns an object that can list and get Stacks.
func (s *stackLister) Stacks(namespace string) StackNamespaceLister {
	return stackNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// StackNamespaceLister helps list and get Stacks.
// All objects returned here must be treated as read-only.
type StackNamespaceLister interface {
	// List lists all Stacks in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.Stack, err error)
	// Get retrieves the Stack from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1.Stack, error)
	StackNamespaceListerExpansion
}

// stackNamespaceLister implements the StackNamespaceLister
// interface.
type stackNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Stacks in the indexer for a given namespace.
func (s stackNamespaceLister) List(selector labels.Selector) (ret []*v1.Stack, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.Stack))
	})
	return ret, err
}

// Get retrieves the Stack from the indexer for a given namespace and name.
func (s stackNamespaceLister) Get(name string) (*v1.Stack, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("stack"), name)
	}
	return obj.(*v1.Stack), nil
}
