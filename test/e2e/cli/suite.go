// Copyright 2024-2025 The kpt and Nephio Authors
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
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	nethttp "net/http"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

const (
	updateGoldenFiles       = "UPDATE_GOLDEN_FILES"
	testGitName             = "gitea"
	testGitNamespace        = "gitea"
	testGitUserOrg          = "nephio"
	testGitPassword         = "secret"
	defaultTestGitServerUrl = "http://localhost:3000"
)

type CliTestSuite struct {
	// The path of the directory containing the test cases.
	TestDataPath string
	// SearchAndReplace contains (search, replace) pairs that will be applied to the command output before comparison.
	SearchAndReplace map[string]string
	// GitServerURL is the URL of the git server to use for the tests.
	GitServerURL string
	// PorchctlCommand is the full path to the porchctl command to be tested.
	PorchctlCommand string
}

// NewCliTestSuite creates a new CliTestSuite based on the configuration in the testdata directory.
func NewCliTestSuite(t *testing.T, testdataDir string) *CliTestSuite {
	var err error

	s := &CliTestSuite{}
	// set base dir
	s.TestDataPath, err = filepath.Abs(testdataDir)
	if err != nil {
		t.Fatalf("Failed to get absolute path to testdata directory: %v", err)
	}
	// find porchctl to test
	s.PorchctlCommand, err = filepath.Abs(filepath.Join("..", "..", "..", ".build", "porchctl"))
	if err != nil {
		t.Fatalf("Failed to get absolute path to .build/porchctl command: %v", err)
	}
	if _, err := os.Stat(s.PorchctlCommand); err != nil {
		t.Fatalf("porchctl command not found at %q: %v", s.PorchctlCommand, err)
	}

	isPorchInCluster := IsPorchServerRunningInCluster(t)
	if isPorchInCluster {
		s.GitServerURL = defaultTestGitServerUrl+"/nephio"
	} else {
		ip := KubectlWaitForLoadBalancerIp(t, testGitNamespace, testGitName)
		s.GitServerURL = "http://" + ip + ":3000/nephio"
	}
	s.SearchAndReplace = map[string]string{}
	if s.GitServerURL != defaultTestGitServerUrl {
		s.SearchAndReplace[defaultTestGitServerUrl] = s.GitServerURL
	}

	// prepare tmp directory used by the commands in the test cases
	err = runUtilityCommand(t, "rm", "-rf", "/tmp/porch-e2e")
	if err != nil {
		t.Fatalf("Failed to clean up older run: %v", err)
	}
	err = runUtilityCommand(t, "mkdir", "/tmp/porch-e2e")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	return s
}

// RunTests runs the test cases in the testdata directory.
func (s *CliTestSuite) RunTests(t *testing.T) {
	testCases := s.ScanTestCases(t)
	for _, tc := range testCases {
		t.Run(tc.TestCase, func(t *testing.T) {
			if tc.Skip != "" {
				t.Skipf("Skipping test: %s", tc.Skip)
			}
			s.RunTestCase(t, tc)
		})
	}
}

// RunTestCase runs a single test case.
func (s *CliTestSuite) RunTestCase(t *testing.T, tc TestCaseConfig) {
	repoURL := s.GitServerURL + "/" + strings.ReplaceAll(tc.TestCase, "/", "-")

	KubectlCreateNamespace(t, tc.TestCase)
	t.Cleanup(func() {
		KubectlDeleteNamespace(t, tc.TestCase)
		deleteRemoteRepo(t, tc.TestCase)
	})
	
	createRemoteRepo(s.TestDataPath+"/test-repo", repoURL, t)

	if tc.Repository != "" {
		s.RegisterRepository(t, repoURL, tc.TestCase, tc.Repository)
	}

	for i := range tc.Commands {
		time.Sleep(1 * time.Second)
		command := &tc.Commands[i]
		for i, arg := range command.Args {
			for search, replace := range s.SearchAndReplace {
				command.Args[i] = strings.ReplaceAll(arg, search, replace)
			}
		}
		if command.Args[0] == "porchctl" {
			// make sure that we are testing the porchctl command built from this codebase
			command.Args[0] = s.PorchctlCommand
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
		for search, replace := range s.SearchAndReplace {
			command.Stdout = strings.ReplaceAll(command.Stdout, search, replace)
			command.Stderr = strings.ReplaceAll(command.Stderr, search, replace)
		}

		if command.StdErrTabToWhitespace {
			stderrStr = strings.ReplaceAll(stderrStr, "\t", "  ") // Replace tabs with spaces
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

		if command.ContainsErrorString {
			if !strings.Contains(got, want) {
				t.Errorf("unexpected stderr content from '%s'; \n Error we got = \n(%s) \n Should contain substring = \n(%s)\n", strings.Join(command.Args, " "), got, want)
			}
		} else {
			if got != want {
				t.Errorf("unexpected stderr content from '%s'; (-want, +got) %s", strings.Join(command.Args, " "), cmp.Diff(want, got))
			}
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

// ScanTestCases parses the test case configs from the testdata directory.
func (s *CliTestSuite) ScanTestCases(t *testing.T) []TestCaseConfig {
	testCases := []TestCaseConfig{}

	if err := filepath.Walk(s.TestDataPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if path == s.TestDataPath {
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

// Porchctl runs the porchctl command under test with the given arguments.
func (s *CliTestSuite) Porchctl(t *testing.T, args ...string) error {
	cmd := exec.Command(s.PorchctlCommand, args...)
	t.Logf("running command: porchctl %v", strings.Join(args, " "))
	outBytes, err := cmd.CombinedOutput()
	t.Logf("porchctl output: %v", string(outBytes))
	return err
}

func (s *CliTestSuite) RegisterRepository(t *testing.T, repoURL, namespace, name string) {
	err := s.Porchctl(t, "repo", "register", "--namespace", namespace, "--name", name, repoURL)
	if err != nil {
		t.Fatalf("Failed to register repository %q: %v", repoURL, err)
	}
}

func runUtilityCommand(t *testing.T, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	t.Logf("running utility command %s %s", command, strings.Join(args, " "))
	return cmd.Run()
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

func deleteRemoteRepo (t *testing.T, testcaseName string) {

	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s", defaultTestGitServerUrl, testGitUserOrg, testcaseName)

	req, err := nethttp.NewRequest("DELETE", apiURL, nil)
	if err != nil {
		t.Fatalf("Failed to create DELETE request: %v", err)
	}
	auth := testGitUserOrg + ":" + testGitPassword
	basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	req.Header.Set("Authorization", basicAuth)

	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == nethttp.StatusNoContent {
		t.Logf("Repository deleted successfully.")
	} else {
		t.Logf("Failed to delete repo: %s\n", resp.Status)
	}
}


func createRemoteRepo (path string, repoUrl string, t *testing.T) {

	t.Cleanup(func() {
		runUtilityCommand(t, "rm", "-rf", path)
	})

	err := runUtilityCommand(t, "mkdir", path)
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	repo, err := git.PlainInit(path, false)
	if err != nil {
		t.Fatalf("Failed to init the repo: %v", err)
	}

	err = repo.Storer.SetReference(
		plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main")),
	)
	if err != nil {
		t.Fatalf("Failed to set refs: %v", err)
	}

	readmePath := path + "/README.md"
	err = os.WriteFile(readmePath, []byte("# My Go-Git Repo\nCreated programmatically."), 0644)
	if err != nil {
		t.Fatalf("Failed to write to file: %v", err)
	}

	wt, _ := repo.Worktree()
	wt.Add("README.md")
	wt.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Your Name",
			Email: "you@example.com",
			When:  time.Now(),
		},
	})

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoUrl},
	})
	if err != nil {
		t.Fatalf("Failed to create init commit: %v", err)
	}

	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth: &http.BasicAuth{
			Username: "nephio", 
			Password: "secret",
		},
		RequireRemoteRefs: []config.RefSpec{},
	})
	if err != nil {
		t.Fatalf("Failed to push test repo: %v", err)
	}
}