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

package e2e

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// THIS WHOLE FILE IS A BUNCH OF HACKING

const platkrmRepo = "https://github.com/platkrm/test-blueprints.git"
const gcrioPrefix = "gcr.io/kpt-fn"
const gcrioPrefixEnv = "PORCH_GCR_PREFIX_URL"

var (
	testName           = flag.String("porch-cli-test-name", "", "The folder name of the test to run. Leaving empty will skip the test")
	testBlueprintsRepo = flag.String("test-blueprints-repo", "", "What to replace https://github.com/platkrm/test-blueprints.git with. Leave empty to not")
	username           = flag.String("username", "", "Will use this username for authentication with `--test-blueprints-repo`")
	password           = flag.String("password", "", "Will use this password for authentication with `--test-blueprints-repo`")
)

func Test(t *testing.T) {
	flag.Parse()
	if *testName == "" {
		t.Skipf("Skipping test because --porch-cli-test-name is not set")
	}

	testSuite := NewCliTestSuite(t, "./testdata")
	hackedTestCase := ReadTestCaseConfig(t, *testName, "./testdata/"+*testName)

	KubectlCreateNamespace(t, *testName)
	var authSecretName string
	if *username != "" || *password != "" {
		authSecretName = createAuthSecret(t)
	}

	if defaultPrefix := os.Getenv(gcrioPrefixEnv); *testBlueprintsRepo != "" || defaultPrefix != "" {
		for i := range hackedTestCase.Commands {
			if *testBlueprintsRepo != "" {
				replacePlatkrmRepo(&hackedTestCase.Commands[i].Args, authSecretName)
			}
			if defaultPrefix != "" {
				for j := range hackedTestCase.Commands[i].Args {
					hackedTestCase.Commands[i].Args[j] = strings.Replace(hackedTestCase.Commands[i].Args[j], gcrioPrefix, defaultPrefix, 1)
				}
				hackedTestCase.Commands[i].Stderr = strings.Replace(hackedTestCase.Commands[i].Stderr, gcrioPrefix, defaultPrefix, 1)
			}
		}
	}

	testSuite.RunTestCase(t, hackedTestCase)
}

func replacePlatkrmRepo(args *[]string, authSecretName string) {
	found := false
	for i := range *args {
		if (*args)[i] == platkrmRepo {
			(*args)[i] = *testBlueprintsRepo
			found = true
			break
		}
	}
	if found && authSecretName != "" {
		*args = append(*args, "--secret-ref="+authSecretName)
	}
}

func createAuthSecret(t *testing.T) (secretName string) {
	t.Helper()
	const secretFormat = `
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: kubernetes.io/basic-auth
stringData:
  username: %s
  password: %s
`
	secretName = "test-auth-" + uuid.New().String()[0:8]
	secretContent := fmt.Sprintf(secretFormat, secretName, *testName, *username, *password)
	tmp := filepath.Join(t.TempDir(), "secret.yaml")
	err := os.WriteFile(tmp, []byte(secretContent), 777)
	if err != nil {
		t.Fatal(err)
	}
	KubectlApply(t, secretContent)

	t.Cleanup(func() { KubectlDelete(t, secretContent) })

	return secretName
}
