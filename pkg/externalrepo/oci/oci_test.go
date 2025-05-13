// Copyright 2022, 2025 The kpt and Nephio Authors
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

package oci

import (
	"context"
	"testing"
	"time"

	"github.com/GoogleContainerTools/kpt/pkg/oci"
	"github.com/nephio-project/porch/v4/pkg/repository"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestBuildPackageRevision(t *testing.T) {
	ociRepo := ociRepository{}
	ociRepo.storage = &oci.Storage{}

	digestName := oci.ImageDigestName{
		Image:  "",
		Digest: "my-digest",
	}
	_, err := ociRepo.buildPackageRevision(context.TODO(), digestName, "package-name", "workspace_name", 1, time.Now())
	assert.True(t, err != nil)

	_, err = ociRepo.buildPackageRevision(context.TODO(), digestName, "package-name", "", 1, time.Now())
	assert.True(t, err != nil)
}

func TestRepoGettersAndSetters(t *testing.T) {
	fakeRepo := ociRepository{}

	assert.Equal(t, nil, fakeRepo.Close(context.TODO()))
}

func TestPackageGettersAndSetters(t *testing.T) {
	fakePr := ociPackageRevision{
		prKey: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				RepoKey: repository.RepositoryKey{
					Name:      "my-repo",
					Namespace: "my-namespace",
				},
				Package: "my-package",
			},
			WorkspaceName: "my-workspace",
		},
	}

	assert.Equal(t, "my-repo.my-package.my-workspace", fakePr.KubeObjectName())
	assert.Equal(t, "my-namespace", fakePr.KubeObjectNamespace())
	assert.Equal(t, types.UID("7007e8aa-0928-50f9-b980-92a44942f055"), fakePr.UID())

	inMeta := metav1.ObjectMeta{}
	inMeta.Name = fakePr.Key().GetPackageKey().RepoKey.Name

	err := fakePr.SetMeta(context.TODO(), inMeta)
	assert.Equal(t, nil, err)
	outMeta := fakePr.GetMeta()
	assert.Equal(t, outMeta.Name, inMeta.Name)

	ociRepo := ociRepository{
		name: "oci-repo-name",
	}
	fakePr.SetRepository(&ociRepo)
	assert.Equal(t, "oci-repo-name", fakePr.parent.name)

	assert.Panics(t, func() { fakePr.ToMainPackageRevision(context.TODO()) }, "The code did not panic")
}
