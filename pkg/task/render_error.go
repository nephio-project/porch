// Copyright 2025 The kpt and Nephio Authors
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

package task

import "fmt"

// custom error type for differentiating render status errors from other types
type RenderError struct {
	Err error
	Msg string
}

func (err *RenderError) Error() string {
	return fmt.Sprintf("%s: %v", err.Msg, err.Err)
}

func (err *RenderError) Unwrap() error {
	return err.Err
}

func NewRenderError(err error, msg string) error {
	return &RenderError{Err: err, Msg: msg}
}
