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
	"os"
	"path/filepath"

	"github.com/nephio-project/porch/v4/internal/kpt/errors"
	"github.com/nephio-project/porch/v4/internal/kpt/pkg"
	"github.com/nephio-project/porch/v4/internal/kpt/types"
	"github.com/nephio-project/porch/v4/internal/kpt/util/pkgutil"
)

type CopyMergeUpdater struct{}

func (u CopyMergeUpdater) Update(options Options) error {
	const op errors.Op = "update.Update"

	err := copyDir(options.UpdatedPath, options.LocalPath, options.IsRoot)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

func copyDir(src, dst string, isRoot bool) error {
	const op errors.Op = "update.Update"
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath) // file in the local fork
		srcPath := filepath.Join(src, relPath) // file in the updated fork
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		} else {
			if err = pkgutil.CopyPackage(srcPath, dstPath, isRoot, pkg.All); err != nil {
				return errors.E(op, types.UniquePath(dstPath), err)
			}
			return nil
		}
	})
}
