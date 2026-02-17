// Copyright 2026 The Nephio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package disaster

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/helper/chroot"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/filesystem"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/test/disaster/api/environment/gitea"
	"github.com/nephio-project/porch/test/disaster/api/environment/kind"
	"github.com/nephio-project/porch/test/disaster/api/environment/packagevariants"
	"github.com/nephio-project/porch/test/disaster/api/environment/packagevariantsets"
	"github.com/nephio-project/porch/test/disaster/api/environment/pods"
	repositories "github.com/nephio-project/porch/test/disaster/api/environment/porchrepositories"
	"github.com/nephio-project/porch/test/disaster/api/environment/postgres"
	"github.com/nephio-project/porch/test/e2e/suiteutils"
	"github.com/stretchr/testify/suite"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PorchDisasterSuite struct {
	suiteutils.MultiClusterTestSuite

	skipVariants, skipVariantSets bool
}

var (
	reconcileDescription = "reconcile [Repositories, PackageVariants, PackageVariantSets]"
)

func TestDisasterRecovery(t *testing.T) {
	// Skip if not running disaster-recovery tests
	if os.Getenv("DISASTER") == "" {
		t.Skip("Skipping disaster-recovery tests in non-disaster-recovery environment")
	}

	suite.Run(t, &PorchDisasterSuite{
		skipVariants:    true,
		skipVariantSets: true,
	})
}

func (t *PorchDisasterSuite) SetupSuite() {
	s := &t.MultiClusterTestSuite

	t.PorchRoot = func() string {

		repo, _ := git.PlainOpen("../../../")

		// Try to grab the repository Storer
		s := repo.Storer.(*filesystem.Storage)
		// Try to get the underlying billy.Filesystem
		fs := s.Filesystem().(*chroot.ChrootHelper)

		return strings.Replace(fs.Root(), "/.git", "", 1)
	}()

	if t.skipVariantSets {
		os.Setenv("SKIP_VARIANT_SETS", "true")
		reconcileDescription = strings.ReplaceAll(reconcileDescription, ", PackageVariantSets", "")
	}
	if t.skipVariants {
		os.Setenv("SKIP_VARIANTS", "true")
		reconcileDescription = strings.ReplaceAll(reconcileDescription, ", PackageVariants", "")
	}

	if os.Getenv("SETUP_ENV") == "true" {
		setupEnv(s)
	}

	s.SetupSuite()
}

func (t *PorchDisasterSuite) SetupTest() {
	s := &t.MultiClusterTestSuite
	kind.UseDBCacheCluster(s)
	actualCounts := t.CountPackageRevisions()
	if t.T().Failed() {
		t.Logf("Unable to get initial count of package revisions - resetting environment")
		setupEnv(s)
	}

	environmentReady := func() bool {
		expectedCountsBefore := &suiteutils.PackageRevisionStatusCounts{
			Total:            1104,
			Draft:            10,
			Proposed:         10,
			Published:        1074,
			DeletionProposed: 10,
		}

		return actualCounts.Total == expectedCountsBefore.Total &&
			actualCounts.Draft == expectedCountsBefore.Draft &&
			actualCounts.Proposed == expectedCountsBefore.Proposed &&
			actualCounts.Published == expectedCountsBefore.Published &&
			actualCounts.DeletionProposed == expectedCountsBefore.DeletionProposed
	}

	if !environmentReady() {
		if _, err := os.Stat("./dumped_db.sql"); err == nil {
			t.Logf("found DB cache backup from previous test - restoring from it")
			postgres.Restore(s)
			kind.UseDBCacheCluster(s)
		} else {
			t.Logf("resetting environment before test")
			resetEnv(s)
		}
	}

	actualCounts = t.CountPackageRevisions()
	if !environmentReady() {
		t.Logf("DB cache restoration did not restore all expected package revisions - resetting environment")
		resetEnv(s)
	}
}

func (t *PorchDisasterSuite) TestCompleteDisaster() {
	s := &t.MultiClusterTestSuite

	gitea.Backup(s)
	postgres.Backup(s)

	kind.UseDBCacheCluster(s)
	repositoriesToReconcile := repositories.Backup(s)
	variantsToReconcile := packagevariants.Backup(s)
	variantSetsToReconcile := packagevariantsets.Backup(s)

	kind.Wipe(s)
	gitea.Wipe(s)
	postgres.Wipe(s)

	kind.Reinstall(s)
	gitea.Restore(s)
	postgres.Restore(s)

	t.TimingHelper(reconcileDescription, func(t *suiteutils.TestSuite) {
		t.T().Helper()
		kind.UseDBCacheCluster(s)
		repositories.Reconcile(s, repositoriesToReconcile, 20)
		packagevariants.Reconcile(s, variantsToReconcile, 20)
		packagevariantsets.Reconcile(s, variantSetsToReconcile, 20)
	})

	expectedCountsAfter := &suiteutils.PackageRevisionStatusCounts{
		Total:            1104,
		Draft:            10,
		Proposed:         10,
		Published:        1074,
		DeletionProposed: 10,
	}
	t.PackageRevisionCountsMustMatch(expectedCountsAfter)
}

func (t *PorchDisasterSuite) TestKubernetesClusterLoss() {
	s := &t.MultiClusterTestSuite

	kind.UseDBCacheCluster(s)
	repositoriesToReconcile := repositories.Backup(s)
	variantsToReconcile := packagevariants.Backup(s)
	variantSetsToReconcile := packagevariantsets.Backup(s)

	kind.Wipe(s)

	kind.Reinstall(s)

	t.TimingHelper(reconcileDescription, func(t *suiteutils.TestSuite) {
		t.T().Helper()
		kind.UseDBCacheCluster(s)
		repositories.Reconcile(s, repositoriesToReconcile, 20)
		packagevariants.Reconcile(s, variantsToReconcile, 20)
		packagevariantsets.Reconcile(s, variantSetsToReconcile, 20)
	})

	expectedCountsAfter := &suiteutils.PackageRevisionStatusCounts{
		Total:            1104,
		Draft:            10,
		Proposed:         10,
		Published:        1074,
		DeletionProposed: 10,
	}
	t.PackageRevisionCountsMustMatch(expectedCountsAfter)
}

func (t *PorchDisasterSuite) TestDBCacheLossWithoutBackup() {
	s := &t.MultiClusterTestSuite

	//************************************************************
	// test case is "without a backup", but we need one afterward
	// to restore the environment to a known state for other tests
	//************************************************************
	postgres.Backup(s)

	kind.UseDBCacheCluster(s)
	repositoriesToReconcile := repositories.Backup(s)
	variantsToReconcile := packagevariants.Backup(s)
	variantSetsToReconcile := packagevariantsets.Backup(s)

	postgres.Wipe(s)

	t.TimingHelper("restart porch-server and rebuild from Git", func(t *suiteutils.TestSuite) {
		t.T().Helper()
		kind.UseDBCacheCluster(s)
		repositories.Reconcile(s, repositoriesToReconcile, 20)
		packagevariants.Reconcile(s, variantsToReconcile, 20)
		packagevariantsets.Reconcile(s, variantSetsToReconcile, 20)

		// no package revisions available with no DB cache
		expectedCountsAfter := &suiteutils.PackageRevisionStatusCounts{
			Total:            0,
			Draft:            0,
			Proposed:         0,
			Published:        0,
			DeletionProposed: 0,
		}
		s.PackageRevisionCountsMustMatch(expectedCountsAfter)

		// restarting porch-server microservice allows it to rebuild what it can from Git
		// Draft, Proposed, and DeletionProposed data is lost
		pods.RestartPorchServer(s)
		expectedCountsAfterServerRestart := &suiteutils.PackageRevisionStatusCounts{
			Total:            1084,
			Draft:            0,
			Proposed:         0,
			Published:        1084,
			DeletionProposed: 0,
		}
		s.PackageRevisionCountsMustMatch(expectedCountsAfterServerRestart)
	})

	//*******************************
	// restore environment after test
	//*******************************
	postgres.Restore(s)
}

func (t *PorchDisasterSuite) TestDBCacheLossWithBackup() {
	s := &t.MultiClusterTestSuite

	postgres.Backup(s)

	kind.UseDBCacheCluster(s)
	repositoriesToReconcile := repositories.Backup(s)
	variantsToReconcile := packagevariants.Backup(s)
	variantSetsToReconcile := packagevariantsets.Backup(s)

	kind.UseDataCluster(s)
	postgres.Wipe(s)

	kind.UseDataCluster(s)
	postgres.Restore(s)

	t.TimingHelper(reconcileDescription, func(t *suiteutils.TestSuite) {
		t.T().Helper()
		kind.UseDBCacheCluster(s)
		repositories.Reconcile(s, repositoriesToReconcile, 20)
		packagevariants.Reconcile(s, variantsToReconcile, 20)
		packagevariantsets.Reconcile(s, variantSetsToReconcile, 20)
	})

	expectedCountsAfter := &suiteutils.PackageRevisionStatusCounts{
		Total:            1104,
		Draft:            10,
		Proposed:         10,
		Published:        1074,
		DeletionProposed: 10,
	}
	t.PackageRevisionCountsMustMatch(expectedCountsAfter)
}

func (t *PorchDisasterSuite) TestPorchPodsUngracefulRestart() {
	s := &t.MultiClusterTestSuite

	t.TimingHelper("recover after Porch pods restarted", func(t *suiteutils.TestSuite) {
		t.T().Helper()
		kind.UseDBCacheCluster(s)
		pods.RestartAllPorchPods(s)

		// wait a bit before trying to reconcile Porch objects
		// give porch-server some time to re-sync everything and get the API available again
		time.Sleep(1 * time.Minute)

		var repos configapi.RepositoryList
		t.ListF(&repos, client.InNamespace(t.Namespace))
		t.WaitUntilMultipleRepositoriesReady(repos.Items)

		wait.PollUntilContextTimeout(t.GetContext(), time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
			_, err := t.Clientset.PorchV1alpha1().PackageRevisions(t.Namespace).
				List(t.GetContext(), metav1.ListOptions{FieldSelector: "spec.repository=" + repos.Items[0].Name})
			if err != nil {
				if apierrors.IsTimeout(err) {
					return false, nil
				} else {
					return false, err
				}
			}
			return true, nil
		})
	})

	expectedCountsAfter := &suiteutils.PackageRevisionStatusCounts{
		Total:            1104,
		Draft:            10,
		Proposed:         10,
		Published:        1074,
		DeletionProposed: 10,
	}
	t.PackageRevisionCountsMustMatch(expectedCountsAfter)
}
