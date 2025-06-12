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

package upgrade

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func createOrigPackageRevision(name, namespace string, revision int) *porchapi.PackageRevision {
	return &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: "porch.kpt.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName: "orig-testpackage",
			Revision:    revision,
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeInit,
					Init: &porchapi.PackageInitTaskSpec{},
				},
			},
			Lifecycle: porchapi.PackageRevisionLifecyclePublished,
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

func createEditPackageRevision(pr *porchapi.PackageRevision, name string, revision int) *porchapi.PackageRevision {
	newPr := pr.DeepCopy()
	newPr.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeEdit,
			Edit: &porchapi.PackageEditTaskSpec{
				Source: &porchapi.PackageRevisionRef{
					Name: pr.Name,
				},
			},
		},
	}
	newPr.Spec.Revision = revision
	newPr.Name = name
	return newPr
}

func createClonePackageRevision(pr *porchapi.PackageRevision, name string, revision int, lifecycle porchapi.PackageRevisionLifecycle) *porchapi.PackageRevision {
	newPr := pr.DeepCopy()
	newPr.Spec.Tasks = []porchapi.Task{
		{
			Type: porchapi.TaskTypeClone,
			Clone: &porchapi.PackageCloneTaskSpec{
				Upstream: porchapi.UpstreamPackage{
					UpstreamRef: &porchapi.PackageRevisionRef{
						Name: pr.Name,
					},
				},
			},
		},
	}
	newPr.Name = name
	newPr.Spec.Lifecycle = lifecycle
	newPr.Spec.Revision = revision
	newPr.Status = porchapi.PackageRevisionStatus{
		UpstreamLock: &porchapi.UpstreamLock{
			Git: &porchapi.GitLock{
				Repo: "https://github.com/user/repo",
				Ref:  "/main",
			},
		},
	}
	return newPr
}

func createRunner(ctx context.Context, c client.Client, prs []porchapi.PackageRevision, ns string, revision int) *runner {
	return &runner{
		ctx:      ctx,
		cfg:      &genericclioptions.ConfigFlags{Namespace: &ns},
		client:   c,
		Command:  NewCommand(ctx, &genericclioptions.ConfigFlags{Namespace: &ns}),
		revision: revision,
		prs:      prs,
	}
}

func createRunnerWithDiscovery(ctx context.Context, c client.Client, prs []porchapi.PackageRevision, ns, discover string, revision int) *runner {
	rnr := createRunner(ctx, c, prs, ns, revision)
	rnr.discover = discover
	return rnr
}

func TestPreRun(t *testing.T) {
	const ns = "ns"

	orig := createOrigPackageRevision("repo-orig", "ns", 1)
	prs := []porchapi.PackageRevision{
		*createClonePackageRevision(orig, "repo", 1, porchapi.PackageRevisionLifecyclePublished),
	}
	r := createRunner(context.Background(), fake.NewClientBuilder().Build(), prs, ns, 2)
	err := r.preRunE(r.Command, []string{"arg1", "arg2", "arg3"})
	assert.Error(t, err)

	err = r.preRunE(r.Command, []string{})
	assert.Error(t, err)

	newRunner := createRunner(context.Background(), fake.NewClientBuilder().Build(), prs, ns, 2)
	err2 := newRunner.preRunE(newRunner.Command, []string{"repo"})
	assert.Error(t, err2)
	assert.ErrorContains(t, err2, "workspace")
}

func TestUpgradeCommand(t *testing.T) {
	ctx := context.Background()

	const (
		origName        = "repo-orig"
		newUpstreamName = "repo-newup"
		localName       = "repo-local"
		localDraftName  = localName + "-draft"
		ns              = "ns"
	)

	origRevision := createOrigPackageRevision(origName, ns, 1)
	newUpstreamRevision := createEditPackageRevision(origRevision, newUpstreamName, 2)
	localRevision := createClonePackageRevision(origRevision, localName, 1, porchapi.PackageRevisionLifecyclePublished)
	localDraftRevision := createClonePackageRevision(origRevision, localName+"-draft", 0, porchapi.PackageRevisionLifecycleDraft)
	prs := []porchapi.PackageRevision{
		*origRevision,
		*newUpstreamRevision,
		*localRevision,
		*localDraftRevision,
	}

	// Create fake client and add the package revision to it
	scheme := runtime.NewScheme()
	if err := porchapi.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add porch API to scheme: %v", err)
	}
	interceptorFuncs := interceptor.Funcs{
		Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if obj.GetObjectKind().GroupVersionKind().Kind == "PackageRevision" {
				obj.SetName("upgraded-pr")
			}
			return nil
		},
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(origRevision, newUpstreamRevision, localRevision, localDraftRevision).
		WithInterceptorFuncs(interceptorFuncs).
		Build()

	output := &bytes.Buffer{}
	commonRunner := createRunner(ctx, client, prs, ns, 2)

	testCases := []struct {
		name           string
		args           []string
		expectedOutput string
		expectedError  string
		runner         *runner
	}{
		{
			name:           "Successful package upgrade",
			args:           []string{localName},
			expectedOutput: fmt.Sprintf("%s upgraded to upgraded-pr\n", localName),
			expectedError:  "",
			runner:         commonRunner,
		},
		{
			name:           "Successful package upgrade by finding latest",
			args:           []string{localName},
			expectedOutput: fmt.Sprintf("%s upgraded to upgraded-pr\n", localName),
			expectedError:  "",
			runner:         createRunner(ctx, client, prs, ns, 0),
		},
		{
			name:           "Draft package revision",
			args:           []string{localDraftName},
			expectedOutput: "",
			expectedError:  fmt.Sprintf("to upgrade a package, it must be in a published state, not %q", porchapi.PackageRevisionLifecycleDraft),
			runner:         commonRunner,
		},
		{
			name:           "Non-existent package revision",
			args:           []string{"non-existent-revision"},
			expectedOutput: "",
			expectedError:  "could not find package revision non-existent-revision",
			runner:         commonRunner,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.runner.Command.SetArgs(tc.args)
			tc.runner.Command.SetOut(output)

			output.Reset()
			err := tc.runner.runE(tc.runner.Command, tc.args)

			if tc.expectedError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expectedError) {
					t.Fatalf("Expected error %q, got %v", tc.expectedError, err)
				}
			} else if err != nil {
				t.Fatalf("Unexpected error: %+v", err)
			}

			if diff := cmp.Diff(strings.TrimSpace(tc.expectedOutput), strings.TrimSpace(output.String())); diff != "" {
				t.Errorf("Unexpected output (-want, +got): %s", diff)
			}
		})
	}
}

func TestFindLatestPR(t *testing.T) {
	const (
		origName        = "repo-orig"
		newUpstreamName = "repo-newup"
		localName       = "repo-local"
		ns              = "ns"
	)

	origRevision := createOrigPackageRevision(origName, ns, 1)
	newUpstreamRevision := createEditPackageRevision(origRevision, newUpstreamName, 2)
	localRevision := createClonePackageRevision(origRevision, localName, 1, porchapi.PackageRevisionLifecyclePublished)
	prs := []porchapi.PackageRevision{
		*origRevision,
		*newUpstreamRevision,
		*localRevision,
	}

	r := createRunner(context.Background(), fake.NewClientBuilder().Build(), prs, ns, 0)

	found := r.findLatestPackageRevisionForRef("orig-testpackage")
	assert.Equal(t, newUpstreamName, found.Name)
	assert.Equal(t, 2, found.Spec.Revision)
}

func TestFindEditOrigin(t *testing.T) {
	const ns = "ns"
	downstreamv1 := porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name: "downstream.v1",
		},
		Spec: porchapi.PackageRevisionSpec{
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeClone,
					Clone: &porchapi.PackageCloneTaskSpec{
						Upstream: porchapi.UpstreamPackage{
							UpstreamRef: &porchapi.PackageRevisionRef{
								Name: "upstream.v1",
							},
						},
					},
				},
			},
		},
	}
	downstreamv2 := porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name: "downstream.v2",
		},
		Spec: porchapi.PackageRevisionSpec{
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeEdit,
					Edit: &porchapi.PackageEditTaskSpec{
						Source: &porchapi.PackageRevisionRef{
							Name: "downstream.v1",
						},
					},
				},
			},
		},
	}
	downstreamv3 := *downstreamv2.DeepCopy()
	downstreamv3.Name = "downstream.v3"
	downstreamv3.Spec.Tasks[0].Edit.Source = &porchapi.PackageRevisionRef{Name: "downstream.v2"}
	prs := []porchapi.PackageRevision{
		downstreamv1,
		downstreamv2,
		downstreamv3,
	}

	r := createRunner(context.Background(), fake.NewClientBuilder().Build(), prs, ns, 0)

	found := r.findUpstreamName(&downstreamv3)
	assert.Equal(t, "upstream.v1", found)
}

func TestDiscoverUpdates(t *testing.T) {
	ctx := context.Background()

	const (
		pkgRevName = "repo-testrevision"
		ns         = "ns"
	)

	basePr := &porchapi.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pkgRevName,
			Namespace: ns,
		},
		Spec: porchapi.PackageRevisionSpec{
			PackageName: "testpackage",
			Lifecycle:   porchapi.PackageRevisionLifecyclePublished,
			Revision:    1,
		},
	}

	basePrV2 := createEditPackageRevision(basePr, "testpackage.v2", 2)

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

	r := createRunnerWithDiscovery(ctx, mockClient, []porchapi.PackageRevision{*basePr, *basePrV2}, ns, "downstream", 0)
	r.Command.SetOut(output)

	output.Reset()
	err := r.discoverUpdates(r.Command, []string{pkgRevName})
	assert.Nil(t, err)
	assert.Equal(t, output.String(), "All downstream packages are up to date.\n")

	output.Reset()
	r = createRunnerWithDiscovery(ctx, mockClient, []porchapi.PackageRevision{*basePr, *basePrV2}, ns, "upstream", 0)
	r.Command.SetOut(output)
	err = r.discoverUpdates(r.Command, []string{pkgRevName})
	assert.Nil(t, err)

	output.Reset()
	r = createRunnerWithDiscovery(ctx, mockClient, []porchapi.PackageRevision{*basePr, *basePrV2}, ns, "", 0)
	r.Command.SetOut(output)
	err = r.discoverUpdates(r.Command, []string{pkgRevName})
	assert.Error(t, err)

	output.Reset()
	testPackageRevision := createClonePackageRevision(basePr, "cloned-pr", 1, porchapi.PackageRevisionLifecycleDraft)

	r = createRunnerWithDiscovery(ctx, mockClient, []porchapi.PackageRevision{*basePr, *testPackageRevision}, ns, "", 2)
	r.Command.SetOut(output)
	_, err = r.doUpgrade(testPackageRevision)
	assert.ErrorContains(t, err, "must be in a published state")

	testPackageRevision.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
	_, err = r.doUpgrade(testPackageRevision)
	assert.EqualError(t, err, "revision 2 does not exist for package testpackage")

	output.Reset()

	mockClient.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	r = createRunnerWithDiscovery(ctx, mockClient, []porchapi.PackageRevision{*basePr, *basePrV2, *testPackageRevision}, ns, "", 2)
	r.Command.SetOut(output)
	_, err = r.doUpgrade(testPackageRevision)
	assert.Nil(t, err)
}
