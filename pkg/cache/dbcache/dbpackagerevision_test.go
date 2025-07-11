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

package dbcache

import (
	"context"
	"testing"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/nephio-project/porch/pkg/repository"
	mockcachetypes "github.com/nephio-project/porch/test/mockery/mocks/porch/pkg/cache/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDBPackageRevision(t *testing.T) {
	mockCache := mockcachetypes.NewMockCache(t)
	cachetypes.CacheInstance = mockCache

	externalrepo.ExternalRepoInUnitTestMode = true

	ctx := context.TODO()

	testRepo := createTestRepo(t, "my-ns", "my-repo-name")
	testRepo.spec = &configapi.Repository{
		Spec: configapi.RepositorySpec{
			Git: &configapi.GitRepository{
				Repo: "https://aurl/repo.git",
			},
		},
	}
	mockCache.EXPECT().GetRepository(mock.Anything).Return(&testRepo).Maybe()

	err := testRepo.OpenRepository(ctx, externalrepotypes.ExternalRepoOptions{})
	assert.Nil(t, err)

	newPRDef := v1alpha1.PackageRevision{
		Spec: v1alpha1.PackageRevisionSpec{
			RepositoryName: "my-repo-name",
			PackageName:    "my-package",
			WorkspaceName:  "my-workspace",
		},
	}

	newPRDraft, err := testRepo.CreatePackageRevisionDraft(ctx, &newPRDef)
	assert.Nil(t, err)
	assert.NotNil(t, newPRDraft)

	dbPR, err := testRepo.ClosePackageRevisionDraft(ctx, newPRDraft, -1)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR)

	assert.Equal(t, "main", dbPR.ToMainPackageRevision(ctx).Key().WorkspaceName)
	dbPR.(*dbPackageRevision).pkgRevKey.PkgKey.RepoKey.PlaceholderWSname = "my-branch"
	assert.Equal(t, "my-branch", dbPR.ToMainPackageRevision(ctx).Key().WorkspaceName)

	meta := dbPR.GetMeta()
	assert.Equal(t, meta.Name, "")

	assert.Nil(t, dbPR.SetMeta(ctx, metav1.ObjectMeta{}))

	prDef, err := dbPR.GetPackageRevision(ctx)
	assert.Nil(t, err)
	assert.Equal(t, "my-workspace", prDef.Spec.WorkspaceName)

	assert.Equal(t, "my-ns", dbPR.KubeObjectNamespace())
	assert.Equal(t, "my-repo-name.my-package.my-workspace", dbPR.KubeObjectName())
	prKey := repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: repository.RepositoryKey{
				Namespace:         "my-ns",
				Name:              "my-repo-name",
				PlaceholderWSname: "my-branch",
			},
			Package: "my-package",
		},
		WorkspaceName: "my-workspace",
	}
	assert.Equal(t, prKey, dbPR.Key())
	assert.Equal(t, v1alpha1.PackageRevisionLifecycleDraft, dbPR.Lifecycle(ctx))

	newPrUp, newPrUpLock, err := dbPR.GetUpstreamLock(ctx)
	assert.NotNil(t, err)
	assert.Nil(t, newPrUp.Git)
	assert.Nil(t, newPrUpLock.Git)

	newPrUp, newPrUpLock, err = dbPR.GetLock()
	assert.Nil(t, err)
	assert.NotNil(t, newPrUp.Git)
	assert.NotNil(t, newPrUpLock.Git)

	prResources, err := dbPR.GetResources(ctx)
	assert.Nil(t, err)
	assert.NotNil(t, prResources)
	assert.Equal(t, 0, len(prResources.Spec.Resources))

	newDBPR := dbPR.(*dbPackageRevision)

	prResources.Spec.Resources["Kptfile"] = `apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: my-kptfile
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  site: https://nephio.org
  description: some kpt package.`

	err = newDBPR.UpdateResources(ctx, prResources, &v1alpha1.Task{})
	assert.Nil(t, err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR)

	gotKptFile, err := newDBPR.GetKptfile(ctx)
	assert.Nil(t, err)
	assert.Equal(t, "Kptfile", gotKptFile.Kind)

	err = dbPR.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecycleProposed)
	assert.Nil(t, err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR)

	err = dbPR.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecyclePublished)
	assert.Nil(t, err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR)

	dbPRdb := dbPR.(*dbPackageRevision)
	dbPR2 := dbPackageRevision{
		repo: dbPRdb.repo,
		pkgRevKey: repository.PackageRevisionKey{
			PkgKey:        dbPR.Key().PKey(),
			Revision:      0,
			WorkspaceName: "my-workspace-2",
		},
		lifecycle: v1alpha1.PackageRevisionLifecycleDraft,
		tasks:     dbPRdb.tasks,
		resources: dbPRdb.resources,
	}

	err = dbPR2.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecycleProposed)
	assert.Nil(t, err)

	dbPR2i, err := testRepo.ClosePackageRevisionDraft(ctx, &dbPR2, 0)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR2i)

	err = dbPR2i.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecyclePublished)
	assert.Nil(t, err)

	dbPR2i, err = testRepo.ClosePackageRevisionDraft(ctx, &dbPR2, 0)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR2i)

	fakeRepo := testRepo.externalRepo.(*fake.Repository)
	fakeExtPR := fake.FakePackageRevision{
		PrKey: dbPR.Key(),
	}
	fakeRepo.PackageRevisions = append(fakeRepo.PackageRevisions, &fakeExtPR)

	dbPR.(*dbPackageRevision).lifecycle = v1alpha1.PackageRevisionLifecyclePublished
	dbPR.(*dbPackageRevision).pkgRevKey.Revision = 1
	err = dbPR.UpdateLifecycle(ctx, v1alpha1.PackageRevisionLifecycleDeletionProposed)
	assert.Nil(t, err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR)

	prDef, err = dbPR.GetPackageRevision(ctx)
	assert.Nil(t, err)
	assert.Equal(t, v1alpha1.PackageRevisionLifecycleDeletionProposed, prDef.Spec.Lifecycle)

	prResources.Spec.Resources["Kptfile"] = `apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: my-kptfile
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  site: https://nephio.org
  description: some kpt package.
upstream:
  type: git
  git:
    repo: http://172.18.255.200:3000/nephio/rpkg-update.git
    directory: basens-edit
    ref: drafts/basens-edit/update-1
upstreamLock:
  type: git
  git:
    repo: http://172.18.255.200:3000/nephio/rpkg-update.git
    directory: basens-edit
    ref: drafts/basens-edit/update-1
    commit: 960e1b80b5245874e46ba2b3090b27deaa61eb9a`

	newDBPR2 := dbPR.(*dbPackageRevision)

	err = newDBPR2.UpdateResources(ctx, prResources, &v1alpha1.Task{})
	assert.Nil(t, err)

	dbPR, err = testRepo.ClosePackageRevisionDraft(ctx, dbPR.(repository.PackageRevisionDraft), 0)
	assert.Nil(t, err)
	assert.NotNil(t, dbPR)

	prDef, err = dbPR.GetPackageRevision(ctx)
	assert.Nil(t, err)
	assert.Equal(t, "basens-edit", prDef.Status.UpstreamLock.Git.Directory)

	err = testRepo.DeletePackageRevision(ctx, dbPR)
	assert.Nil(t, err)

	err = testRepo.Close(ctx)
	assert.Nil(t, err)
}
