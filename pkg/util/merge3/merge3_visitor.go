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

// CommentPreservingVisitor is a merge3 visitor that preserves null nodes if they have comments.
type CommentPreservingVisitor struct {
	merge3.Visitor
}

func (m CommentPreservingVisitor) VisitMap(sources walk.Sources, schema *openapi.ResourceSchema) (*yaml.RNode, error) {

	if isNullNodeWithComments(sources.Dest()) {
		// Null values are treated as a map
		ynode := sources.Dest().YNode()
		if ynode.Tag == yaml.NodeTagNull {
			// Return a new node so that it won't have a "!!null" tag and therefore won't be cleared.
			return yaml.NewScalarRNode(ynode.Value), nil
		}
		return sources.Dest(), nil
	}
	// If the destination is not a null node with comments, proceed with the default merge3 behavior
	return merge3.Visitor{}.VisitMap(sources, schema)
}

func isNullNodeWithComments(node *yaml.RNode) bool {
	if node == nil {
		return false
	}
	if node.YNode().Tag != yaml.NodeTagNull {
		return false
	}
	return node.YNode().HeadComment != "" || node.YNode().LineComment != "" || node.YNode().FootComment != ""
}
