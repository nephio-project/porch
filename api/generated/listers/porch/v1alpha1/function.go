// Copyright 2024 The kpt and Nephio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/nephio-project/porch/api/porch/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// FunctionLister helps list Functions.
// All objects returned here must be treated as read-only.
type FunctionLister interface {
	// List lists all Functions in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.Function, err error)
	// Functions returns an object that can list and get Functions.
	Functions(namespace string) FunctionNamespaceLister
	FunctionListerExpansion
}

// functionLister implements the FunctionLister interface.
type functionLister struct {
	indexer cache.Indexer
}

// NewFunctionLister returns a new FunctionLister.
func NewFunctionLister(indexer cache.Indexer) FunctionLister {
	return &functionLister{indexer: indexer}
}

// List lists all Functions in the indexer.
func (s *functionLister) List(selector labels.Selector) (ret []*v1alpha1.Function, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Function))
	})
	return ret, err
}

// Functions returns an object that can list and get Functions.
func (s *functionLister) Functions(namespace string) FunctionNamespaceLister {
	return functionNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// FunctionNamespaceLister helps list and get Functions.
// All objects returned here must be treated as read-only.
type FunctionNamespaceLister interface {
	// List lists all Functions in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.Function, err error)
	// Get retrieves the Function from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.Function, error)
	FunctionNamespaceListerExpansion
}

// functionNamespaceLister implements the FunctionNamespaceLister
// interface.
type functionNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Functions in the indexer for a given namespace.
func (s functionNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.Function, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Function))
	})
	return ret, err
}

// Get retrieves the Function from the indexer for a given namespace and name.
func (s functionNamespaceLister) Get(name string) (*v1alpha1.Function, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("function"), name)
	}
	return obj.(*v1alpha1.Function), nil
}
