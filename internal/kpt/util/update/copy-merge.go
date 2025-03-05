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
	"io"
	"os"
	"path/filepath"

	"github.com/nephio-project/porch/internal/kpt/errors"
	"github.com/nephio-project/porch/internal/kpt/util/merge"
)

type ComprehensiveUpdater struct{}

func (u ComprehensiveUpdater) Update(options Options) error {
	const op errors.Op = "update.Update"

	err := copyDir(options.OriginPath, options.UpdatedPath, options.LocalPath)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// Simple version of pkgutil.CopyPackage
func copyDir(org string, src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)
		//orgPath := filepath.Join(org, relPath)
		//srcPath := filepath.Join(src, relPath)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		} else {
			// if _, err := os.Stat(dstPath); err == nil {
			// 	// File exists in destination
			// 	if _, err := os.Stat(orgPath); err == nil {
			// 		// File also exists in origin, perform special merge
			// 		return mergeFile(orgPath, srcPath, dstPath)
			// 	}
			// 	// File does not exist in origin, perform simple copy
			// 	return copyFile(path, dstPath)
			// } else if os.IsNotExist(err) {
			// File does not exist in destination, perform simple copy
			return copyFile(path, dstPath)
			// } else {
			// 	return err
			// }
		}
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return out.Close()
}

func mergeFile(org, src, dst string) error {
	const op errors.Op = "update.mergeFile"

	// Call the merge3 tool to perform a 3-way merge
	err := merge.Merge3{
		OriginalPath: org,
		UpdatedPath:  src,
		DestPath:     dst,
		MergeOnPath:  false,
	}.Merge()
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}
