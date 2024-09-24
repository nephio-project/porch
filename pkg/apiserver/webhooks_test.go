// Copyright 2022,2024 The kpt and Nephio Authors
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

package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateCerts(t *testing.T) {
	webhookCfg := WebhookConfig{
		Type:           WebhookTypeUrl,
		Host:           "localhost",
		CertStorageDir: t.TempDir(),
	}
	defer func() {
		require.NoError(t, os.RemoveAll(webhookCfg.CertStorageDir))
	}()

	caCert, err := createCerts(&webhookCfg)
	require.NoError(t, err)

	caStr := strings.TrimSpace(string(caCert))
	require.True(t, strings.HasPrefix(caStr, "-----BEGIN CERTIFICATE-----\n"))
	require.True(t, strings.HasSuffix(caStr, "\n-----END CERTIFICATE-----"))

	crt, err := os.ReadFile(filepath.Join(webhookCfg.CertStorageDir, "tls.crt"))
	require.NoError(t, err)

	key, err := os.ReadFile(filepath.Join(webhookCfg.CertStorageDir, "tls.key"))
	require.NoError(t, err)

	crtStr := strings.TrimSpace(string(crt))
	require.True(t, strings.HasPrefix(crtStr, "-----BEGIN CERTIFICATE-----\n"))
	require.True(t, strings.HasSuffix(crtStr, "\n-----END CERTIFICATE-----"))

	keyStr := strings.TrimSpace(string(key))
	require.True(t, strings.HasPrefix(keyStr, "-----BEGIN RSA PRIVATE KEY-----\n"))
	require.True(t, strings.HasSuffix(keyStr, "\n-----END RSA PRIVATE KEY-----"))
}

func TestLoadCertificate(t *testing.T) {

	//what do i need to test.
	// first create dummy certs for testing
	webhookCfg := WebhookConfig{
		Type:           WebhookTypeUrl,
		Host:           "localhost",
		CertStorageDir: t.TempDir(),
	}
	defer func() {
		require.NoError(t, os.RemoveAll(webhookCfg.CertStorageDir))
	}()

	_, err := createCerts(&webhookCfg)
	require.NoError(t, err)

	//1. test file that cannot be os.stated or causes that function to fail
	_, err1 := loadCertificate(filepath.Join(webhookCfg.CertStorageDir, "nonexistingcrtfile.key"), filepath.Join(webhookCfg.CertStorageDir, "nonexistingkeyfile.key"))
	require.Error(t, err1)
	// reseting back to 0
	certModTime = time.Time{}
	//2. test happy path of os.stat and continue to next error
	//3. test loading good cert happy path
	keypath := filepath.Join(webhookCfg.CertStorageDir, "tls.key")
	crtpath := filepath.Join(webhookCfg.CertStorageDir, "tls.crt")

	_, err2 := loadCertificate(crtpath, keypath)
	require.NoError(t, err2)
	certModTime = time.Time{}

	//4. test loading faulty cert error
	data := []byte("Hello, World!")

	writeErr := os.WriteFile(keypath, data, 0644)
	require.NoError(t, writeErr)
	writeErr2 := os.WriteFile(crtpath, data, 0644)
	require.NoError(t, writeErr2)

	_, err3 := loadCertificate(filepath.Join(webhookCfg.CertStorageDir, "tls.crt"), filepath.Join(webhookCfg.CertStorageDir, "tls.key"))
	require.Error(t, err3)
	certModTime = time.Time{}
}

// method for capturing klog error's
func captureStderr(f func()) string {
	read, write, _ := os.Pipe()
	stderr := os.Stderr
	os.Stderr = write
	outputChannel := make(chan string)

	// Copy the output in a separate goroutine so printing can't block indefinitely.
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, read)
		outputChannel <- buf.String()
	}()

	// Run the provided function and capture the stderr output
	f()

	// Restore the original stderr and close the write-end of the pipe so the goroutine will exit
	os.Stderr = stderr
	write.Close()
	out := <-outputChannel

	return out
}

func TestWatchCertificates(t *testing.T) {
	// method for processing klog output
	assertLogMessages := func(log string) error {
		if len(log) > 0 {
			if log[0] == 'E' || log[0] == 'W' || log[0] == 'F' {
				return errors.New("Error Occured in Watcher")
			}
		}
		return nil
	}
	// Set up the temp directory with dummy certificate files
	webhookCfg := WebhookConfig{
		Type:           WebhookTypeUrl,
		Host:           "localhost",
		CertStorageDir: t.TempDir(),
	}
	defer func() {
		require.NoError(t, os.RemoveAll(webhookCfg.CertStorageDir))
	}()

	_, err := createCerts(&webhookCfg)
	require.NoError(t, err)

	keyFile := filepath.Join(webhookCfg.CertStorageDir, "tls.key")
	certFile := filepath.Join(webhookCfg.CertStorageDir, "tls.crt")

	// Create a context with a cancel function
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// firstly test error occuring from invalid entity for watcher to watch. aka invalid dir, err expected
	go watchCertificates(ctx, "Dummy Directory that does not exist", certFile, keyFile)

	invalid_watch_entity_logs := captureStderr(func() {
		time.Sleep(1 * time.Second) // Give some time for the logs to be flushed
	})
	t.Log(invalid_watch_entity_logs)
	err = assertLogMessages(invalid_watch_entity_logs)
	require.Error(t, err)

	go watchCertificates(ctx, webhookCfg.CertStorageDir, certFile, keyFile)
	time.Sleep(1 * time.Second)

	//create file to trigger change but not alter the certificate contents
	//should trigger reload and certificate reloaded successfully
	newFilePath := filepath.Join(webhookCfg.CertStorageDir, "new_temp_file.txt")
	_, err = os.Create(newFilePath)
	require.NoError(t, err)

	valid_reload_logs := captureStderr(func() {
		time.Sleep(1 * time.Second) // Give some time for the logs to be flushed
	})
	t.Log(valid_reload_logs)
	err = assertLogMessages(valid_reload_logs)
	require.NoError(t, err)

	// Modify the certificate file to trigger a file system event
	// should cause an error log since cert contents are not valid anymore
	certModTime = time.Time{}
	err = os.WriteFile(certFile, []byte("dummy text"), 0660)
	require.NoError(t, err)

	invalid_reload_logs := captureStderr(func() {
		time.Sleep(1 * time.Second)
	})
	t.Log(invalid_reload_logs)
	err = assertLogMessages(invalid_reload_logs)
	require.Error(t, err)

	go watchCertificates(ctx, webhookCfg.CertStorageDir, certFile, keyFile)
	// trigger graceful termination
	cancel()
	graceful_termination_logs := captureStderr(func() {
		time.Sleep(1 * time.Second)
	})
	t.Log(graceful_termination_logs)
	err = assertLogMessages(graceful_termination_logs)
	require.NoError(t, err)
}

func TestValidateDeletion(t *testing.T) {
	t.Run("invalid content-type", func(t *testing.T) {
		request, err := http.NewRequest(http.MethodPost, serverEndpoint, nil)
		require.NoError(t, err)
		request.Header.Set("Content-Type", "foo")
		response := httptest.NewRecorder()

		validateDeletion(response, request)
		require.Equal(t,
			"error getting admission review from request: expected Content-Type 'application/json'",
			response.Body.String())
	})
	t.Run("valid content-type, but no body", func(t *testing.T) {
		request, err := http.NewRequest(http.MethodPost, serverEndpoint, nil)
		require.NoError(t, err)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()

		validateDeletion(response, request)
		require.Equal(t,
			"error getting admission review from request: admission review request is empty",
			response.Body.String())
	})
	t.Run("wrong GVK in request", func(t *testing.T) {
		request, err := http.NewRequest(http.MethodPost, serverEndpoint, nil)
		require.NoError(t, err)

		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()

		admissionReviewRequest := admissionv1.AdmissionReview{
			TypeMeta: v1.TypeMeta{
				Kind:       "AdmissionReview",
				APIVersion: "admission.k8s.io/v1",
			},
			Request: &admissionv1.AdmissionRequest{
				Resource: v1.GroupVersionResource{
					Group:    "porch.kpt.dev",
					Version:  "v1alpha1",
					Resource: "not-a-package-revision",
				},
			},
		}

		body, err := json.Marshal(admissionReviewRequest)
		require.NoError(t, err)

		request.Body = io.NopCloser(bytes.NewReader(body))
		validateDeletion(response, request)
		require.Equal(t,
			"did not receive PackageRevision, got not-a-package-revision",
			response.Body.String())
	})
}
