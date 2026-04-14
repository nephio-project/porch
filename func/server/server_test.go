// Copyright 2022-2025 The kpt and Nephio Authors
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

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	m.Run()
}

func TestRunBaseline(t *testing.T) {

	// Make main() exit early (test-only behavior added in run()) to avoid starting gRPC server.
	err := os.Setenv("TEST_MAIN_EXIT", "1")
	assert.Nil(t, err)
	// Use an ephemeral port to avoid collisions.
	origArgs := os.Args
	os.Args = []string{origArgs[0], "-port=0"}
	defer func() {
		os.Args = origArgs
		_ = os.Unsetenv("TEST_MAIN_EXIT")
	}()

	// Call main() directly; run() will return early due to TEST_MAIN_EXIT and main will exit without calling os.Exit.
	main()

}

func TestRun(t *testing.T) {
	t.Run("invalid port", func(t *testing.T) {
		o := &options{
			port:            -1,     // invalid port -> net.Listen should fail
			disableRuntimes: "exec", // disable exec so we don't need other env/configs
		}
		err := run(o)
		assert.NotNil(t, err)
		assert.ErrorContains(t, err, "failed to listen")
	})
	t.Run("wrapper image env error", func(t *testing.T) {
		fakeKubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:1
  name: fake
contexts:
- context:
    cluster: fake
    user: fake
  name: fake
current-context: fake
users:
- name: fake
  user:
    token: fake`

		kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
		err := os.WriteFile(kubeconfigPath, []byte(fakeKubeconfig), 0600)
		require.NoError(t, err)
		t.Setenv("KUBECONFIG", kubeconfigPath)

		o := &options{
			port:            0,      // ephemeral port, net.Listen should succeed
			disableRuntimes: "exec", // disable exec so pod runtime is attempted
		}
		// Ensure the wrapper server image env var is not set
		_ = os.Unsetenv(wrapperServerImageEnv)

		err = run(o)
		assert.ErrorContains(t, err, "environment variable WRAPPER_SERVER_IMAGE must be set to use pod function evaluator runtime")
	})
}
