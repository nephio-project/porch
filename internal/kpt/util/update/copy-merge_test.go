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

package update_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nephio-project/porch/internal/kpt/testutil"
	"github.com/nephio-project/porch/internal/kpt/testutil/pkgbuilder"
	. "github.com/nephio-project/porch/internal/kpt/util/update"
	"github.com/stretchr/testify/assert"
)

const copyMergeLiteral = "copy-merge"

func TestCopyMerge(t *testing.T) {
	testCases := map[string]struct {
		origin         *pkgbuilder.RootPkg
		local          *pkgbuilder.RootPkg
		updated        *pkgbuilder.RootPkg
		relPackagePath string
		isRoot         bool
		expected       *pkgbuilder.RootPkg
	}{
		"only kpt file update": {
			origin: pkgbuilder.NewRootPkg(),
			local: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "A0", "1", copyMergeLiteral),
				),
			updated: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "A0", "22", copyMergeLiteral),
				),
			relPackagePath: "/",
			isRoot:         true,
			expected: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "A0", "22", copyMergeLiteral),
				),
		},
		"new package and subpackage": {
			origin: pkgbuilder.NewRootPkg(),
			local: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "A", "1", copyMergeLiteral),
				),
			updated: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "A", "22", copyMergeLiteral),
				).
				WithResource(pkgbuilder.DeploymentResource).
				WithSubPackages(
					pkgbuilder.NewSubPkg("B").
						WithKptfile(
							pkgbuilder.NewKptfile().
								WithUpstream(kptRepo, "b", "1", copyMergeLiteral),
						).
						WithResource(pkgbuilder.DeploymentResource),
				),
			relPackagePath: "/",
			isRoot:         true,
			expected: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "A", "22", copyMergeLiteral),
				).
				WithResource(pkgbuilder.DeploymentResource).
				WithSubPackages(
					pkgbuilder.NewSubPkg("B").
						WithKptfile(
							pkgbuilder.NewKptfile().
								WithUpstream(kptRepo, "b", "1", copyMergeLiteral),
						).
						WithResource(pkgbuilder.DeploymentResource),
				),
		},
		"adds and update package": {
			origin: pkgbuilder.NewRootPkg(),
			local: pkgbuilder.NewRootPkg().
				WithResource(pkgbuilder.DeploymentResource).
				WithSubPackages(
					pkgbuilder.NewSubPkg("pkgA").
						WithResource(pkgbuilder.DeploymentResource),
					pkgbuilder.NewSubPkg("pkgB").
						WithResource(pkgbuilder.DeploymentResource),
				),
			updated: pkgbuilder.NewRootPkg().
				WithResource(pkgbuilder.DeploymentResource).
				WithSubPackages(
					pkgbuilder.NewSubPkg("pkgA").
						WithResource(pkgbuilder.ConfigMapResource),
					pkgbuilder.NewSubPkg("pkgC").
						WithResource(pkgbuilder.ConfigMapResource),
				),
			relPackagePath: "/",
			isRoot:         true,
			expected: pkgbuilder.NewRootPkg().
				WithResource(pkgbuilder.DeploymentResource).
				WithSubPackages(
					pkgbuilder.NewSubPkg("pkgA").
						WithResource(pkgbuilder.ConfigMapResource).
						WithResource(pkgbuilder.DeploymentResource),
					pkgbuilder.NewSubPkg("pkgB").
						WithResource(pkgbuilder.DeploymentResource),
					pkgbuilder.NewSubPkg("pkgC").
						WithResource(pkgbuilder.ConfigMapResource),
				),
		},
		"updates local subpackages": {
			origin: pkgbuilder.NewRootPkg(),
			local: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "/", "master", copyMergeLiteral).
						WithUpstreamLock(kptRepo, "/", "master", "A"),
				).
				WithResource(pkgbuilder.DeploymentResource).
				WithSubPackages(
					pkgbuilder.NewSubPkg("foo").
						WithKptfile().
						WithResource(pkgbuilder.DeploymentResource),
				),
			updated: pkgbuilder.NewRootPkg().
				WithKptfile(pkgbuilder.NewKptfile().
					WithUpstream(kptRepo, "/A", "newBranch", copyMergeLiteral).
					WithUpstreamLock(kptRepo, "/A", "newBranch", "A"),
				).
				WithResource(pkgbuilder.DeploymentResource).
				WithSubPackages(
					pkgbuilder.NewSubPkg("foo2").
						WithKptfile().
						WithResource(pkgbuilder.ConfigMapResource),
				),
			relPackagePath: "/",
			isRoot:         true,
			expected: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "/A", "newBranch", copyMergeLiteral).
						WithUpstreamLock(kptRepo, "/A", "newBranch", "A"),
				).
				WithResource(pkgbuilder.DeploymentResource).
				WithSubPackages(
					pkgbuilder.NewSubPkg("foo2").
						WithKptfile().
						WithResource(pkgbuilder.ConfigMapResource),
					pkgbuilder.NewSubPkg("foo").
						WithKptfile().
						WithResource(pkgbuilder.DeploymentResource),
				),
		},
		"update existing file in origin, local, and updated": {
			origin: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "/origin", "master", copyMergeLiteral),
				).
				WithResource(pkgbuilder.DeploymentResource),
			local: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "/origin", "master", copyMergeLiteral),
				).
				WithResource(pkgbuilder.DeploymentResource),
			updated: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "/origin", "master", copyMergeLiteral).
						WithUpstreamLock(kptRepo, "/origin", "master", "abc123"),
				),
			relPackagePath: "/",
			isRoot:         true,
			expected: pkgbuilder.NewRootPkg().
				WithKptfile(
					pkgbuilder.NewKptfile().
						WithUpstream(kptRepo, "/origin", "master", copyMergeLiteral).
						WithUpstreamLock(kptRepo, "/origin", "master", "abc123"),
				).
				WithResource(pkgbuilder.DeploymentResource),
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {

			repos := testutil.EmptyReposInfo
			origin := tc.origin.ExpandPkg(t, repos)
			local := tc.local.ExpandPkg(t, repos)
			updated := tc.updated.ExpandPkg(t, repos)
			expected := tc.expected.ExpandPkg(t, repos)

			updater := &CopyMergeUpdater{}

			err := updater.Update(Options{
				RelPackagePath: tc.relPackagePath,
				OriginPath:     filepath.Join(origin, tc.relPackagePath),
				LocalPath:      filepath.Join(local, tc.relPackagePath),
				UpdatedPath:    filepath.Join(updated, tc.relPackagePath),
				IsRoot:         tc.isRoot,
			})
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			testutil.KptfileAwarePkgEqual(t, local, expected, false)

		})
	}
}

func TestCopyMergeError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0644)
	os.RemoveAll(src)

	updater := &CopyMergeUpdater{}
	options := Options{
		UpdatedPath: src,
		LocalPath:   dst,
		IsRoot:      true,
	}

	err := updater.Update(options)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")

}

func TestCopyMergeErrorCopyingFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	srcFile := filepath.Join(src, "file.txt")
	os.WriteFile(srcFile, []byte("content"), 0644)

	dstFile := filepath.Join(dst, "file.txt")
	os.Mkdir(dstFile, 0755)

	updater := &CopyMergeUpdater{}
	options := Options{
		UpdatedPath: src,
		LocalPath:   dst,
		IsRoot:      true,
	}

	err := updater.Update(options)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is a directory")
}
