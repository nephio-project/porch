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

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
)

// We take advantage of the cache having a global view of all the packages
// in a repository and compute the latest package revision in the cache
// rather than add another level of caching in the repositories themselves.
// This also reuses the revision comparison code and ensures same behavior
// between Git and OCI.

var _ repository.PackageRevision = &cachedPackageRevision{}

type cachedPackageRevision struct {
	repository.PackageRevision
	isLatestRevision bool
}

func (c *cachedPackageRevision) GetPackageRevision(ctx context.Context) (*api.PackageRevision, error) {
	apiPR, err := c.PackageRevision.GetPackageRevision(ctx)
	if err != nil {
		return nil, err
	}

	apiPR.Annotations = c.GetMeta().Annotations
	apiPR.Finalizers = c.GetMeta().Finalizers
	apiPR.OwnerReferences = c.GetMeta().OwnerReferences
	apiPR.DeletionTimestamp = c.GetMeta().DeletionTimestamp
	apiPR.Labels = c.GetMeta().Labels

	if c.isLatestRevision {
		// copy the labels in case the cached object is being read by another go routine
		labels := make(map[string]string, len(apiPR.Labels))
		for k, v := range apiPR.Labels {
			labels[k] = v
		}
		labels[api.LatestPackageRevisionKey] = api.LatestPackageRevisionValue
		apiPR.Labels = labels
	}

	return apiPR, nil
}
