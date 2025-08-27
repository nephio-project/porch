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

package update

import (
	"github.com/nephio-project/porch/internal/kpt/errors"
	"github.com/nephio-project/porch/internal/kpt/pkg"
	"github.com/nephio-project/porch/internal/kpt/types"
	"github.com/nephio-project/porch/internal/kpt/util/pkgutil"
	"github.com/nephio-project/porch/pkg/kpt/kptfileutil"
)

// CopyMergeUpdater is responsible for synchronizing the destination package
// with the source package by updating the Kptfile and copying and replacing package contents.
type CopyMergeUpdater struct{}

// Update synchronizes the destination/local package with the source/update package by updating the Kptfile
// and copying package contents. It deletes resources from the destination package if they were present
// in the original package, but not present anymore in the source package.
// It takes an Options struct as input, which specifies the paths
// and other parameters for the update operation. Returns an error if the update fails.
func (u CopyMergeUpdater) Update(options Options) error {
	const op errors.Op = "update.Update"

	dst := options.LocalPath
	src := options.UpdatedPath
	org := options.OriginPath

	if err := kptfileutil.UpdateKptfile(dst, src, options.OriginPath, true); err != nil {
		return errors.E(op, types.UniquePath(dst), err)
	}
	if err := pkgutil.CopyPackage(src, dst, options.IsRoot, pkg.All); err != nil {
		return errors.E(op, types.UniquePath(dst), err)
	}
	if err := pkgutil.RemoveStaleItems(org, src, dst, options.IsRoot, pkg.All); err != nil {
		return errors.E(op, types.UniquePath(dst), err)
	}
	return nil
}
