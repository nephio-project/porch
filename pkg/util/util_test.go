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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gotest.tools/assert"
)

func TestParseRepositoryNameOK(t *testing.T) {
	const myRepo = "my-repo"
	const myWS = "my-workspace"

	testCases := map[string]struct {
		pkgRevId string
		expected string
	}{
		"three-dots": {
			pkgRevId: "my-repo.my-package-name.my-workspace",
			expected: myRepo,
		},
		"four-dots": {
			pkgRevId: "my-repo.my-root-dir.my-package-name.my-workspace",
			expected: myRepo,
		},
		"five-dots": {
			pkgRevId: "my-repo.my-root-dir.my-sub-dir.my-package-name.my-workspace",
			expected: myRepo,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			parsedRepo, _ := ParsePkgRevObjNameField(tc.pkgRevId, 0)
			if diff := cmp.Diff(tc.expected, parsedRepo); diff != "" {
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
		repo          string
		dir           string
		pkg           string
		ws            string
		err           bool
		prErrString   string
		repoErrString string
		dirErrString  string
		pkgErrString  string
		wsErrString   string
	}{
		"empty string": {
			repo:          "",
			dir:           "",
			pkg:           "",
			ws:            "",
			err:           true,
			prErrString:   "",
			repoErrString: "must start and end with an alphanumeric character",
			dirErrString:  "",
			pkgErrString:  "must start and end with an alphanumeric character",
			wsErrString:   "must start and end with an alphanumeric character",
		},
		"64 characters long": {
			repo:          "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsdk",
			dir:           "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsdk",
			pkg:           "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsdk",
			ws:            "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsdk",
			err:           true,
			prErrString:   "",
			repoErrString: "must be no more than 63 characters",
			dirErrString:  "must be no more than 63 characters",
			pkgErrString:  "must be no more than 63 characters",
			wsErrString:   "must be no more than 63 characters",
		},
		"all 63 characters long": {
			repo:          "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsk",
			dir:           "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsk",
			pkg:           "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsk",
			ws:            "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsk",
			err:           true,
			prErrString:   "must be no more than 253 characters",
			repoErrString: "",
			dirErrString:  "",
			pkgErrString:  "",
			wsErrString:   "",
		},
		"first two 63 characters long": {
			repo:          "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsk",
			dir:           "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsk",
			pkg:           "abc",
			ws:            "abc",
			err:           false,
			prErrString:   "",
			repoErrString: "",
			dirErrString:  "",
			pkgErrString:  "",
			wsErrString:   "",
		},
		"last two 63 characters long": {
			repo:          "abc",
			dir:           "abc",
			pkg:           "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsk",
			ws:            "abcedfhglaasdkfuaweoihfjghldhsgufhgaakjsdhaflkasdjflksadjfsalsk",
			err:           false,
			prErrString:   "",
			repoErrString: "",
			dirErrString:  "",
			pkgErrString:  "",
			wsErrString:   "",
		},
		"starts with -": {
			repo:          "-hello",
			dir:           "-hello",
			pkg:           "-hello",
			ws:            "-hello",
			err:           true,
			prErrString:   "",
			repoErrString: "and must start and end with an alphanumeric character",
			dirErrString:  "and must start and end with an alphanumeric character",
			pkgErrString:  "and must start and end with an alphanumeric character",
			wsErrString:   "and must start and end with an alphanumeric character",
		},
		"ends with -": {
			repo:          "hello-",
			dir:           "hello-",
			pkg:           "hello-",
			ws:            "hello-",
			err:           true,
			prErrString:   "",
			repoErrString: "and must start and end with an alphanumeric character",
			dirErrString:  "and must start and end with an alphanumeric character",
			pkgErrString:  "and must start and end with an alphanumeric character",
			wsErrString:   "and must start and end with an alphanumeric character",
		},
		"has - in the middle": {
			repo:          "hel-lo-wor-ld",
			dir:           "hel-lo-wor-ld",
			pkg:           "hel-lo-wor-ld",
			ws:            "hel-lo-wor-ld",
			err:           false,
			prErrString:   "",
			repoErrString: "",
			dirErrString:  "",
			pkgErrString:  "",
			wsErrString:   "",
		},
		"has uppercase alphanumeric characters": {
			repo:          "hElLo",
			dir:           "hElLo",
			pkg:           "hElLo",
			ws:            "hElLo",
			err:           true,
			prErrString:   "",
			repoErrString: "must consist of lower case alphanumeric characters",
			dirErrString:  "must consist of lower case alphanumeric characters",
			pkgErrString:  "must consist of lower case alphanumeric characters",
			wsErrString:   "must consist of lower case alphanumeric characters",
		},
		"has other characters": {
			repo:          "hEzzo",
			dir:           "hel_o",
			pkg:           "hE^lo",
			ws:            "hE(lo",
			err:           true,
			prErrString:   "",
			repoErrString: "must consist of lower case alphanumeric characters",
			dirErrString:  "must consist of lower case alphanumeric characters",
			pkgErrString:  "must consist of lower case alphanumeric characters",
			wsErrString:   "must consist of lower case alphanumeric characters",
		},
		"has slashes": {
			repo:          "he/lo",
			dir:           "he/lo",
			pkg:           "he/lo",
			ws:            "he/lo",
			err:           true,
			prErrString:   "",
			repoErrString: "must consist of lower case alphanumeric characters",
			dirErrString:  "",
			pkgErrString:  "must consist of lower case alphanumeric characters",
			wsErrString:   "must consist of lower case alphanumeric characters",
		},
		"has repeated slashes": {
			repo:          "he//lo",
			dir:           "he//lo",
			pkg:           "he//lo",
			ws:            "he//lo",
			err:           true,
			prErrString:   "",
			repoErrString: "must consist of lower case alphanumeric characters",
			dirErrString:  "consecutive '/' characters are not allowed",
			pkgErrString:  "must consist of lower case alphanumeric characters",
			wsErrString:   "must consist of lower case alphanumeric characters",
		},
		"has repeated dots": {
			repo:          "he..lo",
			dir:           "he..lo",
			pkg:           "he..lo",
			ws:            "he..lo",
			err:           true,
			prErrString:   "",
			repoErrString: "must consist of lower case alphanumeric characters",
			dirErrString:  "must consist of lower case alphanumeric characters",
			pkgErrString:  "must consist of lower case alphanumeric characters",
			wsErrString:   "must consist of lower case alphanumeric characters",
		},
		"has repeated dots in composed": {
			repo:          "hello",
			dir:           "/h/e/l/l/o/",
			pkg:           "hello",
			ws:            "hello",
			err:           false,
			prErrString:   "",
			repoErrString: "",
			dirErrString:  "",
			pkgErrString:  "",
			wsErrString:   "",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			got := ValidPkgRevObjName(tc.repo, tc.dir, tc.pkg, tc.ws)
			if got == nil {
				if tc.err == true {
					t.Errorf("didn't get an an error when expecting one")
				}
				return
			}

			errorSlice := strings.Split(got.Error(), "\n")

			assert.Equal(t, errorSlice[0], "package revision object name invalid:")

			assertExpectedPartErrExists(t, errorSlice,
				tc.prErrString,
				"complete object name ",
				ComposePkgRevObjName(tc.repo, tc.dir, tc.pkg, tc.ws))

			assertExpectedPartErrExists(t, errorSlice,
				tc.repoErrString,
				"repository name ",
				tc.repo)

			assertExpectedPartErrExists(t, errorSlice,
				tc.dirErrString,
				"directory name ",
				tc.dir)

			assertExpectedPartErrExists(t, errorSlice,
				tc.pkgErrString,
				"package name ",
				tc.pkg)

			assertExpectedPartErrExists(t, errorSlice,
				tc.wsErrString,
				"workspace name ",
				tc.ws)
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
		assert.Assert(t, strings.Contains(partErrMsg, errString))
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
