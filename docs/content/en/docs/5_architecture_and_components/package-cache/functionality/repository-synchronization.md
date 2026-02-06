---
title: "Repository Synchronization"
type: docs
weight: 1
description: |
  Detailed architecture of repository synchronization with SyncManager, cache handlers, and background processes.
---

## Overview

The Package Cache synchronization system manages the synchronization of package repositories between external Git sources and the internal cache. Both cache implementations (CR Cache and DB Cache) utilize a common SyncManager to handle periodic and one-time synchronization operations.

### High-Level Architecture

![Repository Sync Architecture](/static/images/porch/repository-sync.svg)

{{< rawhtml >}}
<a href="/images/porch/repository-sync-interactive.html" target="_blank">📊 Interactive Architecture Diagram</a>
{{< /rawhtml >}}

## Core Components

### 1. SyncManager

**Purpose**: Central orchestrator for repository synchronization operations.

**Components**:
- **Handler**: Interface for cache-specific sync operations
- **Core Client**: Kubernetes API client for cluster communication
- **Next Sync Time**: Tracks when the next synchronization should occur
- **Last Sync Error**: Records any errors from previous sync attempts

**Goroutines**:

1. **Periodic Sync Goroutine** - Handles recurring synchronization
   - Performs initial sync at startup, then uses timer to track intervals
   - Supports both cron expressions from repository configuration and default frequency fallback
   - Recalculates next sync time when cron expression changes
   - Updates repository status conditions after each sync

2. **One-time Sync Goroutine** - Manages scheduled single synchronizations
   - Monitors repository configuration for one-time sync requests
   - Creates and cancels timers when the scheduled time changes
   - Skips past timestamps and handles timer cleanup
   - Operates independently of periodic sync schedule

### 2. Cache Handlers (Implements SyncHandler)

Both cache implementations follow the same interface pattern:

#### Database Cache Handler
- Persistent storage-backed repository cache
- Synchronizes with external Git repositories
- Thread-safe operations using mutex locks
- Tracks synchronization statistics and metrics

#### Custom Resource Cache Handler
- Memory-based repository cache for faster access
- Synchronizes with external Git repositories
- Thread-safe operations using mutex locks
- Integrates with Kubernetes metadata storage

## Synchronization Flows

### Package Content Synchronization

```
SyncManager → Goroutines   →   Cache Handlers   → Condition Management
     ↓              ↓              ↓                  ↓
  Start()     syncForever()     SyncOnce()      Set/Build/Apply
             handleRunOnceAt()                  RepositoryCondition
```

**Process**:
1. SyncManager starts two goroutines
2. Goroutines call handler.SyncOnce() on cache implementations
3. Cache handlers perform sync operations
4. All components update repository conditions

### Sync Trigger Flow

```
Repository CR  →  SyncManager  →  Cache Handler  →  Git Repository
     ↑                                                      ↓
     |                                              Fetch Package
     |                                               Revisions
     |                                                      ↓
Status Update  ←  Condition Mgmt  ←  Compare & Update  ←─┘
```

**Flow**:
- **Repository CR** configuration triggers sync scheduling
- **SyncManager** orchestrates periodic and one-time syncs
- **Cache Handler** fetches from Git and compares with cache
- **Condition Management** updates Repository CR status

## Sync Process Details

### Common Sync Process (Both Caches)

```
Start Sync
    ↓
Acquire Mutex Lock
    ↓
Set "sync-in-progress"
    ↓
Fetch Cached Packages ←→ Fetch External Packages
    ↓                           ↓
    └─── Compare & Identify Differences ───┘
                    ↓
            Update Cache
         (Add/Remove Packages)
                    ↓
            Release Mutex
                    ↓
          Update Final Condition
                    ↓
                Complete
```

**Process Steps**:
1. **Acquire mutex lock** - Ensures thread-safe access to cache
2. **Set condition to "sync-in-progress"** - Updates repository status for visibility
3. **Fetch cached package revisions** - Retrieves current cache state
4. **Fetch external package revisions** - Queries external Git repository for latest packages
5. **Compare and identify differences** - Determines what packages need to be added/removed
6. **Update cache (add/remove packages)** - Applies changes to internal cache
7. **Release mutex and update final condition** - Completes sync and updates status

### Sync Frequency Configuration

**Default frequency:**
- Configured at Porch server startup (e.g., 60 seconds)
- Applied when no custom schedule specified
- Used as fallback if cron expression invalid

**Cron schedule:**
- Repository CR can specify custom cron expression in `spec.sync.cron`
- Standard cron format supported
- Dynamically recalculated when expression changes
- Invalid expressions fall back to default frequency

**One-time sync:**
- Repository CR can specify specific time for single sync using `spec.sync.runOnceAt`
- Scheduled independently of periodic sync
- Past timestamps skipped automatically
- Timer cancelled if scheduled time changes

**Manual sync:**
- Use `porchctl repo sync <repository-name>` to trigger immediate sync
- Alternatively, update Repository CR `spec.sync.runOnceAt` to a future timestamp
- Useful for testing or forcing sync after configuration changes

## Change Detection

The cache detects changes by comparing cached and external package revisions:

### Version Tracking

```
Cache State          Git Repository
    ↓                      ↓
Version: abc123      Version: def456
    ↓                      ↓
    └──── Compare ─────────┘
            ↓
      Version Changed
            ↓
    Trigger Full Sync
```

**Process:**
- Repository version (Git commit SHA) cached after each sync
- Version compared before fetching to avoid unnecessary work
- If version unchanged, skip expensive Git operations
- If version changed, fetch all package revisions

### Package Revision Comparison

**Comparison process:**
1. Build map of existing cached package revisions by name
2. Build map of new package revisions from Git by name
3. Identify three categories:
   - **Added**: In Git but not in cache (new package revisions)
   - **Modified**: In both but with different resource versions
   - **Deleted**: In cache but not in Git (removed from repository)

**Change notification:**
- Added package revisions trigger `watch.Added` events
- Modified package revisions trigger `watch.Modified` events
- Deleted package revisions trigger `watch.Deleted` events
- Notifications sent before metadata store updates to avoid race conditions

### Sync Scope Differences

**CR Cache:**
- Syncs ALL package revisions from Git
- Includes Draft, Proposed, Published, and DeletionProposed
- Aligns with pass-through approach (all states exist in Git)
- Ensures cache reflects complete Git repository state

**DB Cache:**
- Syncs ONLY Published and DeletionProposed revisions
- Excludes Draft and Proposed (they're database-only)
- Aligns with database-first approach (drafts don't exist in Git)
- Reduces sync overhead by ignoring work-in-progress packages

## Latest Revision Tracking

The cache automatically identifies and tracks the latest package revision for each package:

### Identification Logic

```
All Package Revisions
        ↓
Filter Published Only
        ↓
Compare Revision Numbers
        ↓
Highest Number = Latest
        ↓
Set Latest Flag/Label
```

**Rules:**
- Only Published package revisions considered
- Highest revision number wins
- Draft and branch-tracking revisions excluded
- Recomputed during every sync and cache update

**Latest revision label:**
- `kpt.dev/latest-revision: "true"` added to latest revision
- Used for filtering and queries
- Automatically updated when new revisions published
- Removed from old latest when new latest identified

**Async notification on deletion:**
- When latest revision deleted, async goroutine identifies new latest
- Sends Modified notification for new latest revision
- Ensures clients see latest revision updates without delay

## Condition Management

### Condition States

**sync-in-progress:**
- Repository synchronization actively running
- Indicates sync operation in progress
- Operations should wait for completion

**ready:**
- Repository synchronized and ready for use
- All package revisions up to date
- Safe to perform operations

**error:**
- Synchronization failed with error details
- Error message included in condition
- Automatically retried on next sync cycle
- Check error details before operations

### Condition Functions

**Set Repository Condition:**
- Updates the status of a repository with new condition information
- Called at start and end of sync operations
- Includes timestamp and error details

**Build Repository Condition:**
- Creates condition objects with appropriate status, reason, and message
- Formats error messages for display
- Includes next sync time information

**Apply Repository Condition:**
- Writes condition updates to Repository Custom Resources in Kubernetes
- Handles conflicts with retry logic
- Ensures status reflects current sync state

## Interface Contracts

### SyncHandler Interface

The SyncHandler interface defines the contract for repository synchronization operations:

**SyncOnce:**
- Performs a single synchronization operation with the external repository
- Returns error if sync fails
- Updates cache with latest package revisions

**Key:**
- Returns the unique identifier for the repository being synchronized
- Used for logging and condition updates

**GetSpec:**
- Retrieves the repository configuration specification
- Provides access to sync schedule and configuration

This interface is implemented by two cache types:

- **Database Cache**: Persistent storage implementation for repository synchronization
- **Custom Resource Cache**: In-memory implementation optimized for Kubernetes Custom Resource operations

## Error Handling & Resilience

### SyncManager Errors

**Error capture:**
- Captured in the last sync error field for tracking
- Reflected in repository status conditions for visibility
- Automatically retried on the next scheduled sync cycle

**Error types:**
- Git connection failures
- Authentication errors
- Network timeouts
- Invalid repository configuration

### Sync Operation Errors

**Handling strategy:**
- Errors logged with context and repository key
- Repository condition updated with error state
- Sync continues on schedule despite errors
- Cache remains available with stale data during failures

**Recovery:**
- Next sync attempt may succeed if transient error
- Persistent errors require configuration or connectivity fixes
- Error details available in Repository CR status

### Condition Update Errors

**Behavior:**
- Logged as warnings
- Don't block sync operations
- Include retry logic with conflict resolution
- Status eventually consistent with sync state

## Concurrency & Safety

### Thread Safety

**Database Cache:**
- Uses mutex locks to ensure safe concurrent access during sync operations
- Per-repository mutex prevents simultaneous syncs
- TryLock pattern: fails fast if sync already in progress

**Custom Resource Cache:**
- Uses mutex locks to protect cache data during concurrent access
- Read-write mutex allows concurrent reads
- Write operations acquire exclusive lock

**SyncManager:**
- Each repository has its own SyncManager instance
- Goroutines isolated per repository
- No shared state between repositories

### Context Management

**Cancellable contexts:**
- Used for graceful shutdown
- SyncManager creates cancellable context on Start()
- Context cancelled on Stop()
- Goroutines exit when context cancelled

**Separate contexts:**
- Sync operations use separate contexts
- Allows individual operation cancellation
- Timeout handling for long-running operations

## Monitoring & Observability

### Logging

**Sync operations:**
- Sync start/completion times with duration
- Package revision statistics (cached/external/both)
- Error conditions and warnings
- Schedule changes and next sync times

**Change detection:**
- Added, modified, deleted package counts
- Version comparison results
- Latest revision updates

**Condition updates:**
- Condition state transitions
- Error details and recovery
- Status update conflicts

### Key Metrics (via logging)

**Performance:**
- Sync duration and frequency
- Package counts and changes
- Cache hit/miss patterns

**Reliability:**
- Success/failure rates
- Error types and frequencies
- Retry attempts and outcomes

**Status:**
- Condition transition events
- Repository availability
- Sync schedule adherence
