// Copyright 2025 The kpt and Nephio Authors
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
	"testing"

	api "github.com/nephio-project/porch/api/porch/v1alpha1"
	kptfile "github.com/nephio-project/porch/pkg/kpt/api/kptfile/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRevision2Int(t *testing.T) {
	assert.Equal(t, 123, Revision2Int("123"))
	assert.Equal(t, 123, Revision2Int("v123"))
	assert.Equal(t, -1, Revision2Int("V123"))
	assert.Equal(t, -1, Revision2Int("v123v"))
	assert.Equal(t, -1, Revision2Int("-1"))
	assert.Equal(t, 0, Revision2Int("v0"))
	assert.Equal(t, 0, Revision2Int("0"))
}

func TestRevision2Str(t *testing.T) {
	assert.Equal(t, "123", Revision2Str(123))
	assert.Equal(t, "-1", Revision2Str(-1))
}

func TestComposePkgObjName(t *testing.T) {
	pkgKey := PackageKey{
		RepoKey: RepositoryKey{
			Namespace:         "the-ns",
			Name:              "the-repo",
			Path:              "the/dir/path",
			PlaceholderWSname: "the-placeholder-ws-name",
		},
		Path:    "the/pkg/path",
		Package: "the-package-name",
	}

	assert.Equal(t, "the-repo.the.pkg.path.the-package-name", ComposePkgObjName(pkgKey))

	pkgKey.Path = ""
	assert.Equal(t, "the-repo.the-package-name", ComposePkgObjName(pkgKey))
}

func TestComposePkgRevObjName(t *testing.T) {
	pkgRevKey := PackageRevisionKey{
		PkgKey: PackageKey{
			RepoKey: RepositoryKey{
				Namespace:         "the-ns",
				Name:              "the-repo",
				Path:              "the/dir/path",
				PlaceholderWSname: "the-placeholder-ws-name",
			},
			Path:    "the/pkg/path",
			Package: "the-package-name",
		},
		Revision:      123,
		WorkspaceName: "the-ws-name",
	}

	assert.Equal(t, "the-repo.the.pkg.path.the-package-name.the-ws-name", ComposePkgRevObjName(pkgRevKey))

	pkgRevKey.Revision = -1
	assert.Equal(t, "the-repo.the.pkg.path.the-package-name.the-ws-name", ComposePkgRevObjName(pkgRevKey))
}

func TestToAPIReadinessGate(t *testing.T) {
	kf := kptfile.KptFile{}

	readinessGates := ToAPIReadinessGates(kf)
	assert.Equal(t, 0, len(readinessGates))

	kf.Info = &kptfile.PackageInfo{
		ReadinessGates: []kptfile.ReadinessGate{},
	}
	readinessGates = ToAPIReadinessGates(kf)
	assert.Equal(t, 0, len(readinessGates))

	kf.Info.ReadinessGates = append(kf.Info.ReadinessGates, kptfile.ReadinessGate{
		ConditionType: "AConditionType",
	})
	readinessGates = ToAPIReadinessGates(kf)
	assert.Equal(t, 1, len(readinessGates))
}

func TestToAPIConditions(t *testing.T) {
	kf := kptfile.KptFile{}

	conditions := ToAPIConditions(kf)
	assert.Equal(t, 0, len(conditions))

	kf.Status = &kptfile.Status{
		Conditions: []kptfile.Condition{},
	}
	conditions = ToAPIConditions(kf)
	assert.Equal(t, 0, len(conditions))

	kf.Status.Conditions = append(kf.Status.Conditions, kptfile.Condition{
		Type:    "AConditionType",
		Status:  kptfile.ConditionTrue,
		Reason:  "A Reason",
		Message: "A Message",
	})
	conditions = ToAPIConditions(kf)
	assert.Equal(t, 1, len(conditions))
}

func TestToAPIConditionStatus(t *testing.T) {
	assert.Equal(t, api.ConditionTrue, ToAPIConditionStatus(kptfile.ConditionTrue))
	assert.Equal(t, api.ConditionFalse, ToAPIConditionStatus(kptfile.ConditionFalse))
	assert.Equal(t, api.ConditionUnknown, ToAPIConditionStatus(kptfile.ConditionUnknown))
}

func TestUpsertAPICondition(t *testing.T) {
	conditions := []api.Condition{}

	condition0 := api.Condition{
		Type: "Condition0",
	}

	upsertedConditions := UpsertAPICondition(conditions, condition0)
	assert.Equal(t, 1, len(upsertedConditions))
	assert.Equal(t, condition0, upsertedConditions[0])

	upsertedConditions = UpsertAPICondition(upsertedConditions, condition0)
	assert.Equal(t, 1, len(upsertedConditions))
	assert.Equal(t, condition0, upsertedConditions[0])

	condition1 := api.Condition{
		Type: "Condition1",
	}

	upsertedConditions = UpsertAPICondition(upsertedConditions, condition1)
	assert.Equal(t, 2, len(upsertedConditions))
	assert.Equal(t, condition0, upsertedConditions[0])
	assert.Equal(t, condition1, upsertedConditions[1])

	upsertedConditions = UpsertAPICondition(upsertedConditions, condition0)
	assert.Equal(t, 2, len(upsertedConditions))
	assert.Equal(t, condition0, upsertedConditions[1])
	assert.Equal(t, condition1, upsertedConditions[0])
}

func TestAnyBlockOwnerDeletionSet(t *testing.T) {
	assert.False(t, AnyBlockOwnerDeletionSet(metav1.ObjectMeta{}))

	meta := metav1.ObjectMeta{
		OwnerReferences: []metav1.OwnerReference{},
	}
	assert.False(t, AnyBlockOwnerDeletionSet(meta))

	ownerRef := metav1.OwnerReference{}
	meta.OwnerReferences = append(meta.OwnerReferences, ownerRef)
	assert.True(t, AnyBlockOwnerDeletionSet(meta))

	blockOwnerDeletion := false
	meta.OwnerReferences[0].BlockOwnerDeletion = &blockOwnerDeletion
	assert.False(t, AnyBlockOwnerDeletionSet(meta))

	blockOwnerDeletion = true
	assert.True(t, AnyBlockOwnerDeletionSet(meta))
}

func TestTestPrSliceToMap(t *testing.T) {
	assert.Equal(t, 0, len(PrSlice2Map([]PackageRevision{})))

	prSlice := []PackageRevision{}

	assert.Equal(t, 0, len(PrSlice2Map(prSlice)))
}

func TestKptUpstreamLock2APIUpstreamLock(t *testing.T) {
	kptLock := kptfile.UpstreamLock{}
	assert.Nil(t, KptUpstreamLock2APIUpstreamLock(kptLock).Git)

	kptLock.Git = &kptfile.GitLock{
		Repo: "my-repo",
	}

	assert.Equal(t, "my-repo", KptUpstreamLock2APIUpstreamLock(kptLock).Git.Repo)
}

func TestKptUpstreamLock2KptUpstream(t *testing.T) {
	kptLock := kptfile.UpstreamLock{}
	assert.Nil(t, KptUpstreamLock2KptUpstream(kptLock).Git)

	kptLock.Git = &kptfile.GitLock{
		Repo: "my-repo",
	}

	assert.Equal(t, "my-repo", KptUpstreamLock2KptUpstream(kptLock).Git.Repo)
}
