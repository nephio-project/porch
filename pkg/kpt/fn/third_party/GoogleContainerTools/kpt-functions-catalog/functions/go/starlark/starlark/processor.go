/*
 Copyright 2025 The Nephio Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 You may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package starlark

import (
	"github.com/GoogleContainerTools/kpt-functions-sdk/go/fn"
)

func Process(resourceList *fn.ResourceList) (bool, error) {
	err := func() error {
		sr := &StarlarkRun{}
		if err := sr.Config(resourceList.FunctionConfig); err != nil {
			return err
		}
		return sr.Transform(resourceList)
	}()
	if err != nil {
		resourceList.Results = []*fn.Result{
			{
				Message:  err.Error(),
				Severity: fn.Error,
			},
		}
		return false, nil
	}
	return true, nil
}
