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

func TestPathsOverlapDetailed(t *testing.T) {
	tests := []struct {
		name     string
		path1    string
		path2    string
		overlaps bool
	}{
		{
			name:     "identical paths",
			path1:    "root/sub1",
			path2:    "root/sub1",
			overlaps: true,
		},
		{
			name:     "path2 is child of path1",
			path1:    "root/sub4",
			path2:    "root/sub4/sub4.2",
			overlaps: true,
		},
		{
			name:     "path1 is child of path2",
			path1:    "root/sub4/sub4.3/sub4.3.1",
			path2:    "root/sub4/sub4.3",
			overlaps: true,
		},
		{
			name:     "sibling paths no overlap",
			path1:    "root/sub1",
			path2:    "root/sub2",
			overlaps: false,
		},
		{
			name:     "different branches no overlap",
			path1:    "root/sub4/sub4.1",
			path2:    "root/sub4/sub4.2",
			overlaps: false,
		},
		{
			name:     "root level paths no overlap",
			path1:    "package1",
			path2:    "package2",
			overlaps: false,
		},
		{
			name:     "root vs nested no overlap",
			path1:    "root",
			path2:    "root-other/sub",
			overlaps: false,
		},
		{
			name:     "nested grandchild overlap",
			path1:    "root/sub4",
			path2:    "root/sub4/sub4.3/sub4.3.1",
			overlaps: true,
		},
		{
			name:     "single level packages no overlap",
			path1:    "pkg1",
			path2:    "pkg2",
			overlaps: false,
		},
		{
			name:     "similar prefix no overlap",
			path1:    "test",
			path2:    "test-package",
			overlaps: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pathsOverlap(tt.path1, tt.path2)
			assert.Equal(t, tt.overlaps, result, "pathsOverlap(%q, %q)", tt.path1, tt.path2)
			
			// Test symmetry
			resultReverse := pathsOverlap(tt.path2, tt.path1)
			assert.Equal(t, tt.overlaps, resultReverse, "pathsOverlap(%q, %q) should be symmetric", tt.path2, tt.path1)
		})
	}
}
