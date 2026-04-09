---
title: "Engine Design"
type: docs
weight: 2
description: |
  Architecture and design patterns of the CaD Engine.
---

## CaD Engine Architecture

The Engine follows a **layered architecture** with clear separation of concerns:

```
┌─────────────────────────────────────────┐
│         CaDEngine Interface             │  ← Public API
├─────────────────────────────────────────┤
│         cadEngine Implementation        │  ← Orchestration Logic
├─────────────────────────────────────────┤
│  ┌──────────┬──────────┬─────────────┐  │
│  │  Cache   │   Task   │   Watcher   │  │  ← Dependencies
│  │          │  Handler │   Manager   │  │
│  └──────────┴──────────┴─────────────┘  │
└─────────────────────────────────────────┘
```

**Key architectural patterns:**

### Dependency Injection via Functional Options

The engine is constructed using a **functional options pattern** that allows flexible configuration of dependencies:

- Cache implementation (CR-based or DB-based)
- Task handler for executing package operations
- Function runtimes (builtin, gRPC, or multi-runtime)
- Credential and reference resolvers
- Watcher manager for change notifications
- User info provider for audit trails

This pattern enables testing with mock implementations and supports different deployment configurations without changing the CaDEngine code.

### Draft-Commit Workflow

The engine implements a **draft-commit pattern** for all package revision modifications:

1. **Open Draft**: Create a mutable draft from an existing package revision or start fresh
2. **Apply Changes**: Execute tasks or resource mutations on the draft
3. **Validate**: Ensure changes meet lifecycle and business rule constraints
4. **Commit**: Close the draft to create an immutable package revision
5. **Rollback**: If any step fails, clean up the draft to maintain consistency

This pattern ensures atomicity - either all changes succeed and are persisted, or none are.

### Repository Abstraction

The engine doesn't directly interact with Git repositories. Instead, it:

- Opens repositories through the **cache layer**
- Works with repository abstractions that hide storage implementation details
- Delegates all storage operations to repository adapters
- Maintains separation between business logic and storage mechanisms

### Concurrency Model

The Engine handles concurrent operations through multiple mechanisms:

**Package-Level Locking:**

The engine uses **per-package mutexes** to prevent concurrent modifications to the same package:

- Mutex key is based on: `namespace-repository-package-workspace`
- Acquired using `TryLock()` - fails fast if lock unavailable
- Returns conflict error if another operation is in progress
- Automatically released after operation completes (success or failure)
- Prevents race conditions during package creation and updates

**Optimistic Locking:**

For update operations, the engine uses **Kubernetes resource versions**:

- Client must provide current resource version in update request
- Engine compares provided version with actual version
- Returns conflict error if versions don't match
- Forces client to re-read and retry with latest version
- Prevents lost updates when multiple clients modify same package

**Draft Isolation:**

Drafts provide natural concurrency isolation:

- Each draft is a separate workspace in the repository
- Multiple drafts can exist for different workspaces simultaneously
- Drafts don't interfere with each other until closed
- Closing a draft is an atomic operation

**Read Operations:**

List and Get operations are **lock-free**:

- Read from cached repository state
- No locking required for queries
- May see slightly stale data during cache refresh
- Eventually consistent with repository state

**Concurrency characteristics:**
- **Single package**: Only one write operation at a time (mutex protected)
- **Different packages**: Fully concurrent operations
- **Read operations**: Always concurrent, no blocking
- **Failed lock acquisition**: Immediate conflict error, no waiting

## Interface Design

The Engine exposes a single interface (`CaDEngine`) with operations grouped by resource type:

### PackageRevision Operations

- **ListPackageRevisions**: Query package revisions with filtering
- **CreatePackageRevision**: Create new package revisions with task execution
- **UpdatePackageRevision**: Modify draft/proposed revisions or transition lifecycle states
- **DeletePackageRevision**: Remove package revisions from repositories
- **UpdatePackageResources**: Modify the resource contents of draft package revisions

### Package Operations

- **ListPackages**: Query packages across repositories

### Supporting Operations

- **ObjectCache**: Access the watcher manager for real-time change notifications
- **OpenRepository**: Internal helper to access repositories through the cache

**Interface characteristics:**

- All operations are **context-aware** for cancellation and tracing
- Operations accept **API objects** (porchapi types) and return **repository abstractions**
- The interface is **synchronous** - operations complete before returning
- Errors are returned directly rather than stored in status fields

## Package Lifecycle State Machine

The Engine enforces a strict state machine for package revision lifecycle:

![Package Lifecycle Workflow](/static/images/porch/flowchart.drawio.svg)

### State Transition Rules

**Draft State:**
- Can be created directly or as default when no lifecycle specified
- Allows full modifications: tasks, resources, metadata, lifecycle
- Can transition to: Proposed, Published
- Cannot be created with Published or DeletionProposed lifecycle

**Proposed State:**
- Indicates package revision is ready for review
- Allows modifications like Draft
- Can transition to: Draft (for rework), Published (for approval)
- Typically used in approval workflows

**Published State:**
- **Immutable** - no resource or task modifications allowed
- Only metadata (labels, annotations) and lifecycle can be updated
- Can transition to: DeletionProposed
- Represents the deployed, production-ready state

**DeletionProposed State:**
- Marks package revision for deletion
- Considered "published" for lifecycle checks
- Final state before actual deletion
- Used to signal intent to remove while maintaining audit trail

### Lifecycle Enforcement

The engine enforces lifecycle rules at multiple points:

1. **Creation Validation**: Prevents creating package revisions directly in Published or DeletionProposed states
2. **Update Validation**: Checks current lifecycle before allowing modifications
3. **Resource Update Validation**: Only allows resource changes on Draft package revisions
4. **Transition Validation**: Ensures requested lifecycle transitions are valid
5. **Optimistic Locking**: Uses resource versions to prevent concurrent modification conflicts

### Special Behaviors

- **Published package revisions** are treated as immutable snapshots - their content cannot change
- **DeletionProposed** package revisions are still considered "published" for dependency checks
- **Draft** package revisions can be freely modified until published
- **Proposed -> Published** Lifecycle transitions are one-way - package revisions cannot revert from Published to Draft
