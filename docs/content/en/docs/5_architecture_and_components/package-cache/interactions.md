---
title: "Cache Interactions"
type: docs
weight: 6
description: |
  How the cache integrates with repositories and adapters.
---

## Overview

The Package Cache acts as an intermediary layer that coordinates between the Core Engine, Repository Adapters, and external Git repositories. It provides caching, synchronization, and change notification services while maintaining a clean separation of concerns.

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│                  Package Cache Layer                    │
│                                                         │
│  ┌──────────────┐      ┌──────────────┐      ┌──────┐   │
│  │ Core Engine  │ ───> │    Cache     │ ───> │ Repo │   │
│  │   Requests   │      │  Operations  │      │Adapt │   │
│  │              │      │              │      │ ers  │   │
│  └──────────────┘      └──────────────┘      └──────┘   │
│         ↑                      │                │       │
│         │                      ↓                ↓       │
│         │              ┌──────────────┐      ┌──────┐   │
│         └──────────────│   Watcher    │      │ Git  │   │
│                        │  Notifier    │      │Repos │   │
│                        └──────────────┘      └──────┘   │
└─────────────────────────────────────────────────────────┘
```

## Repository Adapter Integration

The cache wraps repository adapters to provide caching and synchronization services:

### Adapter Wrapping Pattern

```
Cache OpenRepository
        ↓
  Check if Cached
        ↓
   Not Cached? ──Yes──> Create External Repo
        │                      ↓
        │              externalrepo.CreateRepositoryImpl
        │                      ↓
        │              Repository Adapter
        │                      ↓
        │              Wrap in cachedRepository
        │                      ↓
        │              Start SyncManager
        │                      ↓
        └────────────> Store in Cache Map
                               ↓
                        Return Repository
```

**Process:**
1. **Cache receives** OpenRepository request with Repository CR spec
2. **Check cache map** for existing repository instance
3. **If not cached**, create external repository adapter:
   - Call externalrepo.CreateRepositoryImpl with repository spec
   - Factory pattern selects Git or OCI adapter based on type
   - Adapter initialized with credentials and configuration
4. **Wrap adapter** in cachedRepository (CR Cache) or dbRepository (DB Cache)
5. **Start SyncManager** for background synchronization
6. **Store in cache map** keyed by namespace/name
7. **Return wrapped repository** to Core Engine

**Adapter delegation:**

The cached repository delegates operations to the underlying adapter:

```
Cached Repository Method
        ↓
  Check Cache State
        ↓
  Cache Valid? ──Yes──> Return Cached Data
        │
        No
        ↓
  Call Adapter Method
        ↓
  Adapter Executes
        ↓
  Update Cache
        ↓
  Return Result
```

**Delegated operations:**
- **CreatePackageRevisionDraft**: Pass-through to adapter (no caching)
- **ClosePackageRevisionDraft**: Adapter closes, cache updates
- **UpdatePackageRevision**: Pass-through to adapter
- **DeletePackageRevision**: Adapter deletes, cache invalidates
- **ListPackageRevisions**: Adapter lists, cache stores
- **Version**: Adapter provides Git SHA, cache tracks
- **Refresh**: Adapter re-fetches, cache rebuilds

### Credential and Configuration Flow

```
Core Engine
     ↓
Cache Options
     ↓
ExternalRepoOptions
     ↓
Repository Adapter
     ↓
Git/OCI Operations
```

**Configuration passed through cache:**
- **Credential resolver**: Resolves authentication for Git/OCI
- **Reference resolver**: Resolves upstream package references
- **User info provider**: Provides authenticated user for audit
- **Metadata store**: CR Cache metadata storage (CR Cache only)
- **Database handler**: PostgreSQL connection (DB Cache only)

**Cache doesn't handle:**
- Authentication to external repositories
- Git operations (clone, fetch, push)
- OCI registry operations
- Package content parsing

### Repository Lifecycle Management

**Opening repositories:**

```
OpenRepository(spec)
        ↓
  Generate Key
        ↓
  Check Cache Map
        ↓
  Found? ──Yes──> Return Cached
        │
        No
        ↓
  Create Adapter
        ↓
  Wrap & Store
        ↓
  Return New
```

**Closing repositories:**

```
CloseRepository(key)
        ↓
  Stop SyncManager
        ↓
  Delete Metadata
        ↓
  Send Delete Events
        ↓
  Close Adapter
        ↓
  Remove from Map
```

**Repository sharing:**
- Multiple Repository CRs can reference same Git repository
- Cache checks if repository already open before closing
- Only closes adapter when last Repository CR deleted
- Prevents premature connection closure

## Core Engine Access

The Core Engine accesses the cache through a clean interface:

### Cache Interface Operations

```
Core Engine
     ↓
Cache.OpenRepository(spec)
     ↓
Repository Interface
     ↓
  ┌──────┴──────┬──────────┬─────────┐
  ↓             ↓          ↓         ↓
List        Create      Update    Delete
Packages    Draft       Draft     Package
```

**Core Engine workflow:**

1. **Open repository** through cache
2. **Receive repository interface** (cached wrapper)
3. **Call repository methods** (ListPackageRevisions, CreatePackageRevisionDraft, etc.)
4. **Cache handles** caching, synchronization, notifications
5. **Core Engine unaware** of caching implementation details

### Request Flow Examples

**List package revisions:**

```
Core Engine
     ↓
ListPackageRevisions(filter)
     ↓
Cached Repository
     ↓
Check Cache Version
     ↓
Version Match? ──Yes──> Return Cached
     │
     No
     ↓
Fetch from Adapter
     ↓
Update Cache
     ↓
Return Results
```

**Create package revision:**

```
Core Engine
     ↓
CreatePackageRevisionDraft
     ↓
Cached Repository
     ↓
Pass to Adapter
     ↓
Adapter Creates Draft
     ↓
Return Draft
     ↓
Core Engine Modifies
     ↓
ClosePackageRevisionDraft
     ↓
Adapter Commits
     ↓
Cache Updates
     ↓
Notify Watchers
```

**Update package revision:**

```
Core Engine
     ↓
UpdatePackageRevision
     ↓
Cached Repository
     ↓
Unwrap Cached PR
     ↓
Pass to Adapter
     ↓
Adapter Opens Draft
     ↓
Return Draft
     ↓
Core Engine Modifies
     ↓
ClosePackageRevisionDraft
     ↓
Cache Updates
     ↓
Notify Watchers
```

### Cache Transparency

The cache is transparent to the Core Engine:

**Core Engine perspective:**
- Calls repository interface methods
- Receives repository objects
- Unaware of caching layer
- Doesn't know about cache implementation (CR vs DB)
- Doesn't manage synchronization

**Cache responsibilities:**
- Intercepts repository operations
- Manages caching strategy
- Handles background synchronization
- Sends change notifications
- Maintains consistency with Git

## Background Synchronization

The cache synchronizes with external repositories in the background:

### SyncManager Integration

```
Cached Repository
        ↓
  Create SyncManager
        ↓
  Start Goroutines
        ↓
  ┌─────┴─────┐
  ↓           ↓
Periodic   One-time
 Sync       Sync
  ↓           ↓
  └─────┬─────┘
        ↓
  SyncOnce()
        ↓
  Fetch from Git
        ↓
  Compare with Cache
        ↓
  Update Cache
        ↓
  Notify Changes
        ↓
  Update Condition
```

**SyncManager lifecycle:**
1. **Created** when repository opened
2. **Started** with sync frequency configuration
3. **Runs** two goroutines (periodic and one-time)
4. **Calls** SyncOnce on cached repository
5. **Stopped** when repository closed

**Sync frequency sources:**
- **Default frequency**: Configured at Porch server startup
- **Cron schedule**: Repository CR can specify custom cron
- **One-time sync**: Repository CR can schedule single sync

### Change Detection and Notification

```
Background Sync
        ↓
  Fetch Git State
        ↓
  Compare with Cache
        ↓
  Identify Changes
        ↓
  ┌─────┴─────┬─────────┐
  ↓           ↓         ↓
Added      Modified  Deleted
  ↓           ↓         ↓
  └─────┬─────┴─────────┘
        ↓
  Update Cache
        ↓
  Notify Watchers
        ↓
  Update Condition
```

**Change notification flow:**
1. **Sync detects** changes in Git repository
2. **Cache updates** internal state
3. **Notifier called** with event type and package revision
4. **Watcher manager** delivers to matching watchers
5. **API server** sends watch events to clients

**Event types:**
- **Added**: New package revision found in Git
- **Modified**: Existing package revision changed
- **Deleted**: Package revision removed from Git

### Condition Management

```
Sync Operation
      ↓
Set "sync-in-progress"
      ↓
Update Repository CR
      ↓
Perform Sync
      ↓
Success? ──Yes──> Set "ready"
      │
      No
      ↓
Set "error" + Message
      ↓
Update Repository CR
```

**Condition states:**
- **sync-in-progress**: Sync actively running
- **ready**: Sync completed successfully
- **error**: Sync failed with error details

**Condition updates:**
- Set at start of sync operation
- Updated at end of sync operation
- Include error messages on failure
- Include next sync time on success
- Visible in Repository CR status

### Error Handling

**Sync errors:**

```
Sync Failure
    ↓
Log Error
    ↓
Update Condition
    ↓
Keep Stale Cache
    ↓
Retry Next Cycle
```

**Error behavior:**
- Sync errors logged with context
- Repository condition updated with error
- Cache remains available with stale data
- Operations continue with warning
- Automatic retry on next sync cycle

**Adapter errors:**
- Git connection failures
- Authentication errors
- Network timeouts
- Invalid repository configuration

**Recovery:**
- Transient errors may resolve on retry
- Persistent errors require configuration fix
- Error details available in Repository CR status
- Manual refresh can force retry

## Watcher Notification Integration

The cache notifies watchers of package revision changes:

### Notification Flow

```
Cache Operation
        ↓
Package Changed
        ↓
NotifyPackageRevisionChange
        ↓
Watcher Manager
        ↓
Filter Matching
        ↓
Deliver to Watchers
        ↓
API Server Watch
        ↓
Client Receives Event
```

**Notification triggers:**
- **ClosePackageRevisionDraft**: Added event for new package revision
- **UpdatePackageRevision**: Modified event for updated package revision
- **DeletePackageRevision**: Deleted event for removed package revision
- **Background sync**: Added/Modified/Deleted events for Git changes
- **Repository close**: Deleted events for all package revisions

**Notification timing:**
- Sent **after** cache update completes
- Sent **before** metadata store update (avoids race conditions)
- Sent **regardless** of metadata store errors
- Sent **synchronously** from cache operations

### Watch Event Delivery

**Event structure:**
- **Event type**: Added, Modified, Deleted
- **Package revision**: Full object with metadata
- **Timestamp**: When event occurred

**Delivery guarantees:**
- **At-least-once**: Events may be delivered multiple times
- **Ordered**: Events for same package revision ordered
- **Filtered**: Only matching watchers receive events
- **Best-effort**: Network failures may drop events

**Client watch lifecycle:**
1. **Client subscribes** via API server
2. **Watcher registered** with filter
3. **Cache sends** notifications
4. **Watcher delivers** to client
5. **Client receives** watch events
6. **Client disconnects** or cancels
7. **Watcher cleaned up** automatically
