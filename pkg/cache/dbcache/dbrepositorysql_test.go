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
	"strings"
	"testing"
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRepoDBWriteRead(t *testing.T) {

	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace:         "my-ns",
			Name:              "my-repo",
			Path:              "my-path",
			PlaceholderWSname: "my-placeholder-ws",
		},
		meta:       metav1.ObjectMeta{},
		spec:       nil,
		updated:    time.Now().UTC(),
		updatedBy:  "porchuser",
		deployment: false,
	}

	dbRepoUpdate := dbRepository{
		repoKey: dbRepo.Key(),
		meta: metav1.ObjectMeta{
			Name:      "meta-new-name",
			Namespace: "meta-new-namespace",
		},
		spec:       &configapi.Repository{},
		updated:    time.Now().UTC(),
		updatedBy:  "porchuser2",
		deployment: false,
	}

	repoDBWriteReadTest(t, dbRepo, dbRepoUpdate)

	dbRepo.repoKey.Path = ""
	dbRepo.repoKey.PlaceholderWSname = ""
	dbRepoUpdate.repoKey.Path = ""
	dbRepoUpdate.repoKey.PlaceholderWSname = ""
	repoDBWriteReadTest(t, dbRepo, dbRepoUpdate)

	dbRepoUpdate = dbRepository{}
	dbRepoUpdate.repoKey = dbRepo.repoKey
	repoDBWriteReadTest(t, dbRepo, dbRepoUpdate)
}

func TestRepoDBSchema(t *testing.T) {
	dbRepo := dbRepository{}
	err := repoWriteToDB(context.TODO(), &dbRepo)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates check constraint"))

	dbRepo.repoKey = repository.RepositoryKey{
		Namespace: "my-namespace",
	}

	err = repoWriteToDB(context.TODO(), &dbRepo)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates check constraint"))

	dbRepo.repoKey.Name = "my-name"
	err = repoWriteToDB(context.TODO(), &dbRepo)
	assert.Nil(t, err)

	dbRepo.repoKey.Path = "my-path"
	err = repoUpdateDB(context.TODO(), &dbRepo)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "update not allowed on immutable columns"))

	dbRepo.repoKey.Path = ""
	dbRepo.updatedBy = "homer"
	err = repoUpdateDB(context.TODO(), &dbRepo)
	assert.Nil(t, err)

	dbRepo.repoKey.PlaceholderWSname = "my-ws"
	err = repoUpdateDB(context.TODO(), &dbRepo)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "update not allowed on immutable columns"))

	dbRepo.repoKey.PlaceholderWSname = ""
	dbRepo.updatedBy = "lisa"
	err = repoUpdateDB(context.TODO(), &dbRepo)
	assert.Nil(t, err)

	err = repoDeleteFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)
}

func repoDBWriteReadTest(t *testing.T, dbRepo, dbRepoUpdate dbRepository) {
	err := repoWriteToDB(context.TODO(), &dbRepo)
	assert.Nil(t, err)

	readRepo, err := repoReadFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)
	assertReposEqual(t, &dbRepo, readRepo)

	err = repoWriteToDB(context.TODO(), &dbRepoUpdate)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "violates unique constraint"))

	err = repoUpdateDB(context.TODO(), &dbRepoUpdate)
	assert.Nil(t, err)

	readRepo, err = repoReadFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)
	assertReposEqual(t, &dbRepoUpdate, readRepo)

	err = repoDeleteFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)

	_, err = repoReadFromDB(context.TODO(), dbRepo.Key())
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "no rows in result set"))

	err = repoUpdateDB(context.TODO(), &dbRepoUpdate)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "no rows or multiple rows found for updating"))

	err = repoDeleteFromDB(context.TODO(), dbRepo.Key())
	assert.Nil(t, err)
}

func assertReposEqual(t *testing.T, left, right *dbRepository) {
	assert.Equal(t, left.Key(), right.Key())
	assert.Equal(t, left.meta.Namespace, right.meta.Namespace)
	assert.Equal(t, left.meta.Name, right.meta.Name)
	assert.Equal(t, left.spec, right.spec)
	assert.Equal(t, left.updatedBy, right.updatedBy)
}
