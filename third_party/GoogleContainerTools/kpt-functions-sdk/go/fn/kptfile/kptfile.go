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

	kptfileapi "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/third_party/GoogleContainerTools/kpt-functions-sdk/go/fn"
)

const (
	statusFieldName = "status"
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
	Obj *fn.KubeObject
}

// NewKptfileFromKubeObjectList creates a KptfileObject by finding it in the given KubeObjects list
func NewFromKubeObjectList(objs fn.KubeObjects) (*Kptfile, error) {
	var ret Kptfile
	ret.Obj = objs.GetRootKptfile()
	if ret.Obj == nil {
		return nil, fmt.Errorf("the Kptfile object is missing from the package")

	}
	return &ret, nil
}

// NewKptfileFromPackage creates a KptfileObject from the resource (YAML) files of a package
func NewFromPackage(resources map[string]string) (*Kptfile, error) {
	kptfileStr, found := resources[kptfileapi.KptFileName]
	if !found {
		return nil, fmt.Errorf("%s is missing from the package", kptfileapi.KptFileName)
	}

	kos, err := fn.ReadKubeObjectsFromFile(kptfileapi.KptFileName, kptfileStr)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse %s from package: %w", kptfileapi.KptFileName, err)
	}
	return NewFromKubeObjectList(kos)
}

func (kf *Kptfile) WriteToPackage(resources map[string]string) error {
	if kf == nil || kf.Obj == nil {
		return fmt.Errorf("attempt to write empty Kptfile to the package")
	}
	kptfileStr, err := fn.WriteKubeObjectsToString(fn.KubeObjects{kf.Obj})
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
	kptfileStr, _ := fn.WriteKubeObjectsToString(fn.KubeObjects{kf.Obj})
	return kptfileStr
}

// Status returns with the Status field of the Kptfile as a SubObject
// If the Status field doesn't exist, it is added.
func (kf *Kptfile) Status() *fn.SubObject {
	return kf.Obj.UpsertMap(statusFieldName)
}
