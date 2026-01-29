---
title: "Custom Resource Cache"
type: docs
weight: 3
description: |
  CR-based cache implementation details.
---

## Overview

The **Custom Resource (CR) Cache** is the default cache implementation in Porch. It stores package metadata using Kubernetes Custom Resources while keeping repository data in memory. This implementation is designed for standard Porch deployments where Kubernetes etcd provides sufficient storage and performance.

**Key characteristics:**

- **Default implementation**: Used when no cache type is explicitly configured
- **Hybrid storage**: In-memory repository cache + CR-based metadata storage
- **Kubernetes-native**: Leverages Kubernetes API and etcd for persistence
- **Suitable for**: Small to medium deployments with moderate package counts
- **No external dependencies**: Only requires a Kubernetes cluster
- **Git interaction**: Interacts with Git at every stage of package revision lifecycle for presistence

## Implementation Details

### Storage Architecture

The CR Cache uses a two-tier storage model:

```
In-Memory Layer
  │
  ├─ Repository connections
  ├─ Package revisions (cached)
  ├─ Package metadata (cached)
  └─ Repository version tracking
               ↓
Persistent Layer (Kubernetes)
  │
  └─ PackageRev CRs (metadata only)
```

**In-Memory Storage:**
- Repository instances and connections
- Cached package revisions from Git
- Package metadata (names, paths, revisions)
- Repository version for change detection
- Latest revision tracking per package

**Persistent Storage (PackageRev CRs):**
- Labels and annotations
- Finalizers
- Owner references
- Deletion timestamps
- Resource versions

### Metadata Store

The CR Cache uses a **CRD-based metadata store** that manages `PackageRev` custom resources:

**PackageRev CR structure:**
- One PackageRev CR per PackageRevision
- Stored in same namespace as Repository CR
- Labeled with repository name for filtering
- Contains only Kubernetes metadata (no package content)
- Includes finalizer for cleanup coordination

**Metadata operations:**
- **Create**: Creates PackageRev CR when package revision is published
- **Get**: Retrieves metadata for a specific package revision
- **List**: Lists all PackageRev CRs for a repository
- **Update**: Updates labels, annotations, finalizers, owner references
- **Delete**: Removes PackageRev CR (with optional finalizer clearing)

**Special labels:**
- `internal.porch.kpt.dev/repository`: Links PackageRev to Repository
- Used for efficient filtering and cleanup

**Finalizer handling:**
- `internal.porch.kpt.dev/packagerevision`: Prevents premature deletion
- Ensures coordination between PackageRevision and PackageRev CR
- Cleared only when all other finalizers are removed

### Repository Caching

Each repository is wrapped in a `cachedRepository` that provides:

**Caching behavior:**
- First access: Fetches all package revisions from Git
- Subsequent access: Returns cached data
- Version tracking: Compares repository version to detect changes
- Lazy refresh: Only re-fetches when version changes
- Force refresh: Can bypass cache when explicitly requested

**Cache invalidation:**
- Automatic on repository version change
- Full flush on package deletion
- Incremental updates on package revision changes

**Concurrency control:**
- Per-repository mutex for cache updates
- Read-write lock for cache access
- Lock-free reads when cache is populated
- Prevents simultaneous refresh operations

### Background Synchronization

The CR Cache creates a **sync manager** for each repository:

**Sync process:**
1. Periodically triggers repository refresh (configurable frequency)
2. Fetches latest package revisions from Git
3. Compares with cached revisions
4. Creates/updates/deletes PackageRev CRs as needed
5. Sends watch notifications for changes
6. Updates Repository CR condition with sync status

**Sync scope:**
- Syncs all package revisions from Git (all lifecycles)
- Includes Draft, Proposed, Published, and DeletionProposed
- Aligns with pass-through approach (all states exist in Git)
- Ensures cache reflects complete Git repository state

**Change detection:**
- Compares package revision names between old and new
- Detects additions, modifications, deletions
- Checks resource versions to identify changes
- Identifies new latest revisions

**Notification flow:**
- Added: New package revisions found in Git
- Modified: Existing package revisions changed
- Deleted: Package revisions removed from Git
- Notifications sent before PackageRev CR creation (avoids race conditions)

### Latest Revision Tracking

The CR Cache computes the latest package revision:

**Identification logic:**
- Only considers Published package revisions
- Compares semantic versions when available
- Highest revision number wins
- Excludes draft and branch-tracking revisions
- Recomputed on every cache refresh

**Latest revision label:**
- `kpt.dev/latest-revision: "true"` added to latest revision
- Used for filtering and queries
- Automatically updated when new revisions published
- Removed from old latest when new latest identified
- External modifications to this label are rejected by API strategy validation

**Async notification:**
- When latest revision deleted, async goroutine identifies new latest
- Sends Modified notification for new latest revision
- Ensures clients see latest revision updates

## Storage Mechanism

### Draft Package Handling

The CR Cache has a **pass-through approach** to draft packages:

**Draft lifecycle:**
- CreatePackageRevisionDraft passes directly to Git repository adapter
- Draft packages are immediately created as Git branches
- All draft modifications go directly to Git
- UpdatePackageRevision operations modify Git branches in real-time
- ClosePackageRevisionDraft commits and tags/branches in Git

**Git interaction pattern:**
- **Draft creation**: Creates Git branch immediately
- **Draft updates**: Modifies Git branch on each update
- **Draft closure**: Creates Git tag or finalizes branch
- **Every operation** touches the external Git repository

**Implications:**
- Draft work is immediately visible in Git repository
- Git repository reflects all draft states
- Network latency affects draft operations
- Git repository must be available for all draft operations

### PackageRev Custom Resource

The CR Cache stores metadata in `PackageRev` CRs:

**Resource structure:**
```yaml
apiVersion: internal.porch.kpt.dev/v1alpha1
kind: PackageRev
metadata:
  name: <repo>-<package>-<workspace>
  namespace: <repository-namespace>
  labels:
    internal.porch.kpt.dev/repository: <repo-name>
  finalizers:
    - internal.porch.kpt.dev/packagerevision
  ownerReferences:
    - apiVersion: porch.kpt.dev/v1alpha1
      kind: PackageRevision
      name: <same-as-metadata-name>
```

**Why PackageRev CRs?**
- Separates metadata from package content
- Enables Kubernetes-native metadata management
- Supports owner references and finalizers
- Allows label/annotation queries
- Provides resource version for optimistic locking

### Memory Management

The CR Cache manages memory usage:

**Cache structure:**
- Map of repository keys to cached repositories
- Per-repository map of package revision keys to cached revisions
- Per-repository map of package keys to cached packages
- Shared metadata store across all repositories

**Memory characteristics:**
- Grows with number of repositories and package revisions
- Full repository content cached in memory
- No automatic eviction (cache persists until repository closed)
- Flush on package deletion to free memory

**Scalability considerations:**
- Suitable for hundreds of repositories
- Thousands of package revisions per repository
- Memory usage proportional to package count
- For larger deployments, consider using the DB Cache

### Repository Lifecycle

**Opening a repository:**
1. Check if repository already cached
2. If cached, return existing instance
3. If not cached, create repository adapter
4. Wrap adapter in cachedRepository
5. Start background sync manager
6. Store in cache map

**Closing a repository:**
1. Stop background sync manager
2. Delete all PackageRev CRs for repository
3. Send Delete notifications for all package revisions
4. Close underlying repository adapter
5. Remove from cache map

**Repository sharing:**
- Multiple Repository CRs can point to same Git repository
- Cache checks if repository already open before closing
- Only closes when last Repository CR is deleted
- Prevents premature connection closure

### Cache Invalidation

The CR Cache uses **full cache flush** for package deletion:

**Package deletion:**
- Deletes package from Git repository
- Flushes entire in-memory cache for the repository
- Forces re-fetch from Git on next access
- Ensures cache consistency after deletion

**Implications:**
- Simple invalidation strategy
- Temporary performance impact on next access
- Guarantees no stale data remains
- Trade-off: rebuilds cache even for unaffected packages
