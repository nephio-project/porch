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

package merge3

import (
	"strings"
	"testing"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var original = `
apiVersion: example.com/v1alpha1
kind: Deployment
metadata:
  name: resource-name
spec:
  testNull: #! This is a inline comment
`
var updated = `
apiVersion: example.com/v1alpha1
kind: Deployment
metadata:
  name: resource-name
spec:
  #! This is a head comment
  testNull: #! This is a inline comment
  #! This is a foot comment

`
var dest = `
apiVersion: example.com/v1alpha1
kind: Deployment
metadata:
  name: resource-name
spec:
  #! This is a head comment
  testNull: #! This is a inline comment
  #! This is a foot comment
`

func Test_tupleMerge_PreservesNullKeys(t *testing.T) {
	origNode, err := yaml.Parse(original)
	if err != nil {
		t.Fatalf("failed to parse original: %v", err)
	}
	updatedNode, err := yaml.Parse(updated)
	if err != nil {
		t.Fatalf("failed to parse updated: %v", err)
	}
	destNode, err := yaml.Parse(dest)
	if err != nil {
		t.Fatalf("failed to parse dest: %v", err)
	}

	tuple := &tuple{
		original: origNode,
		updated:  updatedNode,
		dest:     destNode,
	}

	merged, err := tuple.merge()
	if err != nil {
		t.Fatalf("tuple.merge() error = %v", err)
	}

	out, err := merged.String()
	if err != nil {
		t.Fatalf("failed to convert merged node to string: %v", err)
	}

	count := strings.Count(out, "#!")
	if count != 3 {
		t.Errorf("Expected merged output to contain '#!' exactly 3 times, got %d times:\n%s", count, out)
	}
}
