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
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommitOperationBuilder(t *testing.T) {
	builder := NewCommitOperationBuilder()
	require.NotNil(t, builder)
	assert.Empty(t, builder.operations)
}

func TestAddPackageApproval(t *testing.T) {
	builder := NewCommitOperationBuilder()
	draft := "test-draft"
	tag := plumbing.ReferenceName("refs/tags/test-tag")

	builder.AddPackageApproval(draft, tag)

	ops := builder.GetOperations()
	require.Len(t, ops, 1)

	op := ops[0]
	assert.Equal(t, "approval", op.Type)

	data := op.Data.(map[string]interface{})
	assert.Equal(t, draft, data["draft"])
	assert.Equal(t, tag, data["tag"])
}

func TestAddPackageDeletion(t *testing.T) {
	builder := NewCommitOperationBuilder()
	branch := plumbing.ReferenceName("refs/heads/test-branch")
	prKey := "test-key"

	builder.AddPackageDeletion(branch, prKey)

	ops := builder.GetOperations()
	require.Len(t, ops, 1)

	op := ops[0]
	assert.Equal(t, "deletion", op.Type)

	data := op.Data.(map[string]interface{})
	assert.Equal(t, branch, data["branch"])
	assert.Equal(t, prKey, data["prKey"])
}

func TestMultipleOperations(t *testing.T) {
	builder := NewCommitOperationBuilder()

	builder.AddPackageApproval("draft1", plumbing.ReferenceName("refs/tags/tag1"))
	builder.AddPackageDeletion(plumbing.ReferenceName("refs/heads/branch1"), "key1")
	builder.AddPackageApproval("draft2", plumbing.ReferenceName("refs/tags/tag2"))

	ops := builder.GetOperations()
	require.Len(t, ops, 3)

	assert.Equal(t, "approval", ops[0].Type)
	assert.Equal(t, "deletion", ops[1].Type)
	assert.Equal(t, "approval", ops[2].Type)
}
