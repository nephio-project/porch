// Copyright 2022, 2024-2025 The kpt and Nephio Authors
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

package crcache

import (
	"context"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
	"github.com/nephio-project/porch/pkg/cache/crcache/meta"
	cachetypes "github.com/nephio-project/porch/pkg/cache/types"
	"github.com/nephio-project/porch/pkg/repository"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog/v2"
)

// We take advantage of the cache having a global view of all the packages
// in a repository and compute the latest package revision in the cache
// rather than add another level of caching in the repositories themselves.
// This also reuses the revision comparison code and ensures same behavior
// between Git and OCI.

var _ repository.Repository = &cachedRepository{}

type cachedRepository struct {
	key      repository.RepositoryKey
	repoSpec *configapi.Repository
	repo     repository.Repository
	cancel   context.CancelFunc

	lastVersion string

	mutex                  sync.RWMutex
	cachedPackageRevisions map[repository.PackageRevisionKey]*cachedPackageRevision
	cachedPackages         map[repository.PackageKey]*cachedPackage
	// Error encountered on repository refresh by the refresh goroutine.
	// This is returned back by the cache to the background goroutine when it calls the periodic refresh to resync repositories.
	refreshRevisionsError error

	metadataStore        meta.MetadataStore
	repoPRChangeNotifier cachetypes.RepoPRChangeNotifier
}

func newRepository(repoKey repository.RepositoryKey, repoSpec *configapi.Repository, repo repository.Repository,
	metadataStore meta.MetadataStore, options cachetypes.CacheOptions) *cachedRepository {
	ctx, cancel := context.WithCancel(context.Background())
	r := &cachedRepository{
		key:                  repoKey,
		repoSpec:             repoSpec,
		repo:                 repo,
		metadataStore:        metadataStore,
		repoPRChangeNotifier: options.RepoPRChangeNotifier,
		cancel:               cancel,
	}

	// TODO: Should we fetch the packages here?

	go r.pollForever(ctx, options.RepoSyncFrequency)

	return r
}

func (r *cachedRepository) KubeObjectNamespace() string {
	return r.Key().Namespace
}

func (r *cachedRepository) KubeObjectName() string {
	return r.Key().Name
}

func (r *cachedRepository) Key() repository.RepositoryKey {
	return r.key
}

func (r *cachedRepository) Refresh(ctx context.Context) error {

	_, _, err := r.refreshAllCachedPackages(ctx)

	return err
}

func (r *cachedRepository) Version(ctx context.Context) (string, error) {
	ctx, span := tracer.Start(ctx, "cachedRepository::Version", trace.WithAttributes())
	defer span.End()

	return r.repo.Version(ctx)
}

func (r *cachedRepository) ListPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter) ([]repository.PackageRevision, error) {
	packages, err := r.getPackageRevisions(ctx, filter, false)
	if err != nil {
		return nil, err
	}

	for _, pr := range packages {
		pkgRevMeta, err := r.metadataStore.Get(ctx, types.NamespacedName{
			Name:      pr.KubeObjectName(),
			Namespace: pr.KubeObjectNamespace(),
		})
		if err != nil {
			// If a PackageRev CR doesn't exist, we treat the
			// Packagerevision as not existing.
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		if err := pr.SetMeta(ctx, pkgRevMeta); err != nil {
			return nil, err
		}
	}

	return packages, nil
}

func (r *cachedRepository) getRefreshError() error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// TODO: This should also check r.refreshPkgsError when
	//   the package resource is fully supported.

	return r.refreshRevisionsError
}

func (r *cachedRepository) getPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter, forceRefresh bool) ([]repository.PackageRevision, error) {

	_, packageRevisions, err := r.getCachedPackages(ctx, forceRefresh)
	if err != nil {
		return nil, err
	}
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return toPackageRevisionSlice(ctx, packageRevisions, filter), nil
}

func (r *cachedRepository) getPackages(ctx context.Context, filter repository.ListPackageFilter, forceRefresh bool) ([]repository.Package, error) {

	packages, _, err := r.getCachedPackages(ctx, forceRefresh)
	if err != nil {
		return nil, err
	}
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return toPackageSlice(packages, filter), nil
}

// getCachedPackages returns cachedPackages; fetching it if not cached or if forceRefresh.
func (r *cachedRepository) getCachedPackages(ctx context.Context, forceRefresh bool) (map[repository.PackageKey]*cachedPackage, map[repository.PackageRevisionKey]*cachedPackageRevision, error) {
	r.mutex.RLock()
	packages := r.cachedPackages
	packageRevisions := r.cachedPackageRevisions
	r.mutex.RUnlock()
	var err error

	if forceRefresh {
		packages = nil
		packageRevisions = nil

		r.mutex.Lock()
		err := r.repo.Refresh(ctx)
		r.mutex.Unlock()

		if err != nil {
			return nil, nil, err
		}
	}

	if packages == nil {
		packages, packageRevisions, err = r.refreshAllCachedPackages(ctx)
	}

	return packages, packageRevisions, err
}

func (r *cachedRepository) CreatePackageRevisionDraft(ctx context.Context, obj *v1alpha1.PackageRevision) (repository.PackageRevisionDraft, error) {
	return r.repo.CreatePackageRevisionDraft(ctx, obj)
}

func (r *cachedRepository) ClosePackageRevisionDraft(ctx context.Context, prd repository.PackageRevisionDraft, version int) (repository.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "cachedRepository::ClosePackageRevisionDraft", trace.WithAttributes())
	defer span.End()

	v, err := r.Version(ctx)
	if err != nil {
		return nil, err
	}
	r.mutex.Lock()
	ver := r.lastVersion
	r.mutex.Unlock()
	if v != ver {
		_, _, err = r.refreshAllCachedPackages(ctx)
		if err != nil {
			return nil, err
		}
	}

	revisions, err := r.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Key: repository.PackageRevisionKey{
			PkgKey: repository.PackageKey{
				Path:    prd.Key().PkgKey.Path,
				Package: prd.Key().PkgKey.Package,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	highestRevision := 0
	for _, rev := range revisions {
		if v1alpha1.LifecycleIsPublished(rev.Lifecycle(ctx)) && rev.Key().Revision > highestRevision {
			highestRevision = rev.Key().Revision
		}
	}

	closedPr, err := r.repo.ClosePackageRevisionDraft(ctx, prd, highestRevision+1)
	if err != nil {
		return nil, err
	}

	cachedPr, err := r.update(ctx, closedPr)
	if err != nil {
		return nil, err
	}
	pkgRevMeta := metav1.ObjectMeta{
		Name:      cachedPr.KubeObjectName(),
		Namespace: cachedPr.KubeObjectNamespace(),
		Labels: func() map[string]string {
			if kptfile, err := cachedPr.GetKptfile(ctx); err == nil {
				if kptfile.Labels != nil {
					labels := kptfile.Labels
					maps.Copy(labels, prd.GetMeta().Labels)
					return labels
				}
			} else {
				klog.Warningf("error getting Kptfile labels from cached package: %q", err)
			}

			return prd.GetMeta().Labels
		}(),
		Annotations:     prd.GetMeta().Annotations,
		Finalizers:      prd.GetMeta().Finalizers,
		OwnerReferences: prd.GetMeta().OwnerReferences,
	}

	pkgRevMeta, err = r.metadataStore.Create(ctx, pkgRevMeta, r.Key().Name, cachedPr.UID())
	if err != nil {
		return nil, err
	}

	if err := cachedPr.SetMeta(ctx, pkgRevMeta); err != nil {
		return nil, err
	}

	sent := r.repoPRChangeNotifier.NotifyPackageRevisionChange(watch.Added, cachedPr)
	klog.Infof("cache: sent %d for new PackageRevision %s/%s", sent, cachedPr.KubeObjectNamespace(), cachedPr.KubeObjectName())
	return cachedPr, nil
}

func (r *cachedRepository) UpdatePackageRevision(ctx context.Context, old repository.PackageRevision) (repository.PackageRevisionDraft, error) {
	// Unwrap
	unwrapped := old.(*cachedPackageRevision).PackageRevision

	return r.repo.UpdatePackageRevision(ctx, unwrapped)
}

func (r *cachedRepository) update(ctx context.Context, updated repository.PackageRevision) (*cachedPackageRevision, error) {
	// TODO: Technically we only need this package, not all packages
	if _, _, err := r.getCachedPackages(ctx, false); err != nil {
		klog.Warningf("failed to get cached packages: %v", err)
		// TODO: Invalidate all watches? We're dropping an add/update event
		return nil, err
	}

	r.mutex.Lock()
	if v1alpha1.LifecycleIsPublished(updated.Lifecycle(ctx)) {
		prevKey := updated.Key()
		prevKey.Revision = 0 // Drafts always have revision of 0
		delete(r.cachedPackageRevisions, prevKey)
	}

	cached := &cachedPackageRevision{
		PackageRevision: updated,
		metadataStore:   r.metadataStore,
	}
	r.cachedPackageRevisions[updated.Key()] = cached

	// Recompute latest package revisions.
	identifyLatestRevisions(ctx, r.cachedPackageRevisions)
	r.mutex.Unlock()

	// Create the main package revision
	if v1alpha1.LifecycleIsPublished(updated.Lifecycle(ctx)) {
		updatedMain := updated.ToMainPackageRevision(ctx)
		err := r.createMainPackageRevision(ctx, updatedMain)
		if err != nil {
			return nil, err
		}
	} else {
		version, err := r.repo.Version(ctx)
		if err != nil {
			return nil, err
		}
		r.mutex.Lock()
		r.lastVersion = version
		r.mutex.Unlock()
	}

	return cached, nil
}

func (r *cachedRepository) createMainPackageRevision(ctx context.Context, updatedMain repository.PackageRevision) error {
	// Search and delete any old main pkgRev of an older workspace in the cache
	var oldMainPR *cachedPackageRevision
	r.mutex.Lock()
	revisionsMap := r.cachedPackageRevisions
	r.mutex.Unlock()
	for pkgRevKey := range revisionsMap {
		r.mutex.Lock()
		rev := pkgRevKey.Revision
		key := pkgRevKey.PkgKey
		r.mutex.Unlock()
		if rev == -1 {
			if key == updatedMain.Key().PkgKey {
				r.mutex.Lock()
				oldMainPR = r.cachedPackageRevisions[pkgRevKey]
				r.mutex.Unlock()
				break
			}
		}
	}
	if oldMainPR != nil {
		delete(r.cachedPackageRevisions, oldMainPR.Key())
	}

	r.mutex.Lock()
	cachedMain := &cachedPackageRevision{
		PackageRevision: updatedMain,
		metadataStore:   r.metadataStore,
	}
	r.cachedPackageRevisions[updatedMain.Key()] = cachedMain
	r.mutex.Unlock()
	pkgRevMetaNN := types.NamespacedName{
		Name:      updatedMain.KubeObjectName(),
		Namespace: updatedMain.KubeObjectNamespace(),
	}

	// Create the package if it doesn't exist
	_, err := r.metadataStore.Get(ctx, pkgRevMetaNN)
	if apierrors.IsNotFound(err) {
		pkgRevMeta := metav1.ObjectMeta{
			Name:      updatedMain.KubeObjectName(),
			Namespace: updatedMain.KubeObjectNamespace(),
		}
		_, err := r.metadataStore.Create(ctx, pkgRevMeta, r.Key().Name, updatedMain.UID())
		if err != nil {
			klog.Warningf("unable to create PackageRev CR for %s/%s: %v",
				updatedMain.KubeObjectNamespace(), updatedMain.KubeObjectName(), err)
		}
	}
	version, err := r.repo.Version(ctx)
	if err != nil {
		return err
	}
	r.mutex.Lock()
	r.lastVersion = version
	r.mutex.Unlock()
	return nil
}

func (r *cachedRepository) DeletePackageRevision(ctx context.Context, prToDelete repository.PackageRevision) error {
	// We delete the PackageRev regardless of any finalizers, since it
	// will always have the same finalizers as the PackageRevision. This
	// will put the PackageRev, and therefore the PackageRevision in the
	// terminating state.
	// But we only delete the PackageRevision from the repo once all finalizers
	// have been removed.
	namespacedName := types.NamespacedName{
		Name:      prToDelete.KubeObjectName(),
		Namespace: prToDelete.KubeObjectNamespace(),
	}
	pkgRevMeta, err := r.metadataStore.Delete(ctx, namespacedName, false)
	if err != nil && !apierrors.IsNotFound(err) {
		klog.Warningf("Error deleting PkgRevMeta %s: %v", namespacedName.String(), err)
	}

	if len(pkgRevMeta.Finalizers) > 0 {
		klog.Infof("PackageRevision %s deleted, but still have finalizers: %s", prToDelete.KubeObjectName(), strings.Join(pkgRevMeta.Finalizers, ","))
		sent := r.repoPRChangeNotifier.NotifyPackageRevisionChange(watch.Modified, prToDelete)
		klog.Infof("crcache: sent %d modified for deleted PackageRevision %s/%s with finalizers", sent, prToDelete.KubeObjectNamespace(), prToDelete.KubeObjectName())
		return nil
	}
	klog.Infof("PackageRevision %s deleted for real since no finalizers", prToDelete.KubeObjectName())

	// Unwrap
	unwrapped := prToDelete.(*cachedPackageRevision).PackageRevision
	if err := r.repo.DeletePackageRevision(ctx, unwrapped); err != nil {
		return err
	}

	r.mutex.Lock()
	if r.cachedPackages != nil {
		k := prToDelete.Key()
		delete(r.cachedPackageRevisions, k)

		// Recompute latest package revisions.
		// TODO: Only for affected object / key?
		identifyLatestRevisions(ctx, r.cachedPackageRevisions)
	}

	r.mutex.Unlock()

	if _, err := r.metadataStore.Delete(ctx, namespacedName, true); err != nil {
		// If this fails, the CR will be cleaned up by the background job.
		if !apierrors.IsNotFound(err) {
			klog.Warningf("Error deleting PkgRevMeta %s: %v", namespacedName.String(), err)
		}
	}

	sent := r.repoPRChangeNotifier.NotifyPackageRevisionChange(watch.Deleted, prToDelete)
	klog.Infof("crcache: sent %d for deleted PackageRevision %s/%s", sent, prToDelete.KubeObjectNamespace(), prToDelete.KubeObjectName())

	return nil
}

func (r *cachedRepository) ListPackages(ctx context.Context, filter repository.ListPackageFilter) ([]repository.Package, error) {
	packages, err := r.getPackages(ctx, filter, false)
	if err != nil {
		return nil, err
	}

	return packages, nil
}

func (r *cachedRepository) CreatePackage(ctx context.Context, obj *v1alpha1.PorchPackage) (repository.Package, error) {
	klog.Infoln("cachedRepository::CreatePackage")
	return r.repo.CreatePackage(ctx, obj)
}

func (r *cachedRepository) DeletePackage(ctx context.Context, old repository.Package) error {
	// Unwrap
	unwrapped := old.(*cachedPackage).Package
	if err := r.repo.DeletePackage(ctx, unwrapped); err != nil {
		return err
	}

	// TODO: Do something more efficient than a full cache flush
	r.flush()

	return nil
}

func (r *cachedRepository) Close(ctx context.Context) error {
	r.cancel()

	// Make sure that watch events are sent for packagerevisions that are
	// removed as part of closing the repository.
	sent := 0
	for _, pr := range r.cachedPackageRevisions {
		nn := types.NamespacedName{
			Name:      pr.KubeObjectName(),
			Namespace: pr.KubeObjectNamespace(),
		}
		// There isn't really any correct way to handle finalizers here. We are removing
		// the repository, so we have to just delete the PackageRevision regardless of any
		// finalizers.
		klog.Infof("repo %+v: deleting packagerev %s/%s because repository is closed", r.Key(), nn.Namespace, nn.Name)
		_, err := r.metadataStore.Delete(context.TODO(), nn, true)
		if err != nil {
			// There isn't much use in returning an error here, so we just log it
			// and create a PackageRevisionMeta with just name and namespace. This
			// makes sure that the Delete event is sent.
			klog.Warningf("repo %+v: error deleting packagerev for %s: %v", r.Key(), nn.Name, err)
		}
		klog.Infof("repo %+v: successfully deleted packagerev %s/%s", r.Key(), nn.Namespace, nn.Name)
		sent += r.repoPRChangeNotifier.NotifyPackageRevisionChange(watch.Deleted, pr)
	}
	klog.Infof("repo %+v: sent %d notifications for %d package revisions during close", r.Key(), sent, len(r.cachedPackageRevisions))
	return r.repo.Close(ctx)
}

// pollForever will continue polling until the signal channel is closed or the ctx is done.
func (r *cachedRepository) pollForever(ctx context.Context, repoSyncFrequency time.Duration) {
	for {
		select {
		case <-ctx.Done():
			klog.V(2).Infof("repo %+v: exiting repository poller, because context is done: %v", r.Key(), ctx.Err())
			return
		default:
			r.pollOnce(ctx)
			time.Sleep(repoSyncFrequency)
		}
	}
}

func (r *cachedRepository) pollOnce(ctx context.Context) {
	start := time.Now()
	klog.Infof("repo %+v: poll started", r.Key())
	defer func() { klog.Infof("repo %+v: poll finished in %f secs", r.Key(), time.Since(start).Seconds()) }()
	ctx, span := tracer.Start(ctx, "[START]::Repository::pollOnce", trace.WithAttributes())
	defer span.End()

	if _, err := r.getPackageRevisions(ctx, repository.ListPackageRevisionFilter{}, true); err != nil {
		r.refreshRevisionsError = err
		klog.Warningf("error polling repo packages %s: %v", r.Key(), err)
	} else {
		r.refreshRevisionsError = nil
	}
	// TODO: Uncomment when package resources are fully supported
	//if _, err := r.getPackages(ctx, repository.ListPackageRevisionFilter{}, true); err != nil {
	//	klog.Warningf("error polling repo packages %s: %v", r.Key(), err)
	//}
}

func (r *cachedRepository) flush() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.cachedPackageRevisions = nil
	r.cachedPackages = nil
}

// refreshAllCachedPackages updates the cached map for this repository with all new packages,
// it also triggers notifications for all package changes.
// mutex must be held.
func (r *cachedRepository) refreshAllCachedPackages(ctx context.Context) (map[repository.PackageKey]*cachedPackage, map[repository.PackageRevisionKey]*cachedPackageRevision, error) {
	ctx, span := tracer.Start(ctx, "cachedRepository::refreshAllCachedPackages", trace.WithAttributes())
	defer span.End()

	// TODO: Avoid simultaneous fetches?
	// TODO: Push-down partial refresh?

	start := time.Now()
	defer func() { klog.Infof("repo %+v: refresh finished in %f secs", r.Key(), time.Since(start).Seconds()) }()

	curVer, err := r.Version(ctx)
	if err != nil {
		return nil, nil, err
	}
	r.mutex.RLock()
	lastVer := r.lastVersion
	r.mutex.RUnlock()
	if curVer == lastVer {
		return r.cachedPackages, r.cachedPackageRevisions, nil
	}

	// Look up all existing PackageRevCRs so we can compare those to the
	// actual Packagerevisions found in git/oci, and add/prune PackageRevCRs
	// as necessary.
	existingPkgRevCRs, err := r.metadataStore.List(ctx, r.repoSpec)
	if err != nil {
		return nil, nil, err
	}
	// Create a map so we can quickly check if a specific PackageRevisionMeta exists.
	existingPkgRevCRsMap := make(map[string]metav1.ObjectMeta)
	for _, pr := range existingPkgRevCRs {
		existingPkgRevCRsMap[pr.Name] = pr
	}

	// TODO: Can we avoid holding the lock for the ListPackageRevisions / identifyLatestRevisions section?
	newPackageRevisions, err := r.repo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "error listing packages")
	}

	// Build mapping from kubeObjectName to PackageRevisions for new PackageRevisions.
	r.mutex.Lock()
	newPackageRevisionNames := make(map[string]*cachedPackageRevision, len(newPackageRevisions))
	for _, newPackageRevision := range newPackageRevisions {
		kname := newPackageRevision.KubeObjectName()
		if newPackageRevisionNames[kname] != nil {
			klog.Warningf("repo %+v: found duplicate packages with name %v", r.Key(), kname)
		}

		pkgRev := &cachedPackageRevision{
			PackageRevision:  newPackageRevision,
			metadataStore:    r.metadataStore,
			isLatestRevision: false,
		}
		newPackageRevisionNames[kname] = pkgRev
	}
	r.mutex.Unlock()

	// Build mapping from kubeObjectName to PackageRevisions for existing PackageRevisions
	r.mutex.Lock()
	oldPackageRevisionNames := make(map[string]*cachedPackageRevision, len(r.cachedPackageRevisions))
	for _, oldPackage := range r.cachedPackageRevisions {
		oldPackageRevisionNames[oldPackage.KubeObjectName()] = oldPackage
	}
	r.mutex.Unlock()
	// We go through all PackageRev CRs that represents PackageRevisions
	// in the current repo and make sure they all have a corresponding
	// PackageRevision. The ones that doesn't is removed.
	for _, prm := range existingPkgRevCRs {
		if _, found := newPackageRevisionNames[prm.Name]; !found {
			klog.Infof("repo %+v: deleting PackageRev %s/%s because parent PackageRevision was not found",
				r.Key(), prm.Namespace, prm.Name)
			if _, err := r.metadataStore.Delete(ctx, types.NamespacedName{
				Name:      prm.Name,
				Namespace: prm.Namespace,
			}, true); err != nil {
				if !apierrors.IsNotFound(err) {
					// This will be retried the next time the sync runs.
					klog.Warningf("repo %+v: unable to delete PackageRev CR for %s/%s: %v",
						r.Key(), prm.Name, prm.Namespace, err)
				}
			}
		}
	}

	// Send notification for packages that changed before the creation of PkgRev to avoid race conditions because of ownerReferences.
	addSent := 0
	modSent := 0
	for kname, newPackage := range newPackageRevisionNames {
		if oldPackage, present := oldPackageRevisionNames[kname]; !present || oldPackage == nil {
			addSent += r.repoPRChangeNotifier.NotifyPackageRevisionChange(watch.Added, newPackage)
		} else {

			if kptfile, err := newPackage.GetKptfile(ctx); err == nil {
				meta := newPackage.GetMeta()
				meta.Labels = kptfile.Labels
				if err := newPackage.SetMeta(ctx, meta); err != nil {
					klog.Errorf("error setting meta after updating with labels from Kptfile: %q", err)
				}
			} else {
				klog.Errorf("error getting Kptfile labels from package: %q", err)
			}

			if oldPackage.ResourceVersion() != newPackage.ResourceVersion() {
				modSent += r.repoPRChangeNotifier.NotifyPackageRevisionChange(watch.Modified, newPackage)
			}
		}
	}

	// We go through all the PackageRevisions and make sure they have
	// a corresponding PackageRev CR.
	for pkgRevName, pkgRev := range newPackageRevisionNames {
		if _, found := existingPkgRevCRsMap[pkgRevName]; !found {

			pkgRevMeta := metav1.ObjectMeta{
				Name:      pkgRevName,
				Namespace: r.Key().Namespace,
				Labels: func() map[string]string {
					if kptfile, err := pkgRev.GetKptfile(ctx); err == nil {
						return kptfile.Labels
					}

					klog.Warningf("error getting Kptfile labels from package: %q", err)
					return nil

				}(),
			}
			if _, err := r.metadataStore.Create(ctx, pkgRevMeta, r.Key().Name, pkgRev.UID()); err != nil {
				// TODO: We should try to find a way to make these errors available through
				// either the repository CR or the PackageRevision CR. This will be
				// retried on the next sync.
				klog.Warningf("unable to create PackageRev CR for %s/%s: %v",
					r.Key().Namespace, pkgRevName, err)
			}
		}
	}

	delSent := 0
	// Send notifications for packages that were deleted in the SoT
	for kname, oldPackage := range oldPackageRevisionNames {
		if newPackageRevisionNames[kname] == nil {
			nn := types.NamespacedName{
				Name:      oldPackage.KubeObjectName(),
				Namespace: oldPackage.KubeObjectNamespace(),
			}
			klog.Infof("repo %+v: deleting PackageRev %s/%s because PackageRevision was removed from SoT",
				r.Key(), nn.Namespace, nn.Name)
			delSent += r.repoPRChangeNotifier.NotifyPackageRevisionChange(watch.Deleted, oldPackage)
		}
	}
	klog.Infof("repo %+v: addSent %d, modSent %d, delSent for %d old and %d new repo packages", r.Key(), addSent, modSent, len(oldPackageRevisionNames), len(newPackageRevisionNames))

	newPackageRevisionMap := make(map[repository.PackageRevisionKey]*cachedPackageRevision, len(newPackageRevisions))
	for _, newPackage := range newPackageRevisions {
		k := newPackage.Key()
		pkgRev := &cachedPackageRevision{
			PackageRevision:  newPackage,
			metadataStore:    r.metadataStore,
			isLatestRevision: false,
		}
		newPackageRevisionMap[k] = pkgRev
	}

	identifyLatestRevisions(ctx, newPackageRevisionMap)

	newPackageMap := make(map[repository.PackageKey]*cachedPackage)

	for _, newPackageRevision := range newPackageRevisionMap {
		if !newPackageRevision.isLatestRevision {
			continue
		}
	}
	r.mutex.Lock()
	r.cachedPackageRevisions = newPackageRevisionMap
	r.cachedPackages = newPackageMap
	r.lastVersion = curVer
	r.mutex.Unlock()

	return newPackageMap, newPackageRevisionMap, nil
}
