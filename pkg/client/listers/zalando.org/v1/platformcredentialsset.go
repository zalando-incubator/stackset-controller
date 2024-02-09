/*
Copyright 2024 The Kubernetes Authors.

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

// PlatformCredentialsSetLister helps list PlatformCredentialsSets.
// All objects returned here must be treated as read-only.
type PlatformCredentialsSetLister interface {
	// List lists all PlatformCredentialsSets in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.PlatformCredentialsSet, err error)
	// PlatformCredentialsSets returns an object that can list and get PlatformCredentialsSets.
	PlatformCredentialsSets(namespace string) PlatformCredentialsSetNamespaceLister
	PlatformCredentialsSetListerExpansion
}

// platformCredentialsSetLister implements the PlatformCredentialsSetLister interface.
type platformCredentialsSetLister struct {
	indexer cache.Indexer
}

// NewPlatformCredentialsSetLister returns a new PlatformCredentialsSetLister.
func NewPlatformCredentialsSetLister(indexer cache.Indexer) PlatformCredentialsSetLister {
	return &platformCredentialsSetLister{indexer: indexer}
}

// List lists all PlatformCredentialsSets in the indexer.
func (s *platformCredentialsSetLister) List(selector labels.Selector) (ret []*v1.PlatformCredentialsSet, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.PlatformCredentialsSet))
	})
	return ret, err
}

// PlatformCredentialsSets returns an object that can list and get PlatformCredentialsSets.
func (s *platformCredentialsSetLister) PlatformCredentialsSets(namespace string) PlatformCredentialsSetNamespaceLister {
	return platformCredentialsSetNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// PlatformCredentialsSetNamespaceLister helps list and get PlatformCredentialsSets.
// All objects returned here must be treated as read-only.
type PlatformCredentialsSetNamespaceLister interface {
	// List lists all PlatformCredentialsSets in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.PlatformCredentialsSet, err error)
	// Get retrieves the PlatformCredentialsSet from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1.PlatformCredentialsSet, error)
	PlatformCredentialsSetNamespaceListerExpansion
}

// platformCredentialsSetNamespaceLister implements the PlatformCredentialsSetNamespaceLister
// interface.
type platformCredentialsSetNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all PlatformCredentialsSets in the indexer for a given namespace.
func (s platformCredentialsSetNamespaceLister) List(selector labels.Selector) (ret []*v1.PlatformCredentialsSet, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.PlatformCredentialsSet))
	})
	return ret, err
}

// Get retrieves the PlatformCredentialsSet from the indexer for a given namespace and name.
func (s platformCredentialsSetNamespaceLister) Get(name string) (*v1.PlatformCredentialsSet, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("platformcredentialsset"), name)
	}
	return obj.(*v1.PlatformCredentialsSet), nil
}
