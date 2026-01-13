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

package git

import (
	"github.com/go-git/go-git/v5/plumbing"
)

type CommitOperation struct {
	Type string
	Data interface{}
}

type CommitOperationBuilder struct {
	operations []CommitOperation
}

func NewCommitOperationBuilder() *CommitOperationBuilder {
	return &CommitOperationBuilder{
		operations: []CommitOperation{},
	}
}

func (c *CommitOperationBuilder) AddPackageApproval(draft interface{}, tag plumbing.ReferenceName) {
	c.operations = append(c.operations, CommitOperation{
		Type: "approval",
		Data: map[string]interface{}{"draft": draft, "tag": tag},
	})
}

func (c *CommitOperationBuilder) AddPackageDeletion(branch plumbing.ReferenceName, prKey interface{}) {
	c.operations = append(c.operations, CommitOperation{
		Type: "deletion",
		Data: map[string]interface{}{"branch": branch, "prKey": prKey},
	})
}

func (c *CommitOperationBuilder) GetOperations() []CommitOperation {
	return c.operations
}
