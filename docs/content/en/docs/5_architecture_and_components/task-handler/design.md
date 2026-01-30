---
title: "Task Handler Design"
type: docs
weight: 2
description: |
  Architecture and design patterns of the Task Handler.
---

## Task Handler Architecture

The Task Handler follows a **mutation-based architecture** where all package transformations are implemented as mutations that follow a common interface pattern:

```
┌─────────────────────────────────────────┐
│          genericTaskHandler             │  ← Orchestrator
├─────────────────────────────────────────┤
│  ┌──────────┬──────────┬─────────────┐  │
│  │  Repo    │ Function │  Reference  │  │  ← Dependencies
│  │  Opener  │ Runtime  │  Resolver   │  │
│  └──────────┴──────────┴─────────────┘  │
├─────────────────────────────────────────┤
│         Mutation Implementations        │  ← Transformations
│  ┌──────────┬──────────┬─────────────┐  │
│  │   Init   │  Clone   │    Edit     │  │
│  ├──────────┼──────────┼─────────────┤  │
│  │ Upgrade  │  Render  │   Replace   │  │
│  └──────────┴──────────┴─────────────┘  │
└─────────────────────────────────────────┘
```

**Key architectural characteristics:**

### Stateless Transformations

All mutations are stateless transformations:
- Accept input resources (may be empty)
- Return transformed resources
- No side effects or persistent state
- Pure functions that can be tested in isolation

### Dependency Injection

The task handler uses dependency injection for all external dependencies:
- **Repository Opener**: Opens repositories through cache for fetching upstream packages
- **Function Runtime**: Executes KRM functions (builtin, gRPC, or multi-runtime)
- **Credential Resolver**: Resolves authentication for Git operations
- **Reference Resolver**: Resolves package references to repositories
- **Runner Options Resolver**: Provides namespace-specific function runner configuration

Dependencies are injected during initialization via setter methods, enabling:
- Testing with mock implementations
- Flexible runtime configuration
- Clear separation of concerns

### Composition Over Inheritance

The task handler composes mutations rather than using inheritance:
- Each mutation is independent
- Mutations can be chained together
- New mutations can be added without modifying existing code
- Follows functional programming principles

## Mutation Pattern

All package transformations implement a common mutation interface that defines a single operation.

### Mutation Interface Contract

The mutation interface defines the contract for all package transformations:

**Operation signature:**
- Accepts context for cancellation and tracing
- Accepts current package resources as input
- Returns transformed package resources
- Returns task result with metadata
- Returns error if transformation fails

**Characteristics:**
- **Stateless**: No internal state maintained between calls
- **Pure**: Same input produces same output
- **Composable**: Output of one mutation can be input to another
- **Testable**: Easy to unit test with mock inputs

### Mutation Implementations

The task handler provides several mutation implementations:

**Package creation mutations:**
- **initPackageMutation**: Creates new package from scratch with Kptfile and README
- **clonePackageMutation**: Copies package from upstream source (registered repo, Git, OCI)
- **editPackageMutation**: Creates new revision from existing published package

**Package update mutations:**
- **upgradePackageMutation**: Performs three-way merge to upgrade to new upstream version
- **replaceResourcesMutation**: Replaces package resources while preserving comments

**Function execution mutations:**
- **renderPackageMutation**: Executes KRM function pipeline from Kptfile
- **builtinEvalMutation**: Applies builtin functions (package-context generator)

### Mutation Chaining

Mutations are chained together to implement complex workflows:

```
ApplyTask Flow:
---------------------------------------------
Task Mutation (init/clone/edit/upgrade)
      ↓
Builtin Functions (if deployment package)
      ↓
Render Mutation (execute function pipeline)
      ↓
Return Transformed Resources
---------------------------------------------
```

**Chaining characteristics:**
- Sequential execution (one after another)
- Output of previous mutation becomes input to next
- First error stops the chain
- All-or-nothing semantics (no partial transformations)

### Mutation Factory Pattern

The task handler uses a factory pattern to create mutations:

**Task-to-mutation mapping:**
- Examines task type from PackageRevision spec
- Validates task configuration (required fields present)
- Creates appropriate mutation implementation
- Configures mutation with dependencies and parameters
- Returns ready-to-execute mutation

**Benefits:**
- Centralized mutation creation logic
- Consistent configuration across mutations
- Easy to add new mutation types
- Type-safe task handling

## Builtin Functions

Builtin functions are KRM functions that are automatically applied by Porch without being defined in the package's Kptfile.

### Package Context Generator

The primary builtin function is the **package-context generator**, which creates a ConfigMap containing package metadata for deployment packages.

**Purpose:**
- Provides package metadata to deployed resources
- Enables resources to know their package context
- Used by Nephio controllers for package management

**Generated content:**
- Package name
- Repository name
- Target namespace
- Deployment flag (whether package is deployable)

**When applied:**
- Automatically after clone task for deployment packages
- Not visible in task list (implicit operation)
- Applied before render pipeline execution

**Generated resource characteristics:**
- ConfigMap named "package-context"
- Marked as local-config (not deployed to cluster)
- Contains key-value pairs with package metadata
- Stored in package alongside other resources

### Builtin Function Execution

Builtin functions use the same mutation pattern as other transformations:

**Execution flow:**
1. Create function runner for builtin function
2. Create KIO pipeline (Reader → Filter → Writer)
3. Execute pipeline to transform resources
4. Return transformed resources

**KIO (Kubernetes I/O) pipeline:**
- **Reader**: Reads package resources as RNodes (YAML nodes)
- **Filter**: Executes function transformation on RNodes
- **Writer**: Writes transformed RNodes to output

**Integration with function runtime:**
- Uses builtin runtime (in-process execution)
- No external function runner service needed
- Fast execution with minimal overhead
- Deterministic behavior

### Builtin vs User-Defined Functions

**Builtin functions:**
- Applied automatically by Porch
- Not defined in Kptfile
- Executed before render pipeline
- In-process execution (fast)
- Deterministic and reliable

**User-defined functions:**
- Defined in Kptfile pipeline section
- Executed during render task
- May use external function runner (pods)
- User-controlled behavior
- Can fail validation

## Design Patterns

### Strategy Pattern

The task handler uses the strategy pattern for task execution:
- Different task types (init, clone, edit, upgrade) are different strategies
- genericTaskHandler selects strategy based on task type
- Each strategy implements the same mutation interface
- Strategies can be swapped without changing orchestrator

### Template Method Pattern

The orchestration methods follow a template method pattern:

**ApplyTask template:**
1. Validate task count
2. Map task to mutation
3. Execute mutation
4. Execute render
5. Update draft
6. Return result

**DoPRMutations template:**
1. Check lifecycle
2. Get current resources
3. Patch Kptfile
4. Execute render
5. Update draft
6. Return result

**DoPRResourceMutations template:**
1. Create replace mutation
2. Get current resources
3. Execute replace
4. Execute render
5. Capture status
6. Update draft
7. Return result

### Dependency Inversion Principle

The task handler depends on abstractions, not concrete implementations:
- Depends on Repository interface (not Git adapter)
- Depends on FunctionRuntime interface (not specific runtime)
- Depends on Resolver interfaces (not specific resolvers)
- Enables testing with mocks and flexible configuration

## Error Handling Strategy

### Task Execution Errors

**Immediate failure:**
- Task errors returned immediately to Engine
- Engine triggers rollback (draft cleanup)
- No partial package revisions created
- Clear error messages for debugging

**Error types:**
- Configuration errors (invalid task spec)
- Upstream fetch errors (package not found)
- Merge conflicts (upgrade failures)
- Resource errors (invalid YAML)

### Render Errors

**Deferred failure:**
- Render errors captured in RenderStatus
- Error returned with status to Engine
- Engine returns error to client
- Draft NOT closed (package not pushed)
- Client must fix locally and retry

**Error wrapping:**
- Render errors wrapped with context
- Explains package not pushed to remote
- Instructs user to fix locally
- Includes original error details

**Rationale:**
- Render errors indicate invalid package state
- Package should not be pushed with errors
- User must fix and verify locally
- Prevents broken packages in repository

### Builtin Function Errors

**Critical failure:**
- Builtin function errors are critical
- Returned immediately to Engine
- Trigger rollback and cleanup
- Indicate system issue, not user error

**Rationale:**
- Builtin functions required for package validity
- Errors indicate Porch system problem
- Should not occur in normal operation
- Require investigation and fix
