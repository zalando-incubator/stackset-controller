/*
Copyright 2020 The Kubernetes Authors.

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

// StackSetLister helps list StackSets.
// All objects returned here must be treated as read-only.
type StackSetLister interface {
	// List lists all StackSets in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.StackSet, err error)
	// StackSets returns an object that can list and get StackSets.
	StackSets(namespace string) StackSetNamespaceLister
	StackSetListerExpansion
}

// stackSetLister implements the StackSetLister interface.
type stackSetLister struct {
	indexer cache.Indexer
}

// NewStackSetLister returns a new StackSetLister.
func NewStackSetLister(indexer cache.Indexer) StackSetLister {
	return &stackSetLister{indexer: indexer}
}

// List lists all StackSets in the indexer.
func (s *stackSetLister) List(selector labels.Selector) (ret []*v1.StackSet, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.StackSet))
	})
	return ret, err
}

// StackSets returns an object that can list and get StackSets.
func (s *stackSetLister) StackSets(namespace string) StackSetNamespaceLister {
	return stackSetNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// StackSetNamespaceLister helps list and get StackSets.
// All objects returned here must be treated as read-only.
type StackSetNamespaceLister interface {
	// List lists all StackSets in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.StackSet, err error)
	// Get retrieves the StackSet from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1.StackSet, error)
	StackSetNamespaceListerExpansion
}

// stackSetNamespaceLister implements the StackSetNamespaceLister
// interface.
type stackSetNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all StackSets in the indexer for a given namespace.
func (s stackSetNamespaceLister) List(selector labels.Selector) (ret []*v1.StackSet, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.StackSet))
	})
	return ret, err
}

// Get retrieves the StackSet from the indexer for a given namespace and name.
func (s stackSetNamespaceLister) Get(name string) (*v1.StackSet, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("stackset"), name)
	}
	return obj.(*v1.StackSet), nil
}
