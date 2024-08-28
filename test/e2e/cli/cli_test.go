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

package e2e

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	updateGoldenFiles       = "UPDATE_GOLDEN_FILES"
	testGitName             = "git-server"
	testGitNamespace        = "test-git-namespace"
	defaultTestGitServerUrl = "http://" + testGitName + "." + testGitNamespace + ".svc.cluster.local:8080"
)

func TestPorch(t *testing.T) {
	e2e := os.Getenv("E2E")
	if e2e == "" {
		t.Skip("set E2E to run this test")
	}

	abs, err := filepath.Abs(filepath.Join(".", "testdata"))
	if err != nil {
		t.Fatalf("Failed to get absolute path to testdata directory: %v", err)
	}
	runTests(t, abs)
}

func runUtilityCommand(t *testing.T, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	t.Logf("running utility command %s %s", command, strings.Join(args, " "))
	return cmd.Run()
}

func runTests(t *testing.T, path string) {
	var searchAndReplace = map[string]string{}

	gitServerURL := startGitServer(t, path)
	if gitServerURL != defaultTestGitServerUrl {
		searchAndReplace[defaultTestGitServerUrl] = gitServerURL
	}

	testCases := scanTestCases(t, path)

	// remove any tmp files from previous test runs
	err := runUtilityCommand(t, "rm", "-rf", "/tmp/porch-e2e")
	if err != nil {
		t.Fatalf("Failed to clean up older run: %v", err)
	}
	err = runUtilityCommand(t, "mkdir", "/tmp/porch-e2e")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.TestCase, func(t *testing.T) {
			if tc.Skip != "" {
				t.Skipf("Skipping test: %s", tc.Skip)
			}
			repoURL := gitServerURL + "/" + strings.ReplaceAll(tc.TestCase, "/", "-")
			runTestCase(t, repoURL, tc, searchAndReplace)
		})
	}
}

func normalizeWhitespace(s1 string) string {
	parts := strings.Split(s1, " ")
	words := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			words = append(words, part)
		}
	}
	return strings.Join(words, " ")
}

func runTestCase(t *testing.T, repoURL string, tc TestCaseConfig, searchAndReplace map[string]string) {
	KubectlCreateNamespace(t, tc.TestCase)
	t.Cleanup(func() {
		KubectlDeleteNamespace(t, tc.TestCase)
	})

	if tc.Repository != "" {
		RegisterRepository(t, repoURL, tc.TestCase, tc.Repository)
	}

	for i := range tc.Commands {
		time.Sleep(1 * time.Second)
		command := &tc.Commands[i]
		for i, arg := range command.Args {
			for search, replace := range searchAndReplace {
				command.Args[i] = strings.ReplaceAll(arg, search, replace)
			}
		}
		if command.Args[0] == "porchctl" {
			// make sure that we are testing the porchctl command built from this codebase
			fullCmdPath, err := filepath.Abs(filepath.Join("..", "..", "..", ".build", "porchctl"))
			if err != nil {
				t.Fatalf("Failed to get absolute path to .build directory: %v", err)
			}
			command.Args[0] = fullCmdPath
		}
		cmd := exec.Command(command.Args[0], command.Args[1:]...)

		var stdout, stderr bytes.Buffer
		if command.Stdin != "" {
			cmd.Stdin = strings.NewReader(command.Stdin)
		}
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		t.Logf("running command %v", strings.Join(cmd.Args, " "))
		err := cmd.Run()

		if command.Yaml {
			reorderYamlStdout(t, &stdout)
		}

		cleanupStderr(t, &stderr)

		stdoutStr := stdout.String()
		stderrStr := stderr.String()
		for search, replace := range searchAndReplace {
			command.Stdout = strings.ReplaceAll(command.Stdout, search, replace)
			command.Stderr = strings.ReplaceAll(command.Stderr, search, replace)
		}

		if command.IgnoreWhitespace {
			command.Stdout = normalizeWhitespace(command.Stdout)
			command.Stderr = normalizeWhitespace(command.Stderr)
			stdoutStr = normalizeWhitespace(stdoutStr)
			stderrStr = normalizeWhitespace(stderrStr)
		}

		if os.Getenv(updateGoldenFiles) != "" {
			updateCommand(command, err, stdout.String(), stderr.String())
		}

		if got, want := exitCode(err), command.ExitCode; got != want {
			t.Errorf("unexpected exit code from '%s'; got %d, want %d", strings.Join(command.Args, " "), got, want)
		}
		if got, want := stdoutStr, command.Stdout; got != want {
			t.Errorf("unexpected stdout content from '%s'; (-want, +got) %s", strings.Join(command.Args, " "), cmp.Diff(want, got))
		}
		got, want := stderrStr, command.Stderr
		got = removeArmPlatformWarning(got)

		if got != want {
			t.Errorf("unexpected stderr content from '%s'; (-want, +got) %s", strings.Join(command.Args, " "), cmp.Diff(want, got))
		}

		// hack here; but if the command registered a repo, give a few extra seconds for the repo to reach readiness
		for _, arg := range command.Args {
			if arg == "register" {
				time.Sleep(5 * time.Second)
			}
		}
	}

	if os.Getenv(updateGoldenFiles) != "" {
		WriteTestCaseConfig(t, &tc)
	}
}

func removeArmPlatformWarning(got string) string {
	got = strings.Replace(
		got,
		"    \"WARNING: The requested image's platform (linux/amd64) does not match the detected host platform (linux/arm64/v8) and no specific platform was requested\"\n",
		"",
		1)

	if strings.HasSuffix(got, "  Stderr:\n") {
		// The warning message was the only message on stderr
		return strings.Replace(got, "  Stderr:\n", "", 1)
	} else {
		// There are other messages on stderr, so leave the "Stderr:"" tag in place
		return got
	}
}

// remove PASS lines from kpt fn eval, which includes a duration and will vary
func cleanupStderr(t *testing.T, buf *bytes.Buffer) {
	scanner := bufio.NewScanner(buf)
	var newBuf bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "[PASS]") {
			newBuf.Write([]byte(line))
			newBuf.Write([]byte("\n"))
		}
	}

	buf.Reset()
	if _, err := buf.Write(newBuf.Bytes()); err != nil {
		t.Fatalf("Failed to update cleaned up stderr: %v", err)
	}
}

func reorderYamlStdout(t *testing.T, buf *bytes.Buffer) {
	if buf.Len() == 0 {
		return
	}

	// strip out the resourceVersion:, creationTimestamp:
	// because that will change with every run
	scanner := bufio.NewScanner(buf)
	var newBuf bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "resourceVersion:") &&
			!strings.Contains(line, "creationTimestamp:") {
			newBuf.Write([]byte(line))
			newBuf.Write([]byte("\n"))
		}
	}

	var data interface{}
	if err := yaml.Unmarshal(newBuf.Bytes(), &data); err != nil {
		// not yaml.
		return
	}

	var stable bytes.Buffer
	encoder := yaml.NewEncoder(&stable)
	encoder.SetIndent(2)
	if err := encoder.Encode(data); err != nil {
		t.Fatalf("Failed to re-encode yaml output: %v", err)
	}
	buf.Reset()
	if _, err := buf.Write(stable.Bytes()); err != nil {
		t.Fatalf("Failed to update reordered yaml output: %v", err)
	}
}

func startGitServer(t *testing.T, path string) string {
	isPorchInCluster := IsPorchServerRunningInCluster(t)
	gitServerImage := GetGitServerImageName(t)
	t.Logf("Git Image: %s", gitServerImage)

	configFile := filepath.Join(path, "git-server.yaml")
	configBytes, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read git server config file %q: %v", configFile, err)
	}
	config := string(configBytes)
	config = strings.ReplaceAll(config, "GIT_SERVER_IMAGE", gitServerImage)
	if !isPorchInCluster {
		config = strings.ReplaceAll(config, "ClusterIP", "LoadBalancer")
	}

	t.Cleanup(func() {
		KubectlDeleteNamespace(t, testGitNamespace)
	})

	KubectlApply(t, config)
	KubectlWaitForDeployment(t, testGitNamespace, testGitName)
	KubectlWaitForService(t, testGitNamespace, testGitName)

	gitServerURL := defaultTestGitServerUrl
	if isPorchInCluster {
		KubectlWaitForGitDNS(t, gitServerURL)
	} else {
		ip := KubectlWaitForLoadBalancerIp(t, testGitNamespace, testGitName)
		gitServerURL = "http://" + ip + ":8080"
	}

	return gitServerURL
}

func scanTestCases(t *testing.T, root string) []TestCaseConfig {
	testCases := []TestCaseConfig{}

	if err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}

		tc := ReadTestCaseConfig(t, info.Name(), path)
		testCases = append(testCases, tc)

		return nil
	}); err != nil {
		t.Fatalf("Failed to scan test cases: %v", err)
	}

	return testCases
}

func updateCommand(command *Command, exit error, stdout, stderr string) {
	command.ExitCode = exitCode(exit)
	command.Stdout = stdout
	command.Stderr = stderr
}

func exitCode(exit error) int {
	var ee *exec.ExitError
	if errors.As(exit, &ee) {
		return ee.ExitCode()
	}
	return 0
}
