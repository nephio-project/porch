---
title: "Task Orchestration"
type: docs
weight: 3
description: |
  Detailed architecture of task orchestration and coordination with Engine.
---

## Overview

The Task Handler orchestrates task execution through the genericTaskHandler, which coordinates package transformations, function execution, and integration with the Engine. It implements the TaskHandler interface and manages the complete task execution lifecycle.

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Task Orchestration System                  │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │  Engine          │      │   genericTask    │         │
│  │  Integration     │ ───> │   Handler        │         │
│  │                  │      │                  │         │
│  │  • ApplyTask     │      │  • Task Mapping  │         │
│  │  • DoPRMutations │      │  • Mutation Exec │         │
│  │  • DoPRResource  │      │  • Render Coord  │         │
│  │    Mutations     │      │                  │         │
│  └──────────────────┘      └──────────────────┘         │
│           │                         │                   │
│           └────────┬────────────────┘                   │
│                    ↓                                    │
│          ┌──────────────────┐                           │
│          │    Mutation      │                           │
│          │    Pattern       │                           │
│          │                  │                           │
│          │  apply(ctx,      │                           │
│          │    resources)    │                           │
│          └──────────────────┘                           │
└─────────────────────────────────────────────────────────┘
```

## genericTaskHandler

The main orchestrator implementing the TaskHandler interface.

### TaskHandler Interface

The genericTaskHandler implements the TaskHandler interface, which provides two categories of operations:

**Configuration operations:**
- Configure function runtime for executing KRM functions
- Set runner options resolver for namespace-specific function runner configuration
- Configure repository opener for accessing repositories through cache
- Set credential resolver for authentication to upstream repositories
- Configure reference resolver for resolving package references
- Set retry attempts for repository operations

**Core operations:**
- **ApplyTask**: Executes task during package revision creation (receives draft, repository, package revision, and package config)
- **DoPRMutations**: Applies mutations during package revision updates (receives repository package revision, old and new objects, and draft)
- **DoPRResourceMutations**: Handles resource mutations during resource updates (receives package revision, draft, old and new resources; returns RenderStatus)

All core operations return errors on failure, and DoPRResourceMutations additionally returns RenderStatus containing function execution results.

## ApplyTask - Creation Orchestration

Orchestrates task execution during package revision creation.

### ApplyTask Flow

```
     Engine
        ↓
  ApplyTask(draft, repo, pr, config)
        ↓
genericTaskHandler
        ↓
  Validate Task Count (must be 1)
        ↓
  Map Task to Mutation
        ↓
  Execute Mutation
        ↓
  Execute Render
        ↓
  Update Draft Resources
        ↓
  Return to Engine
```

**Process:**
1. **Validate task count**: Must be exactly 1 task
2. **Map task to mutation**: Create mutation implementation based on task type
3. **Execute mutation**: Apply task transformation
4. **Execute render**: Run function pipeline
5. **Update draft resources**: Write transformed resources to draft
6. **Return**: Success or error to Engine

**Task mapping:**
```
Task Type → Mutation Implementation
    ↓
  init    → initPackageMutation
  clone   → clonePackageMutation
  edit    → editPackageMutation
  upgrade → upgradePackageMutation
```

**Render after task:**
- Always executed after task completion
- Uses namespace from draft metadata
- Applies Kptfile function pipeline
- Errors returned to Engine

### Task-to-Mutation Mapping

```
mapTaskToMutation(obj, task, isDeployment, packageConfig)
        ↓
  Check Task Type
        ↓
  ┌────┴────┬────────┬─────────┬─────────┐
  ↓         ↓        ↓         ↓         ↓
Init     Clone    Edit    Upgrade   Unknown
  ↓         ↓        ↓         ↓         ↓
Create   Create   Create   Create    Error
Mutation Mutation Mutation Mutation
  ↓         ↓        ↓         ↓
  └────┬────┴────────┴─────────┘
       ↓
  Return Mutation
```

**Mapping process:**
1. **Check task type** from task spec
2. **Validate task configuration**: Ensure required fields present
3. **Create mutation implementation**: Instantiate appropriate mutation
4. **Configure mutation**: Pass dependencies and parameters
5. **Return mutation**: Ready for execution

**Task validation:**
- **Init**: task.Init must be set
- **Clone**: task.Clone must be set
- **Edit**: task.Edit must be set
- **Upgrade**: task.Upgrade must be set
- **Unknown**: Return error

**Mutation configuration:**
- Pass task handler dependencies (repo opener, resolvers)
- Pass task-specific parameters (package name, namespace, etc.)
- Pass package configuration (for clone deployment packages)

## DoPRMutations - Update Orchestration

Orchestrates mutations during package revision updates.

### DoPRMutations Flow

```
     Engine
        ↓
  DoPRMutations(repoPR, oldObj, newObj, draft)
        ↓
genericTaskHandler
        ↓
  Check Lifecycle (Draft only)
        ↓
  Get Current Resources
        ↓
  Patch Kptfile
        ↓
  • Update readiness gates
  • Update conditions
        ↓
  Execute Render
        ↓
  Update Draft Resources
        ↓
  Return to Engine
```

**Process:**
1. **Check lifecycle**: Only process if Draft (skip for Proposed/Published)
2. **Get current resources**: Fetch package resources from repository
3. **Patch Kptfile**: Update readiness gates and conditions
4. **Execute render**: Run function pipeline
5. **Update draft resources**: Write transformed resources to draft
6. **Return**: Success or error to Engine

**Lifecycle check:**
- Only Draft packages are processed
- Proposed/Published packages skip mutations (metadata-only updates)
- Returns immediately for non-Draft packages

**Kptfile patching:**
```
  patchKptfile(ctx, repoPR, newObj)
        ↓
  Get Current Kptfile
        ↓
  Extract Readiness Gates from newObj
        ↓
  Extract Conditions from newObj
        ↓
  Update Kptfile.Info.ReadinessGates
        ↓
  Update Kptfile.Status.Conditions
        ↓
  Return Patched Kptfile
```

**Kptfile updates:**
- **ReadinessGates**: Copied from PackageRevision spec
- **Conditions**: Copied from PackageRevision status
- **Marshalled**: Converted to YAML and written to resources

## DoPRResourceMutations - Resource Update Orchestration

Orchestrates resource mutations during package resource updates.

### DoPRResourceMutations Flow

```
Engine
        ↓
  DoPRResourceMutations(pr, draft, oldRes, newRes)
        ↓
genericTaskHandler
        ↓
  Create Replace Mutation
        ↓
  Get Current Resources
        ↓
  Execute Replace Mutation
        ↓
  Execute Render
        ↓
  Capture RenderStatus
        ↓
  Update Draft Resources
        ↓
  Return RenderStatus to Engine
```

**Process:**
1. **Create replace mutation**: Instantiate replaceResourcesMutation
2. **Get current resources**: Fetch package resources from repository
3. **Execute replace mutation**: Replace resources with healing
4. **Execute render**: Run function pipeline
5. **Capture RenderStatus**: Store render results
6. **Update draft resources**: Write transformed resources to draft
7. **Return**: RenderStatus to Engine

**Replace mutation:**
- Replaces package resources with new content
- Heals comments from old resources
- Preserves user documentation

**Render execution:**
- Always executed after resource replacement
- Validates new resources
- Transforms resources through function pipeline

**RenderStatus handling:**
```
Execute Render
        ↓
  Success? ──Yes──> RenderStatus with results
        │
        No
        ↓
  RenderStatus with error
        ↓
  Return Error + RenderStatus
```

**RenderStatus characteristics:**
- Always returned (even on error)
- Contains function execution results
- Contains error message if render failed
- Used by Engine to populate API response

## Mutation Pattern

All package transformations follow a common mutation pattern.

### Mutation Interface

All package transformations implement a common mutation interface with a single apply operation:

**Interface contract:**
- **Input**: Accepts current package resources (may be empty for init/clone)
- **Output**: Returns transformed package resources and task result
- **Error**: Returns error if transformation fails
- **Stateless**: Pure transformation with no side effects

**Mutation implementations:**
- `initPackageMutation`
- `clonePackageMutation`
- `editPackageMutation`
- `upgradePackageMutation`
- `renderPackageMutation`
- `replaceResourcesMutation`
- `builtinEvalMutation`

### Mutation Execution Pattern

```
Create Mutation
        ↓
  Configure Dependencies
        ↓
  Execute mutation.apply()
        ↓
  • Transform resources
  • Return results
        ↓
  Check Error
        ↓
  Error? ──Yes──> Return Error
        │
        No
        ↓
  Continue Workflow
```

**Execution characteristics:**
- **Stateless**: No side effects, pure transformation
- **Composable**: Mutations can be chained
- **Isolated**: Each mutation independent
- **Testable**: Easy to unit test

### Mutation Chaining

Mutations are chained in sequence:

```
ApplyTask:
  Task Mutation → Render Mutation → Return

DoPRMutations:
  Kptfile Patch → Render Mutation → Return

DoPRResourceMutations:
  Replace Mutation → Render Mutation → Return
```

**Chaining pattern:**
1. **Execute first mutation**: Transform resources
2. **Pass output to next mutation**: Chain transformations
3. **Execute final mutation**: Complete transformation
4. **Return final result**: To Engine

**Error handling:**
- First error stops chain
- No partial transformations applied
- Error returned to Engine

## Render Coordination

The task handler coordinates render execution across all operations.

### Render Mutation

```
renderMutation(namespace)
        ↓
  Create renderPackageMutation
        ↓
  Configure:
        ↓
  • RunnerOptions (from namespace)
  • FunctionRuntime
        ↓
  Return Mutation
```

**Render mutation creation:**
- Namespace-specific runner options
- Function runtime from task handler
- Ready for execution

**Render execution:**
- Write resources to in-memory filesystem
- Execute kpt renderer
- Read rendered resources
- Return resources + RenderStatus

### Render Error Handling

```
Render Execution
        ↓
  Error? ──No──> Return Success
        │
       Yes
        ↓
  Wrap Error with Context
        ↓
  Return renderError(err)
```

**Error wrapping:**

Render errors are wrapped with context explaining that the package was not pushed to the remote repository and instructing the user to fix the package locally until render succeeds before retrying. The original error details are included in the wrapped error message.

**Error message characteristics:**
- Clear indication of render failure
- Instructs user to fix locally
- Explains package not pushed
- Includes original error details

## Configuration and Dependencies

The task handler is configured during initialization.

### Dependency Injection

```
Engine Initialization
        ↓
  Create TaskHandler
        ↓
  Configure Dependencies:
        ↓
  • SetRuntime(functionRuntime)
  • SetRunnerOptionsResolver(resolver)
  • SetRepoOpener(repoOpener)
  • SetCredentialResolver(credResolver)
  • SetReferenceResolver(refResolver)
  • SetRepoOperationRetryAttempts(attempts)
        ↓
  Return Configured TaskHandler
```

**Configuration methods:**
- **SetRuntime**: Function runtime for KRM functions
- **SetRunnerOptionsResolver**: Namespace-specific runner config
- **SetRepoOpener**: Opens repositories through cache
- **SetCredentialResolver**: Resolves authentication credentials
- **SetReferenceResolver**: Resolves package references
- **SetRepoOperationRetryAttempts**: Retry count for repository operations

**Dependency usage:**
- **Function runtime**: Used by render mutation
- **Runner options**: Used by render mutation (namespace-specific)
- **Repo opener**: Used by clone, edit, upgrade mutations
- **Credential resolver**: Used by clone mutation (Git authentication)
- **Reference resolver**: Used by clone, edit, upgrade mutations
- **Retry attempts**: Used by clone mutation (Git operations)

### Default Task Handler

```
GetDefaultTaskHandler()
        ↓
  Create genericTaskHandler
        ↓
  Return Unconfigured Handler
        ↓
  Engine Configures
```

**Factory function:**
- Returns new genericTaskHandler instance
- No dependencies configured
- Engine configures after creation

## Error Handling

The task handler handles errors at multiple levels.

### Task Execution Errors

```
Task Execution
        ↓
  Error? ──No──> Continue
        │
       Yes
        ↓
  Return Error
        ↓
  Engine
        ↓
  Rollback Draft
```

**Error types:**
- Task validation errors (invalid configuration)
- Upstream fetch errors (package not found)
- Merge errors (upgrade conflicts)
- Resource errors (invalid YAML)

**Error handling:**
- Errors returned immediately
- Engine triggers rollback
- No partial package created

### Render Errors

```
Render Execution
        ↓
  Error? ──No──> Continue
        │
       Yes
        ↓
  Capture in RenderStatus
        ↓
  Return Error + RenderStatus
        ↓
  Engine
        ↓
  Return Error to Client
        ↓
  Draft NOT Closed
```

**Error handling:**
- Render errors captured in RenderStatus
- Error returned with status
- Engine returns error to client
- Draft not closed (package not pushed)

**Rationale:**
- Render errors indicate invalid package
- Client must fix locally
- Package should not be pushed with errors

## Orchestration Patterns

The task handler uses specific orchestration patterns.

### Sequential Execution

```
Operation Request
        ↓
  Step 1: Task/Mutation
        ↓
  Success? ──No──> Return Error
        │
       Yes
        ↓
  Step 2: Render
        ↓
  Success? ──No──> Return Error
        │
       Yes
        ↓
  Step 3: Update Draft
        ↓
  Return Success
```

**Sequential pattern:**
- Steps executed in order
- Each step must succeed
- First error stops execution
- No parallel execution

### Mutation Composition

```
Mutation 1
        ↓
  Output Resources
        ↓
Mutation 2
        ↓
  Output Resources
        ↓
Mutation N
        ↓
  Final Resources
```

**Composition pattern:**
- Mutations chained together
- Output of one is input to next
- Composable transformations
- Functional programming style

### Dependency Injection

```
Engine
        ↓
  Configure TaskHandler
        ↓
  • Inject dependencies
  • Set configuration
        ↓
TaskHandler
        ↓
  Use Dependencies
        ↓
  • Open repositories
  • Resolve credentials
  • Execute functions
```

**Injection pattern:**
- Dependencies injected at initialization
- Task handler doesn't create dependencies
- Testable with mock dependencies
- Flexible configuration
