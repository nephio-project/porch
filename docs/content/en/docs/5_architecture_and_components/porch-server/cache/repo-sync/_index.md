---
title: "Repository Sync"
type: docs
weight: 4
description: "Porch repository synchronization architecture with SyncManager, cache handlers, and background processes for Git/OCI repositories."
---

## Overview

The Porch sync system manages the synchronization of package repositories between external sources (Git/OCI*) and the internal cache. It consists of two main cache implementations that both utilize a common sync manager to handle periodic and one-time synchronization operations. The architecture consists of two main flows: **SyncManager-driven synchronization** for package content and **Background process** for Repository CR lifecycle management.

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
- Synchronizes with external Git/OCI* repositories
- Thread-safe operations using mutex locks
- Tracks synchronization statistics and metrics

#### Custom Resource Cache Handler
- Memory-based repository cache for faster access
- Synchronizes with external Git/OCI* repositories
- Thread-safe operations using mutex locks
- Integrates with Kubernetes metadata storage

### 3. Background Process

**Purpose**: Manages Repository CR lifecycle and cache updates.

**Components**:
- **K8S API** - Source of Repository CRs
- **Repository CRs** - Custom resources defining repositories
- **Watch Events** - Real-time CR change notifications
- **Periodic Ticker** - RepoSyncFrequency-based updates

## Architecture Flows

### Package Content Synchronization

<pre>
SyncManager → Goroutines   →   Cache Handlers   → Condition Management
     ↓              ↓              ↓                  ↓
  Start()     syncForever()     SyncOnce()      Set/Build/Apply
             handleRunOnceAt()                  RepositoryCondition
</pre>

**Process**:
1. SyncManager starts two goroutines
2. Goroutines call handler.SyncOnce() on cache implementations
3. Cache handlers perform sync operations
4. All components update repository conditions

### Repository Lifecycle Management

<pre>
K8S API   →  Repository CRs   →  Watch Events   →  Background.go    →  Cache Spec Update
    ↓             ↓                 ↓                    ↓                  ↓
Kubernetes    CR Changes        Added/Modified/      Event Handler      OpenRepository/
 Cluster                           Deleted          cacheRepository     CloseRepository
</pre>

**Process**:
1. Repository CRs created/modified/deleted in Kubernetes
2. Watch events generated for CR changes
3. Background.go receives and processes events
4. Cache updated via OpenRepository/CloseRepository calls
5. Periodic ticker ensures consistency

### Event-Driven Status Updates

<pre>
Repository CRs  →  Watch Events  →  Background Process
        ↑                                        ↓
        |                                 Cache Updates
        |                                        ↓
Status Updates  ←  Condition Mgmt  ←  Sync Operations
        ↑                                        ↑
        └─────────── Sync Triggers ──────────────┘
</pre>

**Flow**:
- **Repository CRs** generate watch events when created/modified/deleted
- **Background Process** receives events and triggers cache updates
- **Cache Updates** initiate sync operations through SyncManagers
- **Sync Operations** update conditions, which flow back to Repository CR status

## Sync Process Details

### Common Sync Process (Both Caches)

<pre>
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
</pre>

**Process Steps**:
1. **Acquire mutex lock** (if applicable) - Ensures thread-safe access to cache
2. **Set condition to "sync-in-progress"** - Updates repository status for visibility
3. **Fetch cached package revisions** - Retrieves current cache state
4. **Fetch external package revisions** - Queries external repository for latest packages
5. **Compare and identify differences** - Determines what packages need to be added/removed
6. **Update cache (add/remove packages)** - Applies changes to internal cache
7. **Release mutex and update final condition** - Completes sync and updates status

### Background Event Handling
1. **Added/Modified Events**: Initialize or update repository cache when repositories are created or changed
2. **Deleted Events**: Clean up and remove repository cache when repositories are deleted
3. **Bookmark Events**: Update resource version tracking to maintain watch continuity
4. **Status Updates**: Refresh Repository Custom Resource status conditions

## Condition Management

### Condition States
- **sync-in-progress**: Repository synchronization actively running
  - ⚠️ **Important**: Do not perform API operations (create, update, delete packages) on the repository while this condition is active. Wait for the sync to complete and the repository to return to "ready" state to avoid conflicts and data inconsistencies.
- **ready**: Repository synchronized and ready for use
- **error**: Synchronization failed with error details
  - ⚠️ **Important**: Do not perform API operations on the repository while in error state. Check the error message in the condition details, debug and resolve the underlying issue (e.g., network connectivity, authentication, repository access), then wait for the repository to return to "ready" state before running API calls. See the [troubleshooting guide]({{% relref "/docs/9_troubleshooting_and_faq/repository-sync/" %}}) for common sync issues and solutions.

### Condition Functions
- **Set Repository Condition**: Updates the status of a repository with new condition information
- **Build Repository Condition**: Creates condition objects with appropriate status, reason, and message
- **Apply Repository Condition**: Writes condition updates to Repository Custom Resources in Kubernetes

## Interface Contracts

### SyncHandler Interface

The SyncHandler interface defines the contract for repository synchronization operations:

- **SyncOnce**: Performs a single synchronization operation with the external repository
- **Key**: Returns the unique identifier for the repository being synchronized
- **GetSpec**: Retrieves the repository configuration specification

This interface is implemented by two cache types:

- **Database Cache**: Persistent storage implementation for repository synchronization
- **Custom Resource Cache**: In-memory implementation optimized for Kubernetes Custom Resource operations

## Configuration

For repository sync configuration options, see the [Repository Sync Configuration]({{% relref "/docs/6_configuration_and_deployments/configurations/repository-sync.md" %}}) documentation.

### Background Process Configuration
- **RepoSyncFrequency**: Periodic sync interval
- **Watch Reconnection**: Exponential backoff (1s - 30s)

## Error Handling & Resilience

### SyncManager Errors
- Captured in the last sync error field for tracking
- Reflected in repository status conditions for visibility
- Automatically retried on the next scheduled sync cycle

### Background Process Errors
- Watch connection failures → Exponential backoff reconnection
- Repository validation errors → Status condition with error message
- API conflicts on status updates → Retry with backoff

### Condition Update Errors
- Logged as warnings
- Don't block sync operations
- Include retry logic with conflict resolution

## Concurrency & Safety

### Thread Safety
- **Database Cache**: Uses mutex locks to ensure safe concurrent access during sync operations
- **Custom Resource Cache**: Uses mutex locks to protect cache data during concurrent access
- **Background Process**: Serializes watch events to prevent race conditions

### Context Management
- Cancellable contexts for graceful shutdown
- Separate contexts for sync operations
- Timeout handling for long-running operations

## Monitoring & Observability

### Logging
- Sync start/completion times with duration
- Package revision statistics (cached/external/both)
- Error conditions and warnings
- Schedule changes and next sync times
- Background event processing
- Watch connection status

### Key Metrics (via logging)
- Sync duration and frequency
- Package counts and changes
- Success/failure rates
- Condition transition events
- Background event processing rates

---

{{% alert title="Note" color="primary" %}}
OCI repository support is experimental and may not have full feature parity with Git repositories.
{{% /alert %}}