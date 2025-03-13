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
	kptfile *KubeObject
}

// NewKptfileFromKubeObjectList creates a KptfileObject by finding it in the given KubeObjects list
func NewKptfileFromKubeObjectList(objs KubeObjects) (*Kptfile, error) {
	var ret Kptfile
	ret.kptfile = objs.GetRootKptfile()
	if ret.kptfile == nil {
		return nil, fmt.Errorf("the Kptfile object is missing from the package")

	}
	return &ret, nil
}

// NewKptfileFromPackage creates a KptfileObject from the resource (YAML) files of a package
func NewKptfileFromPackage(resources map[string]string) (*Kptfile, error) {
	kptfileStr, found := resources[kptfileapi.KptFileName]
	if !found {
		return nil, fmt.Errorf("'%s' is missing from the package", kptfileapi.KptFileName)
	}

	kos, err := ReadKubeObjectsFromFile(kptfileapi.KptFileName, kptfileStr)
	if err != nil {
		return nil, err
	}
	return NewKptfileFromKubeObjectList(kos)
}

func (kf *Kptfile) WriteToPackage(resources map[string]string) error {
	if kf == nil || kf.kptfile == nil {
		return fmt.Errorf("attempt to write empty Kptfile to the package")
	}
	kptfileStr, err := WriteKubeObjectsToString(KubeObjects{kf.kptfile})
	if err != nil {
		return err
	}
	resources[kptfileapi.KptFileName] = kptfileStr
	return nil
}

func (kf *Kptfile) String() string {
	if kf.kptfile == nil {
		return ""
	}
	kptfileStr, _ := WriteKubeObjectsToString(KubeObjects{kf.kptfile})
	return kptfileStr
}

// Status returns with the Status field of the Kptfile as a SubObject
// If the Status field doesn't exist, it is added.
func (kf *Kptfile) Status() *SubObject {
	return kf.kptfile.UpsertMap(statusFieldName)
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
	statusObj := kf.kptfile.GetMap(statusFieldName)
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
			return conditionSubObj.UpdateNestedStringMap(map[string]string{
				"status":  string(condition.Status),
				"reason":  condition.Reason,
				"message": condition.Message,
			})
		}
	}
	ko, err := NewFromTypedObject(condition)
	if err != nil {
		return err
	}
	conditions = append(conditions, &ko.SubObject)
	return kf.SetConditions(conditions)
}

// DeleteByTpe deletes all conditions with the given type
func (kf *Kptfile) DeleteConditionByType(conditionType string) error {
	oldConditions, found, err := kf.kptfile.NestedSlice(conditionsFieldName)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	newConditions := make([]*SubObject, 0, len(oldConditions))
	for _, c := range oldConditions {
		if c.GetString("type") != conditionType {
			newConditions = append(newConditions, c)
		}
	}
	return kf.SetConditions(newConditions)
}

func (kf *Kptfile) AddReadinessGates(gates []porchapi.ReadinessGate) error {
	info := kf.kptfile.UpsertMap("info")
	gateObjs := info.GetSlice("readinessGates")
	for _, gate := range gates {
		found := false
		for _, gateObj := range gateObjs {
			if gateObj.GetString("conditionType") == gate.ConditionType {
				found = true
				break
			}
		}
		if !found {
			ko, err := NewFromTypedObject(gate)
			if err != nil {
				return err
			}
			gateObjs = append(gateObjs, &ko.SubObject)
		}
	}
	info.SetSlice(gateObjs, "readinessGates")
	return nil
}
