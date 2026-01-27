---
title: "Cache Invalidation"
type: docs
weight: 3
description: |
  Detailed architecture of cache invalidation strategies and metadata cleanup.
---

## Overview

The Package Cache invalidates stale data to maintain consistency with external Git repositories. The invalidation system uses different strategies for automatic and manual invalidation, with implementation differences between CR Cache and DB Cache.

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│                 Invalidation System                     │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │    Automatic     │      │     Manual       │         │
│  │  Invalidation    │      │  Invalidation    │         │
│  │                  │      │                  │         │
│  │ • Background     │      │ • Explicit       │         │
│  │   Sync           │      │   Refresh        │         │
│  │ • Version        │      │ • Package        │         │
│  │   Detection      │      │   Deletion       │         │
│  └──────────────────┘      └──────────────────┘         │
│           │                         │                   │
│           └────────┬────────────────┘                   │
│                    ↓                                    │
│          ┌──────────────────┐                           │
│          │   Invalidation   │                           │
│          │    Strategies    │                           │
│          │                  │                           │
│          │ • CR: Full Flush │                           │
│          │ • DB: Targeted   │                           │
│          └──────────────────┘                           │ 
└─────────────────────────────────────────────────────────┘
```

## Invalidation Strategies

The cache uses different invalidation strategies depending on the trigger and implementation:

### Automatic Invalidation

**Background Sync Invalidation:**

```
Background Sync
      ↓
Fetch Git State
      ↓
Compare with Cache
      ↓
   Changes? ──No──> No Action
      │
     Yes
      ↓
  ┌───┴────┬────────┬─────────┐
  ↓        ↓        ↓         ↓
Added   Modified Deleted  Unchanged
  ↓        ↓        ↓         ↓
Insert  Update   Remove    Skip
```

**Process:**
1. **Periodic sync** detects changes in Git repository
2. **Compare** cached package revisions with Git state
3. **Identify changes**: Added, Modified, Deleted, Unchanged
4. **Apply changes**:
   - Added: Insert new package revisions into cache
   - Modified: Update existing package revisions in cache
   - Deleted: Remove package revisions from cache
   - Unchanged: No action needed

**Triggers:**
- Periodic sync operations (configurable frequency)
- Cron-scheduled sync operations
- One-time sync operations

**Version-Based Invalidation:**

```
Operation Request
      ↓
Check Cache Version
      ↓
Fetch Git Version
      ↓
Versions Match? ──Yes──> Serve from Cache
      │
      No
      ↓
Invalidate Cache
      ↓
Fetch from Git
      ↓
Update Cache
      ↓
Serve Data
```

**Process:**
1. **Repository version** compared on each operation
2. **Version mismatch** detected (Git commit SHA changed)
3. **Cache invalidated** and refreshed from Git
4. **Operations see** current state

**Triggers:**
- Any operation that checks repository version
- Core Engine requests for package revisions
- Repository refresh operations

### Manual Invalidation

**Explicit Refresh:**

```
Refresh() Call
      ↓
Bypass Version Check
      ↓
Force Fetch from Git
      ↓
Invalidate Cache
      ↓
Update with Latest
      ↓
Return Success
```

**Process:**
1. **Explicit Refresh()** call on repository
2. **Bypass version check** (force refresh)
3. **Fetch all package revisions** from Git
4. **Update cache** with latest state
5. **Return** to caller

**Triggers:**
- Explicit Refresh() call from Core Engine
- Recovery from sync errors
- Manual cache refresh operations

**Package Deletion Invalidation:**

**CR Cache approach:**
```
DeletePackage Request
      ↓
Delete from Git
      ↓
Flush Entire Cache
      ↓
Cache Empty
      ↓
Next Access Rebuilds
```

**DB Cache approach:**
```
DeletePackage Request
      ↓
Delete from Git
      ↓
Delete Package from DB
      ↓
Foreign Keys Cascade
      ↓
Revisions Deleted
      ↓
Resources Deleted
```

**Process differences:**
- **CR Cache**: Full cache flush, simple but impacts all packages
- **DB Cache**: Targeted deletion, only affects deleted package

**Triggers:**
- Package deletion operations from Core Engine
- Repository cleanup operations

## Metadata Cleanup

The cache cleans up metadata resources to prevent accumulation of stale data:

### Orphaned Metadata Removal

**CR Cache cleanup:**

```
Sync Operation
      ↓
List PackageRev CRs
      ↓
List Git Packages
      ↓
Compare Lists
      ↓
Orphaned CRs? ──No──> Continue
      │
     Yes
      ↓
Delete Orphaned CRs
      ↓
Log Cleanup
```

**Process:**
1. **Sync operation** lists all PackageRev CRs for repository
2. **Compare** with package revisions from Git
3. **Identify orphans**: CRs without corresponding Git package
4. **Delete orphaned CRs** from Kubernetes
5. **Log cleanup** for monitoring

**Occurs when:**
- Package revision removed from Git externally
- Manual deletion in Git repository
- Repository reset or force push

**DB Cache cleanup:**

```
Sync Operation
      ↓
Query DB Packages
      ↓
List Git Packages
      ↓
Compare Lists
      ↓
Orphaned Records? ──No──> Continue
      │
     Yes
      ↓
Delete from DB
      ↓
Cascade to Related
```

**Process:**
1. **Sync operation** queries database for cached packages
2. **Compare** with package revisions from Git
3. **Identify orphans**: DB records without corresponding Git package
4. **Delete orphaned records** from database
5. **Foreign key cascade** deletes related resources

**Occurs when:**
- Package revision removed from Git externally
- Manual deletion in Git repository
- Repository reset or force push

### Missing Metadata Creation

**CR Cache creation:**

```
Sync Operation
      ↓
List Git Packages
      ↓
List PackageRev CRs
      ↓
Compare Lists
      ↓
Missing CRs? ──No──> Continue
      │
     Yes
      ↓
Create Missing CRs
      ↓
Set Owner References
      ↓
Log Creation
```

**Process:**
1. **Sync operation** lists package revisions from Git
2. **Compare** with existing PackageRev CRs
3. **Identify missing**: Git packages without CRs
4. **Create missing CRs** in Kubernetes
5. **Set owner references** for lifecycle management
6. **Log creation** for monitoring

**Occurs when:**
- Package revision added to Git externally
- Manual commit to Git repository
- Repository clone or import

**DB Cache creation:**

```
Sync Operation
      ↓
List Git Packages
      ↓
Query DB Packages
      ↓
Compare Lists
      ↓
Missing Records? ──No──> Continue
      │
     Yes
      ↓
Insert into DB
      ↓
Set Metadata
      ↓
Log Creation
```

**Process:**
1. **Sync operation** lists package revisions from Git
2. **Compare** with database records
3. **Identify missing**: Git packages without DB records
4. **Insert missing records** into database
5. **Set metadata** (labels, annotations, etc.)
6. **Log creation** for monitoring

**Occurs when:**
- Package revision added to Git externally
- Manual commit to Git repository
- Repository clone or import

### Repository Closure Cleanup

**CR Cache closure:**

```
CloseRepository Request
      ↓
Stop SyncManager
      ↓
List All PackageRev CRs
      ↓
Delete Each CR
      ↓
Send Delete Events
      ↓
Close Adapter
      ↓
Remove from Cache
```

**Process:**
1. **Stop SyncManager** (cancel goroutines)
2. **List all PackageRev CRs** for repository
3. **Delete each CR** (ignore finalizers)
4. **Send delete notifications** to watchers
5. **Close repository adapter**
6. **Remove from cache map**

**DB Cache closure:**

```
CloseRepository Request
      ↓
Stop SyncManager
      ↓
Delete Repository Record
      ↓
Cascade to Packages
      ↓
Cascade to Revisions
      ↓
Cascade to Resources
      ↓
Close Adapter
```

**Process:**
1. **Stop SyncManager** (cancel goroutines)
2. **Delete repository record** from database
3. **Foreign key cascade** deletes packages
4. **Foreign key cascade** deletes package revisions
5. **Foreign key cascade** deletes resources
6. **Close repository adapter**

## Implementation Differences

The two cache implementations use fundamentally different invalidation strategies:

### CR Cache Strategy

**Full Cache Flush:**

```
Package Deletion
      ↓
Delete from Git
      ↓
Flush Cache Maps
      ↓
cachedPackages = nil
      ↓
cachedPackageRevisions = nil
      ↓
Next Access Triggers
      ↓
Full Rebuild from Git
```

**Characteristics:**
- **Simple implementation**: Clear all cache maps
- **Temporary impact**: All packages affected until rebuild
- **Guarantees freshness**: No stale data possible
- **Performance cost**: Next access rebuilds entire cache
- **Memory freed**: Cache maps cleared immediately

**When used:**
- Package deletion operations
- Repository-wide invalidation needed
- Recovery from cache corruption

**Trade-offs:**
- ✅ Simple and reliable
- ✅ Guarantees no stale data
- ❌ Impacts all packages temporarily
- ❌ Rebuild cost on next access

### DB Cache Strategy

**Targeted Deletion:**

```
Package Deletion
      ↓
Delete from Git
      ↓
Delete Package Record
      ↓
Foreign Key Cascade
      ↓
Revisions Deleted
      ↓
Resources Deleted
      ↓
Other Packages Unaffected
```

**Characteristics:**
- **Precise invalidation**: Only deleted package affected
- **No rebuild needed**: Other packages remain cached
- **Database-enforced**: Foreign keys ensure referential integrity
- **Efficient**: No cache flush or rebuild
- **Atomic**: Transaction ensures consistency

**When used:**
- Package deletion operations
- Targeted invalidation needed
- Maintaining cache for other packages

**Trade-offs:**
- ✅ Efficient (only affects deleted package)
- ✅ No rebuild required
- ✅ Other packages unaffected
- ❌ More complex implementation
- ❌ Requires database transaction support

### Comparison Table

| Aspect | CR Cache | DB Cache |
|--------|----------|----------|
| **Deletion Strategy** | Full flush | Targeted deletion |
| **Impact Scope** | All packages | Only deleted package |
| **Rebuild Required** | Yes (on next access) | No |
| **Referential Integrity** | Manual cleanup | Database-enforced |
| **Performance Impact** | Temporary for all | None for others |
| **Implementation** | Simple | Complex |
| **Atomicity** | Cache-level | Transaction-level |

### When Each Strategy is Appropriate

**Use CR Cache (Full Flush) when:**
- Simplicity is preferred over efficiency
- Package count is low (rebuild cost acceptable)
- Memory constraints require clearing cache
- Cache corruption suspected

**Use DB Cache (Targeted Deletion) when:**
- Efficiency is critical
- Package count is high (rebuild cost prohibitive)
- Other packages must remain available
- Database infrastructure available

## Invalidation Triggers Summary

### Automatic Triggers

1. **Background Sync**
   - Frequency: Periodic (configurable)
   - Scope: Changed packages only
   - Impact: Incremental updates

2. **Version Mismatch**
   - Frequency: Per operation
   - Scope: Entire repository
   - Impact: Full refresh

### Manual Triggers

1. **Explicit Refresh**
   - Frequency: On demand
   - Scope: Entire repository
   - Impact: Full refresh

2. **Package Deletion**
   - Frequency: On demand
   - Scope: CR Cache (all), DB Cache (one package)
   - Impact: CR Cache (rebuild), DB Cache (targeted)

3. **Repository Closure**
   - Frequency: On demand
   - Scope: Entire repository
   - Impact: Complete cleanup

## Error Handling

### Invalidation Errors

**CR Cache errors:**
```
Invalidation Error
      ↓
Log Warning
      ↓
Keep Stale Cache
      ↓
Retry on Next Sync
```

**DB Cache errors:**
```
Invalidation Error
      ↓
Rollback Transaction
      ↓
Log Warning
      ↓
Keep Stale Cache
      ↓
Retry on Next Sync
```

**Error handling strategy:**
- Invalidation errors logged but don't block operations
- Stale cache remains available during failures
- Automatic retry on next sync cycle
- Repository condition updated with error state

### Cleanup Errors

**Metadata cleanup errors:**
- Logged as warnings
- Don't block sync operations
- Retried on next sync cycle
- Orphaned metadata eventually cleaned up

**Repository closure errors:**
- Logged but don't prevent closure
- Best-effort cleanup attempted
- Delete notifications sent regardless
- Manual cleanup may be required
