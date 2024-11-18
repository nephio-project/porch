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

package repository

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nephio-project/porch/internal/kpt/util/update"
)

// defaultPackageUpdater implements packageUpdater interface.
type DefaultPackageUpdater struct{}

func (m *DefaultPackageUpdater) Update(
	ctx context.Context,
	localResources,
	originalResources,
	upstreamResources PackageResources) (updatedResources PackageResources, err error) {

	localDir, err := os.MkdirTemp("", "kpt-pkg-update-*")
	if err != nil {
		return PackageResources{}, err
	}
	defer os.RemoveAll(localDir)

	originalDir, err := os.MkdirTemp("", "kpt-pkg-update-*")
	if err != nil {
		return PackageResources{}, err
	}
	defer os.RemoveAll(originalDir)

	upstreamDir, err := os.MkdirTemp("", "kpt-pkg-update-*")
	if err != nil {
		return PackageResources{}, err
	}
	defer os.RemoveAll(upstreamDir)

	if err := writeResourcesToDirectory(localDir, localResources); err != nil {
		return PackageResources{}, err
	}

	if err := writeResourcesToDirectory(originalDir, originalResources); err != nil {
		return PackageResources{}, err
	}

	if err := writeResourcesToDirectory(upstreamDir, upstreamResources); err != nil {
		return PackageResources{}, err
	}

	if err := m.do(ctx, localDir, originalDir, upstreamDir); err != nil {
		return PackageResources{}, err
	}

	return loadResourcesFromDirectory(localDir)
}

// PkgUpdate is a wrapper around `kpt pkg update`, running it against the package in packageDir
func (m *DefaultPackageUpdater) do(_ context.Context, localPkgDir, originalPkgDir, upstreamPkgDir string) error {
	relPath := "."
	localPath := filepath.Join(localPkgDir, relPath)
	updatedPath := filepath.Join(upstreamPkgDir, relPath)
	originPath := filepath.Join(originalPkgDir, relPath)
	isRoot := true

	updateOptions := update.Options{
		RelPackagePath: relPath,
		LocalPath:      localPath,
		UpdatedPath:    updatedPath,
		OriginPath:     originPath,
		IsRoot:         isRoot,
	}
	updater := update.ResourceMergeUpdater{}
	if err := updater.Update(updateOptions); err != nil {
		return err
	}

	return nil
}

func writeResourcesToDirectory(dir string, resources PackageResources) error {
	for k, v := range resources.Contents {
		p := filepath.Join(dir, k)
		dir := filepath.Dir(p)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", dir, err)
		}
		if err := os.WriteFile(p, []byte(v), 0644); err != nil {
			return fmt.Errorf("failed to write file %q: %w", dir, err)
		}
	}
	return nil
}

func loadResourcesFromDirectory(dir string) (PackageResources, error) {
	// TODO: return abstraction instead of loading everything
	result := PackageResources{
		Contents: map[string]string{},
	}
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("cannot compute relative path %q, %q, %w", dir, path, err)
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read file %q: %w", dir, err)
		}
		result.Contents[rel] = string(contents)
		return nil
	}); err != nil {
		return PackageResources{}, err
	}

	return result, nil
}
