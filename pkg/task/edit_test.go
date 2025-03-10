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

package task

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/externalrepo/fake"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/nephio-project/porch/pkg/repository"
)

func TestEdit(t *testing.T) {
	pkg := "pkg"
	packageName := "repo.1234567890.ws"
	repositoryName := "repo"
	revision := "v1"
	packageRevision := &fake.FakePackageRevision{
		Name: packageName,
		PackageRevisionKey: repository.PackageRevisionKey{
			Package:    pkg,
			Repository: repositoryName,
			Revision:   revision,
		},
		PackageLifecycle: v1alpha1.PackageRevisionLifecyclePublished,
		Resources: &v1alpha1.PackageRevisionResources{
			Spec: v1alpha1.PackageRevisionResourcesSpec{
				PackageName:    pkg,
				Revision:       revision,
				RepositoryName: repositoryName,
				Resources: map[string]string{
					kptfile.KptFileName: strings.TrimSpace(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: example
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  description: sample description
					`),
				},
			},
		},
	}
	repo := &fake.Repository{
		PackageRevisions: []repository.PackageRevision{
			packageRevision,
		},
	}
	repoOpener := &fakeRepositoryOpener{
		repository: repo,
	}

	epm := editPackageMutation{
		task: &v1alpha1.Task{
			Type: "edit",
			Edit: &v1alpha1.PackageEditTaskSpec{
				Source: &v1alpha1.PackageRevisionRef{
					Name: packageName,
				},
			},
		},

		namespace:         "test-namespace",
		packageName:       pkg,
		repositoryName:    repositoryName,
		referenceResolver: &fakeReferenceResolver{},
		repoOpener:        repoOpener,
	}

	res, _, err := epm.apply(context.Background(), repository.PackageResources{})
	if err != nil {
		t.Errorf("task apply failed: %v", err)
	}

	want := strings.TrimSpace(`
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: example
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  description: sample description
	`)
	got := strings.TrimSpace(res.Contents[kptfile.KptFileName])
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

// Implementation of the ReferenceResolver interface for testing.
type fakeReferenceResolver struct{}

func (f *fakeReferenceResolver) ResolveReference(ctx context.Context, namespace, name string, result repository.Object) error {
	return nil
}

type fakeRepositoryOpener struct {
	repository repository.Repository
}

func (f *fakeRepositoryOpener) OpenRepository(ctx context.Context, repositorySpec *configapi.Repository) (repository.Repository, error) {
	return f.repository, nil
}
