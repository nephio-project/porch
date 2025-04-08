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

package update

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func createTestPackageRevision(pkgRevName, ns string, revision int, up porchapi.UpstreamPackage) porchapi.PackageRevision {
	return porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "kptfile",
			APIVersion: "v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pkgRevName,
			Namespace: ns,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName: "testpackage",
			Revision:    revision,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: up,
					},
				},
			},
		},
		Status: porchapi.PackageRevisionStatus{
			UpstreamLock: &porchapi.UpstreamLock{
				Git: &porchapi.GitLock{
					Repo: "https://github.com/user/repo",
					Ref:  "/main",
				},
			},
		},
	}
}

func createRunner(ctx context.Context, ns string, c client.Client, pkgRevName string, discover string, revision int) *runner {
	return &runner{
		ctx:      ctx,
		cfg:      &genericclioptions.ConfigFlags{Namespace: &ns},
		client:   c,
		Command:  NewCommand(ctx, &genericclioptions.ConfigFlags{Namespace: &ns}),
		revision: 2,
		discover: discover,
		prs: []porchapi.PackageRevision{
			createTestPackageRevision(pkgRevName, ns, revision, porchapi.UpstreamPackage{
				Type: porchapi.RepositoryTypeGit,
				Git: &porchapi.GitPackage{
					Ref: "v1",
				},
			}),
		},
	}
}

func TestPreRun(t *testing.T) {
	r := createRunner(context.Background(), "ns", fake.NewClientBuilder().Build(), "repo", "", 1)
	err := r.preRunE(r.Command, []string{"arg1", "arg2", "arg3"})
	assert.Error(t, err)

	err = r.preRunE(r.Command, []string{})
	assert.Error(t, err)

	newRunner := createRunner(context.Background(), "ns", fake.NewClientBuilder().Build(), "repo", "notupstream", 1)
	err2 := newRunner.preRunE(newRunner.Command, []string{"repo"})
	assert.Error(t, err2)

}

func TestUpdateCommand(t *testing.T) {
	ctx := context.Background()
	pkgRevName := "repo-testrevision"
	ns := "ns"
	upstream := porchapi.UpstreamPackage{
		Type: porchapi.RepositoryTypeGit,
		Git: &porchapi.GitPackage{
			Ref: "v1",
		},
	}
	testPackageRevision := createTestPackageRevision(pkgRevName, ns, 1, upstream)

	// Create fake client and add the package revision to it
	scheme := runtime.NewScheme()
	if err := porchapi.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add porch API to scheme: %v", err)
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(&testPackageRevision).
		Build()

	output := &bytes.Buffer{}
	r := createRunner(ctx, ns, client, pkgRevName, "", 1)

	testCases := []struct {
		name           string
		args           []string
		expectedOutput string
		expectedError  string
	}{
		{
			name:           "Successful package update",
			args:           []string{pkgRevName},
			expectedOutput: fmt.Sprintf("%s updated\n", pkgRevName),
			expectedError:  "",
		},
		{
			name:           "Non-existent package revision",
			args:           []string{"non-existent-revision"},
			expectedOutput: "",
			expectedError:  "could not find package revision non-existent-revision",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r.Command.SetArgs(tc.args)
			r.Command.SetOut(output)

			output.Reset()
			err := r.runE(r.Command, tc.args)

			if tc.expectedError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expectedError) {
					t.Fatalf("Expected error %q, got %v", tc.expectedError, err)
				}
			} else if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if diff := cmp.Diff(strings.TrimSpace(tc.expectedOutput), strings.TrimSpace(output.String())); diff != "" {
				t.Errorf("Unexpected output (-want, +got): %s", diff)
			}
		})
	}

	err := r.discoverUpdates(r.Command, []string{pkgRevName})
	assert.Error(t, err)
}

func TestDiscoverUpdates(t *testing.T) {
	ctx := context.Background()
	pkgRevName := "repo-testrevision"
	ns := "ns"

	mockClient := mockclient.NewMockClient(t)
	mockClient.EXPECT().
		List(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).
		Run(func(_ context.Context, list client.ObjectList, opts ...client.ListOption) {
			list.(*configapi.RepositoryList).Items = make([]configapi.Repository, 1)
			list.(*configapi.RepositoryList).Items[0] = configapi.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sample-repo-2",
					Namespace: "main",
				},
				Spec: configapi.RepositorySpec{
					Description: "Sample Git repository",
					Deployment:  false,
					Type:        "git",
					Git: &configapi.GitRepository{
						Repo:   "https://github.com/user/repo",
						Branch: "/main",
						Author: "James Bond",
						Email:  "jamesbond@007.com",
					},
				},
			}
		})
	output := &bytes.Buffer{}
	r := createRunner(ctx, ns, mockClient, pkgRevName, "downstream", 1)

	r.Command.SetArgs([]string{pkgRevName})
	r.Command.SetOut(output)

	output.Reset()
	err := r.discoverUpdates(r.Command, []string{pkgRevName})
	assert.Nil(t, err)
	assert.Equal(t, output.String(), "All downstream packages are up to date.\n")

	output.Reset()
	r = createRunner(ctx, ns, mockClient, pkgRevName, "upstream", 1)
	err = r.discoverUpdates(r.Command, []string{pkgRevName})
	assert.Nil(t, err)

	output.Reset()
	r = createRunner(ctx, ns, mockClient, pkgRevName, "", 1)
	err = r.discoverUpdates(r.Command, []string{pkgRevName})
	assert.Error(t, err)

	output.Reset()
	upstream := porchapi.UpstreamPackage{
		UpstreamRef: &porchapi.PackageRevisionRef{
			Name: pkgRevName,
		},
	}
	testPackageRevision := createTestPackageRevision(pkgRevName, ns, 1, upstream)

	r = createRunner(ctx, ns, mockClient, pkgRevName, "", 1)
	err = r.doUpdate(&testPackageRevision)
	assert.Error(t, err)
	assert.EqualError(t, err, "revision 2 does not exist for package testpackage")

	output.Reset()
	mockClient.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	r = createRunner(ctx, ns, mockClient, pkgRevName, "", 2)
	err = r.doUpdate(&testPackageRevision)
	assert.Nil(t, err)
}
