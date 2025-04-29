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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestParseRepositoryNameOK(t *testing.T) {
	const myRepo = "my-repo"

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

func TestGenerateUid(t *testing.T) {
	assert.Equal(t, types.UID("272983f7-a30d-58e0-a809-c38e8638666d"), GenerateUid("my-prefix", "my-namespace", "my.kube.name.of-something"))
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

func TestCompareObjectMeta(t *testing.T) {
	left := metav1.ObjectMeta{}
	right := metav1.ObjectMeta{}
	assert.True(t, CompareObjectMeta(left, right))

	left.Name = "my_name"
	assert.False(t, CompareObjectMeta(left, right))
	right.Name = "my_name"
	assert.True(t, CompareObjectMeta(left, right))

	left.Namespace = "my_ns"
	assert.False(t, CompareObjectMeta(left, right))
	right.Namespace = "my_ns"
	assert.True(t, CompareObjectMeta(left, right))

	left.Labels = map[string]string{}
	assert.False(t, CompareObjectMeta(left, right))
	right.Labels = map[string]string{}
	assert.True(t, CompareObjectMeta(left, right))

	left.Annotations = map[string]string{}
	assert.False(t, CompareObjectMeta(left, right))
	right.Annotations = map[string]string{}
	assert.True(t, CompareObjectMeta(left, right))

	left.Finalizers = []string{}
	assert.False(t, CompareObjectMeta(left, right))
	right.Finalizers = []string{}
	assert.True(t, CompareObjectMeta(left, right))

	left.OwnerReferences = []metav1.OwnerReference{}
	assert.False(t, CompareObjectMeta(left, right))
	right.OwnerReferences = []metav1.OwnerReference{}
	assert.True(t, CompareObjectMeta(left, right))
}

func TestRetryOnErrorConditional(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		shouldRetryFunc func(error) bool
		retryCount      int
		shouldFail      bool
	}{
		{
			name:            "no error",
			err:             nil,
			shouldRetryFunc: func(error) bool { return false },
			retryCount:      2,
			shouldFail:      false,
		},
		{
			name:            "error without retries",
			err:             fmt.Errorf("some error"),
			shouldRetryFunc: func(error) bool { return false },
			retryCount:      2,
			shouldFail:      true,
		},
		{
			name:            "error with retries",
			err:             fmt.Errorf("some error"),
			shouldRetryFunc: func(error) bool { return true },
			retryCount:      10,
			shouldFail:      true,
		},
		{
			name:            "no tries",
			err:             fmt.Errorf("some error"),
			shouldRetryFunc: func(error) bool { return true },
			retryCount:      0,
			shouldFail:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RetryOnErrorConditional(tt.retryCount, tt.shouldRetryFunc, func(_ int) error {
				return tt.err
			})

			if (got != nil) != tt.shouldFail {
				t.Errorf("RetryOnErrorConditional() = %v, shouldFail %v", got, tt.shouldFail)
			}
		})
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
