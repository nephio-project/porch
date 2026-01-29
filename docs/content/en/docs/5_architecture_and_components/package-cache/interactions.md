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

The cache uses a SyncManager to synchronize with external repositories in the background. Each repository gets its own SyncManager instance that:

- Runs periodic sync based on configured frequency or cron schedule
- Handles one-time sync requests via `spec.sync.runOnceAt`
- Detects changes (added/modified/deleted package revisions)
- Updates cache and sends watch notifications
- Updates Repository CR status conditions

**Manual sync:**
- Use `porchctl repo sync <repository-name> -n <namespace>` for immediate sync
- Or set `spec.sync.runOnceAt` in Repository CR to future timestamp

For detailed synchronization architecture, see [Repository Synchronization](functionality/repository-synchronization).

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
