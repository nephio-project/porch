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
	"math/rand"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	porchapi "github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
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

func makeWsName(revision int) string {
	if revision == -1 {
		return "main"
	}
	if revision > 0 {
		return fmt.Sprintf("v%d", revision)
	}
	return fmt.Sprintf("ws%d", rand.Int()%10000)
}

func setLifecycleAndName(pr *porchapi.PackageRevision, revision int) {
	if revision == 0 {
		pr.Spec.Lifecycle = porchapi.PackageRevisionLifecycleDraft
		pr.Status.UpstreamLock = &porchapi.UpstreamLock{
			Git: &porchapi.GitLock{
				Ref:  fmt.Sprintf("/drafts/%s/%s", pr.Spec.PackageName, pr.Spec.WorkspaceName),
				Repo: "https://github.com/user/repo",
			},
		}
	} else {
		pr.Spec.Lifecycle = porchapi.PackageRevisionLifecyclePublished
		pr.Status.UpstreamLock = &porchapi.UpstreamLock{
			Git: &porchapi.GitLock{
				Ref:  fmt.Sprintf("/%s/v%d", pr.Spec.PackageName, pr.Spec.Revision),
				Repo: "https://github.com/user/repo",
			},
		}
	}

	pr.Name = repository.ComposePkgRevObjName(repository.PackageRevisionKey{
		PkgKey: repository.PackageKey{
			RepoKey: repository.RepositoryKey{
				Namespace: pr.Namespace,
				Name:      pr.Spec.RepositoryName,
			},
			Package: pr.Spec.PackageName,
		},
		Revision:      revision,
		WorkspaceName: pr.Spec.WorkspaceName,
	})
}

func createOrigPackageRevision(namespace, repo, pkgName string, revision int) *porchapi.PackageRevision {
	pr := &porchapi.PackageRevision{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PackageRevision",
			APIVersion: "porch.kpt.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: porchapi.PackageRevisionSpec{
			RepositoryName: repo,
			PackageName:    pkgName,
			Revision:       revision,
			WorkspaceName:  makeWsName(revision),
			Tasks: []porchapi.Task{
				{
					Type: porchapi.TaskTypeInit,
					Init: &porchapi.PackageInitTaskSpec{},
				},
			},
		},
		Status: porchapi.PackageRevisionStatus{},
	}

	setLifecycleAndName(pr, revision)

	return pr
}

func createEditPackageRevision(pr *porchapi.PackageRevision, revision int) *porchapi.PackageRevision {
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
	newPr.Spec.WorkspaceName = makeWsName(revision)

	setLifecycleAndName(newPr, revision)

	return newPr
}

func createClonePackageRevision(pr *porchapi.PackageRevision, pkgName string, revision int) *porchapi.PackageRevision {
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
	newPr.Spec.PackageName = pkgName
	newPr.Spec.Revision = revision
	newPr.Spec.WorkspaceName = makeWsName(revision)

	setLifecycleAndName(newPr, revision)

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

	orig := createOrigPackageRevision(ns, "repo", "orig", 1)
	clone := createClonePackageRevision(orig, "clone", 1)
	prs := []porchapi.PackageRevision{*orig, *clone}
	r := createRunner(context.Background(), fake.NewClientBuilder().Build(), prs, ns, 2)
	err := r.preRunE(r.Command, []string{"arg1", "arg2", "arg3"})
	assert.Error(t, err)

	err = r.preRunE(r.Command, []string{})
	assert.Error(t, err)

	r = createRunner(context.Background(), fake.NewClientBuilder().Build(), prs, ns, 2)
	err = r.preRunE(r.Command, []string{"clone"})
	assert.Error(t, err)
	assert.ErrorContains(t, err, "workspace")
}

func TestUpgradeCommand(t *testing.T) {
	ctx := context.Background()

	origRevision := createOrigPackageRevision("ns", "repo", "orig", 1)
	newUpstreamRevision := createEditPackageRevision(origRevision, 2)
	localRevision := createClonePackageRevision(origRevision, "clone", 1)
	localDraftRevision := createClonePackageRevision(origRevision, "clone-draft", 0)
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
	commonRunner := createRunner(ctx, client, prs, "ns", 2)

	testCases := []struct {
		name           string
		args           []string
		expectedOutput string
		expectedError  string
		runner         *runner
	}{
		{
			name:           "Successful package upgrade",
			args:           []string{localRevision.Name},
			expectedOutput: fmt.Sprintf("%s upgraded to upgraded-pr\n", localRevision.Name),
			expectedError:  "",
			runner:         commonRunner,
		},
		{
			name:           "Successful package upgrade by finding latest",
			args:           []string{localRevision.Name},
			expectedOutput: fmt.Sprintf("%s upgraded to upgraded-pr\n", localRevision.Name),
			expectedError:  "",
			runner:         createRunner(ctx, client, prs, "ns", 0),
		},
		{
			name:           "Draft package revision",
			args:           []string{localDraftRevision.Name},
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
	origRevision := createOrigPackageRevision("ns", "repo", "orig", 1)
	newUpstreamRevision := createEditPackageRevision(origRevision, 2)
	localRevision := createClonePackageRevision(origRevision, "clone", 1)
	prs := []porchapi.PackageRevision{
		*origRevision,
		*newUpstreamRevision,
		*localRevision,
	}

	r := createRunner(context.Background(), fake.NewClientBuilder().Build(), prs, "ns", 0)

	found := r.findLatestPackageRevisionForRef("orig")
	assert.Equal(t, "repo.orig.v2", found.Name)
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
	const ns = "ns"

	ctx := context.Background()

	basePr := createOrigPackageRevision("ns", "repo", "orig", 1)
	basePrV2 := createEditPackageRevision(basePr, 2)

	mockClient := mockclient.NewMockClient(t)
	mockClient.EXPECT().
		List(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).
		Run(func(_ context.Context, list client.ObjectList, opts ...client.ListOption) {
			list.(*configapi.RepositoryList).Items = make([]configapi.Repository, 1)
			list.(*configapi.RepositoryList).Items[0] = configapi.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "repo",
					Namespace: ns,
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

	t.Run("pr does not exist", func(t *testing.T) {
		r := createRunnerWithDiscovery(ctx, mockClient, []porchapi.PackageRevision{}, ns, "upstream", 0)
		r.Command.SetOut(output)

		err := r.discoverUpdates(r.Command, []string{basePr.Name})
		assert.ErrorContains(t, err, "could not find")
	})

	t.Run("all downstream up to date", func(t *testing.T) {
		r := createRunnerWithDiscovery(ctx, mockClient, []porchapi.PackageRevision{*basePr}, ns, "downstream", 0)
		r.Command.SetOut(output)

		err := r.discoverUpdates(r.Command, []string{})
		assert.NoError(t, err)
		assert.Contains(t, output.String(), "All downstream packages are up to date.")
	})

	output.Reset()

	t.Run("downstream update found", func(t *testing.T) {
		clonedPr := createClonePackageRevision(basePr, "clone", 1)
		r := createRunnerWithDiscovery(ctx, mockClient, []porchapi.PackageRevision{*basePr, *basePrV2, *clonedPr}, ns, "downstream", 0)
		r.Command.SetOut(output)
		err := r.discoverUpdates(r.Command, []string{basePrV2.Name})
		assert.NoError(t, err)
		assert.Regexp(t, regexp.MustCompile(`repo\.orig\.v2\s+repo.orig.v1\s+v1->v2`), output.String())
	})

	output.Reset()

	t.Run("all upstreams up to date", func(t *testing.T) {
		r := createRunnerWithDiscovery(ctx, mockClient, []porchapi.PackageRevision{*basePr}, ns, "upstream", 0)
		r.Command.SetOut(output)
		err := r.discoverUpdates(r.Command, []string{basePr.Name})
		assert.NoError(t, err)
		assert.Regexp(t, regexp.MustCompile(`repo\.orig\.v1\s+repo\s+No update available`), output.String())
	})

	output.Reset()

	t.Run("upstream update found", func(t *testing.T) {
		r := createRunnerWithDiscovery(ctx, mockClient, []porchapi.PackageRevision{*basePr, *basePrV2}, ns, "upstream", 0)
		r.Command.SetOut(output)
		err := r.discoverUpdates(r.Command, []string{basePr.Name})
		assert.NoError(t, err)
		assert.Regexp(t, regexp.MustCompile(`repo\.orig\.v1\s+repo\s+v2`), output.String())
	})

	output.Reset()

	t.Run("invalid discovery param", func(t *testing.T) {
		r := createRunnerWithDiscovery(ctx, mockClient, []porchapi.PackageRevision{*basePr, *basePrV2}, ns, "lowstream", 0)
		r.Command.SetOut(output)
		err := r.discoverUpdates(r.Command, []string{basePr.Name})
		assert.ErrorContains(t, err, "invalid argument \"lowstream\" for --discover")
	})
}

func TestPreRunStrategyValidation(t *testing.T) {
	ns := "ns"
	fakeClient := fake.NewClientBuilder().Build()
	cfg := &genericclioptions.ConfigFlags{Namespace: &ns}
	ctx := context.Background()

	testCases := []struct {
		name          string
		strategy      string
		expectErr     bool
		expectedError string
	}{
		{
			name:          "Valid strategy: copy-merge",
			strategy:      string(porchapi.CopyMerge),
			expectErr:     true,
			expectedError: "the server is currently unable to handle the request",
		},
		{
			name:          "Empty strategy is valid (uses default resource-merge)",
			strategy:      "",
			expectErr:     true,
			expectedError: "the server is currently unable to handle the request",
		},
		{
			name:          "Invalid strategy",
			strategy:      "non-existent-strategy",
			expectErr:     true,
			expectedError: "invalid strategy \"non-existent-strategy\"; must be one of:",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner{
				ctx:       ctx,
				cfg:       cfg,
				client:    fakeClient,
				revision:  2,
				workspace: "v2",
				strategy:  tc.strategy,
			}
			r.Command = NewCommand(r.ctx, r.cfg)

			err := r.preRunE(r.Command, []string{"some-package-revision"})

			if tc.expectErr {
				assert.Error(t, err)
				if tc.expectedError != "" {
					assert.Contains(t, err.Error(), tc.expectedError)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
