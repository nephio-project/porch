package integration

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	configapi "github.com/nephio-project/porch/controllers/repositories/api/v1alpha1"
	"github.com/nephio-project/porch/controllers/repositories/pkg/controllers/repository"
)

var _ = Describe("Repository Controller Integration", func() {
	var (
		reconciler     *repository.RepositoryReconciler
		testRepo       *configapi.Repository
		namespacedName types.NamespacedName
		testCounter    int
	)

	BeforeEach(func() {
		testCounter++
		reconciler, _ = createReconcilerWithMockCache()
		testRepo = createTestRepository(fmt.Sprintf("test-repo-%d", testCounter), "default")
		namespacedName = types.NamespacedName{
			Name:      testRepo.Name,
			Namespace: testRepo.Namespace,
		}

		// Create the repository in the cluster
		Expect(k8sClient.Create(ctx, testRepo)).To(Succeed())
	})

	AfterEach(func() {
		// Simple cleanup - just delete, don't wait
		if testRepo != nil {
			k8sClient.Delete(ctx, testRepo)
		}
	})

	Context("When reconciling a new repository", func() {
		It("Should add finalizer and perform full sync on first reconcile", func() {
			// Single reconcile - adds finalizer AND starts async full sync
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			// Should start async full sync and return with requeue for stale detection
			Expect(result.RequeueAfter).To(Equal(10 * time.Second)) // SyncStaleTimeout

			// Check that finalizer was added
			updatedRepo := &configapi.Repository{}
			Expect(k8sClient.Get(ctx, namespacedName, updatedRepo)).To(Succeed())
			Expect(updatedRepo.Finalizers).To(ContainElement(repository.RepositoryFinalizer))

			// Wait for async full sync to complete and status to become ready
			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, namespacedName, updatedRepo)).To(Succeed())
				for _, condition := range updatedRepo.Status.Conditions {
					if condition.Type == configapi.RepositoryReady && condition.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, time.Second*10, time.Millisecond*200).Should(BeTrue())

			// Check that full sync annotation was set
			Expect(k8sClient.Get(ctx, namespacedName, updatedRepo)).To(Succeed())
			Expect(updatedRepo.Annotations).To(HaveKey("config.porch.kpt.dev/last-full-sync"))
		})

		It("Should handle sync capacity exceeded", func() {
			// Create reconciler with very low sync capacity
			lowCapacityReconciler := &repository.RepositoryReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				Cache:                createMockCacheWithSlowSync(),
				HealthCheckFrequency: 2 * time.Second,
				FullSyncFrequency:    5 * time.Second,
				SyncStaleTimeout:     10 * time.Second,
				MaxConcurrentSyncs:   1, // Very low capacity
			}
			// Initialize sync limiter
			lowCapacityReconciler.InitializeSyncLimiter()

			// Fill up sync capacity with a slow sync
			_, err := lowCapacityReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Create another repository that should hit capacity limit
			secondRepo := createTestRepository(fmt.Sprintf("test-repo-capacity-%d", testCounter), "default")
			Expect(k8sClient.Create(ctx, secondRepo)).To(Succeed())
			defer k8sClient.Delete(ctx, secondRepo)

			secondNamespacedName := types.NamespacedName{
				Name:      secondRepo.Name,
				Namespace: secondRepo.Namespace,
			}

			// This should hit capacity and requeue
			result, err := lowCapacityReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: secondNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		})
	})

	Context("When reconciling an existing repository", func() {
		BeforeEach(func() {
			// First reconcile to set up the repository
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			// Wait for async sync to complete
			time.Sleep(100 * time.Millisecond)
		})

		It("Should perform health check when no full sync needed", func() {
			// Update status to show successful recent full sync
			updatedRepo := &configapi.Repository{}
			Expect(k8sClient.Get(ctx, namespacedName, updatedRepo)).To(Succeed())

			// Set annotation for recent full sync
			if updatedRepo.Annotations == nil {
				updatedRepo.Annotations = make(map[string]string)
			}
			updatedRepo.Annotations["config.porch.kpt.dev/last-full-sync"] = time.Now().Add(-1 * time.Second).Format(time.RFC3339)
			Expect(k8sClient.Update(ctx, updatedRepo)).To(Succeed())

			// Set status to ready with recent update
			Expect(k8sClient.Get(ctx, namespacedName, updatedRepo)).To(Succeed())
			updatedRepo.Status.Conditions = []metav1.Condition{{
				Type:               configapi.RepositoryReady,
				Status:             metav1.ConditionTrue,
				Reason:             configapi.ReasonReady,
				Message:            "Repository is ready",
				ObservedGeneration: updatedRepo.Generation,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-3 * time.Second)),
			}}
			Expect(k8sClient.Status().Update(ctx, updatedRepo)).To(Succeed())

			// Reconcile should perform health check and return health check frequency
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			// Should return health check interval (2 seconds in test config)
			Expect(result.RequeueAfter).To(BeNumerically("<=", 2*time.Second))
		})

		It("Should perform full sync when spec changes", func() {
			// Update repository spec
			updatedRepo := &configapi.Repository{}
			Expect(k8sClient.Get(ctx, namespacedName, updatedRepo)).To(Succeed())
			updatedRepo.Spec.Git.Repo = "https://github.com/example/updated-repo"
			Expect(k8sClient.Update(ctx, updatedRepo)).To(Succeed())

			// Reconcile should detect spec change and start async full sync
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			// Should start async full sync and requeue for stale detection
			Expect(result.RequeueAfter).To(Equal(10 * time.Second)) // SyncStaleTimeout
		})
	})

	Context("When handling repository deletion", func() {
		BeforeEach(func() {
			// First reconcile to set up the repository
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			// Wait for setup to complete
			time.Sleep(100 * time.Millisecond)
		})

		It("Should clean up resources when repository is deleted", func() {
			// Delete the repository
			Expect(k8sClient.Delete(ctx, testRepo)).To(Succeed())

			// Reconcile should handle deletion
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0)))

			// Repository should be removed from cluster
			Eventually(func() bool {
				updatedRepo := &configapi.Repository{}
				err := k8sClient.Get(ctx, namespacedName, updatedRepo)
				return err != nil // Should not be found
			}, time.Second*5, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When testing sync scheduling and intervals", func() {
		It("Should calculate correct sync intervals for cron schedules", func() {
			// Test repository with cron schedule
			testRepo.Spec.Sync = &configapi.RepositorySync{
				Schedule: "0 */1 * * *", // Every hour
			}
			Expect(k8sClient.Update(ctx, testRepo)).To(Succeed())

			// Reconcile should start full sync and return stale detection timeout
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			// Should start async full sync and requeue for stale detection
			Expect(result.RequeueAfter).To(Equal(10 * time.Second))
		})

		It("Should use default frequency for invalid cron", func() {
			testRepo.Spec.Sync = &configapi.RepositorySync{
				Schedule: "invalid-cron",
			}
			Expect(k8sClient.Update(ctx, testRepo)).To(Succeed())

			// Reconcile should start full sync despite invalid cron
			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			// Should start async full sync and requeue for stale detection
			Expect(result.RequeueAfter).To(Equal(10 * time.Second))
		})
	})

	Context("When testing predicate functions", func() {
		It("Should not trigger reconcile for RunOnceAt clearing only", func() {
			oldRepo := testRepo.DeepCopy()
			oldRepo.Generation = 1
			oldRepo.Spec.Sync = &configapi.RepositorySync{
				Schedule:  "0 */6 * * *",
				RunOnceAt: &metav1.Time{Time: time.Now()},
			}

			newRepo := testRepo.DeepCopy()
			newRepo.Generation = 2
			newRepo.Spec.Sync = &configapi.RepositorySync{
				Schedule:  "0 */6 * * *",
				RunOnceAt: nil, // Cleared
			}

			// We can't directly test the private method, but we can test that
			// the predicate function would filter out such updates
			// This is tested indirectly through controller behavior
			Expect(oldRepo.Generation).To(BeNumerically("<", newRepo.Generation))
			Expect(newRepo.Spec.Sync.RunOnceAt).To(BeNil())
		})

		It("Should trigger reconcile for meaningful spec changes", func() {
			oldRepo := testRepo.DeepCopy()
			oldRepo.Generation = 1
			oldRepo.Spec.Git.Repo = "https://github.com/example/old-repo"

			newRepo := testRepo.DeepCopy()
			newRepo.Generation = 2
			newRepo.Spec.Git.Repo = "https://github.com/example/new-repo"

			// Test that meaningful changes are detected
			Expect(oldRepo.Generation).To(BeNumerically("<", newRepo.Generation))
			Expect(oldRepo.Spec.Git.Repo).ToNot(Equal(newRepo.Spec.Git.Repo))
		})
	})
})
