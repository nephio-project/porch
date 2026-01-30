---
title: "Draft-Commit Workflow Orchestration"
type: docs
weight: 3
description: |
  Detailed architecture of the draft-commit workflow pattern and rollback mechanisms.
---

## Overview

The Engine orchestrates a draft-commit workflow for all package revision modifications. This pattern ensures atomicity - either all changes succeed and are persisted, or none are. The workflow uses mutable drafts for changes and immutable package revisions for storage.

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│          Draft-Commit Workflow Orchestration            │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │   Draft Phase    │      │   Commit Phase   │         │
│  │                  │ ───> │                  │         │
│  │  • Open Draft    │      │  • Close Draft   │         │
│  │  • Apply Changes │      │  • Persist       │         │
│  │  • Validate      │      │  • Immutable     │         │
│  └──────────────────┘      └──────────────────┘         │
│           │                         │                   │
│           └────────┬────────────────┘                   │
│                    ↓                                    │
│          ┌──────────────────┐                           │
│          │     Rollback     │                           │
│          │    Mechanism     │                           │
│          │                  │                           │
│          │  • On Error      │                           │
│          │  • Cleanup       │                           │
│          └──────────────────┘                           │
└─────────────────────────────────────────────────────────┘
```

## Draft-Commit Pattern

The Engine uses a two-phase workflow for all package revision modifications:

### Pattern Overview

```
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│   Request   │ ──────> │    Draft    │ ──────> │  Immutable  │
│             │         │   (Mutable) │         │   Package   │
│  Create or  │         │             │         │  Revision   │
│   Update    │         │  Changes    │         │             │
└─────────────┘         └─────────────┘         └─────────────┘
                              │
                              │ Error?
                              ↓
                        ┌─────────────┐
                        │  Rollback   │
                        │   Delete    │
                        │    Draft    │
                        └─────────────┘
```

**Two phases:**
1. **Draft Phase**: Mutable workspace where changes are applied
2. **Commit Phase**: Draft closed to create immutable package revision

**Key characteristics:**
- **Atomicity**: All changes succeed or all fail
- **Isolation**: Draft changes don't affect other revisions
- **Consistency**: Validation before commit
- **Durability**: Committed revisions are immutable

## Create Package Revision Workflow

The Engine orchestrates package revision creation through a draft-commit workflow:

### Creation Flow

```
CreatePackageRevision Request
        ↓
  Build Package Config
        ↓
  Validate Request
        ↓
  Open Repository
        ↓
  Create Draft ──────────────┐
        ↓                    │
  Setup Rollback Handler     │
        ↓                    │
  Apply Task                 │
        ↓                    │
  Error? ──Yes──> Rollback ──┘
        │
        No
        ↓
  Update Lifecycle
        ↓
  Error? ──Yes──> Rollback ──┘
        │
        No
        ↓
  Close Draft
        ↓
  Error? ──Yes──> Return Error (no rollback)
        │
        No
        ↓
  Return PackageRevision
```

**Process steps:**
1. **Build package config** from request and parent (if any)
2. **Validate request** (lifecycle, tasks, workspace name, etc.)
3. **Open repository** through cache
4. **Create draft** in repository (mutable workspace)
5. **Setup rollback handler** for error recovery
6. **Apply task** through task handler (init, clone, edit, upgrade)
7. **Update lifecycle** to requested state
8. **Close draft** to create immutable package revision
9. **Return** created package revision

### Draft Creation

```
Repository.CreatePackageRevisionDraft
        ↓
  Allocate Workspace
        ↓
  Initialize Resources
        ↓
  Return Draft Handle
```

**Draft characteristics:**
- **Mutable**: Can be modified before closing
- **Isolated**: Separate workspace from other revisions
- **Temporary**: Exists only during creation workflow
- **Lightweight**: No persistent storage until closed

**Draft operations:**
- UpdateResources: Modify package resources
- UpdateLifecycle: Change lifecycle state
- GetResources: Read current resources
- GetPackageRevision: Get package revision metadata

### Task Application

```
TaskHandler.ApplyTask
        ↓
  Execute Task Type
        ↓
  ┌────┴────┬────────┬─────────┐
  ↓         ↓        ↓         ↓
Init     Clone    Edit    Upgrade
  ↓         ↓        ↓         ↓
  └────┬────┴────────┴─────────┘
       ↓
  Apply Builtin Functions
       ↓
  Return Modified Draft
```

**Task execution:**
- Delegates to task handler for actual work
- Task handler modifies draft resources
- Builtin functions applied (package context, etc.)
- Draft returned with modifications

**Error handling:**
- Task errors trigger rollback
- Draft cleaned up on failure
- No partial package revisions created

### Draft Closure

```
Repository.ClosePackageRevisionDraft
        ↓
  Validate Draft State
        ↓
  Persist to Storage
        ↓
  Create Immutable PR
        ↓
  Return PackageRevision
```

**Closure process:**
- Draft validated before closure
- Resources persisted to repository (Git commit/tag)
- Immutable package revision created
- Draft workspace cleaned up

**Closure failure:**
- Error returned to caller
- No rollback attempted (would likely fail again)
- Draft may remain in repository
- Manual cleanup may be required

## Update Package Revision Workflow

The Engine orchestrates package revision updates through a draft-commit workflow:

### Update Flow

```
UpdatePackageRevision Request
        ↓
  Validate Resource Version
        ↓
  Check Current Lifecycle
        ↓
  Draft/Proposed? ──No──> Metadata Only Update
        │
       Yes
        ↓
  Open Repository
        ↓
  Update to Draft ────────────┐
        ↓                     │
  Apply Mutations             │
        ↓                     │
  Error? ──Yes──> Return Error (draft cleanup?)
        │
        No
        ↓
  Update Lifecycle
        ↓
  Error? ──Yes──> Return Error
        │
        No
        ↓
  Close Draft
        ↓
  Error? ──Yes──> Return Error
        │
        No
        ↓
  Update Metadata
        ↓
  Notify Watchers
        ↓
  Return Updated PR
```

**Process steps:**
1. **Validate resource version** (optimistic locking)
2. **Check current lifecycle** (determines update path)
3. **Open repository** through cache
4. **Update to draft** (opens existing revision as draft)
5. **Apply mutations** through task handler
6. **Update lifecycle** if changed
7. **Close draft** to persist changes
8. **Update metadata** (labels, annotations, finalizers)
9. **Notify watchers** of change
10. **Return** updated package revision

### Draft Opening from Existing Revision

```
Repository.UpdatePackageRevision
        ↓
  Load Existing Revision
        ↓
  Create Draft Workspace
        ↓
  Copy Resources to Draft
        ↓
  Return Draft Handle
```

**Draft from existing:**
- Loads current package revision content
- Creates mutable draft workspace
- Copies resources to draft for modification
- Preserves original revision (immutable)

**Draft operations:**
- Same as creation draft
- Can modify resources, lifecycle, metadata
- Changes isolated until closed

### Mutation Application

```
TaskHandler.DoPRMutations
        ↓
  Compare Old vs New Spec
        ↓
  Identify Changes
        ↓
  New Tasks? ──Yes──> Apply New Tasks
        │
        No
        ↓
  Return Modified Draft
```

**Mutation types:**
- **Task additions**: New tasks appended to task list
- **Lifecycle changes**: Handled separately (UpdateLifecycle)
- **Metadata changes**: Handled after draft closure

**Mutation process:**
- Compares old and new package revision specs
- Identifies new tasks to apply
- Delegates to task handler for execution
- Returns modified draft

### Metadata-Only Update Path

```
UpdatePackageRevision (Published/DeletionProposed)
        ↓
  Lifecycle Changed? ──Yes──> Update Lifecycle
        │
        No
        ↓
  Update Metadata
        ↓
  Notify Watchers
        ↓
  Return Updated PR
```

**Metadata-only updates:**
- For Published and DeletionProposed package revisions
- No draft-commit workflow needed
- Direct metadata update on immutable revision
- Only labels, annotations, finalizers, owner references

**Why metadata-only:**
- Published packages are immutable (content cannot change)
- Metadata doesn't affect package content
- Allows operational updates without content changes

## Update Package Resources Workflow

The Engine orchestrates resource updates through a draft-commit workflow:

### Resource Update Flow

```
UpdatePackageResources Request
        ↓
  Get PackageRevision
        ↓
  Validate Resource Version
        ↓
  Check Lifecycle (Draft only)
        ↓
  Open Repository
        ↓
  Update to Draft
        ↓
  Apply Resource Mutations
        ↓
  Execute Render
        ↓
  Error? ──Yes──> Return Error + RenderStatus
        │
        No
        ↓
  Close Draft (no lifecycle change)
        ↓
  Return PR + RenderStatus
```

**Process steps:**
1. **Get package revision** from repository object
2. **Validate resource version** (optimistic locking)
3. **Check lifecycle** (must be Draft)
4. **Open repository** through cache
5. **Update to draft** (opens existing revision)
6. **Apply resource mutations** through task handler
7. **Execute render** (run function pipeline)
8. **Close draft** without lifecycle change
9. **Return** updated package revision and render status

### Resource Mutation

```
TaskHandler.DoPRResourceMutations
        ↓
  Update Package Resources
        ↓
  Execute Render Task
        ↓
  Run Function Pipeline
        ↓
  Return RenderStatus
```

**Resource mutation process:**
- Updates package resource content directly
- Executes render task (runs KRM functions)
- Returns render status with function results
- No lifecycle change (resources updated in-place)

**Render execution:**
- Runs configured KRM function pipeline
- Functions can validate, transform, generate resources
- Results returned in RenderStatus
- Errors don't prevent draft closure (render status indicates failure)

## Rollback Mechanism

The Engine implements rollback to ensure atomicity on errors:

### Rollback Strategy

```
Operation Error
        ↓
  Rollback Handler Invoked
        ↓
  Close Draft (version=0)
        ↓
  Convert to PackageRevision
        ↓
  Delete PackageRevision
        ↓
  Success? ──No──> Log Warning
        │
       Yes
        ↓
  Return Original Error
```

**Rollback process:**
1. **Error detected** during operation
2. **Rollback handler invoked** automatically
3. **Close draft** with version=0 (converts to package revision)
4. **Delete package revision** from repository
5. **Log warning** if cleanup fails
6. **Return original error** to caller

### Rollback Handler Setup

**Rollback handler creation:**

When a draft is created, the engine sets up a rollback handler that will be invoked if any subsequent operation fails. The handler:

1. **Closes the draft** with a special version indicator (0) to convert it to a package revision
2. **Deletes the package revision** from the repository to clean up
3. **Logs warnings** if cleanup fails (best-effort cleanup)
4. **Captures references** to the draft and repository for later use

The rollback handler is a closure that captures the necessary context and can be invoked automatically when errors occur during the operation.

**Rollback handler characteristics:**
- **Closure**: Captures draft and repository references
- **Best effort**: Logs warning if cleanup fails
- **Non-blocking**: Doesn't prevent error return
- **Automatic**: Invoked on any operation error

### Rollback Triggers

**When rollback invoked:**
- Task application errors
- Lifecycle update errors
- Validation errors during operation
- Any error after draft creation

**When rollback NOT invoked:**
- Draft closure errors (would likely fail again)
- Errors before draft creation (nothing to clean up)
- Metadata update errors (draft already closed)

### Rollback Limitations

**Rollback may fail:**
- Repository connection lost
- Permission errors
- Draft already closed
- Repository in inconsistent state

**Failure handling:**
- Warning logged with error details
- Original operation error still returned
- Draft may remain in repository
- Manual cleanup may be required
- Repository garbage collection may clean up eventually

### Rollback vs Transaction

**Not a true transaction:**
- No two-phase commit
- No distributed transaction support
- Best-effort cleanup only

**Why not a transaction:**
- Git doesn't support transactions
- Repository operations not transactional
- Rollback is cleanup, not undo

**Atomicity guarantee:**
- Either package revision created/updated or not
- No partial package revisions visible to clients
- Draft isolation prevents intermediate state visibility

## Draft Lifecycle

Drafts have a specific lifecycle within the workflow:

### Draft States

```
┌─────────────┐
│   Created   │ ← CreatePackageRevisionDraft
└──────┬──────┘
       │
       ↓
┌─────────────┐
│  Modified   │ ← ApplyTask, UpdateLifecycle, UpdateResources
└──────┬──────┘
       │
       ↓
┌─────────────┐
│   Closed    │ ← ClosePackageRevisionDraft
└──────┬──────┘
       │
       ↓
┌─────────────┐
│  Immutable  │ ← PackageRevision created
│   Package   │
│  Revision   │
└─────────────┘
```

**Draft lifecycle:**
1. **Created**: Draft allocated with empty or copied resources
2. **Modified**: Changes applied through task handler
3. **Closed**: Draft persisted to repository
4. **Immutable**: Package revision created, draft cleaned up

### Draft Isolation

**Isolation guarantees:**
- Draft changes not visible to other operations
- Draft workspace separate from other revisions
- Multiple drafts can exist simultaneously (different workspaces)
- Draft closure is atomic (all or nothing)

**Concurrency:**
- Multiple drafts for different packages: Fully concurrent
- Multiple drafts for same package: Prevented by workspace name uniqueness
- Draft and read operations: Concurrent (reads don't see draft)

## Workflow Optimization

The Engine optimizes the draft-commit workflow:

### Optimization Strategies

**Lazy draft creation:**
- Draft created only when needed
- Not created for metadata-only updates
- Not created for read operations

**Early validation:**
- Validation before draft creation
- Fails fast without expensive operations
- Reduces rollback frequency

**Efficient draft closure:**
- Single repository operation
- Atomic commit to Git
- Minimal overhead

### Performance Characteristics

**Draft creation cost:**
- Allocates workspace in repository
- Copies resources (for updates)
- Relatively lightweight

**Draft modification cost:**
- In-memory operations
- No repository access during modifications
- Task handler does actual work

**Draft closure cost:**
- Git commit/tag creation
- Repository write operation
- Most expensive part of workflow

**Rollback cost:**
- Draft closure + deletion
- Two repository operations
- Only on error path

## Error Handling

The Engine handles errors at each workflow stage:

### Error Categories

**Pre-draft errors:**
- Validation failures
- Repository access errors
- No rollback needed (no draft created)

**Draft-phase errors:**
- Task execution failures
- Lifecycle update failures
- Rollback invoked (draft cleanup)

**Closure errors:**
- Repository write failures
- Git operation failures
- No rollback (would likely fail again)

**Post-closure errors:**
- Metadata update failures
- Watcher notification failures
- Operation considered successful (package revision created)

### Error Recovery

**Client retry:**
- Optimistic locking errors: Re-read and retry
- Transient errors: Retry with backoff
- Validation errors: Fix input and retry

**Automatic recovery:**
- Rollback on draft-phase errors
- Repository garbage collection for orphaned drafts
- Cache refresh for stale data

**Manual recovery:**
- Rollback failures may require manual cleanup
- Repository inspection and repair
- Draft deletion through repository tools
