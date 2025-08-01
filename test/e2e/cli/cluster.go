// Copyright 2022-2025 The kpt Authors
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
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func IsPorchServerRunningInCluster(t *testing.T) bool {
	cmd := exec.Command("kubectl", "get", "--namespace=porch-system", "service", "api",
		"--output=jsonpath={.spec.selector}")

	var stderr bytes.Buffer
	var stdout bytes.Buffer

	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil || stderr.String() != "" {
		t.Fatalf("Error when getting porch api Service: %v: %s", err, stderr.String())
	}
	return stdout.String() != ""
}

func KubectlApply(t *testing.T, config string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(config)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl apply failed: %v\ninput: %s\n\noutput:%s", err, config, string(out))
	}
	t.Logf("kubectl apply\n%s\noutput:\n%s", config, string(out))
}

func KubectlDelete(t *testing.T, config string) {
	cmd := exec.Command("kubectl", "delete", "-f", "-")
	cmd.Stdin = strings.NewReader(config)
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "NotFound") {
		t.Fatalf("kubectl delete failed: %v\ninput: %s\n\noutput:%s", err, config, string(out))
	}
	t.Logf("kubectl delete\n%s\noutput:\n%s", config, string(out))
}

func KubectlWaitForLoadBalancerIp(t *testing.T, namespace, name string) string {
	args := []string{"get", "service", "--namespace", namespace, name, "--output=jsonpath={.status.loadBalancer.ingress[0].ip}"}

	giveUp := time.Now().Add(1 * time.Minute)
	for {
		cmd := exec.Command("kubectl", args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		ip := stdout.String()
		if err == nil && len(ip) > 0 { // LoadBalancer assigned an external IP
			t.Logf("LoadBalancer external IP: %s", ip)
			return ip
		}

		if time.Now().After(giveUp) {
			var msg string
			if err != nil {
				msg = err.Error()
			}
			t.Fatalf("LoadBalancer service %s/%s hasn't been assigned an external IP on time. Giving up: %s", namespace, name, msg)
		}

		time.Sleep(5 * time.Second)
	}
}

func KubectlWaitForRepoReady(t *testing.T, repoName, namespace string) {
	t.Logf("waiting for repo %s/%s to become Ready", namespace, repoName)
	args := []string{"get", "repository", repoName, "--namespace", namespace, "--output=jsonpath={.status.conditions[?(@.type=='Ready')].status}"}
	giveUp := time.Now().Add(1 * time.Minute)
	for {
		cmd := exec.Command("kubectl", args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		t.Logf("running command %v", strings.Join(cmd.Args, " "))
		err := cmd.Run()
		ready := stdout.String()
		if err == nil && string(ready) == "True" {
			t.Logf("Repo %s/%s is Ready", namespace, repoName)
			return
		}
		if time.Now().After(giveUp) {
			var msg string
			if err != nil {
				msg = err.Error()
			}
			t.Fatalf("Repo %s/%s has not become Ready. Giving up: %s", namespace, repoName, msg)
		}
		time.Sleep(2 * time.Second)
	}
}

func KubectlCreateNamespace(t *testing.T, name string) {
	cmd := exec.Command("kubectl", "create", "namespace", name)
	t.Logf("running command %v", strings.Join(cmd.Args, " "))
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "AlreadyExists") {
		t.Fatalf("Failed to create namespace %q: %v\n%s", name, err, string(out))
	}
	t.Logf("output: %v", string(out))
}

func KubectlDeleteNamespace(t *testing.T, name string) {
	//Removing Finalizers from PackageRevs in the test NameSpace to avoid locking when deleting
	RemovePackagerevFinalizers(t, name)
	cmd := exec.Command("kubectl", "delete", "namespace", name)
	t.Logf("running command %v", strings.Join(cmd.Args, " "))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Failed to delete namespace %q: %v\n%s", name, err, string(out))
	}
	t.Logf("output: %v", string(out))
}

func RemovePackagerevFinalizers(t *testing.T, namespace string) {
	cmd := exec.Command("kubectl", "get", "packagerevs", "--namespace", namespace, "--output=jsonpath={.items[*].metadata.name}")
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("Error when getting packagerevs from namespace: %v: %s", err, stderr.String())
	}

	packagerevs := reallySplit(stdout.String(), " ")
	if len(packagerevs) == 0 {
		t.Log("kubectl get packagerevs didn't return any objects - continue")
		return
	}
	t.Logf("Removing Finalizers from PackageRevs: %v", packagerevs)

	for _, pkgrev := range packagerevs {
		cmd := exec.Command("kubectl", "patch", "packagerev", pkgrev, "--type", "json", "--patch=[{\"op\": \"remove\", \"path\": \"/metadata/finalizers\"}]", "--namespace", namespace)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to remove Finalizer from %q: %v\n%s", pkgrev, err, string(out))
		}
	}
}

func reallySplit(s, sep string) []string {
	if len(s) == 0 {
		return []string{}
	}
	return strings.Split(s, sep)
}
