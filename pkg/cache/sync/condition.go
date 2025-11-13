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

package sync

import (
	"context"
	"fmt"
	"time"

	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache/util"
	"github.com/nephio-project/porch/pkg/repository"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetRepositoryCondition sets the repository condition status
func SetRepositoryCondition(ctx context.Context, coreClient client.WithWatch, repoKey repository.RepositoryKey, status util.RepositoryStatus, syncError error, nextSyncTime *time.Time) error {
	if coreClient == nil {
		return fmt.Errorf("client is nil")
	}

	repo := &configapi.Repository{}
	key := types.NamespacedName{
		Name:      repoKey.Name,
		Namespace: repoKey.Namespace,
	}

	if err := coreClient.Get(ctx, key, repo); err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}

	errorMsg := ""
	if status == util.RepositoryStatusError && syncError != nil {
		errorMsg = syncError.Error()
	}

	condition, err := util.BuildRepositoryCondition(repo, status, errorMsg, nextSyncTime)
	if err != nil {
		return err
	}

	return util.ApplyRepositoryCondition(ctx, coreClient, repo, condition, status)
}
