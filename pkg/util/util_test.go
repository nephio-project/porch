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
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseRepositoryNameOK(t *testing.T) {
	const myRepo = "my-repo"
	const myWS = "my-workspace"

	testCases := map[string]struct {
		pkgRevId string
		expected []string
		err      bool
	}{
		"three-dots": {
			pkgRevId: "my-repo.my-package-name.my-workspace",
			expected: []string{myRepo, "my-package-name", myWS},
			err:      false,
		},
		"four-dots": {
			pkgRevId: "my-repo.my-root-dir.my-package-name.my-workspace",
			expected: []string{myRepo, "my-root-dir.my-package-name", myWS},
			err:      false,
		},
		"five-dots": {
			pkgRevId: "my-repo.my-root-dir.my-sub-dir.my-package-name.my-workspace",
			expected: []string{myRepo, "my-root-dir.my-sub-dir.my-package-name", myWS},
			err:      false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			parsedRepo, _ := ParseRevisionName(tc.pkgRevId)
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
			_, err := ParseRevisionName(tc.pkgRevId)
			if err == nil {
				t.Errorf("expected an error but got no error")
			}
		})
	}
}
