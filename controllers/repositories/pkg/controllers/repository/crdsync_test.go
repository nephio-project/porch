// Copyright 2026 The kpt and Nephio Authors
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

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	kptfilev1 "github.com/kptdev/kpt/pkg/api/kptfile/v1"
	porchv1alpha1 "github.com/nephio-project/porch/api/porch/v1alpha1"
	porchv1alpha2 "github.com/nephio-project/porch/api/porch/v1alpha2"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mockclient "github.com/nephio-project/porch/test/mockery/mocks/external/sigs.k8s.io/controller-runtime/pkg/client"
)

// --- Test helpers ---

func newTestCRDRepo() *configapi.Repository {
	return &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-repo",
			Namespace: "default",
			UID:       types.UID("repo-uid"),
		},
		Spec: configapi.RepositorySpec{Deployment: true},
	}
}

func newFakePkgRev(pkg, workspace string, lifecycle porchv1alpha2.PackageRevisionLifecycle) *fakePackageRevision {
	return &fakePackageRevision{
		key: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				RepoKey: repository.RepositoryKey{Namespace: "default", Name: "my-repo"},
				Package: pkg,
			},
			WorkspaceName: workspace,
		},
		lifecycle: porchv1alpha1.PackageRevisionLifecycle(lifecycle),
		kptfile:   kptfilev1.KptFile{},
	}
}

// mockListReturning sets up a mock List that populates the result with the given CRDs.
func mockListReturning(m *mockclient.MockClient, items []porchv1alpha2.PackageRevision) {
	m.EXPECT().List(mock.Anything, mock.AnythingOfType("*v1alpha2.PackageRevisionList"), mock.Anything, mock.Anything).
		Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
			list.(*porchv1alpha2.PackageRevisionList).Items = items
		}).Return(nil)
}

// mockApplySuccess sets up mock expectations for a successful SSA apply (Patch + Status().Patch).
func mockApplySuccess(t *testing.T, m *mockclient.MockClient) {
	t.Helper()
	m.EXPECT().Patch(mock.Anything, mock.AnythingOfType("*v1alpha2.PackageRevision"), mock.Anything, mock.Anything, mock.Anything).Return(nil)
	sw := mockclient.NewMockSubResourceWriter(t)
	m.EXPECT().Status().Return(sw)
	sw.EXPECT().Patch(mock.Anything, mock.AnythingOfType("*v1alpha2.PackageRevision"), mock.Anything, mock.Anything, mock.Anything).Return(nil)
}

// --- fakePackageRevision ---

// fakePackageRevision implements repository.PackageRevision and the optional
// GetCommitInfo/IsLatestRevision interfaces for testing.
type fakePackageRevision struct {
	key          repository.PackageRevisionKey
	lifecycle    porchv1alpha1.PackageRevisionLifecycle
	kptfile      kptfilev1.KptFile
	upstreamLock kptfilev1.Locator
	selfLock     kptfilev1.Locator
	commitTime   time.Time
	commitAuthor string
	isLatest     bool
}

func (f *fakePackageRevision) KubeObjectNamespace() string                  { return f.key.RKey().Namespace }
func (f *fakePackageRevision) KubeObjectName() string                       { return repository.ComposePkgRevObjName(f.key) }
func (f *fakePackageRevision) Key() repository.PackageRevisionKey           { return f.key }
func (f *fakePackageRevision) UID() types.UID                               { return "" }
func (f *fakePackageRevision) ResourceVersion() string                      { return "" }
func (f *fakePackageRevision) GetMeta() metav1.ObjectMeta                   { return metav1.ObjectMeta{} }
func (f *fakePackageRevision) SetMeta(_ context.Context, _ metav1.ObjectMeta) error { return nil }
func (f *fakePackageRevision) Lifecycle(_ context.Context) porchv1alpha1.PackageRevisionLifecycle {
	return f.lifecycle
}
func (f *fakePackageRevision) UpdateLifecycle(_ context.Context, _ porchv1alpha1.PackageRevisionLifecycle) error {
	return nil
}
func (f *fakePackageRevision) GetPackageRevision(_ context.Context) (*porchv1alpha1.PackageRevision, error) {
	return nil, nil
}
func (f *fakePackageRevision) GetResources(_ context.Context) (*porchv1alpha1.PackageRevisionResources, error) {
	return nil, nil
}
func (f *fakePackageRevision) GetUpstreamLock(_ context.Context) (kptfilev1.Upstream, kptfilev1.Locator, error) {
	return kptfilev1.Upstream{}, f.upstreamLock, nil
}
func (f *fakePackageRevision) GetKptfile(_ context.Context) (kptfilev1.KptFile, error) {
	return f.kptfile, nil
}
func (f *fakePackageRevision) GetLock(_ context.Context) (kptfilev1.Upstream, kptfilev1.Locator, error) {
	return kptfilev1.Upstream{}, f.selfLock, nil
}
func (f *fakePackageRevision) ToMainPackageRevision(_ context.Context) repository.PackageRevision {
	return nil
}
func (f *fakePackageRevision) GetCommitInfo() (time.Time, string) {
	return f.commitTime, f.commitAuthor
}
func (f *fakePackageRevision) IsLatestRevision() bool { return f.isLatest }

// --- Tests: buildPackageRevisionCRD ---

func TestBuildPackageRevisionCRD(t *testing.T) {
	ctx := context.Background()
	repo := newTestCRDRepo()

	t.Run("published with full metadata", func(t *testing.T) {
		pkgRev := newFakePkgRev("my-pkg", "v3", porchv1alpha2.PackageRevisionLifecyclePublished)
		pkgRev.key.PkgKey.Path = "path/to"
		pkgRev.key.Revision = 3
		pkgRev.commitTime = time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
		pkgRev.commitAuthor = "user@example.com"
		pkgRev.kptfile = kptfilev1.KptFile{
			Info: &kptfilev1.PackageInfo{
				ReadinessGates: []kptfilev1.ReadinessGate{{ConditionType: "Ready"}},
			},
			Status: &kptfilev1.Status{
				Conditions: []kptfilev1.Condition{
					{Type: "Ready", Status: kptfilev1.ConditionTrue, Reason: "AllGood"},
				},
			},
		}
		pkgRev.upstreamLock = kptfilev1.Locator{
			Type: kptfilev1.GitOrigin,
			Git:  &kptfilev1.GitLock{Repo: "https://github.com/upstream.git", Ref: "v1.0", Directory: "/", Commit: "abc"},
		}
		pkgRev.selfLock = kptfilev1.Locator{
			Type: kptfilev1.GitOrigin,
			Git:  &kptfilev1.GitLock{Repo: "https://github.com/self.git", Ref: "main", Directory: "path/to/my-pkg", Commit: "def"},
		}

		crd, err := buildPackageRevisionCRD(ctx, repo, pkgRev)
		assert.NoError(t, err)

		// Metadata
		assert.Equal(t, "PackageRevision", crd.Kind)
		assert.Equal(t, porchv1alpha2.SchemeGroupVersion.Identifier(), crd.APIVersion)
		assert.Equal(t, "default", crd.Namespace)
		assert.Equal(t, repo.Name, crd.OwnerReferences[0].Name)
		assert.Equal(t, repo.UID, crd.OwnerReferences[0].UID)

		// Spec
		assert.Equal(t, "path/to/my-pkg", crd.Spec.PackageName)
		assert.Equal(t, "my-repo", crd.Spec.RepositoryName)
		assert.Equal(t, "v3", crd.Spec.WorkspaceName)
		assert.Equal(t, porchv1alpha2.PackageRevisionLifecyclePublished, crd.Spec.Lifecycle)
		assert.Len(t, crd.Spec.ReadinessGates, 1)
		assert.Equal(t, "Ready", crd.Spec.ReadinessGates[0].ConditionType)

		// Status — published fields
		assert.Equal(t, 3, crd.Status.Revision)
		assert.Equal(t, "user@example.com", crd.Status.PublishedBy)
		assert.NotNil(t, crd.Status.PublishedAt)
		assert.True(t, crd.Status.Deployment)

		// Status — locks
		assert.Equal(t, "https://github.com/upstream.git", crd.Status.UpstreamLock.Git.Repo)
		assert.Equal(t, "https://github.com/self.git", crd.Status.SelfLock.Git.Repo)

		// Status — conditions
		assert.Len(t, crd.Status.PackageConditions, 1)
		assert.Equal(t, "Ready", crd.Status.PackageConditions[0].Type)

		// Labels
		assert.Equal(t, "my-repo", crd.Labels[RepositoryLabel])
	})

	t.Run("draft has no publish metadata", func(t *testing.T) {
		pkgRev := newFakePkgRev("draft-pkg", "ws1", porchv1alpha2.PackageRevisionLifecycleDraft)

		crd, err := buildPackageRevisionCRD(ctx, repo, pkgRev)
		assert.NoError(t, err)

		assert.Equal(t, 0, crd.Status.Revision)
		assert.Empty(t, crd.Status.PublishedBy)
		assert.Nil(t, crd.Status.PublishedAt)
		assert.Equal(t, porchv1alpha2.PackageRevisionLifecycleDraft, crd.Spec.Lifecycle)
	})

	t.Run("empty kptfile yields nil optional fields", func(t *testing.T) {
		pkgRev := newFakePkgRev("bare-pkg", "ws1", porchv1alpha2.PackageRevisionLifecycleDraft)

		crd, err := buildPackageRevisionCRD(ctx, repo, pkgRev)
		assert.NoError(t, err)

		assert.Nil(t, crd.Spec.ReadinessGates)
		assert.Nil(t, crd.Spec.PackageMetadata)
		assert.Nil(t, crd.Status.PackageConditions)
		assert.Nil(t, crd.Status.UpstreamLock)
		assert.Nil(t, crd.Status.SelfLock)
	})
}

// --- Tests: packageRevisionCRDUpToDate ---

func TestPackageRevisionCRDUpToDate(t *testing.T) {
	base := &porchv1alpha2.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{RepositoryLabel: "repo1"},
		},
		Spec: porchv1alpha2.PackageRevisionSpec{
			PackageName: "pkg1",
			Lifecycle:   porchv1alpha2.PackageRevisionLifecyclePublished,
		},
		Status: porchv1alpha2.PackageRevisionStatus{Revision: 1},
	}

	tests := []struct {
		name     string
		modify   func(*porchv1alpha2.PackageRevision)
		expected bool
	}{
		{name: "identical", modify: nil, expected: true},
		{name: "lifecycle changed - still up to date (client-owned)", modify: func(pr *porchv1alpha2.PackageRevision) {
			pr.Spec.Lifecycle = porchv1alpha2.PackageRevisionLifecycleDraft
		}, expected: true},
		{name: "status changed (packageConditions — PR controller owned)", modify: func(pr *porchv1alpha2.PackageRevision) {
			pr.Status.PackageConditions = []porchv1alpha2.PackageCondition{{Type: "new"}}
		}, expected: true},
		{name: "labels changed", modify: func(pr *porchv1alpha2.PackageRevision) {
			pr.Labels[porchv1alpha2.LatestPackageRevisionKey] = porchv1alpha2.LatestPackageRevisionValue
		}, expected: false},
		{name: "annotations differ - still up to date", modify: func(pr *porchv1alpha2.PackageRevision) {
			pr.Annotations = map[string]string{"foo": "bar"}
		}, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := base.DeepCopy()
			if tt.modify != nil {
				tt.modify(desired)
			}
			assert.Equal(t, tt.expected, packageRevisionCRDUpToDate(base, desired))
		})
	}
}

// --- Tests: packageRevisionLabels ---

func TestPackageRevisionLabels(t *testing.T) {
	tests := []struct {
		name       string
		isLatest   bool
		wantLatest bool
	}{
		{name: "non-latest", isLatest: false, wantLatest: false},
		{name: "latest", isLatest: true, wantLatest: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkgRev := &fakePackageRevision{isLatest: tt.isLatest}
			labels := packageRevisionLabels("my-repo", pkgRev)
			assert.Equal(t, "my-repo", labels[RepositoryLabel])
			if tt.wantLatest {
				assert.Equal(t, porchv1alpha2.LatestPackageRevisionValue, labels[porchv1alpha2.LatestPackageRevisionKey])
			} else {
				assert.Equal(t, "false", labels[porchv1alpha2.LatestPackageRevisionKey])
			}
		})
	}
}

// --- Tests: syncPackageRevisionCRDs ---

func TestSyncPackageRevisionCRDs(t *testing.T) {
	ctx := context.Background()
	repo := newTestCRDRepo()
	draftPkgRev := newFakePkgRev("pkg1", "ws1", porchv1alpha2.PackageRevisionLifecycleDraft)

	staleCRD := porchv1alpha2.PackageRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stale-crd",
			Namespace: "default",
			Labels:    map[string]string{RepositoryLabel: "my-repo"},
		},
	}

	// Pre-build the "up to date" CRD for the skip test
	upToDateCRD, _ := buildPackageRevisionCRD(ctx, repo, draftPkgRev)

	tests := []struct {
		name        string
		pkgRevs     []repository.PackageRevision
		setupMocks  func(t *testing.T, m *mockclient.MockClient)
		expectError string
	}{
		{
			name:    "creates new CRD",
			pkgRevs: []repository.PackageRevision{draftPkgRev},
			setupMocks: func(t *testing.T, m *mockclient.MockClient) {
				mockListReturning(m, nil)
				mockApplySuccess(t, m)
			},
		},
		{
			name:    "skips up-to-date CRD",
			pkgRevs: []repository.PackageRevision{draftPkgRev},
			setupMocks: func(t *testing.T, m *mockclient.MockClient) {
				mockListReturning(m, []porchv1alpha2.PackageRevision{*upToDateCRD})
			},
		},
		{
			name:    "deletes stale CRD",
			pkgRevs: nil,
			setupMocks: func(t *testing.T, m *mockclient.MockClient) {
				mockListReturning(m, []porchv1alpha2.PackageRevision{staleCRD})
				m.EXPECT().Delete(mock.Anything, mock.AnythingOfType("*v1alpha2.PackageRevision")).Return(nil)
			},
		},
		{
			name:    "deletes only stale CRDs when mixed with current",
			pkgRevs: []repository.PackageRevision{draftPkgRev},
			setupMocks: func(t *testing.T, m *mockclient.MockClient) {
				// List returns both the current CRD (matching draftPkgRev) and a stale one
				mockListReturning(m, []porchv1alpha2.PackageRevision{*upToDateCRD, staleCRD})
				// Only the stale CRD should be deleted
				m.EXPECT().Delete(mock.Anything, mock.MatchedBy(func(obj client.Object) bool {
					return obj.GetName() == "stale-crd"
				})).Return(nil)
			},
		},
		{
			name:    "delete error logged but sync succeeds",
			pkgRevs: nil,
			setupMocks: func(t *testing.T, m *mockclient.MockClient) {
				mockListReturning(m, []porchv1alpha2.PackageRevision{staleCRD})
				m.EXPECT().Delete(mock.Anything, mock.AnythingOfType("*v1alpha2.PackageRevision")).Return(fmt.Errorf("delete failed"))
			},
		},
		{
			name: "list error",
			setupMocks: func(t *testing.T, m *mockclient.MockClient) {
				m.EXPECT().List(mock.Anything, mock.AnythingOfType("*v1alpha2.PackageRevisionList"), mock.Anything, mock.Anything).
					Return(fmt.Errorf("list failed"))
			},
			expectError: "list failed",
		},
		{
			name:    "apply error logged but sync succeeds",
			pkgRevs: []repository.PackageRevision{draftPkgRev},
			setupMocks: func(t *testing.T, m *mockclient.MockClient) {
				mockListReturning(m, nil)
				m.EXPECT().Patch(mock.Anything, mock.AnythingOfType("*v1alpha2.PackageRevision"), mock.Anything, mock.Anything, mock.Anything).
					Return(fmt.Errorf("apply failed"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mockclient.NewMockClient(t)
			tt.setupMocks(t, mockClient)

			r := &RepositoryReconciler{Client: mockClient}
			err := r.syncPackageRevisionCRDs(ctx, repo, tt.pkgRevs)

			if tt.expectError != "" {
				assert.ErrorContains(t, err, tt.expectError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- Tests: applyPackageRevisionCRD ---

func TestApplyPackageRevisionCRD(t *testing.T) {
	ctx := context.Background()

	crd := &porchv1alpha2.PackageRevision{
		TypeMeta:   metav1.TypeMeta{Kind: "PackageRevision", APIVersion: porchv1alpha2.SchemeGroupVersion.Identifier()},
		ObjectMeta: metav1.ObjectMeta{Name: "test-crd", Namespace: "default"},
		Spec:       porchv1alpha2.PackageRevisionSpec{PackageName: "pkg1"},
		Status:     porchv1alpha2.PackageRevisionStatus{Revision: 1},
	}

	tests := []struct {
		name        string
		setupMocks  func(t *testing.T, m *mockclient.MockClient)
		expectError string
	}{
		{
			name: "success",
			setupMocks: func(t *testing.T, m *mockclient.MockClient) {
				mockApplySuccess(t, m)
			},
		},
		{
			name: "spec patch fails",
			setupMocks: func(t *testing.T, m *mockclient.MockClient) {
				m.EXPECT().Patch(mock.Anything, mock.AnythingOfType("*v1alpha2.PackageRevision"), mock.Anything, mock.Anything, mock.Anything).
					Return(fmt.Errorf("spec patch failed"))
			},
			expectError: "spec patch failed",
		},
		{
			name: "status patch fails",
			setupMocks: func(t *testing.T, m *mockclient.MockClient) {
				m.EXPECT().Patch(mock.Anything, mock.AnythingOfType("*v1alpha2.PackageRevision"), mock.Anything, mock.Anything, mock.Anything).Return(nil)
				sw := mockclient.NewMockSubResourceWriter(t)
				m.EXPECT().Status().Return(sw)
				sw.EXPECT().Patch(mock.Anything, mock.AnythingOfType("*v1alpha2.PackageRevision"), mock.Anything, mock.Anything, mock.Anything).
					Return(fmt.Errorf("status patch failed"))
			},
			expectError: "status patch failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mockclient.NewMockClient(t)
			tt.setupMocks(t, mockClient)

			r := &RepositoryReconciler{Client: mockClient}
			err := r.applyPackageRevisionCRD(ctx, crd.DeepCopy(), true)

			if tt.expectError != "" {
				assert.ErrorContains(t, err, tt.expectError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}


func TestBuildPackageRevisionCRDForUpdateOmitsKptfileFields(t *testing.T) {
	ctx := context.Background()
	repo := &configapi.Repository{
		ObjectMeta: metav1.ObjectMeta{Name: "my-repo", Namespace: "default", UID: "repo-uid"},
		Spec:       configapi.RepositorySpec{Deployment: true},
	}

	pkgRev := &fakePackageRevision{}
	pkgRev.key = repository.PackageRevisionKey{
		PkgKey:        repository.PackageKey{RepoKey: repository.RepositoryKey{Namespace: "default", Name: "my-repo"}, Package: "my-pkg"},
		WorkspaceName: "v1",
	}
	pkgRev.kptfile = kptfilev1.KptFile{
		Info: &kptfilev1.PackageInfo{
			ReadinessGates: []kptfilev1.ReadinessGate{{ConditionType: "Ready"}},
		},
		Status: &kptfilev1.Status{
			Conditions: []kptfilev1.Condition{
				{Type: "Ready", Status: kptfilev1.ConditionTrue},
			},
		},
	}

	crd, err := buildPackageRevisionCRDForUpdate(ctx, repo, pkgRev)
	assert.NoError(t, err)

	// Verify identity fields are present.
	assert.Equal(t, "my-pkg", crd.Spec.PackageName)
	assert.Equal(t, "my-repo", crd.Spec.RepositoryName)
	assert.Equal(t, "v1", crd.Spec.WorkspaceName)
	assert.True(t, crd.Status.Deployment)

	// Verify Kptfile-derived fields are omitted (PR controller owns them).
	assert.Nil(t, crd.Spec.ReadinessGates)
	assert.Nil(t, crd.Spec.PackageMetadata)
	assert.Nil(t, crd.Status.PackageConditions)
}
