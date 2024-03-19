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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/nephio-project/porch/api/porch/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakePorchPkgRevisions implements PorchPkgRevisionInterface
type FakePorchPkgRevisions struct {
	Fake *FakePorchV1alpha1
	ns   string
}

var porchpkgrevisionsResource = schema.GroupVersionResource{Group: "porch.kpt.dev", Version: "v1alpha1", Resource: "porchpkgrevisions"}

var porchpkgrevisionsKind = schema.GroupVersionKind{Group: "porch.kpt.dev", Version: "v1alpha1", Kind: "PorchPkgRevision"}

// Get takes name of the porchPkgRevision, and returns the corresponding porchPkgRevision object, and an error if there is any.
func (c *FakePorchPkgRevisions) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.PorchPkgRevision, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(porchpkgrevisionsResource, c.ns, name), &v1alpha1.PorchPkgRevision{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PorchPkgRevision), err
}

// List takes label and field selectors, and returns the list of PorchPkgRevisions that match those selectors.
func (c *FakePorchPkgRevisions) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.PorchPkgRevisionList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(porchpkgrevisionsResource, porchpkgrevisionsKind, c.ns, opts), &v1alpha1.PorchPkgRevisionList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.PorchPkgRevisionList{ListMeta: obj.(*v1alpha1.PorchPkgRevisionList).ListMeta}
	for _, item := range obj.(*v1alpha1.PorchPkgRevisionList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested porchPkgRevisions.
func (c *FakePorchPkgRevisions) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(porchpkgrevisionsResource, c.ns, opts))

}

// Create takes the representation of a porchPkgRevision and creates it.  Returns the server's representation of the porchPkgRevision, and an error, if there is any.
func (c *FakePorchPkgRevisions) Create(ctx context.Context, porchPkgRevision *v1alpha1.PorchPkgRevision, opts v1.CreateOptions) (result *v1alpha1.PorchPkgRevision, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(porchpkgrevisionsResource, c.ns, porchPkgRevision), &v1alpha1.PorchPkgRevision{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PorchPkgRevision), err
}

// Update takes the representation of a porchPkgRevision and updates it. Returns the server's representation of the porchPkgRevision, and an error, if there is any.
func (c *FakePorchPkgRevisions) Update(ctx context.Context, porchPkgRevision *v1alpha1.PorchPkgRevision, opts v1.UpdateOptions) (result *v1alpha1.PorchPkgRevision, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(porchpkgrevisionsResource, c.ns, porchPkgRevision), &v1alpha1.PorchPkgRevision{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PorchPkgRevision), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakePorchPkgRevisions) UpdateStatus(ctx context.Context, porchPkgRevision *v1alpha1.PorchPkgRevision, opts v1.UpdateOptions) (*v1alpha1.PorchPkgRevision, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(porchpkgrevisionsResource, "status", c.ns, porchPkgRevision), &v1alpha1.PorchPkgRevision{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PorchPkgRevision), err
}

// Delete takes name of the porchPkgRevision and deletes it. Returns an error if one occurs.
func (c *FakePorchPkgRevisions) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(porchpkgrevisionsResource, c.ns, name, opts), &v1alpha1.PorchPkgRevision{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakePorchPkgRevisions) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(porchpkgrevisionsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.PorchPkgRevisionList{})
	return err
}

// Patch applies the patch and returns the patched porchPkgRevision.
func (c *FakePorchPkgRevisions) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.PorchPkgRevision, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(porchpkgrevisionsResource, c.ns, name, pt, data, subresources...), &v1alpha1.PorchPkgRevision{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PorchPkgRevision), err
}

// UpdateApproval takes the representation of a porchPkgRevision and updates it. Returns the server's representation of the porchPkgRevision, and an error, if there is any.
func (c *FakePorchPkgRevisions) UpdateApproval(ctx context.Context, porchPkgRevisionName string, porchPkgRevision *v1alpha1.PorchPkgRevision, opts v1.UpdateOptions) (result *v1alpha1.PorchPkgRevision, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(porchpkgrevisionsResource, "approval", c.ns, porchPkgRevision), &v1alpha1.PorchPkgRevision{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PorchPkgRevision), err
}