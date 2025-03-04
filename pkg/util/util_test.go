// Copyright 2022 The kpt and Nephio Authors
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

package util

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v2"
	"gotest.tools/assert"
)

func TestParseRepositoryName(t *testing.T) {
	testCases := map[string]struct {
		pkgRevId string
		expected string
		err      error
	}{
		"correct-rev-3-dash": {
			pkgRevId: "my-package-name-akoshjadfhijao[ifj[adsfj[adsf",
			expected: "my-package-name",
			err:      nil,
		},
		"correct-rev-1-dash": {
			pkgRevId: "mypackagename-akoshjadfhijao[ifj[adsfj[adsf",
			expected: "mypackagename",
			err:      nil,
		},
		"correct-rev-1-dash-no-suffix": {
			pkgRevId: "mypackagename-",
			expected: "mypackagename",
			err:      nil,
		},
		"no-dash": {
			pkgRevId: "mypackagenameakoshjadfhijao[ifj[adsfj[adsf",
			expected: "",
			err:      fmt.Errorf("malformed package revision name; expected at least one hyphen: \"mypackagenameakoshjadfhijao[ifj[adsfj[adsf\""),
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			repoName, err := ParseRevisionName(tc.pkgRevId)
			if tc.err != err && tc.err.Error() != err.Error() {
				t.Errorf("unexpected error: expected %q, got %q", tc.err, err)
			}
			if diff := cmp.Diff(tc.expected, repoName); diff != "" {
				t.Errorf("unexpected diff (+got,-want): %s", diff)
			}
		})
	}
}

func TestParseRepositoryNameErr(t *testing.T) {
	testCases := map[string]struct {
		pkgRevId string
	}{
		"no-dot": {
			pkgRevId: "my-repomy-package-namemy-workspace",
		},
		"one-dot-one": {
			pkgRevId: "my-repomy-package-name.my-workspace",
		},
		"one-dot-two": {
			pkgRevId: "my-repo.my-package-namemy-workspace",
		},
		"rev-3-dash": {
			pkgRevId: "my-package-name-akoshjadfhijao[ifj[adsfj[adsf",
		},
		"rev-1-dash": {
			pkgRevId: "mypackagename-akoshjadfhijao[ifj[adsfj[adsf",
		},
		"rev-1-dash-no-suffix": {
			pkgRevId: "mypackagename-",
		},
		"no-dash": {
			pkgRevId: "mypackagenameakoshjadfhijao[ifj[adsfj[adsf",
		},
		"empty": {
			pkgRevId: "",
		},
		"white-space": {
			pkgRevId: "   \t \n  ",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			_, err := ParsePkgRevObjNameField(tc.pkgRevId, 0)
			if err == nil {
				t.Errorf("expected an error but got no error")
			}
		})
	}
}

func TestValidatePkgRevName(t *testing.T) {
	testCases := map[string]struct {
		Repo          string
		Dir           string
		Pkg           string
		Ws            string
		Err           bool
		PrErrString   string
		RepoErrString string
		DirErrString  string
		PkgErrString  string
		WsErrString   string
	}{}

	testFileName := filepath.Join("testdata", "pkg-rev-name.yaml")

	testCasesBA, err := os.ReadFile(testFileName)
	if err != nil {
		t.Errorf("could not read test case data from %s: %v", testFileName, err)
		return
	}

	err = yaml.Unmarshal(testCasesBA, &testCases)
	if err != nil {
		t.Errorf("could not unmarshal test case data from %s: %v", testFileName, err)
		return
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			got := ValidPkgRevObjName(tc.Repo, tc.Dir, tc.Pkg, tc.Ws)
			if got == nil {
				if tc.Err == true {
					t.Errorf("didn't get an an error when expecting one")
				}
				return
			}

			errorSlice := strings.Split(got.Error(), "\n")

			assert.Equal(t, errorSlice[0], "package revision object name invalid:")

			assertExpectedPartErrExists(t, errorSlice,
				tc.PrErrString,
				"complete object name ",
				ComposePkgRevObjName(tc.Repo, tc.Dir, tc.Pkg, tc.Ws))

			assertExpectedPartErrExists(t, errorSlice,
				tc.RepoErrString,
				"repository name ",
				tc.Repo)

			assertExpectedPartErrExists(t, errorSlice,
				tc.DirErrString,
				"directory name ",
				tc.Dir)

			assertExpectedPartErrExists(t, errorSlice,
				tc.PkgErrString,
				"package name ",
				tc.Pkg)

			assertExpectedPartErrExists(t, errorSlice,
				tc.WsErrString,
				"workspace name ",
				tc.Ws)
		})
	}
}

func assertExpectedPartErrExists(t *testing.T, errorSlice []string, errString, prefix, part string) {
	if len(errString) == 0 {
		return
	}

	errStart := prefix + part + " invalid:"

	partErrMsg := getPartErrMsg(errorSlice, errStart)
	if len(partErrMsg) == 0 {
		t.Errorf("expected to find error message starting with: %q", errStart)
	} else {
		if result := strings.Contains(partErrMsg, errString); result != true {
			t.Errorf("error message %q should contain %q", partErrMsg, errString)
		}
	}
}

func getPartErrMsg(errorSlice []string, start string) string {
	for i := range errorSlice {
		if strings.HasPrefix(errorSlice[i], start) {
			return errorSlice[i]
		}
	}

	return ""
}
