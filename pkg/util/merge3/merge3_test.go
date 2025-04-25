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

package merge3

import (
	"path/filepath"
	"testing"

	"github.com/nephio-project/porch/third_party/GoogleContainerTools/kpt-functions-sdk/go/fn"
	"github.com/stretchr/testify/suite"
)

const (
	testDataDir = "testdata"
	originalPrr = "original.yaml"
	updatedPrr  = "updated.yaml"
	destPrr     = "destination.yaml"

	testAppName = "test-app"
	testCrName  = "test-fruit-store"
)

type Merge3TestSuite struct {
	suite.Suite
}

func TestMerge3(t *testing.T) {
	suite.Run(t, &Merge3TestSuite{})
}

type testCase struct {
	dir     string
	crds    []string
	checkFn func(*Merge3TestSuite, fn.KubeObjects)
}

var testCases = []testCase{
	{
		dir:     "simple-conflict",
		checkFn: basicImageCheck,
	},
	{
		// TODO: expand
		dir:     "simple-subpackage-conflict",
		checkFn: basicImageCheck,
	},
	{
		dir:     "one-key-crd",
		crds:    []string{"fruitstore.crd.yaml"},
		checkFn: assocListMergeCheck,
	},
	{
		dir:     "infer-crd",
		checkFn: assocListMergeCheck,
	},
}

func (t *Merge3TestSuite) TestMerge3() {
	for _, tc := range testCases {
		fullpath, err := filepath.Abs(filepath.Join(testDataDir, tc.dir))
		t.Require().NoError(err)
		orig := filepath.Join(fullpath, originalPrr)
		updated := filepath.Join(fullpath, updatedPrr)
		local := filepath.Join(fullpath, destPrr)

		addSchemas := t.parseCrds(fullpath, tc.crds)
		t.Run(tc.dir, func() { t.innerTest(orig, updated, local, addSchemas, tc.checkFn) })
	}
}
