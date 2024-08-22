// Copyright 2023 The kpt and Nephio Authors
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
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	registrationapi "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// KubernetesName returns the passed id if it less than maxLen, otherwise
// a truncated version of id with a unique hash of length hashLen appended
// with length maxLen. maxLen must be at least 5 + hashLen, and hashLen
// must be at least 4.
func KubernetesName(id string, hashLen, maxLen int) string {
	if hashLen < 4 {
		hashLen = 4
	}
	if maxLen < hashLen+5 {
		maxLen = hashLen + 5
	}

	if len(id) <= maxLen {
		return id
	}

	hash := sha1.Sum([]byte(id))
	stubIdx := maxLen - hashLen - 1
	return fmt.Sprintf("%s-%s", id[:stubIdx], hex.EncodeToString(hash[:])[:hashLen])
}

func GetInClusterNamespace() (string, error) {
	ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", fmt.Errorf("failed to read in-cluster namespace: %w", err)
	}
	return string(ns), nil
}

func GetPorchApiServiceKey(ctx context.Context) (client.ObjectKey, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return client.ObjectKey{}, fmt.Errorf("failed to get K8s config: %w", err)
	}

	scheme := runtime.NewScheme()
	err = registrationapi.AddToScheme(scheme)
	if err != nil {
		return client.ObjectKey{}, fmt.Errorf("failed to add apiregistration API to scheme: %w", err)
	}

	c, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return client.ObjectKey{}, fmt.Errorf("failed to create K8s client: %w", err)
	}

	apiSvc := registrationapi.APIService{}
	err = c.Get(ctx, client.ObjectKey{
		Name: "v1alpha1.porch.kpt.dev",
	}, &apiSvc)
	if err != nil {
		return client.ObjectKey{}, fmt.Errorf("failed to get APIService 'v1alpha1.porch.kpt.dev': %w", err)
	}

	return client.ObjectKey{
		Namespace: apiSvc.Spec.Service.Namespace,
		Name:      apiSvc.Spec.Service.Name,
	}, nil
}
