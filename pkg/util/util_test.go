// Copyright 2022,2025-2026 The kpt Authors
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

package util

import (
	"fmt"
	"testing"

	kptfilev1 "github.com/kptdev/kpt/api/kptfile/v1"
	configapi "github.com/kptdev/porch/api/porchconfig/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestGenerateUid(t *testing.T) {
	assert.Equal(t, types.UID("272983f7-a30d-58e0-a809-c38e8638666d"), GenerateUid("my-prefix", "my-namespace", "my.kube.name.of-something"))
}

func TestCompareObjectMeta(t *testing.T) {
	left := metav1.ObjectMeta{}
	right := metav1.ObjectMeta{}
	assert.True(t, CompareObjectMeta(left, right))

	left.Name = "my_name"
	assert.False(t, CompareObjectMeta(left, right))
	right.Name = "my_name"
	assert.True(t, CompareObjectMeta(left, right))

	left.Namespace = "my_ns"
	assert.False(t, CompareObjectMeta(left, right))
	right.Namespace = "my_ns"
	assert.True(t, CompareObjectMeta(left, right))

	left.Labels = map[string]string{}
	assert.False(t, CompareObjectMeta(left, right))
	right.Labels = map[string]string{}
	assert.True(t, CompareObjectMeta(left, right))

	left.Annotations = map[string]string{}
	assert.False(t, CompareObjectMeta(left, right))
	right.Annotations = map[string]string{}
	assert.True(t, CompareObjectMeta(left, right))

	left.Finalizers = []string{}
	assert.False(t, CompareObjectMeta(left, right))
	right.Finalizers = []string{}
	assert.True(t, CompareObjectMeta(left, right))

	left.OwnerReferences = []metav1.OwnerReference{}
	assert.False(t, CompareObjectMeta(left, right))
	right.OwnerReferences = []metav1.OwnerReference{}
	assert.True(t, CompareObjectMeta(left, right))
}

func TestRetryOnErrorConditional(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		shouldRetryFunc func(error) bool
		retryCount      int
		shouldFail      bool
	}{
		{
			name:            "no error",
			err:             nil,
			shouldRetryFunc: func(error) bool { return false },
			retryCount:      2,
			shouldFail:      false,
		},
		{
			name:            "error without retries",
			err:             fmt.Errorf("some error"),
			shouldRetryFunc: func(error) bool { return false },
			retryCount:      2,
			shouldFail:      true,
		},
		{
			name:            "error with retries",
			err:             fmt.Errorf("some error"),
			shouldRetryFunc: func(error) bool { return true },
			retryCount:      10,
			shouldFail:      true,
		},
		{
			name:            "no tries",
			err:             fmt.Errorf("some error"),
			shouldRetryFunc: func(error) bool { return true },
			retryCount:      0,
			shouldFail:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RetryOnErrorConditional(tt.retryCount, tt.shouldRetryFunc, func(_ int) error {
				return tt.err
			})

			if (got != nil) != tt.shouldFail {
				t.Errorf("RetryOnErrorConditional() = %v, shouldFail %v", got, tt.shouldFail)
			}
		})
	}
}

func TestGetRepoPackageRefFromUpstream(t *testing.T) {
	tests := []struct {
		name           string
		upstream       *kptfilev1.Upstream
		wantRepoSpec   *configapi.RepositorySpec
		wantPkg        string
		wantRef        string
		wantManagedRef bool
		wantErr        string
	}{
		{
			name:     "nil upstream returns error",
			upstream: nil,
			wantErr:  "upstream does not contain a valid git repository",
		},
		{
			name:     "nil git returns error",
			upstream: &kptfilev1.Upstream{Git: nil},
			wantErr:  "upstream does not contain a valid git repository",
		},
		{
			name: "empty repo returns error",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "",
					Ref:       "mypkg/v1",
					Directory: "mypkg",
				},
			},
			wantErr: "upstream does not contain a valid git repository",
		},
		{
			name: "empty directory returns error",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/example/repo.git",
					Ref:       "mypkg/v1",
					Directory: "",
				},
			},
			wantErr: "git directory reference",
		},
		{
			name: "directory with leading slash returns error",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/example/repo.git",
					Ref:       "mypkg/v1",
					Directory: "/mypkg",
				},
			},
			wantErr: "git directory reference \"/mypkg\" in upstream is invalid",
		},
		{
			name: "directory with trailing slash returns error",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/example/repo.git",
					Ref:       "mypkg/v1",
					Directory: "mypkg/",
				},
			},
			wantErr: "git directory reference \"mypkg/\" in upstream is invalid",
		},
		{
			name: "empty ref returns error",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/example/repo.git",
					Ref:       "",
					Directory: "mypkg",
				},
			},
			wantErr: "git ref reference",
		},
		{
			name: "unmanaged ref - ref does not contain directory pattern",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/example/repo.git",
					Ref:       "main",
					Directory: "mypkg",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "https://github.com/example/repo",
					Directory: "mypkg",
				},
			},
			wantPkg:        "mypkg",
			wantRef:        "main",
			wantManagedRef: false,
		},
		{
			name: "unmanaged ref - arbitrary tag",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/example/repo.git",
					Ref:       "release-2.0",
					Directory: "packages/foo",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "https://github.com/example/repo",
					Directory: "packages/foo",
				},
			},
			wantPkg:        "packages.foo",
			wantRef:        "release-2.0",
			wantManagedRef: false,
		},
		{
			name: "managed ref - simple package",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/example/repo.git",
					Ref:       "mypkg/v1",
					Directory: "mypkg",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "https://github.com/example/repo",
					Directory: "",
				},
			},
			wantPkg:        "mypkg",
			wantRef:        "v1",
			wantManagedRef: true,
		},
		{
			name: "managed ref - nested package",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/example/repo.git",
					Ref:       "packages/mypkg/v2",
					Directory: "packages/mypkg",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "https://github.com/example/repo",
					Directory: "",
				},
			},
			wantPkg:        "packages.mypkg",
			wantRef:        "v2",
			wantManagedRef: true,
		},
		{
			name: "managed ref - with repo directory prefix",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/example/repo.git",
					Ref:       "blueprints/mypkg/v3",
					Directory: "mypkg",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "https://github.com/example/repo",
					Directory: "blueprints",
				},
			},
			wantPkg:        "mypkg",
			wantRef:        "v3",
			wantManagedRef: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoSpec, pkg, ref, managedRef, err := GetRepoPackageRefFromUpstream(tt.upstream)

			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, repoSpec)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantRepoSpec, repoSpec)
				assert.Equal(t, tt.wantPkg, pkg)
				assert.Equal(t, tt.wantRef, ref)
				assert.Equal(t, tt.wantManagedRef, managedRef)
			}
		})
	}
}

func TestGetRepoPackageRefFromUpstreamRealData(t *testing.T) {
	tests := []struct {
		name           string
		upstream       *kptfilev1.Upstream
		wantRepoSpec   *configapi.RepositorySpec
		wantPkg        string
		wantRef        string
		wantManagedRef bool
		wantErr        string
	}{
		{
			name: "real test case 1",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "http://172.18.255.204:3000/porch/porch-test.git",
					Directory: "upstream-function1",
					Ref:       "upstream-function1/v1",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "http://172.18.255.204:3000/porch/porch-test",
					Directory: "",
				},
			},
			wantPkg:        "upstream-function1",
			wantRef:        "v1",
			wantManagedRef: true,
		},
		{
			name: "real test case 2",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "http://172.18.255.204:3000/porch/porch-test.git",
					Directory: "r2/r2/r2/r2",
					Ref:       "r2/r2/r2/r2/r2/r2/v1",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "http://172.18.255.204:3000/porch/porch-test",
					Directory: "r2/r2",
				},
			},
			wantPkg:        "r2.r2.r2.r2",
			wantRef:        "v1",
			wantManagedRef: true,
		},
		{
			name: "real test case 3",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "http://172.18.255.204:3000/porch/porch-test.git",
					Directory: "upstream-function",
					Ref:       "l1/upstream-function/v1",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "http://172.18.255.204:3000/porch/porch-test",
					Directory: "l1",
				},
			},
			wantPkg:        "upstream-function",
			wantRef:        "v1",
			wantManagedRef: true,
		},
		{
			name: "real test case 4",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "http://172.18.255.204:3000/porch/porch-test.git",
					Directory: "upstream-function",
					Ref:       "r1/upstream-function/v1",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "http://172.18.255.204:3000/porch/porch-test",
					Directory: "r1",
				},
			},
			wantPkg:        "upstream-function",
			wantRef:        "v1",
			wantManagedRef: true,
		},
		{
			name: "real test case 5",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "http://172.18.255.204:3000/porch/porch-test.git",
					Directory: "upstream-function",
					Ref:       "l3/l3/l3/upstream-function/v1",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "http://172.18.255.204:3000/porch/porch-test",
					Directory: "l3/l3/l3",
				},
			},
			wantPkg:        "upstream-function",
			wantRef:        "v1",
			wantManagedRef: true,
		},
		{
			name: "real test case 6",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "http://172.18.255.204:3000/porch/porch-test.git",
					Directory: "upstream-dir/upstream-function",
					Ref:       "l3/l3/l3/upstream-dir/upstream-function/v1",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "http://172.18.255.204:3000/porch/porch-test",
					Directory: "l3/l3/l3",
				},
			},
			wantPkg:        "upstream-dir.upstream-function",
			wantRef:        "v1",
			wantManagedRef: true,
		},
		{
			name: "real test case 7",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "http://172.18.255.204:3000/porch/porch-test.git",
					Directory: "upstream-function",
					Ref:       "r3/r3/r3/upstream-function/v1",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "http://172.18.255.204:3000/porch/porch-test",
					Directory: "r3/r3/r3",
				},
			},
			wantPkg:        "upstream-function",
			wantRef:        "v1",
			wantManagedRef: true,
		},
		{
			name: "real test case 8",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/kptdev/kpt.git",
					Directory: "package-examples/wordpress",
					Ref:       "v1.0.0-beta.64",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "https://github.com/kptdev/kpt",
					Directory: "package-examples/wordpress",
				},
			},
			wantPkg:        "package-examples.wordpress",
			wantRef:        "v1.0.0-beta.64",
			wantManagedRef: false,
		},
		{
			name: "real test case 9",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/kptdev/kpt-samples.git",
					Directory: "echo",
					Ref:       "main",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "https://github.com/kptdev/kpt-samples",
					Directory: "echo",
				},
			},
			wantPkg:        "echo",
			wantRef:        "main",
			wantManagedRef: false,
		},
		{
			name: "real test case 10",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/kptdev/kpt-samples.git",
					Directory: "echo",
					Ref:       "v0",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "https://github.com/kptdev/kpt-samples",
					Directory: "echo",
				},
			},
			wantPkg:        "echo",
			wantRef:        "v0",
			wantManagedRef: false,
		},
		{
			name: "real test case 11",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "http://172.18.255.204:3000/porch/porch-test.git",
					Directory: "r2/r2/r2/r2/r2/r2",
					Ref:       "r2/r2/r2/r2/r2/r2/v1",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "http://172.18.255.204:3000/porch/porch-test",
					Directory: "",
				},
			},
			wantPkg:        "r2.r2.r2.r2.r2.r2",
			wantRef:        "v1",
			wantManagedRef: true,
		},
		{
			name: "real test case 12",
			upstream: &kptfilev1.Upstream{
				Git: &kptfilev1.Git{
					Repo:      "https://github.com/nephio-project/catalog.git",
					Directory: "nephio/core/workload-crds",
					Ref:       "nephio/core/workload-crds/v3.0.0",
				},
			},
			wantRepoSpec: &configapi.RepositorySpec{
				Type: configapi.RepositoryTypeGit,
				Git: &configapi.GitRepository{
					Repo:      "https://github.com/nephio-project/catalog",
					Directory: "",
				},
			},
			wantPkg:        "nephio.core.workload-crds",
			wantRef:        "v3.0.0",
			wantManagedRef: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoSpec, pkg, ref, managedRef, err := GetRepoPackageRefFromUpstream(tt.upstream)

			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, repoSpec)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantRepoSpec, repoSpec)
				assert.Equal(t, tt.wantPkg, pkg)
				assert.Equal(t, tt.wantRef, ref)
				assert.Equal(t, tt.wantManagedRef, managedRef)
			}
		})
	}
}
