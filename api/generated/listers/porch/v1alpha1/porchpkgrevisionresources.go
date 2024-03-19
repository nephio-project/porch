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

// PorchPkgRevisionResourcesLister helps list PorchPkgRevisionResourceses.
// All objects returned here must be treated as read-only.
type PorchPkgRevisionResourcesLister interface {
	// List lists all PorchPkgRevisionResourceses in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.PorchPkgRevisionResources, err error)
	// PorchPkgRevisionResourceses returns an object that can list and get PorchPkgRevisionResourceses.
	PorchPkgRevisionResourceses(namespace string) PorchPkgRevisionResourcesNamespaceLister
	PorchPkgRevisionResourcesListerExpansion
}

// porchPkgRevisionResourcesLister implements the PorchPkgRevisionResourcesLister interface.
type porchPkgRevisionResourcesLister struct {
	indexer cache.Indexer
}

// NewPorchPkgRevisionResourcesLister returns a new PorchPkgRevisionResourcesLister.
func NewPorchPkgRevisionResourcesLister(indexer cache.Indexer) PorchPkgRevisionResourcesLister {
	return &porchPkgRevisionResourcesLister{indexer: indexer}
}

// List lists all PorchPkgRevisionResourceses in the indexer.
func (s *porchPkgRevisionResourcesLister) List(selector labels.Selector) (ret []*v1alpha1.PorchPkgRevisionResources, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.PorchPkgRevisionResources))
	})
	return ret, err
}

// PorchPkgRevisionResourceses returns an object that can list and get PorchPkgRevisionResourceses.
func (s *porchPkgRevisionResourcesLister) PorchPkgRevisionResourceses(namespace string) PorchPkgRevisionResourcesNamespaceLister {
	return porchPkgRevisionResourcesNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// PorchPkgRevisionResourcesNamespaceLister helps list and get PorchPkgRevisionResourceses.
// All objects returned here must be treated as read-only.
type PorchPkgRevisionResourcesNamespaceLister interface {
	// List lists all PorchPkgRevisionResourceses in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.PorchPkgRevisionResources, err error)
	// Get retrieves the PorchPkgRevisionResources from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.PorchPkgRevisionResources, error)
	PorchPkgRevisionResourcesNamespaceListerExpansion
}

// porchPkgRevisionResourcesNamespaceLister implements the PorchPkgRevisionResourcesNamespaceLister
// interface.
type porchPkgRevisionResourcesNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all PorchPkgRevisionResourceses in the indexer for a given namespace.
func (s porchPkgRevisionResourcesNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.PorchPkgRevisionResources, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.PorchPkgRevisionResources))
	})
	return ret, err
}

// Get retrieves the PorchPkgRevisionResources from the indexer for a given namespace and name.
func (s porchPkgRevisionResourcesNamespaceLister) Get(name string) (*v1alpha1.PorchPkgRevisionResources, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("porchpkgrevisionresources"), name)
	}
	return obj.(*v1alpha1.PorchPkgRevisionResources), nil
}
