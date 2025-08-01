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

package merge3

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestEqualWillSkip(t *testing.T) {
	orig := yaml.MustParse(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
data:
  foo.txt: "bar"
`)
	dest := orig.Copy()

	strat := GetHandlingStrategy(orig, nil, dest)
	assert.Equal(t, filters.Skip, strat)
}

func TestNotEqualWillKeepDest(t *testing.T) {
	orig := yaml.MustParse(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
data:
  foo.txt: "bar"
`)
	dest := orig.Copy()
	dest.SetDataMap(map[string]string{"foo.txt": "baz"})

	strat := GetHandlingStrategy(orig, nil, dest)
	assert.Equal(t, filters.KeepDest, strat)
}
