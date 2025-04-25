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
	"slices"

	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	kptfileapi "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/third_party/GoogleContainerTools/kpt-functions-sdk/go/fn"
)

const (
	conditionsFieldName = "conditions"
)

func (kf *Kptfile) Conditions() fn.SliceSubObjects {
	return kf.Status().GetSlice(conditionsFieldName)
}

func (kf *Kptfile) SetConditions(conditions fn.SliceSubObjects) error {
	return kf.Status().SetSlice(conditions, conditionsFieldName)
}

// TypedConditions returns with (a copy of) the list of current conditions of the kpt package
func (kf *Kptfile) TypedConditions() []kptfileapi.Condition {
	statusObj := kf.GetMap(statusFieldName)
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
	ko, err := fn.NewFromTypedObject(condition)
	if err != nil {
		return fmt.Errorf("failed to apply default condition %q: %w", condition.Type, err)
	}
	conditions = append(conditions, &ko.SubObject)
	return kf.SetConditions(conditions)
}

// DeleteByTpe deletes all conditions with the given type
func (kf *Kptfile) DeleteConditionByType(conditionType string) error {
	conditions, found, err := kf.NestedSlice(conditionsFieldName)
	if err != nil {
		return fmt.Errorf("failed to read conditions from Kptfile: %w", err)
	}
	if !found {
		return nil
	}
	conditions = slices.DeleteFunc(conditions, func(c *fn.SubObject) bool {
		return c.GetString("type") == conditionType
	})
	return kf.SetConditions(conditions)
}

func (kf *Kptfile) AddReadinessGates(gates []porchapi.ReadinessGate) error {
	info := kf.UpsertMap("info")
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
			ko, err := fn.NewFromTypedObject(gate)
			if err != nil {
				return fmt.Errorf("failed to add readiness gate %s: %w", gate.ConditionType, err)
			}
			gateObjs = append(gateObjs, &ko.SubObject)
		}
	}
	info.SetSlice(gateObjs, "readinessGates")
	return nil
}
