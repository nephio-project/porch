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

package fn

import (
	"fmt"
	"reflect"
	"slices"
	"sort"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	kptfileapi "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
)

const (
	statusFieldName     = "status"
	conditionsFieldName = "conditions"
)

var (
	BoolToConditionStatus = map[bool]kptfileapi.ConditionStatus{
		true:  kptfileapi.ConditionTrue,
		false: kptfileapi.ConditionFalse,
	}
)

// Kptfile provides an API to manipulate the Kptfile of a kpt package
type Kptfile struct {
	// TODO: try to make Kptfile to be also a KubeObject
	Obj *KubeObject
}

// NewKptfileFromKubeObjectList creates a KptfileObject by finding it in the given KubeObjects list
func NewKptfileFromKubeObjectList(objs KubeObjects) (*Kptfile, error) {
	var ret Kptfile
	ret.Obj = objs.GetRootKptfile()
	if ret.Obj == nil {
		return nil, fmt.Errorf("the Kptfile object is missing from the package")

	}
	return &ret, nil
}

// NewKptfileFromPackage creates a KptfileObject from the resource (YAML) files of a package
func NewKptfileFromPackage(resources map[string]string) (*Kptfile, error) {
	kptfileStr, found := resources[kptfileapi.KptFileName]
	if !found {
		return nil, fmt.Errorf("%s is missing from the package", kptfileapi.KptFileName)
	}

	kos, err := ReadKubeObjectsFromFile(kptfileapi.KptFileName, kptfileStr)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse %s from package: %w", kptfileapi.KptFileName, err)
	}
	return NewKptfileFromKubeObjectList(kos)
}

func (kf *Kptfile) WriteToPackage(resources map[string]string) error {
	if kf == nil || kf.Obj == nil {
		return fmt.Errorf("attempt to write empty Kptfile to the package")
	}
	kptfileStr, err := WriteKubeObjectsToString(KubeObjects{kf.Obj})
	if err != nil {
		return err
	}
	resources[kptfileapi.KptFileName] = kptfileStr
	return nil
}

func (kf *Kptfile) String() string {
	if kf.Obj == nil {
		return ""
	}
	kptfileStr, _ := WriteKubeObjectsToString(KubeObjects{kf.Obj})
	return kptfileStr
}

// Status returns with the Status field of the Kptfile as a SubObject
// If the Status field doesn't exist, it is added.
func (kf *Kptfile) Status() *SubObject {
	return kf.Obj.UpsertMap(statusFieldName)
}

func (kf *Kptfile) Conditions() SliceSubObjects {
	return kf.Status().GetSlice(conditionsFieldName)
}

func (kf *Kptfile) SetConditions(conditions SliceSubObjects) error {
	sort.SliceStable(conditions, func(i, j int) bool {
		return conditions[i].GetString("type") < conditions[j].GetString("type")
	})
	return kf.Status().SetSlice(conditions, conditionsFieldName)
}

// TypedConditions returns with (a copy of) the list of current conditions of the kpt package
func (kf *Kptfile) TypedConditions() []kptfileapi.Condition {
	statusObj := kf.Obj.GetMap(statusFieldName)
	if statusObj == nil {
		return nil
	}
	var status kptfileapi.Status
	err := statusObj.As(&status)
	if err != nil {
		return nil
	}
	return status.Conditions
}

// GetTypedCondition returns with the condition whose type is `conditionType` as its first return value, and
// whether the component exists or not as its second return value
func (kf *Kptfile) GetTypedCondition(conditionType string) (kptfileapi.Condition, bool) {
	for _, cond := range kf.TypedConditions() {
		if cond.Type == conditionType {
			return cond, true
		}
	}
	return kptfileapi.Condition{}, false
}

// SetTypedCondition creates or updates the given condition using the Type field as the primary key
func (kf *Kptfile) SetTypedCondition(condition kptfileapi.Condition) error {
	conditions := kf.Conditions()
	for _, conditionSubObj := range conditions {
		if conditionSubObj.GetString("type") == condition.Type {
			// use the SetNestedString methods as opposed to SetNestedStringMap
			// in order to keep the order of new fields deterministic
			conditionSubObj.SetNestedString(string(condition.Status), "status")
			conditionSubObj.SetNestedString(condition.Reason, "reason")
			conditionSubObj.SetNestedString(condition.Message, "message")
		}
	}
	return kf.SetConditions(conditions)
}

// ApplyDefaultCondition adds the given condition to the Kptfile if a condition
// with the same type doesn't exist yet.
func (kf *Kptfile) ApplyDefaultCondition(condition kptfileapi.Condition) error {
	conditions := kf.Conditions()
	for _, conditionSubObj := range conditions {
		if conditionSubObj.GetString("type") == condition.Type {
			return nil
		}
	}
	ko, err := NewFromTypedObject(condition)
	if err != nil {
		return fmt.Errorf("failed to apply default condition %q: %w", condition.Type, err)
	}
	conditions = append(conditions, &ko.SubObject)
	return kf.SetConditions(conditions)
}

// DeleteByTpe deletes all conditions with the given type
func (kf *Kptfile) DeleteConditionByType(conditionType string) error {
	conditions, found, err := kf.Obj.NestedSlice(conditionsFieldName)
	if err != nil {
		return fmt.Errorf("failed to read conditions from Kptfile: %w", err)
	}
	if !found {
		return nil
	}
	conditions = slices.DeleteFunc(conditions, func(c *SubObject) bool {
		return c.GetString("type") == conditionType
	})
	return kf.SetConditions(conditions)
}

func (kf *Kptfile) AddReadinessGates(gates []porchapi.ReadinessGate) error {
	info := kf.Obj.UpsertMap("info")
	gateObjs := info.GetSlice("readinessGates")
	for _, gate := range gates {
		// check if readiness gate already exists
		found := false
		for _, gateObj := range gateObjs {
			if gateObj.GetString("conditionType") == gate.ConditionType {
				found = true
				break
			}
		}
		// add if not found
		if !found {
			ko, err := NewFromTypedObject(gate)
			if err != nil {
				return fmt.Errorf("failed to add readiness gate %s: %w", gate.ConditionType, err)
			}
			gateObjs = append(gateObjs, &ko.SubObject)
		}
	}
	info.SetSlice(gateObjs, "readinessGates")
	return nil
}

func (kf *Kptfile) UpsertPipelineFunctions(fns []kptfileapi.Function, fieldName string, insertPosition int) error {
	if len(fns) == 0 {
		return nil
	}
	pipelineKObj := kf.Obj.UpsertMap("pipeline")
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
	fnKObjs SliceSubObjects,
	newKrmFn kptfileapi.Function,
	insertPosition int,
) (SliceSubObjects, error) {

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
	newFuncObj, err := NewFromTypedObject(newKrmFn)
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
func findFunctionByName(haystack SliceSubObjects, name string) *SubObject {
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
func findFunctionByContent(haystack SliceSubObjects, needle *kptfileapi.Function) (*SubObject, error) {
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
