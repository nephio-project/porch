// Copyright 2025 The Nephio Authors
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

package dbcache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUtil(t *testing.T) {
	// We can't marshal a function into JSON
	jsonVal := valueAsJSON(TestDBSQL)
	assert.Equal(t, "", jsonVal)

	secondValue := time.Second
	setValueFromJSON("", &secondValue)
	assert.Equal(t, time.Second, secondValue)
}
