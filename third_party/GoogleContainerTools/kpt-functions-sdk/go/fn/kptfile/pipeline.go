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

package kptfile

import (
	"fmt"
	"reflect"
	"slices"

	kptfileapi "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/third_party/GoogleContainerTools/kpt-functions-sdk/go/fn"
)

func (kf *Kptfile) UpsertPipelineFunctions(fns []kptfileapi.Function, fieldName string, insertPosition int) error {
	if len(fns) == 0 {
		return nil
	}
	pipelineKObj := kf.UpsertMap("pipeline")
	fnKObjs, _, _ := pipelineKObj.NestedSlice(fieldName)
	for _, newKrmFn := range fns {
		var err error
		fnKObjs, err = UpsertKrmFunction(fnKObjs, newKrmFn, insertPosition)
		if err != nil {
			return err
		}
	}
	return pipelineKObj.SetSlice(fnKObjs, fieldName)
}

func (kf *Kptfile) UpsertMutatorFunctions(fns []kptfileapi.Function, insertPosition int) error {
	return kf.UpsertPipelineFunctions(fns, "mutators", insertPosition)
}

func (kf *Kptfile) UpsertValidatorFunctions(fns []kptfileapi.Function, insertPosition int) error {
	return kf.UpsertPipelineFunctions(fns, "validators", insertPosition)
}

// UpsertKrmFunction ensures that a KRM function is added or updated in the given list of function objects.
// If the function already exists, it is updated. If it doesn't exist, it is added at the specified position.
// If insertPosition is negative, the insert position is counted backwards from the end of the list
// (i.e. -1 will append to the list).
func UpsertKrmFunction(
	fnKObjs fn.SliceSubObjects,
	newKrmFn kptfileapi.Function,
	insertPosition int,
) (fn.SliceSubObjects, error) {

	if newKrmFn.Name == "" {
		// match by content
		fnObj, err := findFunctionByContent(fnKObjs, &newKrmFn)
		if err != nil {
			return nil, err
		}
		if fnObj != nil {
			// function already exists, skip to avoid duplicates
			return fnKObjs, nil
		}
	} else {
		// match by name
		fnObj := findFunctionByName(fnKObjs, newKrmFn.Name)
		if fnObj != nil {
			// function with the same name exists, update it
			var origKrmFn kptfileapi.Function
			err := fnObj.As(&origKrmFn)
			if err != nil {
				return fnKObjs, fmt.Errorf("failed to parse KRM function from YAML: %w", err)
			}
			err = fnObj.SetFromTypedObject(newKrmFn)
			if err != nil {
				return fnKObjs, fmt.Errorf("failed to update KRM function in Kptfile: %w", err)
			}
			return fnKObjs, nil
		}
	}

	// function does not exist, insert it
	newFuncObj, err := fn.NewFromTypedObject(newKrmFn)
	if err != nil {
		return fnKObjs, err
	}
	if insertPosition < 0 {
		insertPosition = len(fnKObjs) + insertPosition + 1
	}
	fnKObjs = slices.Insert(fnKObjs, insertPosition, &newFuncObj.SubObject)
	return fnKObjs, nil
}

// findFunction returns with the first KRM function in the list with the given name
func findFunctionByName(haystack fn.SliceSubObjects, name string) *fn.SubObject {
	for _, fnObj := range haystack {
		// match by name
		objName, found, _ := fnObj.NestedString("name")
		if found && objName == name {
			return fnObj
		}
	}
	return nil
}

// findFunctionByContent returns with the first KRM function in the list that matches the content of the needle
func findFunctionByContent(haystack fn.SliceSubObjects, needle *kptfileapi.Function) (*fn.SubObject, error) {
	for _, fnObj := range haystack {
		var krmFn kptfileapi.Function
		err := fnObj.As(&krmFn)
		if err != nil {
			return nil, fmt.Errorf("failed to parse KRM function from YAML: %w", err)
		}
		// ignore diff in name
		krmFn.Name = needle.Name
		if reflect.DeepEqual(krmFn, needle) {
			return fnObj, nil
		}
	}
	return nil, nil
}
