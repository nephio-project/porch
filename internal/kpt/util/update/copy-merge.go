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

type CopyMergeUpdater struct{}

func (u CopyMergeUpdater) Update(options Options) error {
	return copyDirectories(options.UpdatedPath, options.LocalPath, options.IsRoot)
}

func copyDirectories(src, dst string, isRoot bool) error {
	const op errors.Op = "update.Update"
	if err := kptfileutil.UpdateKptfile(dst, src, dst, true); err != nil {
		return errors.E(op, types.UniquePath(dst), err)
	}
	if err := pkgutil.CopyPackage(src, dst, isRoot, pkg.All); err != nil {
		return errors.E(op, types.UniquePath(dst), err)
	}
	return nil
}
