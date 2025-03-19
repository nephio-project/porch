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
	"sync"
	"time"

	"github.com/nephio-project/porch/api/porch/v1alpha1"
	configapi "github.com/nephio-project/porch/api/porchconfig/v1alpha1"
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
	id string
	// We need the kubernetes object so we can add the appropritate
	// ownerreferences to PackageRevision resources.
	repoSpec *configapi.Repository
	repo     repository.Repository
	cancel   context.CancelFunc

	lastVersion string

	mutex                  sync.RWMutex
	cachedPackageRevisions map[repository.PackageRevisionKey]*cachedPackageRevision
	cachedPackages         map[repository.PackageKey]*cachedPackage
	// Error encountered on repository refresh by the refresh goroutine.
	// This is returned back by the cache to the background goroutine when it calls periodicall to resync repositories.
	refreshRevisionsError error

	options cachetypes.CacheOptions
}

func newRepository(id string, repoSpec *configapi.Repository, repo repository.Repository, options cachetypes.CacheOptions) *cachedRepository {
	ctx, cancel := context.WithCancel(context.Background())
	r := &cachedRepository{
		id:       id,
		repoSpec: repoSpec,
		repo:     repo,
		cancel:   cancel,
		options:  options,
	}

	// TODO: Should we fetch the packages here?

	go r.pollForever(ctx, options.RepoSyncFrequency)

	return r
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
	packages := r.cachedPackages
	packageRevisions := r.cachedPackageRevisions
	err := r.refreshRevisionsError

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

func (r *cachedRepository) CreatePackageRevision(ctx context.Context, obj *v1alpha1.PackageRevision) (repository.PackageRevisionDraft, error) {
	return r.repo.CreatePackageRevision(ctx, obj)
}

func (r *cachedRepository) ClosePackageRevisionDraft(ctx context.Context, prd repository.PackageRevisionDraft, version string) (repository.PackageRevision, error) {
	ctx, span := tracer.Start(ctx, "cachedRepository::ClosePackageRevisionDraft", trace.WithAttributes())
	defer span.End()

	v, err := r.Version(ctx)
	if err != nil {
		return nil, err
	}

	if v != r.lastVersion {
		_, _, err = r.refreshAllCachedPackages(ctx)
		if err != nil {
			return nil, err
		}
	}

	revisions, err := r.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{
		Package: prd.GetName(),
	})
	if err != nil {
		return nil, err
	}

	var publishedRevisions []string
	for _, rev := range revisions {
		if v1alpha1.LifecycleIsPublished(rev.Lifecycle(ctx)) {
			publishedRevisions = append(publishedRevisions, rev.Key().Revision)
		}
	}

	nextVersion, err := repository.NextRevisionNumber(ctx, publishedRevisions)
	if err != nil {
		return nil, err
	}

	if closed, err := r.repo.ClosePackageRevisionDraft(ctx, prd, nextVersion); err != nil {
		return nil, err
	} else {
		return r.update(ctx, closed)
	}
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
	defer r.mutex.Unlock()

	k := updated.Key()
	// previous := r.cachedPackageRevisions[k]

	if v1alpha1.LifecycleIsPublished(updated.Lifecycle(ctx)) {
		oldKey := repository.PackageRevisionKey{
			Repository:    k.Repository,
			Package:       k.Package,
			WorkspaceName: k.WorkspaceName,
		}
		delete(r.cachedPackageRevisions, oldKey)
	}

	cached := &cachedPackageRevision{PackageRevision: updated}
	r.cachedPackageRevisions[k] = cached

	// Recompute latest package revisions.
	identifyLatestRevisions(ctx, r.cachedPackageRevisions)

	// Create the main package revision
	if v1alpha1.LifecycleIsPublished(updated.Lifecycle(ctx)) {
		updatedMain := updated.ToMainPackageRevision()
		err := r.createMainPackageRevision(ctx, updatedMain)
		if err != nil {
			return nil, err
		}
	} else {
		version, err := r.repo.Version(ctx)
		if err != nil {
			return nil, err
		}
		r.lastVersion = version
	}

	return cached, nil
}

func (r *cachedRepository) createMainPackageRevision(ctx context.Context, updatedMain repository.PackageRevision) error {
	//Search and delete any old main pkgRev of an older workspace in the cache
	for pkgRevKey := range r.cachedPackageRevisions {
		if (pkgRevKey.Repository == updatedMain.Key().Repository) && (pkgRevKey.Package == updatedMain.Key().Package) && (pkgRevKey.Revision == updatedMain.Key().Revision) {
			oldMainKey := repository.PackageRevisionKey{
				Repository:    updatedMain.Key().Repository,
				Package:       updatedMain.Key().Package,
				Revision:      updatedMain.Key().Revision,
				WorkspaceName: pkgRevKey.WorkspaceName,
			}
			delete(r.cachedPackageRevisions, oldMainKey)
		}
	}
	cachedMain := &cachedPackageRevision{PackageRevision: updatedMain}
	r.cachedPackageRevisions[updatedMain.Key()] = cachedMain

	pkgRevMetaNN := types.NamespacedName{
		Name:      updatedMain.KubeObjectName(),
		Namespace: updatedMain.KubeObjectNamespace(),
	}

	// Create the package if it doesn't exist
	_, err := r.options.MetadataStore.Get(ctx, pkgRevMetaNN)
	if apierrors.IsNotFound(err) {
		pkgRevMeta := metav1.ObjectMeta{
			Name:      updatedMain.KubeObjectName(),
			Namespace: updatedMain.KubeObjectNamespace(),
		}
		_, err := r.options.MetadataStore.Create(ctx, pkgRevMeta, r.repoSpec.Name, updatedMain.UID())
		if err != nil {
			klog.Warningf("unable to create PackageRev CR for %s/%s: %v",
				updatedMain.KubeObjectNamespace(), updatedMain.KubeObjectName(), err)
		}
	}
	version, err := r.repo.Version(ctx)
	if err != nil {
		return err
	}
	r.lastVersion = version

	return nil
}

func (r *cachedRepository) DeletePackageRevision(ctx context.Context, old repository.PackageRevision) error {
	// Unwrap
	unwrapped := old.(*cachedPackageRevision).PackageRevision
	if err := r.repo.DeletePackageRevision(ctx, unwrapped); err != nil {
		return err
	}

	r.mutex.Lock()
	if r.cachedPackages != nil {
		k := old.Key()
		// previous := r.cachedPackages[k]
		delete(r.cachedPackageRevisions, k)

		// Recompute latest package revisions.
		// TODO: Only for affected object / key?
		identifyLatestRevisions(ctx, r.cachedPackageRevisions)
	}

	r.mutex.Unlock()

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

func (r *cachedRepository) Close() error {
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
		klog.Infof("repo %s: deleting packagerev %s/%s because repository is closed", r.id, nn.Namespace, nn.Name)
		_, err := r.options.MetadataStore.Delete(context.TODO(), nn, true)
		if err != nil {
			// There isn't much use in returning an error here, so we just log it
			// and create a PackageRevisionMeta with just name and namespace. This
			// makes sure that the Delete event is sent.
			klog.Warningf("repo %s: error deleting packagerev for %s: %v", r.id, nn.Name, err)
		}
		klog.Infof("repo %s: successfully deleted packagerev %s/%s", r.id, nn.Namespace, nn.Name)
		sent += r.options.RepoPRChangeNotifier.NotifyPackageRevisionChange(watch.Deleted, pr)
	}
	klog.Infof("repo %s: sent %d notifications for %d package revisions during close", r.id, sent, len(r.cachedPackageRevisions))
	return r.repo.Close()
}

// pollForever will continue polling until signal channel is closed or ctx is done.
func (r *cachedRepository) pollForever(ctx context.Context, repoSyncFrequency time.Duration) {
	r.pollOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			klog.V(2).Infof("repo %s: exiting repository poller, because context is done: %v", r.id, ctx.Err())
			return
		default:
			r.pollOnce(ctx)
			time.Sleep(repoSyncFrequency)
		}
	}
}

func (r *cachedRepository) pollOnce(ctx context.Context) {
	start := time.Now()
	klog.Infof("repo %s: poll started", r.id)
	defer func() { klog.Infof("repo %s: poll finished in %f secs", r.id, time.Since(start).Seconds()) }()
	ctx, span := tracer.Start(ctx, "[START]::Repository::pollOnce", trace.WithAttributes())
	defer span.End()

	if _, err := r.getPackageRevisions(ctx, repository.ListPackageRevisionFilter{}, true); err != nil {
		klog.Warningf("error polling repo packages %s: %v", r.id, err)
	}
	// TODO: Uncomment when package resources are fully supported
	//if _, err := r.getPackages(ctx, repository.ListPackageRevisionFilter{}, true); err != nil {
	//	klog.Warningf("error polling repo packages %s: %v", r.id, err)
	//}
}

func (r *cachedRepository) flush() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.cachedPackageRevisions = nil
	r.cachedPackages = nil
}

// refreshAllCachedPackages updates the cached map for this repository with all the newPackages,
// it also triggers notifications for all package changes.
// mutex must be held.
func (r *cachedRepository) refreshAllCachedPackages(ctx context.Context) (map[repository.PackageKey]*cachedPackage, map[repository.PackageRevisionKey]*cachedPackageRevision, error) {
	ctx, span := tracer.Start(ctx, "cachedRepository::refreshAllCachedPackages", trace.WithAttributes())
	defer span.End()

	// TODO: Avoid simultaneous fetches?
	// TODO: Push-down partial refresh?
	r.mutex.Lock()
	defer r.mutex.Unlock()

	start := time.Now()
	defer func() { klog.Infof("repo %s: refresh finished in %f secs", r.id, time.Since(start).Seconds()) }()

	curVer, err := r.Version(ctx)
	if err != nil {
		return nil, nil, err
	}

	if curVer == r.lastVersion {
		return r.cachedPackages, r.cachedPackageRevisions, nil
	}

	// Look up all existing PackageRevCRs so we an compare those to the
	// actual Packagerevisions found in git/oci, and add/prune PackageRevCRs
	// as necessary.
	existingPkgRevCRs, err := r.options.MetadataStore.List(ctx, r.repoSpec)
	if err != nil {
		return nil, nil, err
	}
	// Create a map so we can quickly check if a specific PackageRevisionMeta exists.
	existingPkgRevCRsMap := make(map[string]metav1.ObjectMeta)
	for i := range existingPkgRevCRs {
		pr := existingPkgRevCRs[i]
		existingPkgRevCRsMap[pr.Name] = pr
	}

	// TODO: Can we avoid holding the lock for the ListPackageRevisions / identifyLatestRevisions section?
	newPackageRevisions, err := r.repo.ListPackageRevisions(ctx, repository.ListPackageRevisionFilter{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "error listing packages")
	}

	// Build mapping from kubeObjectName to PackageRevisions for new PackageRevisions.
	newPackageRevisionNames := make(map[string]*cachedPackageRevision, len(newPackageRevisions))
	for _, newPackage := range newPackageRevisions {
		kname := newPackage.KubeObjectName()
		if newPackageRevisionNames[kname] != nil {
			klog.Warningf("repo %s: found duplicate packages with name %v", r.repo, kname)
		}

		pkgRev := &cachedPackageRevision{
			PackageRevision:  newPackage,
			isLatestRevision: false,
		}
		newPackageRevisionNames[newPackage.KubeObjectName()] = pkgRev
	}

	// Build mapping from kubeObjectName to PackageRevisions for existing PackageRevisions
	oldPackageRevisionNames := make(map[string]*cachedPackageRevision, len(r.cachedPackageRevisions))
	for _, oldPackage := range r.cachedPackageRevisions {
		oldPackageRevisionNames[oldPackage.KubeObjectName()] = oldPackage
	}

	// We go through all PackageRev CRs that represents PackageRevisions
	// in the current repo and make sure they all have a corresponding
	// PackageRevision. The ones that doesn't is removed.
	for _, prm := range existingPkgRevCRs {
		if _, found := newPackageRevisionNames[prm.Name]; !found {
			klog.Infof("repo %s: deleting PackageRev %s/%s because parent PackageRevision was not found",
				r.id, prm.Namespace, prm.Name)
			if _, err := r.options.MetadataStore.Delete(ctx, types.NamespacedName{
				Name:      prm.Name,
				Namespace: prm.Namespace,
			}, true); err != nil {
				if !apierrors.IsNotFound(err) {
					// This will be retried the next time the sync runs.
					klog.Warningf("repo %s: unable to delete PackageRev CR for %s/%s: %v",
						r.id, prm.Name, prm.Namespace, err)
				}
			}
		}
	}

	// Send notification for packages that changed before the creation of PkgRev to avoid race conditions because of ownerReferences.
	addSent := 0
	modSent := 0
	for kname, newPackage := range newPackageRevisionNames {
		oldPackage := oldPackageRevisionNames[kname]
		if oldPackage == nil {
			addSent += r.options.RepoPRChangeNotifier.NotifyPackageRevisionChange(watch.Added, newPackage)
		} else {
			if oldPackage.ResourceVersion() != newPackage.ResourceVersion() {
				modSent += r.options.RepoPRChangeNotifier.NotifyPackageRevisionChange(watch.Modified, newPackage)
			}
		}
	}

	// We go through all the PackageRevisions and make sure they have
	// a corresponding PackageRev CR.
	for pkgRevName, pkgRev := range newPackageRevisionNames {
		if _, found := existingPkgRevCRsMap[pkgRevName]; !found {
			pkgRevMeta := metav1.ObjectMeta{
				Name:      pkgRevName,
				Namespace: r.repoSpec.Namespace,
			}
			if _, err := r.options.MetadataStore.Create(ctx, pkgRevMeta, r.repoSpec.Name, pkgRev.UID()); err != nil {
				// TODO: We should try to find a way to make these errors available through
				// either the repository CR or the PackageRevision CR. This will be
				// retried on the next sync.
				klog.Warningf("unable to create PackageRev CR for %s/%s: %v",
					r.repoSpec.Namespace, pkgRevName, err)
			}
		}
	}

	delSent := 0
	// Send notifications for packages that was deleted in the SoT
	for kname, oldPackage := range oldPackageRevisionNames {
		if newPackageRevisionNames[kname] == nil {
			nn := types.NamespacedName{
				Name:      oldPackage.KubeObjectName(),
				Namespace: oldPackage.KubeObjectNamespace(),
			}
			klog.Infof("repo %s: deleting PackageRev %s/%s because PackageRevision was removed from SoT",
				r.id, nn.Namespace, nn.Name)
			delSent += r.options.RepoPRChangeNotifier.NotifyPackageRevisionChange(watch.Deleted, oldPackage)
		}
	}
	klog.Infof("repo %s: addSent %d, modSent %d, delSent for %d old and %d new repo packages", r.id, addSent, modSent, len(oldPackageRevisionNames), len(newPackageRevisionNames))

	newPackageRevisionMap := make(map[repository.PackageRevisionKey]*cachedPackageRevision, len(newPackageRevisions))
	for _, newPackage := range newPackageRevisions {
		k := newPackage.Key()
		pkgRev := &cachedPackageRevision{
			PackageRevision:  newPackage,
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
		// TODO: Build package?
		// newPackage := &cachedPackage{
		// }
		// newPackageMap[newPackage.Key()] = newPackage
	}

	r.cachedPackageRevisions = newPackageRevisionMap
	r.cachedPackages = newPackageMap
	r.lastVersion = curVer

	return newPackageMap, newPackageRevisionMap, nil
}
