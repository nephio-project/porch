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
	"sigs.k8s.io/kustomize/kyaml/openapi"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	"sigs.k8s.io/kustomize/kyaml/yaml/merge3"
	"sigs.k8s.io/kustomize/kyaml/yaml/walk"
)

// commentPreservingVisitor wraps the default merge3.Visitor
// if it finds a null scalar with only comments it gets copied over
// otherwise it behaves like the default merge3.Visitor
type commentPreservingVisitor struct {
	merge3.Visitor
}

func isNullScalarWithComments(node *yaml.RNode) bool {
	if node == nil {
		return false
	}

	if node.YNode().Kind != yaml.ScalarNode || node.YNode().Tag != yaml.NodeTagNull {
		return false
	}

	return node.YNode().HeadComment != "" || node.YNode().LineComment != "" || node.YNode().FootComment != ""
}

// VisitScalar is an override that checks for comment-only null scalars before merging.
func (v *commentPreservingVisitor) VisitScalar(nodes walk.Sources, s *openapi.ResourceSchema) (*yaml.RNode, error) {
	updated := nodes.Updated()

	if isNullScalarWithComments(updated) {
		return updated.Copy(), nil
	}

	return v.Visitor.VisitScalar(nodes, s)
}
