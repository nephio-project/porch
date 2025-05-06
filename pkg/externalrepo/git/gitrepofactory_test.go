/*
 Copyright 2025 The Nephio Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 You may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package git

import (
	"context"
	"strings"
	"testing"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/stretchr/testify/assert"
)

func TestGitRepoFactory(t *testing.T) {

	gf := GitRepoFactory{}

	repoSpec := configapi.Repository{}
	_, err := gf.NewRepositoryImpl(context.TODO(), &repoSpec, externalrepotypes.ExternalRepoOptions{})
	assert.True(t, strings.Contains(err.Error(), "git property is required"))

	repoSpec.Spec = configapi.RepositorySpec{
		Git: &configapi.GitRepository{},
	}
	_, err = gf.NewRepositoryImpl(context.TODO(), &repoSpec, externalrepotypes.ExternalRepoOptions{})
	assert.True(t, strings.Contains(err.Error(), "git.repo property is required"))
	assert.True(t, err != nil)

	repoSpec.Spec.Git.Repo = "my-repo"
	repoSpec.Spec.Git.CreateBranch = true
	_, err = gf.NewRepositoryImpl(context.TODO(), &repoSpec, externalrepotypes.ExternalRepoOptions{})
	assert.True(t, err != nil)

	repoSpec.Spec.Git.Repo = "my-repo"
	repoSpec.Spec.Git.CreateBranch = false
	_, err = gf.NewRepositoryImpl(context.TODO(), &repoSpec, externalrepotypes.ExternalRepoOptions{})
	assert.True(t, err != nil)
}
