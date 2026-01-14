---
title: "Design Rationale"
type: docs
weight: 1
description: Why the Engine exists and key design decisions
---

This section explains why the Engine exists as a separate component, what alternatives were considered, and the key design decisions that shaped its implementation.

## Why a Separate Engine Component?

### The Problem

Porch needs to perform complex, multi-step package operations:
- Creating a package involves: validation → draft creation → task execution → function rendering → lifecycle update → commit
- Updating a package involves: validation → draft creation → applying changes → re-rendering → commit → watch notification
- Each operation touches multiple subsystems: Cache, Repository Adapters, Task Handler, Function Runtime

Without a central orchestrator, this logic would be scattered across components, leading to:
- **Duplication**: Same orchestration logic repeated in API Server, CLI, controllers
- **Inconsistency**: Different code paths might handle operations differently
- **Tight Coupling**: API Server would need to know about Git operations, function execution, caching
- **Hard to Test**: No single place to test business logic
- **Hard to Maintain**: Changes require updates across multiple components

### The Solution: Engine as Orchestrator

The Engine centralizes all orchestration logic in one place:

```
Without Engine:                    With Engine:
┌──────────────┐                  ┌──────────────┐
│  API Server  │                  │  API Server  │
│              │                  │              │
│ - Validation │                  │ - HTTP/Auth  │
│ - Git ops    │                  └──────┬───────┘
│ - Functions  │                         │
│ - Caching    │                         ▼
│ - Rollback   │                  ┌──────────────┐
│ - Watches    │                  │    Engine    │
└──────────────┘                  │              │
                                  │ - Validation │
Duplicated in:                    │ - Orchestrate│
- CLI                             │ - Rollback   │
- Controllers                     │ - Watches    │
- Tests                           └──────┬───────┘
                                         │
                                    Delegates to:
                                    - Cache
                                    - Task Handler
                                    - Function Runtime
```

## Design Decisions

### 1. Interface-Based Design

**Decision**: Engine exposes a clean interface (CaDEngine) with well-defined operations.

**Why**:
- Allows multiple implementations (production, testing, mocking)
- Clear contract between API Server and Engine
- Easy to test in isolation
- Future-proof for alternative implementations

**Interface**:
```go
type CaDEngine interface {
    CreatePackageRevision(...)
    UpdatePackageRevision(...)
    DeletePackageRevision(...)
    ListPackageRevisions(...)
    UpdatePackageResources(...)
    // ...
}
```

### 2. Dependency Injection via Options

**Decision**: Engine dependencies are injected via functional options pattern.

**Why**:
- Flexible configuration without breaking API
- Easy to add new dependencies
- Clear separation of concerns
- Testable (can inject mocks)

**Example**:
```go
engine := NewCaDEngine(
    WithCache(cache),
    WithGRPCFunctionRuntime(options),
    WithCredentialResolver(resolver),
    WithWatcherManager(watcherManager),
)
```

### 3. Optimistic Concurrency Control

**Decision**: Use resourceVersion-based optimistic locking instead of pessimistic locks.

**Alternatives Considered**:
- **Pessimistic Locking**: Hold locks during operations
  - ❌ Would block other operations
  - ❌ Deadlock risk
  - ❌ Poor scalability
  - ❌ Long-running operations (Git, functions) would hold locks too long

- **Last-Write-Wins**: No concurrency control
  - ❌ Lost updates
  - ❌ Data corruption
  - ❌ Unpredictable behavior

- **Optimistic Locking** (Chosen):
  - ✅ No blocking
  - ✅ Scales well
  - ✅ Conflicts are rare in practice
  - ✅ Clients can easily retry
  - ✅ Kubernetes-native pattern

**How It Works**:
1. Client reads package (gets resourceVersion "123")
2. Client modifies package locally
3. Client sends update with resourceVersion "123"
4. Engine checks if current resourceVersion is still "123"
5. If yes: Update succeeds, resourceVersion becomes "124"
6. If no: Update fails with 409 Conflict, client must retry

### 4. Rollback on Failure

**Decision**: Engine provides transaction-like semantics with automatic rollback.

**Why**:
- Package operations are multi-step (create draft → apply tasks → render → commit)
- Failure at any step should not leave partial state
- Users should never see corrupted packages
- Repositories should stay consistent

**Implementation**:
```go
// Create draft
draft, err := repo.CreatePackageRevisionDraft(ctx, newPr)
if err != nil {
    return nil, err
}

// Setup rollback function
rollback := func() {
    if pkgRev, err := repo.ClosePackageRevisionDraft(ctx, draft, 0); err == nil {
        repo.DeletePackageRevision(ctx, pkgRev)
    }
}

// Apply tasks
if err := taskHandler.ApplyTask(ctx, draft, ...); err != nil {
    rollback()  // Clean up on failure
    return nil, err
}

// Update lifecycle
if err := draft.UpdateLifecycle(ctx, lifecycle); err != nil {
    rollback()  // Clean up on failure
    return nil, err
}

// Success - commit
return repo.ClosePackageRevisionDraft(ctx, draft, 0)
```

### 5. Pluggable Function Runtime

**Decision**: Support multiple function execution strategies via runtime abstraction.

**Why**:
- Different deployment scenarios need different runtimes
- Built-in functions (fast, no network) for common operations
- gRPC runtime (scalable, isolated) for external functions
- Multi-runtime (combine both) for flexibility

**Runtimes**:

1. **Built-in Runtime**: Executes functions directly in Engine process
   - ✅ Fast (no network overhead)
   - ✅ Simple deployment
   - ❌ Limited to pre-compiled functions
   - **Use case**: apply-replacements, set-namespace, starlark

2. **gRPC Runtime**: Delegates to Function Runner service
   - ✅ Scalable (separate service)
   - ✅ Isolated (functions run in pods)
   - ✅ Supports any function image
   - ❌ Network overhead
   - **Use case**: Production deployments

3. **Multi-Runtime**: Tries built-in first, falls back to gRPC
   - ✅ Best of both worlds
   - ✅ Fast for built-ins, flexible for others
   - **Use case**: Most deployments

### 6. Watch Manager Integration

**Decision**: Engine manages watch notifications for real-time updates.

**Why**:
- Kubernetes clients expect watch support
- Controllers need to react to package changes
- Users want real-time feedback (kubectl get -w)
- Engine is the single source of truth for package changes

**How It Works**:
- Clients register watches with Engine's WatcherManager
- Engine sends watch events after successful operations
- Events: ADDED (create), MODIFIED (update), DELETED (delete)
- Watchers automatically cleaned up when client disconnects

### 7. Validation Layers

**Decision**: Multiple validation layers at different stages.

**Why**:
- Fail fast for simple errors (schema validation)
- Enforce business rules (lifecycle transitions)
- Validate storage constraints (Git limitations)
- Provide clear error messages

**Validation Layers**:

1. **API Server Schema Validation**: Catches format errors
2. **Engine Business Logic Validation**: Enforces rules
   - Lifecycle transitions (can't create Published package)
   - Workspace name uniqueness
   - Clone source validation
   - Package path overlap checks
3. **Repository Validation**: Storage-specific constraints
   - Git branch naming rules
   - File size limits
   - Character encoding

## Trade-offs and Limitations

### Trade-off: Complexity vs Flexibility

**Decision**: Accept Engine complexity for system-wide simplicity.

- Engine code is complex (orchestration, rollback, concurrency)
- But API Server, CLI, controllers are simple (just call Engine)
- Net result: Easier to maintain overall system

### Trade-off: Performance vs Consistency

**Decision**: Prioritize consistency over raw performance.

- Optimistic locking adds retry overhead
- Rollback adds cleanup overhead
- But ensures data consistency and correctness
- Performance is still good (operations complete in seconds)

### Limitation: No Distributed Transactions

**Decision**: Engine doesn't support true distributed transactions.

**Why**:
- Git/OCI repositories don't support transactions
- Rollback is best-effort (may fail)
- Rare failure cases may require manual cleanup

**Mitigation**:
- Rollback works in 99.9% of cases
- Failed operations are logged
- Operators can manually clean up if needed

## Evolution and Future Directions

### Current State

The Engine has evolved from kpt's package manipulation logic:
- Originally: kpt CLI performed operations locally
- Now: Engine performs operations server-side
- Reuses kpt's proven algorithms (3-way merge, function pipeline)

### Future Enhancements

Potential improvements being considered:

1. **Better Rollback**: More robust cleanup mechanisms
2. **Operation Queuing**: Queue operations per package to avoid conflicts
3. **Async Operations**: Long-running operations return immediately, complete in background
4. **Audit Trail**: Record all operations for compliance
5. **Metrics**: Expose operation latency, success rates, etc.

## Summary

The Engine exists to:
- **Centralize** orchestration logic in one place
- **Simplify** other components (API Server, CLI, controllers)
- **Ensure** consistency through validation and rollback
- **Enable** flexibility through pluggable runtimes
- **Provide** real-time updates through watch management

Key design decisions:
- Interface-based design for testability
- Dependency injection for flexibility
- Optimistic locking for scalability
- Rollback for consistency
- Pluggable runtimes for different deployment scenarios
- Watch integration for real-time updates
- Multi-layer validation for clear errors

These decisions make the Engine complex internally but keep the overall system simple, maintainable, and correct.
