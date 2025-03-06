// Copyright 2022 Google LLC
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

package fn

import (
	"fmt"
	"math"
	"strings"

	v1 "github.com/nephio-project/porch/third_party/GoogleContainerTools/kpt-functions-sdk/go/api/kptfile/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type KubeObjects []*KubeObject

func (o KubeObjects) Len() int      { return len(o) }
func (o KubeObjects) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o KubeObjects) Less(i, j int) bool {
	idi := o[i].resourceIdentifier()
	idj := o[j].resourceIdentifier()
	idStrI := fmt.Sprintf("%s %s %s %s", idi.GetAPIVersion(), idi.GetKind(), idi.GetNamespace(), idi.GetName())
	idStrJ := fmt.Sprintf("%s %s %s %s", idj.GetAPIVersion(), idj.GetKind(), idj.GetNamespace(), idj.GetName())
	return idStrI < idStrJ
}

func (o KubeObjects) String() string {
	var elems []string
	for _, obj := range o {
		elems = append(elems, strings.TrimSpace(obj.String()))
	}
	return strings.Join(elems, "\n---\n")
}

// EnsureSingleItem checks if KubeObjects contains exactly one item and returns it, or an error if it doesn't.
func (o KubeObjects) EnsureSingleItem() (*KubeObject, error) {
	if len(o) == 0 || len(o) > 1 {
		return nil, fmt.Errorf("%v objects found, but exactly 1 is expected", len(o))
	}
	return o[0], nil
}

func (o KubeObjects) EnsureSingleItemAs(out any) error {
	obj, err := o.EnsureSingleItem()
	if err != nil {
		return err
	}
	return obj.As(out)
}

// Upsert updates or insert the given KubeObject into the list
// If the list contains an object with the same (Group, Version, Kind, Namespace, Name), then Upsert replaces it with `newObj`,
// otherwise it appends `newObj` to the list
func (objs *KubeObjects) Upsert(newObj *KubeObject) {

	for i, kobj := range *objs {
		if newObj.HasSameId(kobj) {
			(*objs)[i] = newObj
			return
		}
	}
	*objs = append(*objs, newObj)
}

// UpsertTypedObject attempts to convert `newObj` to a KubeObject and then calls Upsert().
func (objs *KubeObjects) UpsertTypedObject(newObj any) error {
	if newObj == nil {
		return fmt.Errorf("obj is nil")
	}

	newKubeObj, err := NewFromTypedObject(newObj)
	if err != nil {
		return err
	}

	objs.Upsert(newKubeObj)
	return nil
}

// Where will return the subset of objects in KubeObjects such that f(object) returns 'true'.
func (o KubeObjects) Where(f func(*KubeObject) bool) KubeObjects {
	var result KubeObjects
	for _, obj := range o {
		if f(obj) {
			result = append(result, obj)
		}
	}
	return result
}

// Not returns will return a function that returns the opposite of f(object), i.e. !f(object)
func Not(f func(*KubeObject) bool) func(o *KubeObject) bool {
	return func(o *KubeObject) bool {
		return !f(o)
	}
}

// WhereNot will return the subset of objects in KubeObjects such that f(object) returns 'false'.
// This is a shortcut for Where(Not(f)).
func (o KubeObjects) WhereNot(f func(o *KubeObject) bool) KubeObjects {
	return o.Where(Not(f))
}

// IsGVK returns a function that checks if a KubeObject has a certain GVK.
// Deprecated: Prefer exact matching with IsGroupVersionKind or IsGroupKind
func IsGVK(group, version, kind string) func(*KubeObject) bool {
	return func(o *KubeObject) bool {
		return o.IsGVK(group, version, kind)
	}
}

// IsGroupVersionKind returns a function that checks if a KubeObject has a certain GroupVersionKind.
func IsGroupVersionKind(gvk schema.GroupVersionKind) func(*KubeObject) bool {
	return func(o *KubeObject) bool {
		return o.IsGroupVersionKind(gvk)
	}
}

// IsGroupKind returns a function that checks if a KubeObject has a certain GroupKind.
func IsGroupKind(gk schema.GroupKind) func(*KubeObject) bool {
	return func(o *KubeObject) bool {
		return o.IsGroupKind(gk)
	}
}

// GetRootKptfile returns the root Kptfile. Nested kpt packages can have multiple Kptfile files of the same GVKNN.
func (o KubeObjects) GetRootKptfile() *KubeObject {
	kptfiles := o.Where(IsGVK(v1.KptFileGroup, v1.KptFileVersion, v1.KptFileKind))
	if len(kptfiles) == 0 {
		return nil
	}
	minDepths := math.MaxInt32
	var rootKptfile *KubeObject
	for _, kf := range kptfiles {
		path := kf.GetAnnotation(PathAnnotation)
		depths := len(strings.Split(path, "/"))
		if depths <= minDepths {
			minDepths = depths
			rootKptfile = kf
		}
	}
	return rootKptfile
}

// IsName returns a function that checks if a KubeObject has a certain name.
func IsName(name string) func(*KubeObject) bool {
	return func(o *KubeObject) bool {
		return o.GetName() == name
	}
}

// IsNamespace returns a function that checks if a KubeObject has a certain namespace.
func IsNamespace(namespace string) func(*KubeObject) bool {
	return func(o *KubeObject) bool {
		return o.GetNamespace() == namespace
	}
}

// HasLabels returns a function that checks if a KubeObject has all the given labels.
func HasLabels(labels map[string]string) func(*KubeObject) bool {
	return func(o *KubeObject) bool {
		return o.HasLabels(labels)
	}
}

// HasAnnotations returns a function that checks if a KubeObject has all the given annotations.
func HasAnnotations(annotations map[string]string) func(*KubeObject) bool {
	return func(o *KubeObject) bool {
		return o.HasAnnotations(annotations)
	}
}
