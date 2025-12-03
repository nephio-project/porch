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

package porch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindPathConflict(t *testing.T) {
	tests := []struct {
		name           string
		newPath        string
		existingPaths  []string
		expectConflict string
	}{
		{
			name:           "no conflict with empty list",
			newPath:        "pkg1",
			existingPaths:  []string{},
			expectConflict: "",
		},
		{
			name:           "no conflict with siblings",
			newPath:        "root/sub1",
			existingPaths:  []string{"root/sub2", "root/sub3"},
			expectConflict: "",
		},
		{
			name:           "conflict with parent",
			newPath:        "root/sub4/sub4.2",
			existingPaths:  []string{"root/sub4"},
			expectConflict: "root/sub4",
		},
		{
			name:           "conflict with child",
			newPath:        "root/sub4",
			existingPaths:  []string{"root/sub4/sub4.2"},
			expectConflict: "root/sub4/sub4.2",
		},
		{
			name:           "no conflict with different branches",
			newPath:        "root/sub4/sub4.1",
			existingPaths:  []string{"root/sub4/sub4.2", "root/sub4/sub4.3"},
			expectConflict: "",
		},
		{
			name:           "conflict with grandchild",
			newPath:        "root/sub4",
			existingPaths:  []string{"root/sub4/sub4.3/sub4.3.1"},
			expectConflict: "root/sub4/sub4.3/sub4.3.1",
		},
		{
			name:           "no conflict with similar prefix",
			newPath:        "test",
			existingPaths:  []string{"test-package", "test-other"},
			expectConflict: "",
		},
		{
			name:           "multiple paths, one conflict",
			newPath:        "root/sub4",
			existingPaths:  []string{"root/sub1", "root/sub4/sub4.2", "root/sub5"},
			expectConflict: "root/sub4/sub4.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findPathConflict(tt.newPath, tt.existingPaths)
			assert.Equal(t, tt.expectConflict, result)
		})
	}
}
