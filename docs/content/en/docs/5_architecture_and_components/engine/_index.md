---
title: "Engine (CaD Engine)"
type: docs
weight: 2
description: Core package orchestration logic
---

## What is the Engine?

The Engine (Configuration as Data Engine) is the **orchestration brain** of Porch. It sits between the API Server and the lower-level components (Cache, Task Handler, Repository Adapters, Function Runtime), coordinating all package lifecycle operations.

**Think of it as a conductor**: It doesn't execute tasks directly, access repositories, or run functions itself. Instead, it orchestrates when and how each component performs its role to accomplish complex package operations.

**Source Location**: `pkg/engine/`

## What Does the Engine Do?

The Engine's core responsibilities:

1. **Orchestrates Package Operations**: Coordinates multi-step workflows (create, update, delete, list)
2. **Enforces Business Rules**: Validates lifecycle transitions, workspace uniqueness, package constraints
3. **Manages Lifecycle States**: Controls transitions between Draft → Proposed → Published → DeletionProposed
4. **Handles Concurrency**: Implements optimistic locking to prevent conflicting modifications
5. **Coordinates Components**: Delegates to Cache (storage), Task Handler (operations), Function Runtime (KRM functions)
6. **Provides Rollback**: Ensures atomic operations with cleanup on failure
7. **Manages Watches**: Notifies clients of package changes in real-time

## How It Fits in Porch Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Porch Server                         │
│                                                             │
│  ┌──────────────┐         ┌─────────────────────────────┐  │
│  │  API Server  │────────▶│          Engine             │  │
│  │              │         │  (Orchestration Logic)      │  │
│  └──────────────┘         └─────────────────────────────┘  │
│                                      │                      │
│                           ┌──────────┼──────────┐          │
│                           │          │          │          │
│                           ▼          ▼          ▼          │
│                      ┌────────┐ ┌────────┐ ┌──────────┐   │
│                      │ Cache  │ │  Task  │ │ Function │   │
│                      │        │ │Handler │ │ Runtime  │   │
│                      └────────┘ └────────┘ └──────────┘   │
│                           │                      │          │
└───────────────────────────┼──────────────────────┼──────────┘
                            │                      │
                            ▼                      ▼
                    ┌──────────────┐      ┌──────────────┐
                    │ Git/OCI Repo │      │Function Runner│
                    └──────────────┘      │  (gRPC)      │
                                          └──────────────┘
```

**Flow**: API Server receives request → Engine orchestrates → Cache/Task Handler/Function Runtime execute → Engine returns result

## Why Does the Engine Exist?

### Separation of Concerns

Without the Engine, the API Server would need to know:
- How to interact with Git/OCI repositories
- How to execute KRM functions  
- How to manage caching
- How to handle rollbacks
- All business logic for package lifecycles

This would create tight coupling and make the system unmaintainable.

### Centralized Business Logic

All package lifecycle rules live in one place:
- Which lifecycle transitions are allowed (Draft can become Proposed, but Published cannot become Draft)
- When packages can be modified (only Draft/Proposed, not Published)
- What validations must pass (unique workspace names, valid clone sources)
- How to handle concurrent modifications (optimistic locking)

### Transaction-like Semantics

The Engine provides atomic operations with rollback:
- If any step fails during package creation, the Engine rolls back changes
- Ensures the system never ends up in an inconsistent state
- Provides clean error messages to users

## Quick Example: Creating a Package

```bash
# User runs this command
kubectl apply -f packagerevision.yaml
```

**What the Engine does**:

1. **Validates Request**: Checks lifecycle is Draft/Proposed, workspace name is unique
2. **Opens Repository**: Asks Cache to open the target repository
3. **Creates Draft**: Asks Repository to create a draft package revision
4. **Applies Tasks**: Delegates to Task Handler to execute init/clone/edit tasks
5. **Executes Functions**: If rendering, delegates to Function Runtime
6. **Updates Lifecycle**: Sets the package to requested lifecycle state
7. **Closes Draft**: Commits changes to repository
8. **Notifies Watchers**: Sends watch events to listening clients
9. **Returns Result**: Sends PackageRevision back to API Server

If any step fails, the Engine rolls back the draft creation.

## Key Concepts

### Package Lifecycle States

The Engine enforces these lifecycle states:

- **Draft**: Work in progress, can be modified freely
- **Proposed**: Ready for review, can still be modified
- **Published**: Finalized, immutable (only metadata can change)
- **DeletionProposed**: Marked for deletion, awaiting approval

### Optimistic Concurrency Control

The Engine uses resourceVersion to prevent conflicting updates:
- Each package has a resourceVersion that increments on every change
- Updates must provide the current resourceVersion
- If resourceVersion doesn't match, update fails with 409 Conflict
- Client must fetch latest version and retry

### Rollback on Failure

If any operation fails partway through:
- Engine attempts to rollback changes (delete draft, revert state)
- Ensures repository doesn't end up with partial/corrupted packages
- Returns clear error message to user

## Summary

The Engine is the orchestration layer that makes Porch work. It:

- **Coordinates** between API Server, Cache, Task Handler, Repository Adapters, and Function Runtime
- **Enforces** business rules and lifecycle constraints
- **Provides** atomic operations with rollback
- **Manages** optimistic concurrency control
- **Enables** real-time watch notifications
- **Supports** multiple function execution strategies (built-in, gRPC, multi-runtime)

Understanding the Engine is key to understanding how Porch manages package lifecycles, handles errors, and maintains consistency across distributed operations.
