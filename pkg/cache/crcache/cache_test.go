// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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

package crcache

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache/crcache/meta"
	fakemeta "github.com/nephio-project/porch/pkg/cache/crcache/meta/fake"
	fakecache "github.com/nephio-project/porch/pkg/cache/fake"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/externalrepo/git"
	externalrepotypes "github.com/nephio-project/porch/pkg/externalrepo/types"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func TestLatestPackages(t *testing.T) {
	ctx := context.Background()
	testPath := filepath.Join("..", "..", "externalrepo", "git", "testdata")

	cachedRepo := openRepositoryFromArchive(t, ctx, testPath, "nested")

	wantLatest := map[string]int{
		"sample":                    2,
		"catalog/empty":             1,
		"catalog/gcp/bucket":        1,
		"catalog/namespace/basens":  3,
		"catalog/namespace/istions": 3,
	}
	revisions, err := cachedRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	if err != nil {
		t.Fatalf("ListPackageRevisions failed: %v", err)
	}

	gotLatest := map[string]int{}
	for _, pr := range revisions {
		rev, err := pr.GetPackageRevision(ctx)
		if err != nil {
			t.Errorf("didn't expect error, but got %v", err)
		}

		if latest, ok := rev.Labels[porchapi.LatestPackageRevisionKey]; ok {
			if got, want := latest, porchapi.LatestPackageRevisionValue; got != want {
				t.Errorf("%s label value: got %q, want %q", porchapi.LatestPackageRevisionKey, got, want)
				continue
			}

			if existing, ok := gotLatest[rev.Spec.PackageName]; ok {
				t.Errorf("Multiple latest package revisions for package %q: %q and %q",
					rev.Spec.PackageName, rev.Spec.Revision, existing)
			}

			// latest package
			gotLatest[rev.Spec.PackageName] = rev.Spec.Revision
		}
	}

	if !cmp.Equal(wantLatest, gotLatest) {
		t.Errorf("Latest package revisions differ (-want,+got): %s", cmp.Diff(wantLatest, gotLatest))
	}
}

func TestPublishedLatest(t *testing.T) {
	ctx := context.Background()
	testPath := filepath.Join("..", "..", "externalrepo", "git", "testdata")
	cachedRepo := openRepositoryFromArchive(t, ctx, testPath, "nested")

	revisions, err := cachedRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Key: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				Path:    "catalog/gcp",
				Package: "bucket",
			},
			WorkspaceName: "v2",
		},
	})
	if err != nil {
		t.Fatalf("ListPackageRevisions failed: %v", err)
	}

	// Expect a single result
	if got, want := len(revisions), 1; got != want {
		t.Fatalf("ListPackageRevisions returned %d packages; want %d", got, want)
	}

	bucket := revisions[0]
	// Expect draft package
	if got, want := bucket.Lifecycle(ctx), porchapi.PackageRevisionLifecycleDraft; got != want {
		t.Fatalf("Bucket package lifecycle: got %s, want %s", got, want)
	}

	update, err := cachedRepo.UpdatePackageRevision(ctx, bucket)
	if err != nil {
		t.Fatalf("UpdatePackaeg(%s) failed: %v", bucket.Key(), err)
	}
	if err := update.UpdateLifecycle(ctx, porchapi.PackageRevisionLifecyclePublished); err != nil {
		t.Fatalf("UpdateLifecycle failed; %v", err)
	}
	closed, err := cachedRepo.ClosePackageRevisionDraft(ctx, update, 0)
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	resource, err := closed.GetPackageRevision(ctx)
	if err != nil {
		t.Errorf("didn't expect error, but got %v", err)
	}
	if got, ok := resource.Labels[porchapi.LatestPackageRevisionKey]; !ok {
		t.Errorf("Label %s not found as expected", porchapi.LatestPackageRevisionKey)
	} else if want := porchapi.LatestPackageRevisionValue; got != want {
		t.Errorf("Latest label: got %s, want %s", got, want)
	}
}

func TestDeletePublishedMain(t *testing.T) {
	ctx := context.Background()
	testPath := filepath.Join("../..", "externalrepo", "git", "testdata")
	cachedRepo := openRepositoryFromArchive(t, ctx, testPath, "nested")

	// Verify that top-level package "sample" has a main branch
	publishedRevisionsMain, err := cachedRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Key: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				Package: "sample",
			},
			WorkspaceName: "main",
			Revision:      -1,
		},
		Lifecycles: []porchapi.PackageRevisionLifecycle{porchapi.PackageRevisionLifecyclePublished},
	})
	if err != nil {
		t.Fatalf("ListPackageRevisions failed: %v", err)
	}

	// Expect a single result for top-level package
	if got, want := len(publishedRevisionsMain), 1; got != want {
		t.Fatalf("ListPackageRevisions returned %d packages; want %d", got, want)
	}

	approvedSample := publishedRevisionsMain[0]

	if got, want := approvedSample.Lifecycle(ctx), porchapi.PackageRevisionLifecyclePublished; got != want {
		t.Fatalf("Approved Sample package lifecycle: got %s, want %s", got, want)
	}

	err = approvedSample.UpdateLifecycle(ctx, porchapi.PackageRevisionLifecycleDeletionProposed)
	if err != nil {
		t.Fatalf("Deletion proposal for approved Sample failed; %v", err)
	}
	err = cachedRepo.DeletePackageRevision(ctx, approvedSample)
	if err != nil {
		t.Fatalf("Deleting Main packageRevision failed; %v", err)
	}

	postDeletePublishedRevisions, err := cachedRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Key: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				Package: "sample",
			},
			WorkspaceName: "main",
			Revision:      -1,
		},
		Lifecycles: []porchapi.PackageRevisionLifecycle{porchapi.PackageRevisionLifecyclePublished},
	})

	if err != nil {
		t.Fatalf("ListPackageRevisions failed: %v", err)
	}

	// Expect 0 entries after deletion
	if got, want := len(postDeletePublishedRevisions), 0; got != want {
		t.Fatalf("ListPackageRevisions returned %d packages; want %d", got, want)
	}

	// Verify that subpackages do NOT have main branches
	subpackageMain, err := cachedRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Key: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				Path:    "catalog/gcp",
				Package: "bucket",
			},
			WorkspaceName: "main",
			Revision:      -1,
		},
	})
	if err != nil {
		t.Fatalf("ListPackageRevisions failed: %v", err)
	}

	// Expect 0 entries - subpackages should NOT have main branches
	if got, want := len(subpackageMain), 0; got != want {
		t.Errorf("Subpackage should not have main branch: got %d packages; want %d", got, want)
	}
}

func openRepositoryFromArchive(t *testing.T, ctx context.Context, testPath, name string) repository.Repository {
	t.Helper()

	tempdir := t.TempDir()
	tarfile := filepath.Join(testPath, fmt.Sprintf("%s-repository.tar", name))
	_, address := git.ServeGitRepository(t, tarfile, tempdir)
	metadataStore := createMetadataStoreFromArchive(t, fmt.Sprintf("%s-metadata.yaml", name), name)

	apiRepo := &v1alpha1.Repository{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.TypeRepository.Kind,
			APIVersion: v1alpha1.TypeRepository.APIVersion(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1alpha1.RepositorySpec{
			Deployment: false,
			Type:       v1alpha1.RepositoryTypeGit,
			Git: &v1alpha1.GitRepository{
				Repo: address,
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	fakeClient := k8sfake.NewClientBuilder().WithScheme(scheme).WithObjects(apiRepo).Build()
	cache := &Cache{
		repositories:  map[repository.RepositoryKey]*cachedRepository{},
		locks:         map[repository.RepositoryKey]*sync.Mutex{},
		mainLock:      &sync.RWMutex{},
		metadataStore: metadataStore,
		options: cachetypes.CacheOptions{
			ExternalRepoOptions: externalrepotypes.ExternalRepoOptions{
				LocalDirectory:             t.TempDir(),
				UseUserDefinedCaBundle:     true,
				CredentialResolver:         &fakecache.CredentialResolver{},
				RepoOperationRetryAttempts: 3,
			},
			CoreClient:           fakeClient,
			RepoSyncFrequency:    60 * time.Second,
			RepoPRChangeNotifier: &fakecache.ObjectNotifier{},
		}}
	cachedRepo, err := cache.OpenRepository(ctx, apiRepo)
	if err != nil {
		t.Fatalf("OpenRepository(%q) of %q failed; %v", address, tarfile, err)
	}

	t.Cleanup(func() {
		err := cache.CloseRepository(ctx, apiRepo, []v1alpha1.Repository{*apiRepo})
		if err != nil {
			t.Errorf("CloseRepository(%q) failed: %v", address, err)
		}
		if len(cache.GetRepositories()) != 0 {
			t.Errorf("CloseRepository hasn't deleted repository from cache")
		}
	})
	return cachedRepo
}

func createMetadataStoreFromArchive(t *testing.T, testPath, name string) meta.MetadataStore {
	t.Helper()

	f := filepath.Join("..", "git", "testdata", testPath)
	c, err := os.ReadFile(f)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("Error reading metadata file found for repository %s", name)
	}
	if os.IsNotExist(err) {
		return &fakemeta.MemoryMetadataStore{
			Metas: []metav1.ObjectMeta{},
		}
	}

	var metas []metav1.ObjectMeta
	if err := yaml.Unmarshal(c, &metas); err != nil {
		t.Fatalf("Error unmarshalling metadata file for repository %s", name)
	}

	return &fakemeta.MemoryMetadataStore{
		Metas: metas,
	}
}
