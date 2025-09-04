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

package repository

import (
	"context"
	"strings"
	"testing"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	kptfilev1 "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/storage"
)

func TestRepositoryKey(t *testing.T) {
	repoKey := RepositoryKey{
		Namespace:         "my-ns",
		Name:              "my-repo",
		Path:              "my/dir/path",
		PlaceholderWSname: "my-ws-name",
	}

	assert.Equal(t, "my-ns:my-repo:my/dir/path:my-ws-name", repoKey.String())
	assert.Equal(t, repoKey, repoKey)

	otherRepoKey := RepositoryKey{}
	assert.True(t, otherRepoKey.Matches(repoKey))

	otherRepoKey.Namespace = "other-ns"
	otherRepoKey.Name = "other-repo"
	otherRepoKey.Path = "other/dir/path"
	otherRepoKey.PlaceholderWSname = "other-ws-name"
	assert.False(t, otherRepoKey.Matches(repoKey))

	otherRepoKey.Namespace = "my-ns"
	assert.False(t, otherRepoKey.Matches(repoKey))

	otherRepoKey.Name = "my-repo"
	assert.False(t, otherRepoKey.Matches(repoKey))

	otherRepoKey.Path = "my/dir/path"
	assert.False(t, otherRepoKey.Matches(repoKey))

	otherRepoKey.Path = "my/dir/path"
	otherRepoKey.PlaceholderWSname = "my-ws-name"
	assert.True(t, otherRepoKey.Matches(repoKey))

	copiedRepoKey := RepositoryKey{}
	repoKey.DeepCopy(&copiedRepoKey)
	assert.Equal(t, copiedRepoKey, repoKey)
}

func TestPackageKey(t *testing.T) {
	pkgKey := PackageKey{
		Path:    "my/pkg/path",
		Package: "my-package-name",
	}

	assert.Equal(t, "::::my/pkg/path:my-package-name", pkgKey.String())
	assert.Equal(t, pkgKey, pkgKey)

	otherPkgKey := PackageKey{}
	assert.True(t, otherPkgKey.Matches(pkgKey))

	otherPkgKey.Path = "other/pkg/path"
	otherPkgKey.Package = "other-ws-name"
	assert.False(t, otherPkgKey.Matches(pkgKey))

	otherPkgKey.Path = "my/pkg/path"
	assert.False(t, otherPkgKey.Matches(pkgKey))

	otherPkgKey.Package = "my-package-name"
	assert.True(t, otherPkgKey.Matches(pkgKey))

	assert.Equal(t, "my/pkg/path/my-package-name", pkgKey.ToPkgPathname())
	assert.Equal(t, "my/pkg/path/my-package-name", pkgKey.ToFullPathname())

	pkgKey.RepoKey.Path = "dir/path"
	assert.Equal(t, "dir/path/my/pkg/path/my-package-name", pkgKey.ToFullPathname())

	testRepoKey := RepositoryKey{
		Namespace:         "ns",
		Name:              "repo",
		Path:              "dir/path",
		PlaceholderWSname: "ws-name",
	}
	pkgKey.RepoKey = testRepoKey
	assert.Equal(t, pkgKey, FromFullPathname(testRepoKey, pkgKey.ToPkgPathname()))
	assert.Equal(t, pkgKey, FromFullPathname(pkgKey.RKey(), pkgKey.ToPkgPathname()))

	assert.Equal(t, "ns", pkgKey.K8SNS())
	assert.Equal(t, "repo.my.pkg.path.my-package-name", pkgKey.K8SName())

	pkgKey.RepoKey = RepositoryKey{}
	assert.Equal(t, pkgKey, FromFullPathname(pkgKey.RKey(), pkgKey.ToPkgPathname()))

	pkgKey.Path = ""
	assert.Equal(t, pkgKey, FromFullPathname(pkgKey.RKey(), pkgKey.ToPkgPathname()))

	copiedPkgKey := PackageKey{}
	pkgKey.DeepCopy(&copiedPkgKey)
	assert.Equal(t, copiedPkgKey, pkgKey)

	parPRKey, err := PkgK8sName2Key("my-ns", "repo.pkg.path.package-name.my-ws-name")
	assert.Nil(t, err)
	assert.Equal(t, "repo", parPRKey.RKey().Name)

	_, err = PkgK8sName2Key("my-ns", "aaa")
	assert.NotNil(t, err)

	assert.Equal(t, "pkg", K8SName2PkgName("repo.pkg"))

}

func TestPackageRevisionKey(t *testing.T) {
	pkgRevKey := PackageRevisionKey{
		Revision:      1,
		WorkspaceName: "my-ws-name",
	}

	assert.Equal(t, "::::::1:my-ws-name", pkgRevKey.String())
	assert.Equal(t, pkgRevKey, pkgRevKey)

	otherPkgRevKey := PackageRevisionKey{}
	assert.True(t, otherPkgRevKey.Matches(pkgRevKey))

	otherPkgRevKey.Revision = 2
	otherPkgRevKey.WorkspaceName = "other-ws-name"
	assert.False(t, otherPkgRevKey.Matches(pkgRevKey))

	otherPkgRevKey.Revision = 1
	assert.False(t, otherPkgRevKey.Matches(pkgRevKey))

	otherPkgRevKey.WorkspaceName = "my-ws-name"
	assert.True(t, otherPkgRevKey.Matches(pkgRevKey))

	testPkgKey := PackageKey{
		Path:    "pkg/path",
		Package: "package-name",
	}
	pkgRevKey.PkgKey = testPkgKey
	assert.Equal(t, testPkgKey, pkgRevKey.PKey())

	testRepoKey := RepositoryKey{
		Namespace:         "ns",
		Name:              "repo",
		Path:              "dir/path",
		PlaceholderWSname: "ws-name",
	}
	pkgRevKey.PkgKey.RepoKey = testRepoKey
	assert.Equal(t, testRepoKey, pkgRevKey.RKey())

	copiedPkgRevKey := PackageRevisionKey{}
	pkgRevKey.DeepCopy(&copiedPkgRevKey)
	assert.Equal(t, copiedPkgRevKey, pkgRevKey)

	assert.Equal(t, "ns", pkgRevKey.K8SNS())
	assert.Equal(t, "repo.pkg.path.package-name.my-ws-name", pkgRevKey.K8SName())
	assert.Equal(t, "ws-name", K8SName2PkgRevWSName("pkg-name", "pkg-name.ws-name"))

	parPRKey, err := PkgRevK8sName2Key("my-ns", "repo.pkg.path.package-name.my-ws-name")
	assert.Nil(t, err)
	assert.Equal(t, "repo", parPRKey.RKey().Name)

	_, err = PkgRevK8sName2Key("my-ns", "")
	assert.NotNil(t, err)
}

func TestGetPRWorkspaceName(t *testing.T) {
	_, err := PkgRevK8sName2Key("my-ns", "")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "package name part \"\" of object name invalid"))

	_, err = PkgRevK8sName2Key("my-ns", "hello")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "package name part \"\" of object name invalid"))

	prKey, err := PkgRevK8sName2Key("my-ns", "repo.hello.there")
	assert.Nil(t, err)
	assert.Equal(t, "hello", prKey.PkgKey.Package)
	assert.Equal(t, "", prKey.PkgKey.Path)
	assert.Equal(t, "there", prKey.WorkspaceName)

	prKey, err = PkgRevK8sName2Key("my-ns", "repo..")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "package name part \"\" of object name invalid"))

	prKey, err = PkgRevK8sName2Key("my-ns", "v.")
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "workspace name part \"\" of package revision name invalid"))

	prKey, err = PkgRevK8sName2Key("my-ns", "repo.hello.v1.2.3")
	assert.Nil(t, err)
	assert.Equal(t, "hello", prKey.PkgKey.Package)
	assert.Equal(t, "", prKey.PkgKey.Path)
	assert.Equal(t, "v1.2.3", prKey.WorkspaceName)

	prKey, err = PkgRevK8sName2Key("my-ns", "repo.hello.v1.2")
	assert.Nil(t, err)
	assert.Equal(t, "hello", prKey.PkgKey.Package)
	assert.Equal(t, "", prKey.PkgKey.Path)
	assert.Equal(t, "v1.2", prKey.WorkspaceName)

	prKey, err = PkgRevK8sName2Key("my-ns", "repo.hello.v1")
	assert.Nil(t, err)
	assert.Equal(t, "hello", prKey.PkgKey.Package)
	assert.Equal(t, "", prKey.PkgKey.Path)
	assert.Equal(t, "v1", prKey.WorkspaceName)

	prKey, err = PkgRevK8sName2Key("my-ns", "repo.hello.v1.v1")
	assert.Nil(t, err)
	assert.Equal(t, "v1", prKey.PkgKey.Package)
	assert.Equal(t, "hello", prKey.PkgKey.Path)
	assert.Equal(t, "v1", prKey.WorkspaceName)

	prKey, err = PkgRevK8sName2Key("my-ns", "repo.hello.v1.2.3.v4.5.6")
	assert.Nil(t, err)
	assert.Equal(t, "3", prKey.PkgKey.Package)
	assert.Equal(t, "hello/v1/2", prKey.PkgKey.Path)
	assert.Equal(t, "v4.5.6", prKey.WorkspaceName)

	prKey, err = PkgRevK8sName2Key("my-ns", "repo.hello.v1.2.3.end")
	assert.Nil(t, err)
	assert.Equal(t, "3", prKey.PkgKey.Package)
	assert.Equal(t, "hello/v1/2", prKey.PkgKey.Path)
	assert.Equal(t, "end", prKey.WorkspaceName)
}

func TestRepositoryKey_K8SNS(t *testing.T) {
	k := RepositoryKey{
		Namespace: "foo",
	}
	require.Equal(t, "foo", k.K8SNS())
}

func TestRepositoryKey_K8SName(t *testing.T) {
	k := RepositoryKey{
		Name: "foo",
	}
	require.Equal(t, "foo", k.K8SName())
}

func TestListPackageRevisionFilter_Matches(t *testing.T) {
	tests := []struct {
		name     string
		filter   ListPackageRevisionFilter
		p        PackageRevision
		negative bool
	}{
		{
			name:   "key matches",
			filter: ListPackageRevisionFilter{Key: PackageRevisionKey{PkgKey: PackageKey{RepoKey: RepositoryKey{Namespace: "ns"}}}},
			p:      &fakePackageRevision{namespace: "ns"},
		},
		{
			name:     "key doesn't match",
			filter:   ListPackageRevisionFilter{Key: PackageRevisionKey{PkgKey: PackageKey{RepoKey: RepositoryKey{Namespace: "ns2"}}}},
			p:        &fakePackageRevision{namespace: "ns"},
			negative: true,
		},
		{
			name:   "lifecycle matches",
			filter: ListPackageRevisionFilter{Lifecycles: []api.PackageRevisionLifecycle{"Published"}},
			p:      &fakePackageRevision{lifecycle: api.PackageRevisionLifecyclePublished},
		},
		{
			name:     "lifecycle doesn't match",
			filter:   ListPackageRevisionFilter{Lifecycles: []api.PackageRevisionLifecycle{"Published"}},
			p:        &fakePackageRevision{lifecycle: api.PackageRevisionLifecycleDeletionProposed},
			negative: true,
		},
		{
			name:   "predicate matches",
			filter: ListPackageRevisionFilter{Predicate: &storage.SelectionPredicate{Field: fields.Set{"spec.repository": "someRepo"}.AsSelector(), Label: labels.Everything()}},
			p:      &fakePackageRevision{repoName: "someRepo"},
		},
		{
			name:     "predicate doesn't match",
			filter:   ListPackageRevisionFilter{Predicate: &storage.SelectionPredicate{Field: fields.Set{"spec.repository": "someReopWithAMisspelling"}.AsSelector(), Label: labels.Everything()}},
			p:        &fakePackageRevision{repoName: "someRepo"},
			negative: true,
		},
		{
			name:   "predicate matches name",
			filter: ListPackageRevisionFilter{Predicate: &storage.SelectionPredicate{Field: fields.Set{"metadata.name": "somePackageRevision"}.AsSelector(), Label: labels.Everything()}},
			p:      &fakePackageRevision{name: "somePackageRevision"},
		},
		{
			name:   "predicate matches namespace",
			filter: ListPackageRevisionFilter{Predicate: &storage.SelectionPredicate{Field: fields.Set{"metadata.namespace": "someNamespace"}.AsSelector(), Label: labels.Everything()}},
			p:      &fakePackageRevision{namespace: "someNamespace"},
		},
		{
			name:   "predicate matches revision",
			filter: ListPackageRevisionFilter{Predicate: &storage.SelectionPredicate{Field: fields.Set{"spec.revision": "1"}.AsSelector(), Label: labels.Everything()}},
			p:      &fakePackageRevision{revision: 1},
		},
		{
			name:   "predicate matches package name",
			filter: ListPackageRevisionFilter{Predicate: &storage.SelectionPredicate{Field: fields.Set{"spec.packageName": "someSortOfRadio"}.AsSelector(), Label: labels.Everything()}},
			p:      &fakePackageRevision{packageName: "someSortOfRadio"},
		},
		{
			name:   "predicate matches multi-folder package name",
			filter: ListPackageRevisionFilter{Predicate: &storage.SelectionPredicate{Field: fields.Set{"spec.packageName": "someSortOfNetwork/someSortOfRadio"}.AsSelector(), Label: labels.Everything()}},
			p:      &fakePackageRevision{packagePath: "someSortOfNetwork", packageName: "someSortOfRadio"},
		},
		{
			name:   "predicate matches repository name",
			filter: ListPackageRevisionFilter{Predicate: &storage.SelectionPredicate{Field: fields.Set{"spec.repository": "someRepo"}.AsSelector(), Label: labels.Everything()}},
			p:      &fakePackageRevision{repoName: "someRepo"},
		},
		{
			name:   "predicate matches workspace name",
			filter: ListPackageRevisionFilter{Predicate: &storage.SelectionPredicate{Field: fields.Set{"spec.workspaceName": "main"}.AsSelector(), Label: labels.Everything()}},
			p:      &fakePackageRevision{workspaceName: "main"},
		},
		{
			name:   "predicate matches lifecycle",
			filter: ListPackageRevisionFilter{Predicate: &storage.SelectionPredicate{Field: fields.Set{"spec.lifecycle": "Published"}.AsSelector(), Label: labels.Everything()}},
			p:      &fakePackageRevision{lifecycle: api.PackageRevisionLifecyclePublished},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.negative {
				require.False(t, tt.filter.Matches(context.TODO(), tt.p))
			} else {
				require.True(t, tt.filter.Matches(context.TODO(), tt.p))
			}
		})
	}
}

func TestListPackageFilter_Matches(t *testing.T) {
	f := &ListPackageFilter{Key: PackageKey{RepoKey: RepositoryKey{Namespace: "ns"}}}
	p := &fakePackage{namespace: "ns"}
	require.True(t, f.Matches(p))
}

type fakePackageRevision struct {
	name          string
	namespace     string
	lifecycle     api.PackageRevisionLifecycle
	packagePath   string
	packageName   string
	revision      int
	repoName      string
	workspaceName string
}

func (f *fakePackageRevision) GetPackageRevision(ctx context.Context) (*api.PackageRevision, error) {
	return &api.PackageRevision{ObjectMeta: metav1.ObjectMeta{Namespace: f.namespace}}, nil
}
func (f *fakePackageRevision) KubeObjectNamespace() string { return f.namespace }
func (f *fakePackageRevision) Key() PackageRevisionKey {
	return PackageRevisionKey{
		PkgKey: PackageKey{
			RepoKey: RepositoryKey{
				Namespace: f.namespace,
				Name:      f.repoName,
			},
			Path:    f.packagePath,
			Package: f.packageName,
		},
		Revision:      f.revision,
		WorkspaceName: f.workspaceName,
	}
}
func (f *fakePackageRevision) KubeObjectName() string                           { return f.name }
func (f *fakePackageRevision) UID() types.UID                                   { return "" }
func (f *fakePackageRevision) SetMeta(context.Context, metav1.ObjectMeta) error { return nil }
func (f *fakePackageRevision) ResourceVersion() string                          { return "" }
func (f *fakePackageRevision) Lifecycle(context.Context) api.PackageRevisionLifecycle {

	return api.PackageRevisionLifecycle(f.lifecycle)
}
func (f *fakePackageRevision) GetResources(context.Context) (*api.PackageRevisionResources, error) {
	return nil, nil
}
func (f *fakePackageRevision) UpdateLifecycle(context.Context, api.PackageRevisionLifecycle) error {
	return nil
}
func (f *fakePackageRevision) GetUpstreamLock(context.Context) (kptfilev1.Upstream, kptfilev1.UpstreamLock, error) {
	return kptfilev1.Upstream{}, kptfilev1.UpstreamLock{}, nil
}
func (f *fakePackageRevision) GetKptfile(context.Context) (kptfilev1.KptFile, error) {
	return kptfilev1.KptFile{}, nil
}
func (f *fakePackageRevision) GetLock() (kptfilev1.Upstream, kptfilev1.UpstreamLock, error) {
	return kptfilev1.Upstream{}, kptfilev1.UpstreamLock{}, nil
}
func (f *fakePackageRevision) ToMainPackageRevision(context.Context) PackageRevision {
	return f
}
func (f *fakePackageRevision) GetMeta() metav1.ObjectMeta {
	return metav1.ObjectMeta{Namespace: f.namespace}
}

type fakePackage struct {
	namespace      string
	latestRevision int
}

func (f *fakePackage) GetPackage(ctx context.Context) *api.PorchPackage {
	return &api.PorchPackage{ObjectMeta: metav1.ObjectMeta{Namespace: f.namespace}}
}
func (f *fakePackage) KubeObjectNamespace() string { return f.namespace }
func (f *fakePackage) Key() PackageKey {
	return PackageKey{RepoKey: RepositoryKey{Namespace: f.namespace}}
}
func (f *fakePackage) KubeObjectName() string                    { return "" }
func (f *fakePackage) GetLatestRevision(ctx context.Context) int { return f.latestRevision }
