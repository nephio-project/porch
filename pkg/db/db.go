// Copyright 2024 The Nephio Authors
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

package db

import (
	"context"
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"

	"github.com/nephio-project/porch/pkg/repository"
	"go.opentelemetry.io/otel"
	"k8s.io/klog/v2"
)

const (
	uuidSpace = "aac71d91-5c67-456f-8fd2-902ef6da820e"
)

var tracer = otel.Tracer("db")

func OpenRepository(ctx context.Context, name, namespace string, spec *configapi.DBRepository, deployment bool) (repository.Repository, error) {
	klog.Infof("DB Repo: %q -n %q %q %q", name, namespace, spec.Driver, spec.DataSource)

	if err := OpenDBConnection(spec.Driver, spec.DataSource); err != nil {
		return nil, err
	}

	dbRepo := dbRepository{
		repoKey: repository.RepositoryKey{
			Namespace:  namespace,
			Repository: name,
		},
		updated:    time.Now(),
		updatedBy:  getCurrentUser(),
		deployment: deployment,
	}

	return dbRepo.OpenRepository()
}
