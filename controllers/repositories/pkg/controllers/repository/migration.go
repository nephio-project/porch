// Copyright 2026 The kpt Authors
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
	"maps"
	"strings"

	porchv1alpha1 "github.com/kptdev/porch/api/porch/v1alpha1"
	configapi "github.com/kptdev/porch/api/porchconfig/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Migration support: enriches v1alpha2 CRDs with v1alpha1 metadata during hybrid-mode transitions.

// isMigrating checks if a repository is in active v1alpha2 migration mode.
func isMigrating(repo *configapi.Repository) bool {
	if repo == nil || repo.Annotations == nil {
		return false
	}
	return repo.Annotations[configapi.AnnotationKeyV1Alpha2Migration] == configapi.AnnotationValueMigrationEnabled
}

// enrichMetadataFromV1Alpha1 queries v1alpha1 PackageRevision metadata to preserve user-defined labels/annotations.
func (r *RepositoryReconciler) enrichMetadataFromV1Alpha1(ctx context.Context, repo *configapi.Repository, gitPackageName string) (map[string]string, map[string]string) {
	log := log.FromContext(ctx)

	// Only enrich during active migration
	if !isMigrating(repo) {
		return nil, nil
	}

	// Query v1alpha1 PackageRevision from aggregated API.
	var v1Pr porchv1alpha1.PackageRevision
	prName := gitPackageName // repo controller uses same name for git packages and v1 PRs

	err := r.Get(ctx, client.ObjectKey{
		Namespace: repo.Namespace,
		Name:      prName,
	}, &v1Pr)

	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.V(3).Info("Failed to query v1alpha1 PR for metadata enrichment",
				"pr", prName, "repo", repo.Name, "err", err)
		}
		// Not found is OK; package may not have a v1 counterpart yet
		return nil, nil
	}

	log.V(5).Info("Enriching git package with v1alpha1 PR metadata",
		"pr", prName, "labels", len(v1Pr.Labels), "annotations", len(v1Pr.Annotations))

	// Extract non-system labels and all annotations
	labels := make(map[string]string)
	for k, v := range v1Pr.Labels {
		if !systemLabels[k] {
			labels[k] = v
		}
	}

	annotations := make(map[string]string)
	for k, v := range v1Pr.Annotations {
		// Exclude internal porch annotations like render requests
		if !strings.HasPrefix(k, "porch.kpt.dev/") {
			annotations[k] = v
		}
	}

	return labels, annotations
}

// mergeMetadata combines git and v1alpha1 metadata (git takes precedence).
func mergeMetadata(gitMetadata, v1Metadata map[string]string) map[string]string {
	if len(gitMetadata) == 0 && len(v1Metadata) == 0 {
		return nil
	}

	result := make(map[string]string)

	// Start with v1 metadata, override with git metadata
	maps.Copy(result, v1Metadata)

	// Override with git metadata (git takes precedence)
	maps.Copy(result, gitMetadata)

	return result
}
