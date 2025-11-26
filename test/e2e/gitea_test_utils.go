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
	"net/http"
	"os"
	"strings"
)

const (
	GiteaClusterURL        = "http://gitea.gitea.svc.cluster.local:3000/nephio/"
	GiteaUser              = "nephio"
	GiteaPassword          = "secret"
	PorchTestRepoName      = "porch-test"
	TestBlueprintsRepoName = "test-blueprints"
	GiteaRepoAPi           = "http://localhost:3000/api/v1/repos/nephio/" + PorchTestRepoName
)

// getGiteaURL returns the appropriate Gitea URL based on whether Porch server is running in cluster
func (t *TestSuite) getGiteaURL() string {
	if t.IsPorchServerInCluster() {
		return GiteaClusterURL
	}
	return "http://localhost:3000/nephio/"
}

// GetPorchTestRepoURL returns the dynamic PorchTestRepo URL
func (t *TestSuite) GetPorchTestRepoURL() string {
	return t.getGiteaURL() + PorchTestRepoName + ".git"
}

// GetTestBlueprintsRepoURL returns the dynamic TestBlueprintsRepo URL
func (t *TestSuite) GetTestBlueprintsRepoURL() string {
	return t.getGiteaURL() + TestBlueprintsRepoName + ".git"
}

// IsPorchTestRepo checks if a repository URL is specifically the porch-test repository
func IsPorchTestRepo(repo string) bool {
	return strings.Contains(repo, "porch-test")
}

// RecreateGiteaTestRepo recreates the porch-test repository to its initial state
func (t *TestSuite) RecreateGiteaTestRepo() {
	t.T().Helper()

	// Skip cleanup only if test failed and KEEP_GITEA_ON_FAILURE is set in local development
	if t.T().Failed() && os.Getenv("KEEP_GITEA_ON_FAILURE") == "true" && os.Getenv("CI") == "" {
		t.Logf("Skipping gitea cleanup due to test failure (KEEP_GITEA_ON_FAILURE=true, local dev)")
		return
	}

	t.Logf("recreating gitea porch-test repository to initial state")

	// Delete the repository
	req, _ := http.NewRequest("DELETE", GiteaRepoAPi, nil)
	req.SetBasicAuth(GiteaUser, GiteaPassword)
	if _, err := http.DefaultClient.Do(req); err != nil {
		t.Fatalf("Failed to delete gitea porch-test repository: %v", err)
	}

	// Recreate the repository
	body := `{"name": "porch-test", "auto_init": true, "readme": "Default"}`
	req, _ = http.NewRequest("POST", "http://localhost:3000/api/v1/user/repos", strings.NewReader(body))
	req.SetBasicAuth(GiteaUser, GiteaPassword)
	req.Header.Set("Content-Type", "application/json")
	if _, err := http.DefaultClient.Do(req); err != nil {
		t.Fatalf("Failed to recreate gitea porch-test repository: %v", err)
	}
	t.Logf("Successfully recreated gitea porch-test repository")
}
