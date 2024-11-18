// Copyright 2022, 2024 The kpt and Nephio Authors
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

package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/api/porchconfig/v1alpha1"

	fakecache "github.com/nephio-project/porch/pkg/cache/fake"
	"github.com/nephio-project/porch/pkg/git"
	"github.com/nephio-project/porch/pkg/meta"
	fakemeta "github.com/nephio-project/porch/pkg/meta/fake"
	"github.com/nephio-project/porch/pkg/repository"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func TestLatestPackages(t *testing.T) {
	ctx := context.Background()
	testPath := filepath.Join("..", "..", "git", "testdata")

	cachedRepo := openRepositoryFromArchive(t, ctx, testPath, "nested")

	wantLatest := map[string]string{
		"sample":                    "v2",
		"catalog/empty":             "v1",
		"catalog/gcp/bucket":        "v1",
		"catalog/namespace/basens":  "v3",
		"catalog/namespace/istions": "v3",
	}
	revisions, err := cachedRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	if err != nil {
		t.Fatalf("ListPackageRevisions failed: %v", err)
	}

	gotLatest := map[string]string{}
	for _, pr := range revisions {
		rev, err := pr.GetPackageRevision(ctx)
		if err != nil {
			t.Errorf("didn't expect error, but got %v", err)
		}

		if latest, ok := rev.Labels[api.LatestPackageRevisionKey]; ok {
			if got, want := latest, api.LatestPackageRevisionValue; got != want {
				t.Errorf("%s label value: got %q, want %q", api.LatestPackageRevisionKey, got, want)
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
	testPath := filepath.Join("..", "..", "git", "testdata")
	cachedRepo := openRepositoryFromArchive(t, ctx, testPath, "nested")

	revisions, err := cachedRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Package:       "catalog/gcp/bucket",
		WorkspaceName: "v2",
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
	if got, want := bucket.Lifecycle(), api.PackageRevisionLifecycleDraft; got != want {
		t.Fatalf("Bucket package lifecycle: got %s, want %s", got, want)
	}

	update, err := cachedRepo.UpdatePackageRevision(ctx, bucket)
	if err != nil {
		t.Fatalf("UpdatePackaeg(%s) failed: %v", bucket.Key(), err)
	}
	if err := update.UpdateLifecycle(ctx, api.PackageRevisionLifecyclePublished); err != nil {
		t.Fatalf("UpdateLifecycle failed; %v", err)
	}
	closed, err := update.Close(ctx, "")
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	resource, err := closed.GetPackageRevision(ctx)
	if err != nil {
		t.Errorf("didn't expect error, but got %v", err)
	}
	if got, ok := resource.Labels[api.LatestPackageRevisionKey]; !ok {
		t.Errorf("Label %s not found as expected", api.LatestPackageRevisionKey)
	} else if want := api.LatestPackageRevisionValue; got != want {
		t.Errorf("Latest label: got %s, want %s", got, want)
	}
}

func TestDeletePublishedMain(t *testing.T) {
	ctx := context.Background()
	testPath := filepath.Join("../..", "git", "testdata")
	cachedRepo := openRepositoryFromArchive(t, ctx, testPath, "nested")

	revisions, err := cachedRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Package:       "catalog/gcp/bucket",
		WorkspaceName: "v2",
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
	if got, want := bucket.Lifecycle(), api.PackageRevisionLifecycleDraft; got != want {
		t.Fatalf("Bucket package lifecycle: got %s, want %s", got, want)
	}

	update, err := cachedRepo.UpdatePackageRevision(ctx, bucket)
	if err != nil {
		t.Fatalf("UpdatePackage(%s) failed: %v", bucket.Key(), err)
	}
	if err := update.UpdateLifecycle(ctx, api.PackageRevisionLifecyclePublished); err != nil {
		t.Fatalf("UpdateLifecycle failed; %v", err)
	}
	closed, err := update.Close(ctx, "")
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	_, err = closed.GetPackageRevision(ctx)
	if err != nil {
		t.Errorf("didn't expect error, but got %v", err)
	}

	publishedRevisions, err := cachedRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Package:       "catalog/gcp/bucket",
		WorkspaceName: "v2",
		Lifecycle:     api.PackageRevisionLifecyclePublished,
		Revision:      "main",
	})
	if err != nil {
		t.Fatalf("ListPackageRevisions failed: %v", err)
	}

	// Expect a single result
	if got, want := len(publishedRevisions), 1; got != want {
		t.Fatalf("ListPackageRevisions returned %d packages; want %d", got, want)
	}

	approvedBucket := publishedRevisions[0]

	if got, want := approvedBucket.Lifecycle(), api.PackageRevisionLifecyclePublished; got != want {
		t.Fatalf("Approved Bucket package lifecycle: got %s, want %s", got, want)
	}

	err = approvedBucket.UpdateLifecycle(ctx, api.PackageRevisionLifecycleDeletionProposed)
	if err != nil {
		t.Fatalf("Deletion proposal for approved Bucket failed; %v", err)
	}
	err = cachedRepo.DeletePackageRevision(ctx, approvedBucket)
	if err != nil {
		t.Fatalf("Deleting Main packageRevision failed; %v", err)
	}

	postDeletePublishedRevisions, err := cachedRepo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Package:       "catalog/gcp/bucket",
		WorkspaceName: "v2",
		Lifecycle:     api.PackageRevisionLifecyclePublished,
		Revision:      "main",
	})

	if err != nil {
		t.Fatalf("ListPackageRevisions failed: %v", err)
	}

	//Expect 0 entries
	if got, want := len(postDeletePublishedRevisions), 0; got != want {
		t.Fatalf("ListPackageRevisions returned %d packages; want %d", got, want)
	}

}

func openRepositoryFromArchive(t *testing.T, ctx context.Context, testPath, name string) repository.Repository {
	t.Helper()

	tempdir := t.TempDir()
	tarfile := filepath.Join(testPath, fmt.Sprintf("%s-repository.tar", name))
	_, address := git.ServeGitRepository(t, tarfile, tempdir)
	metadataStore := createMetadataStoreFromArchive(t, fmt.Sprintf("%s-metadata.yaml", name), name)

	cache := NewCache(t.TempDir(), 60*time.Second, true, CacheOptions{
		MetadataStore:      metadataStore,
		ObjectNotifier:     &fakecache.ObjectNotifier{},
		CredentialResolver: &fakecache.CredentialResolver{},
	})
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
	cachedRepo, err := cache.OpenRepository(ctx, apiRepo)
	if err != nil {
		t.Fatalf("OpenRepository(%q) of %q failed; %v", address, tarfile, err)
	}

	t.Cleanup(func() {
		err := cache.CloseRepository(apiRepo, []v1alpha1.Repository{*apiRepo})
		if err != nil {
			t.Errorf("CloseRepository(%q) failed: %v", address, err)
		}
		if len(cache.repositories) != 0 {
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
			Metas: []meta.PackageRevisionMeta{},
		}
	}

	var metas []meta.PackageRevisionMeta
	if err := yaml.Unmarshal(c, &metas); err != nil {
		t.Fatalf("Error unmarshalling metadata file for repository %s", name)
	}

	return &fakemeta.MemoryMetadataStore{
		Metas: metas,
	}
}
