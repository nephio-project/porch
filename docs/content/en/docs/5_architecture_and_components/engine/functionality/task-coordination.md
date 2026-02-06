---
title: "Task Coordination"
type: docs
weight: 4
description: |
  Detailed architecture of task handler integration and coordination.
---

## Overview

The Engine coordinates task execution by delegating to the Task Handler. Tasks represent operations that transform package content (init, clone, edit, upgrade, render). The Engine orchestrates when and how tasks are executed, while the Task Handler implements the actual transformations.

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Task Coordination System                   │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │      Engine      │      │  Task Handler    │         │
│  │  Orchestration   │ ───> │  Execution       │         │
│  │                  │      │                  │         │
│  │  • When to Run   │      │  • Init          │         │
│  │  • Draft Mgmt    │      │  • Clone         │         │
│  │  • Error Handle  │      │  • Edit          │         │
│  └──────────────────┘      │  • Upgrade       │         │
│           │                │  • Render        │         │
│           │                └──────────────────┘         │
│           ↓                         │                   │
│  ┌──────────────────┐               ↓                   │
│  │   Function       │      ┌──────────────────┐         │
│  │   Runtime        │ <─── │  Builtin Funcs   │         │
│  │                  │      │                  │         │
│  │  • gRPC          │      │  • set-namespace │         │
│  │  • Builtin       │      │  • ensure-context│         │
│  └──────────────────┘      └──────────────────┘         │
└─────────────────────────────────────────────────────────┘
```

## Task Handler Integration

The Engine integrates with the Task Handler through three main operations:

### Integration Points

```
   Engine                     Task Handler
     ↓                              ↓
CreatePackageRevision ──────> ApplyTask
     ↓                              ↓
UpdatePackageRevision ──────> DoPRMutations
     ↓                              ↓
UpdatePackageResources ─────> DoPRResourceMutations
```

**Three integration points:**
1. **ApplyTask**: Execute task during package revision creation
2. **DoPRMutations**: Apply mutations during package revision update
3. **DoPRResourceMutations**: Apply resource mutations during resource update

**Engine responsibilities:**
- Determine when to invoke task handler
- Provide draft workspace for modifications
- Handle errors and rollback
- Manage lifecycle transitions

**Task Handler responsibilities:**
- Execute task transformations
- Modify draft resources
- Apply builtin functions
- Return results or errors

## ApplyTask - Creation Task Execution

The Engine invokes ApplyTask during package revision creation:

### ApplyTask Flow

```
CreatePackageRevision
        ↓
  Create Draft
        ↓
  ApplyTask(draft, repo, pr, config)
        ↓
  Task Handler
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
        ↓
  Engine
        ↓
  Update Lifecycle
        ↓
  Close Draft
```

**Process:**
1. **Engine creates draft** (mutable workspace)
2. **Engine invokes ApplyTask** with:
   - Draft to modify
   - Repository object for context
   - PackageRevision spec with task definition
   - Package configuration (path, name, etc.)
3. **Task Handler executes task** based on type
4. **Task Handler applies builtin functions** (package context generation)
5. **Task Handler returns** modified draft or error
6. **Engine updates lifecycle** if successful
7. **Engine closes draft** to create package revision

### ApplyTask Parameters

**Draft:**
- Mutable workspace for modifications
- Provides UpdateResources, GetResources methods
- Isolated from other revisions

**Repository Object:**
- Repository CR specification
- Used for context (repository name, namespace)
- Passed to task implementations

**PackageRevision:**
- Contains task specification in Spec.Tasks
- First task in list executed
- Task type determines which implementation runs

**Package Config:**
- Package path, name, workspace
- Upstream reference (for clone/upgrade)
- Additional metadata

### Task Execution

**Task Handler executes the task:**
- Task type determines which implementation runs (init, clone, edit, upgrade)
- Task handler modifies draft resources based on task logic
- Builtin functions applied after task execution
- Modified draft returned to Engine

**Task types:**
- **init**: Create new package from scratch
- **clone**: Copy package from upstream
- **edit**: Create new revision from existing package
- **upgrade**: Merge changes from new upstream version

**Handback to Engine:**
- **Success**: Returns modified draft
- **Error**: Returns error, triggers rollback

## DoPRMutations - Update Mutations

The Engine invokes DoPRMutations during package revision updates:

### DoPRMutations Flow

```
UpdatePackageRevision
        ↓
  Open Draft from Existing
        ↓
  DoPRMutations(repoPR, oldObj, newObj, draft)
        ↓
  Task Handler
        ↓
  Compare Task Lists
        ↓
  New Tasks? ──No──> Return Draft
        │
       Yes
        ↓
  Apply New Tasks
        ↓
  Return Modified Draft
        ↓
  Engine
        ↓
  Update Lifecycle
        ↓
  Close Draft
```

**Process:**
1. **Engine opens draft** from existing package revision
2. **Engine invokes DoPRMutations** with:
   - Repository package revision (for context)
   - Old PackageRevision spec
   - New PackageRevision spec
   - Draft to modify
3. **Task Handler compares** old and new task lists
4. **Task Handler applies** any new tasks
5. **Task Handler returns** modified draft or error
6. **Engine updates lifecycle** if changed
7. **Engine closes draft** to persist changes

### DoPRMutations Parameters

**Repository PackageRevision:**
- Current package revision from repository
- Provides context for mutations
- Not directly modified (draft is modified)

**Old PackageRevision:**
- Previous PackageRevision spec
- Used for comparison
- Identifies what changed

**New PackageRevision:**
- Desired PackageRevision spec
- Contains new tasks to apply
- Target state

**Draft:**
- Mutable workspace for modifications
- Already contains current package content
- Modified by new tasks

### Task Comparison

```
Compare Task Lists
        ↓
  Old Tasks: [init, clone]
  New Tasks: [init, clone, render]
        ↓
  Identify New Tasks: [render]
        ↓
  Apply New Tasks
```

**Comparison logic:**
- Tasks are append-only (never removed)
- Compare task list lengths
- New tasks are those beyond old list length
- Apply new tasks in order

**Task application:**
- Each new task executed sequentially
- Draft modified by each task
- Errors stop processing and return

## DoPRResourceMutations - Resource Updates

The Engine invokes DoPRResourceMutations during resource updates:

### DoPRResourceMutations Flow

```
UpdatePackageResources
        ↓
  Open Draft from Existing
        ↓
  DoPRResourceMutations(pr, draft, oldRes, newRes)
        ↓
  Task Handler
        ↓
  Update Package Resources
        ↓
  Execute Render Task
        ↓
  Run Function Pipeline
        ↓
  Return RenderStatus
        ↓
  Engine
        ↓
  Close Draft (no lifecycle change)
```

**Process:**
1. **Engine opens draft** from existing package revision
2. **Engine invokes DoPRResourceMutations** with:
   - Package revision (for context)
   - Draft to modify
   - Old PackageRevisionResources
   - New PackageRevisionResources
3. **Task Handler updates** package resources in draft
4. **Task Handler executes render** (runs function pipeline)
5. **Task Handler returns** RenderStatus with function results
6. **Engine closes draft** without lifecycle change

### DoPRResourceMutations Parameters

**PackageRevision:**
- Current package revision from repository
- Provides context for mutations
- Contains function pipeline configuration

**Draft:**
- Mutable workspace for modifications
- Resources updated directly
- Modified by render task

**Old PackageRevisionResources:**
- Previous resource content
- Used for comparison (not currently used)
- Audit trail

**New PackageRevisionResources:**
- Desired resource content
- Applied to draft
- Target state

### Render Execution

**Task Handler executes render:**
- Updates package resources in draft
- Executes render task (runs function pipeline)
- Returns RenderStatus with function results

**Handback to Engine:**
- **RenderStatus**: Contains function execution results
  - Result: Overall success/failure
  - Error: Error message if failed
  - Exit code: Function exit codes
  - Function details: Per-function results
- **Modified draft**: Resources updated by render task

## Task Handler Configuration

The Engine configures the Task Handler during initialization:

### Task Handler Setup

```
NewCaDEngine(opts...)
        ↓
  Create cadEngine
        ↓
  taskHandler = GetDefaultTaskHandler()
        ↓
  Apply Options:
        ↓
    • Function Runtimes
    • Credential Resolvers
    • Reference Resolvers
        ↓
  Return Configured Engine
```

**Configuration options:**
- **Function runtimes**: Builtin, gRPC, or multi-runtime
- **Credential resolver**: For accessing upstream packages
- **Reference resolver**: For resolving package references
- **User info provider**: For audit trails

### Function Runtime Configuration

**Runtime types:**
- **Builtin Runtime**: For built-in functions (set-namespace, etc.)
- **gRPC Runtime**: For external function runner service
- **Multi-Runtime**: Chains multiple runtimes together

**Runtime selection:**
- Configured at Porch server startup
- Passed to task handler during engine initialization
- Task handler uses runtime for function execution

**For details on function runtime implementations, see [Function Runner]({{% relref "/docs/5_architecture_and_components/function-runner/_index.md" %}}).**

## Error Handling

The Engine handles task execution errors:

### Task Error Flow

```
ApplyTask/DoPRMutations/DoPRResourceMutations
        ↓
  Task Handler Executes
        ↓
  Error? ──No──> Return Success
        │
       Yes
        ↓
  Return Error
        ↓
  Engine
        ↓
  Rollback (if creation)
        ↓
  Return Error to Client
```

**Error handling:**
- Task errors returned to Engine
- Engine triggers rollback (for creation)
- Error propagated to client
- No partial package revisions created

### Error Types

**Task execution errors:**
- Invalid task configuration
- Upstream package not found
- Merge conflicts (upgrade)
- Function execution failures

**Function errors:**
- Function image not found
- Function execution timeout
- Function validation failures
- Function runtime errors

**Resource errors:**
- Invalid YAML syntax
- Missing required fields
- Schema validation failures

### Error Recovery

**Client retry:**
- Fix task configuration
- Resolve upstream references
- Fix resource syntax
- Retry operation

**Automatic recovery:**
- Rollback on creation errors
- Draft cleanup
- No manual intervention needed

## Task Coordination Patterns

The Engine uses specific patterns for task coordination:

### Task Execution

```
Task List: [init/clone/edit/upgrade]
        ↓
  Execute [init/clone/edit/upgrade]
        ↓
  Success? ──No──> Return Error
        │
       Yes
        ↓
  Return Success
```

**Sequential execution:**
- Typically one task (init, clone, edit, or upgrade)
- Render tasks may temporarily appear but are cleaned up
- Each task must succeed before next
- First error stops execution
- No parallel task execution

**Rationale:**
- Tasks may depend on previous tasks
- Simplifies error handling
- Maintains consistent state

### Task List Pattern

```
Create: [init]
        ↓
Update: [clone/edit/upgrade]
        ↓
Update: [render]
```

**Task list pattern:**
- Single persistent task indicating [init/clone/edit/upgrade] method
- Render tasks added temporarily during operations
- Render tasks cleaned up after execution
- Task history shows package origin

### Draft Isolation

```
Package Revision A          Package Revision B
        ↓                           ↓
    Draft A                     Draft B
        ↓                           ↓
  Task Execution            Task Execution
        ↓                           ↓
   Independent                 Independent
```

**Isolation guarantees:**
- Each draft has a separate context
- Task execution doesn't affect other drafts
- Concurrent task execution possible (different packages)
- No shared state between tasks

## Task Handler Interface

The Engine interacts with Task Handler through a defined interface:

### Interface Methods

**ApplyTask:**
- Accepts: context, draft workspace, repository object, package revision spec, package configuration
- Returns: error on failure
- Purpose: Execute task during package revision creation

**DoPRMutations:**
- Accepts: context, repository package revision, old spec, new spec, draft workspace
- Returns: error on failure
- Purpose: Apply mutations during package revision update

**DoPRResourceMutations:**
- Accepts: context, package revision, draft workspace, old resources, new resources
- Returns: render status and error
- Purpose: Apply resource mutations and execute render task

### Interface Characteristics

**Context-aware:**
- All methods accept context for cancellation
- Timeout and deadline support
- Tracing and logging context

**Draft-based:**
- All methods work with draft workspaces
- No direct repository modification
- Isolation and atomicity

**Error-returning:**
- Errors indicate task failure
- Engine handles rollback
- Clear error messages for debugging

## Task Coordination Benefits

The task coordination pattern provides several benefits:

### Separation of Concerns

**Engine:**
- Orchestrates workflow
- Manages drafts and lifecycle
- Handles errors and rollback
- Enforces business rules

**Task Handler:**
- Implements task logic
- Transforms package content
- Executes functions
- Returns results

**Benefits:**
- Clear responsibilities
- Easier testing and maintenance
- Pluggable task implementations
- Independent evolution

### Extensibility

**New task types:**
- Implement in task handler
- Engine unchanged
- Register with task handler
- Available for use

**New function runtimes:**
- Implement runtime interface
- Configure at startup
- Task handler uses new runtime
- No CaDEngine changes

### Testability

**Engine testing:**
- Mock task handler
- Test orchestration logic
- Test error handling
- Test rollback mechanism

**Task Handler testing:**
- Mock draft interface
- Test task implementations
- Test function execution
- Independent of CaDEngine
