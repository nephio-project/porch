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
			_, err := ParseRevisionName(tc.pkgRevId)
			if err == nil {
				t.Errorf("expected an error but got no error")
			}
		})
	}
}
