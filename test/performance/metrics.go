package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/nephio-project/porch/api/generated/clientset/versioned"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(porchapi.AddToScheme(scheme))
	utilruntime.Must(configapi.AddToScheme(scheme))
}

// Helper functions for metrics collection and operations
func getPorchClientset(cfg *rest.Config) (*versioned.Clientset, error) {
	return versioned.NewForConfig(cfg)
}

func createGiteaRepo(repoName string) error {
	giteaURL := "http://172.18.255.200:3000/api/v1/user/repos"
	payload := map[string]interface{}{
		"name":        repoName,
		"description": "Test repository for Porch metrics",
		"private":     false,
		"auto_init":   true,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	req, err := http.NewRequest("POST", giteaURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("nephio", "secret")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create repo: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create repo, status: %d", resp.StatusCode)
	}

	return nil
}

func debugPackageStatus(t *testing.T, c client.Client, ctx context.Context, namespace, name string) {
	var pkg porchapi.PackageRevision
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &pkg)
	if err != nil {
		t.Logf("Error getting package: %v", err)
		return
	}

	t.Logf("\nPackage Status Details:")
	t.Logf("  Name: %s", pkg.Name)
	t.Logf("  LifecycleState: %s", pkg.Spec.Lifecycle)
	t.Logf("  WorkspaceName: %s", pkg.Spec.WorkspaceName)
	t.Logf("  Revision: %v", pkg.Spec.Revision)
	t.Logf("  Published: %v", pkg.Status.PublishedAt)
	t.Logf("  Tasks:")
	for i, task := range pkg.Spec.Tasks {
		t.Logf("    %d. Type: %s", i+1, task.Type)
		if task.Type == porchapi.TaskTypeInit && task.Init != nil {
			t.Logf("       Description: %s", task.Init.Description)
			t.Logf("       Keywords: %v", task.Init.Keywords)
		}
	}
	t.Logf("  Conditions:")
	for _, cond := range pkg.Status.Conditions {
		t.Logf("    - Type: %s", cond.Type)
		t.Logf("      Status: %s", cond.Status)
		t.Logf("      Message: %s", cond.Message)
		t.Logf("      Reason: %s", cond.Reason)
	}
}

// ... existing imports and code ...

func deleteGiteaRepo(repoName string) error {
	giteaURL := fmt.Sprintf("http://172.18.255.200:3000/api/v1/repos/nephio/%s", repoName)

	req, err := http.NewRequest("DELETE", giteaURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %v", err)
	}

	req.SetBasicAuth("nephio", "secret")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete repo: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("failed to delete repo, status: %d", resp.StatusCode)
	}

	return nil
}
func waitForRepository(ctx context.Context, c client.Client, t *testing.T, namespace, name string, timeout time.Duration) error {
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for repository to be ready")
		}

		var repo configapi.Repository
		err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &repo)
		if err != nil {
			return err
		}

		t.Logf("\nRepository conditions at %v:", time.Since(start))
		t.Logf("Spec: %+v", repo.Spec)
		t.Logf("Status: %+v", repo.Status)

		ready := false
		for _, cond := range repo.Status.Conditions {
			t.Logf("  - Type: %s, Status: %s, Message: %s",
				cond.Type, cond.Status, cond.Message)
			if cond.Type == "Ready" && cond.Status == "True" {
				ready = true
				break
			}
		}

		if ready {
			return nil
		}

		time.Sleep(2 * time.Second)
	}
}
