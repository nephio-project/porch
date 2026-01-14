---
title: "Functionality"
type: docs
weight: 2
description: What the Engine does and its core capabilities
---

This section describes what the Engine actually does - its core capabilities, workflows, and the operations it performs.

## Core Capabilities

### 1. Package Revision Operations

The Engine provides CRUD operations for PackageRevisions:

#### Create PackageRevision

**What it does**: Creates a new package revision in a repository.

**Supports**:
- **Init**: Create empty package from scratch
- **Clone**: Clone package from another repository
- **Copy**: Create new revision of existing package
- **Upgrade**: Merge upstream changes into package

**Workflow**:
1. Validate request (lifecycle, workspace name, task type)
2. Check workspace name uniqueness
3. Validate package path doesn't overlap with existing packages
4. Create draft in repository
5. Apply task (init/clone/copy/upgrade)
6. Execute function pipeline if specified
7. Set lifecycle state
8. Commit to repository
9. Notify watchers

**Rollback**: If any step fails, deletes the draft

#### Update PackageRevision

**What it does**: Modifies an existing package revision.

**Supports**:
- **Draft/Proposed**: Can modify content, tasks, lifecycle
- **Published**: Can only modify metadata (labels, annotations) and lifecycle
- **Lifecycle transitions**: Draft → Proposed → Published, Published → DeletionProposed

**Workflow**:
1. Validate resourceVersion (optimistic locking)
2. Check current lifecycle state
3. If Published: Only update metadata and lifecycle
4. If Draft/Proposed: Create draft, apply mutations, close draft
5. Update metadata (labels, annotations, finalizers)
6. Notify watchers

**Special case**: If removing last finalizer from terminating package, deletes it instead

#### Delete PackageRevision

**What it does**: Removes a package revision from repository.

**Workflow**:
1. Open repository
2. Delete package revision
3. Repository handles Git/OCI deletion
4. Watchers notified via repository sync

**Note**: Actual deletion happens in repository layer, Engine just coordinates

#### List PackageRevisions

**What it does**: Returns list of package revisions matching filter.

**Supports filtering by**:
- Repository name
- Package name
- Workspace name
- Revision number
- Lifecycle state

**Workflow**:
1. Open repository from cache
2. Ask repository to list package revisions with filter
3. Return results

**Performance**: Served from cache, very fast

### 2. Package Revision Resources Operations

The Engine provides operations for PackageRevisionResources (file contents):

#### Update PackageRevisionResources

**What it does**: Updates the file contents of a Draft package revision.

**Restrictions**:
- Only Draft packages can have resources updated
- Proposed/Published packages are immutable

**Workflow**:
1. Validate resourceVersion (optimistic locking)
2. Check lifecycle is Draft
3. Create draft
4. Apply resource mutations (add/modify/delete files)
5. Execute function pipeline if needed
6. Close draft
7. Return updated package and render status

**Use case**: Direct file editing, applying patches, replacing resources

### 3. Package Operations (Deprecated)

The Engine provides operations for PorchPackage (package-level view):

#### Create Package

**Status**: Implemented but rarely used

**What it does**: Creates a package-level object

#### Update Package

**Status**: Not implemented (returns error)

#### Delete Package

**What it does**: Deletes a package and all its revisions

**Note**: PorchPackage is being deprecated in favor of working directly with PackageRevisions

### 4. Lifecycle Management

The Engine enforces package lifecycle state machine:

```
        ┌─────────┐
        │  Draft  │◄────────────┐
        └────┬────┘             │
             │                  │
             │ propose          │ reject
             ▼                  │
      ┌──────────┐              │
      │ Proposed │──────────────┘
      └─────┬────┘
            │
            │ approve
            ▼
      ┌───────────┐
      │ Published │
      └─────┬─────┘
            │
            │ mark for deletion
            ▼
   ┌──────────────────┐
   │DeletionProposed  │
   └──────────────────┘
```

**Allowed Transitions**:
- Draft → Proposed (propose for review)
- Draft → Published (direct publish)
- Proposed → Draft (reject, needs changes)
- Proposed → Published (approve)
- Published → DeletionProposed (mark for deletion)

**Forbidden Transitions**:
- Published → Draft (can't un-publish)
- Published → Proposed (can't un-publish)
- DeletionProposed → any other state (deletion is final)

**Enforcement**: Engine validates lifecycle transitions and rejects invalid ones

### 5. Validation

The Engine performs multiple validation checks:

#### Workspace Name Validation

**Rule**: Workspace names must be unique within a package

**Why**: Workspace name is used to generate PackageRevision metadata.name

**Check**: Before creating package, Engine checks no existing revision has same workspace name

#### Clone Validation

**Rule**: Clone can only create first revision of a package

**Why**: Subsequent revisions should use Copy, not Clone

**Check**: Engine verifies package doesn't already exist in target repository

#### Upgrade Validation

**Rule**: All source PackageRevisions of upgrade must be Published

**Why**: Can't upgrade from Draft/Proposed (unstable sources)

**Check**: Engine verifies oldUpstream, newUpstream, and localPackageRevisionRef are all Published

#### Package Path Overlap Validation

**Rule**: Package paths cannot overlap (one cannot be subdirectory of another)

**Why**: Prevents ambiguity and conflicts in repository structure

**Check**: Engine validates new package path doesn't overlap with existing packages

#### Lifecycle Validation

**Rule**: Can only create Draft or Proposed packages, not Published

**Why**: Published packages must go through Draft/Proposed workflow

**Check**: Engine rejects attempts to create Published packages directly

### 6. Concurrency Control

The Engine implements optimistic concurrency control:

#### ResourceVersion Checking

**How it works**:
1. Client reads package (gets resourceVersion "v1")
2. Client modifies package
3. Client sends update with resourceVersion "v1"
4. Engine checks if current resourceVersion is still "v1"
5. If match: Update succeeds, resourceVersion becomes "v2"
6. If mismatch: Update fails with 409 Conflict

**Error message**: "the object has been modified; please apply your changes to the latest version and try again"

**Client handling**: Client should fetch latest version and retry

#### Package-Level Locking

**Note**: While Engine uses optimistic locking, the repository layer may use package-level mutexes to prevent concurrent modifications to the same package. This is an implementation detail of the repository, not the Engine.

### 7. Rollback and Error Handling

The Engine provides transaction-like semantics:

#### Rollback on Create Failure

**Scenario**: Package creation fails after draft is created

**Rollback**:
1. Convert draft to PackageRevision
2. Delete PackageRevision
3. Return error to client

**Result**: Repository is left in clean state, no partial packages

#### Rollback on Update Failure

**Scenario**: Package update fails after draft is created

**Rollback**:
1. Draft is automatically discarded (not closed)
2. Original package remains unchanged
3. Return error to client

**Result**: Package is unchanged, no partial updates

#### Error Categories

The Engine returns different error types:

- **Validation Errors**: 400 Bad Request or 422 Unprocessable Entity
- **Conflict Errors**: 409 Conflict (optimistic lock failure)
- **Not Found Errors**: 404 Not Found
- **Internal Errors**: 500 Internal Server Error

### 8. Watch Management

The Engine manages real-time notifications:

#### Watch Registration

**How it works**:
1. Client calls WatchPackageRevisions with filter
2. Engine registers watcher with WatcherManager
3. Watcher receives events for matching packages

**Filters**:
- Repository name
- Package name
- Workspace name
- Namespace

#### Watch Events

**Event types**:
- **ADDED**: New package created
- **MODIFIED**: Package updated
- **DELETED**: Package deleted

**When sent**:
- After successful create: ADDED event
- After successful update: MODIFIED event
- After successful delete: DELETED event (via repository sync)

#### Watch Cleanup

**Automatic cleanup**:
- When client context is cancelled
- When client connection closes
- When watcher returns false from callback

**Slot reuse**: Engine reuses watcher slots for efficiency

### 9. Function Runtime Coordination

The Engine coordinates function execution:

#### Runtime Selection

**Built-in Runtime**: For known functions (apply-replacements, set-namespace, starlark)
- Executes directly in Engine process
- Fast, no network overhead

**gRPC Runtime**: For external functions
- Delegates to Function Runner service
- Spawns function pods
- Handles any function image

**Multi-Runtime**: Tries built-in first, falls back to gRPC
- Best performance for built-ins
- Flexibility for external functions

#### Function Execution Flow

1. Task Handler requests function execution
2. Engine delegates to configured runtime
3. Runtime executes function (built-in or gRPC)
4. Function transforms package resources
5. Results returned to Task Handler
6. Task Handler updates package

**Error handling**: Function errors are captured and returned to user

## Common Workflows

### Creating a New Package (Init)

```
User: kubectl apply -f packagerevision.yaml (with init task)
  ↓
Engine: Validate request
  ↓
Engine: Check workspace name unique
  ↓
Engine: Create draft in repository
  ↓
Engine: Apply init task (create Kptfile, README)
  ↓
Engine: Set lifecycle to Draft
  ↓
Engine: Commit to repository
  ↓
Engine: Notify watchers (ADDED event)
  ↓
User: Receives created PackageRevision
```

### Cloning a Package

```
User: kubectl apply -f packagerevision.yaml (with clone task)
  ↓
Engine: Validate request
  ↓
Engine: Check package doesn't already exist
  ↓
Engine: Check workspace name unique
  ↓
Engine: Create draft in repository
  ↓
Engine: Apply clone task (copy from upstream)
  ↓
Engine: Execute function pipeline (if specified)
  ↓
Engine: Set lifecycle to Draft
  ↓
Engine: Commit to repository
  ↓
Engine: Notify watchers (ADDED event)
  ↓
User: Receives created PackageRevision
```

### Updating Package Content

```
User: kubectl apply -f packagerevisionresources.yaml
  ↓
Engine: Validate resourceVersion
  ↓
Engine: Check lifecycle is Draft
  ↓
Engine: Create draft
  ↓
Engine: Apply resource mutations (file changes)
  ↓
Engine: Execute function pipeline (render)
  ↓
Engine: Close draft (commit)
  ↓
Engine: Notify watchers (MODIFIED event)
  ↓
User: Receives updated PackageRevision
```

### Approving a Package (Lifecycle Transition)

```
User: kubectl patch packagerevision --subresource=approval \
      -p '{"spec":{"lifecycle":"Published"}}'
  ↓
Engine: Validate resourceVersion
  ↓
Engine: Check current lifecycle is Proposed
  ↓
Engine: Update lifecycle to Published
  ↓
Engine: Update metadata
  ↓
Engine: Notify watchers (MODIFIED event)
  ↓
User: Receives updated PackageRevision (now Published)
```

## Summary

The Engine provides:

**Operations**:
- Create, Update, Delete, List PackageRevisions
- Update PackageRevisionResources
- Create, Delete Packages (deprecated)

**Lifecycle Management**:
- Enforces state machine (Draft → Proposed → Published → DeletionProposed)
- Validates transitions
- Prevents invalid state changes

**Validation**:
- Workspace name uniqueness
- Clone restrictions
- Upgrade source validation
- Package path overlap checks
- Lifecycle constraints

**Concurrency Control**:
- Optimistic locking via resourceVersion
- Conflict detection and retry

**Rollback**:
- Automatic cleanup on failure
- Transaction-like semantics
- Consistent state guaranteed

**Watch Management**:
- Real-time notifications
- Event filtering
- Automatic cleanup

**Function Coordination**:
- Multiple runtime strategies
- Built-in and external functions
- Error handling

The Engine is the single source of truth for package operations, ensuring consistency, correctness, and reliability across all package lifecycle workflows.
