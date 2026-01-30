---
title: "Function Execution"
type: docs
weight: 2
description: |
  Detailed architecture of KRM function execution including render pipeline and builtin functions.
---

## Overview

The Task Handler executes KRM functions to validate and transform package resources. Function execution includes both the render pipeline (user-defined functions in Kptfile) and builtin functions (automatically applied by Porch).

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Function Execution System                  │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │  Render Pipeline │      │    Builtin       │         │
│  │                  │      │   Functions      │         │
│  │  • Read Kptfile  │      │                  │         │
│  │  • Execute Funcs │      │  • Package       │         │
│  │  • Capture       │      │    Context Gen   │         │
│  │    Results       │      │                  │         │
│  └──────────────────┘      └──────────────────┘         │
│           │                         │                   │
│           └────────┬────────────────┘                   │
│                    ↓                                    │
│          ┌──────────────────┐                           │
│          │    Function      │                           │
│          │    Runtime       │                           │
│          │                  │                           │
│          │  • Builtin       │                           │
│          │  • gRPC          │                           │
│          │  • Multi         │                           │
│          └──────────────────┘                           │
└─────────────────────────────────────────────────────────┘
```

## Render Pipeline

Executes the KRM function pipeline defined in the package's Kptfile.

### Render Flow

```
Render Request
        ↓
  Write Resources to FS
        ↓
  Find Package Path
        ↓
  Create Renderer
        ↓
  Execute Render
        ↓
  • Read Kptfile pipeline
  • Execute each function
  • Collect results
        ↓
  Convert Results
        ↓
  Read Rendered Resources
        ↓
  Return Resources + RenderStatus
```

**Process:**
1. **Write resources** to in-memory filesystem
2. **Find package path** (topmost directory with Kptfile)
3. **Create renderer** with runner options
4. **Execute render** through kpt renderer:
   - Read Kptfile pipeline configuration
   - Execute each function in sequence
   - Collect function results
5. **Convert results** to Porch API format
6. **Read rendered resources** from filesystem
7. **Return** rendered resources + RenderStatus

**Render parameters:**
- **Package path**: Directory containing Kptfile
- **Function runtime**: Builtin, gRPC, or multi-runtime
- **Runner options**: Namespace-specific configuration

### Kptfile Pipeline

The render pipeline is defined in the Kptfile:

```yaml
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: example-package
pipeline:
  mutators:
    - image: gcr.io/kpt-fn/set-namespace:v0.4.1
      configMap:
        namespace: prod
  validators:
    - image: gcr.io/kpt-fn/kubeval:v0.3.0
```

**Pipeline sections:**
- **mutators**: Functions that modify resources
- **validators**: Functions that validate resources (don't modify)

**Function execution order:**
1. All mutators executed in sequence
2. All validators executed in sequence
3. Execution stops on first error

### Function Runtime Integration

The render pipeline delegates to function runtime:

```
Render Pipeline
        ↓
  For Each Function
        ↓
  Function Runtime
        ↓
  ┌────┴────┬────────┬─────────┐
  ↓         ↓        ↓         ↓
Builtin   gRPC    Multi    (future)
  ↓         ↓        ↓
Execute   Call     Chain
Locally   Service  Runtimes
```

**Runtime types:**
- **Builtin Runtime**: Executes built-in functions locally
- **gRPC Runtime**: Calls external function runner service
- **Multi Runtime**: Chains multiple runtimes together

**Runtime selection:**
- Configured at Porch server startup
- Passed to task handler during initialization
- Used for all function executions

### RenderStatus

The render pipeline returns execution status containing:

**RenderStatus structure:**
- **Result**: Detailed results from each function execution
- **Err**: Overall error message if render failed

**Result list:**
- Contains per-function results
- Each result includes function image, exit code, and result items

**Result items:**
- **Info**: Informational messages
- **Warning**: Warning messages
- **Error**: Error messages (cause failure)

**Render success:**
- All functions executed successfully
- No error items in results
- Exit codes all 0

**Render failure:**
- Function execution error
- Error items in results
- Non-zero exit codes

## Builtin Functions

Builtin functions are automatically applied by Porch without being defined in Kptfile.

### Package Context Generator

Generates package-context.yaml for deployment packages.

```
Package Context Generation
        ↓
  Create PackageConfig
        ↓
  • Name, Repository, Namespace
  • Deployment flag
        ↓
  Execute Builtin Function
        ↓
  Generate package-context.yaml
        ↓
  Add to Package Resources
        ↓
  Return Updated Resources
```

**Process:**
1. **Create PackageConfig** with package metadata
2. **Execute builtin function** (PackageContextGenerator)
3. **Generate package-context.yaml** with:
   - Package name
   - Repository name
   - Namespace
   - Deployment flag
4. **Add to package resources**
5. **Return** updated resources

**PackageConfig structure:**
- **Name**: Package name
- **Repository**: Repository name
- **Namespace**: Target namespace
- **Deployment**: Whether package is deployable

**Generated package-context.yaml example:**
- ConfigMap with package metadata
- Marked as local-config (not deployed)
- Contains package name, repository, namespace, deployment flag

**When applied:**
- After clone task (if deployment package)
- Automatically by task handler
- Not visible in task list

### Builtin Function Execution

Builtin functions use the mutation pattern:

```
Builtin Function Request
        ↓
  Create Function Runner
        ↓
  Create KIO Pipeline
        ↓
  • Reader: Package resources
  • Filter: Function runner
  • Writer: Output resources
        ↓
  Execute Pipeline
        ↓
  Return Transformed Resources
```

**Process:**
1. **Create function runner** (PackageContextGenerator)
2. **Create KIO pipeline**:
   - Reader: Reads package resources as RNodes
   - Filter: Executes function on RNodes
   - Writer: Writes transformed RNodes to output
3. **Execute pipeline**
4. **Return** transformed resources

**KIO (Kubernetes I/O):**
- kyaml library for YAML manipulation
- Pipeline pattern for resource transformations
- Reader → Filter → Writer flow

## Function Execution Timing

Functions are executed at specific points in the workflow:

### During Package Creation

```
ApplyTask
        ↓
  Execute Task (init/clone/edit/upgrade)
        ↓
  Apply Builtin Functions
        ↓
  Execute Render Pipeline
        ↓
  Return Modified Draft
```

**Timing:**
1. **Task execution**: Init, clone, edit, or upgrade
2. **Builtin functions**: Package context generation (if deployment)
3. **Render pipeline**: Execute Kptfile functions
4. **Return**: Modified draft to Engine

### During Package Update

```
DoPRMutations
        ↓
  Patch Kptfile
        ↓
  Execute Render Pipeline
        ↓
  Return Modified Draft
```

**Timing:**
1. **Patch Kptfile**: Update readiness gates and conditions
2. **Render pipeline**: Execute Kptfile functions
3. **Return**: Modified draft to Engine

### During Resource Update

```
DoPRResourceMutations
        ↓
  Replace Resources
        ↓
  Execute Render Pipeline
        ↓
  Return Modified Draft + RenderStatus
```

**Timing:**
1. **Replace resources**: Update package resources
2. **Render pipeline**: Execute Kptfile functions
3. **Return**: Modified draft + RenderStatus to Engine

## Error Handling

Function execution errors are handled differently than task errors:

### Render Errors

```
Render Execution
        ↓
  Error? ──No──> Return Success
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
- Error returned to Engine
- Engine returns error to client
- Draft NOT closed (package not pushed)
- Client can fix and retry

**Error message:**
- "Error rendering package in kpt function pipeline. Package NOT pushed to remote. Fix locally (until 'kpt fn render' succeeds) and retry. Details: {error}"

**Rationale:**
- Render errors indicate invalid package state
- Package should not be pushed with render errors
- Client must fix locally and retry

### Builtin Function Errors

```
Builtin Function Execution
        ↓
  Error? ──No──> Return Success
        │
       Yes
        ↓
  Return Error
        ↓
  Task Handler
        ↓
  Return Error to Engine
        ↓
  Engine Rollback
```

**Error handling:**
- Builtin function errors returned immediately
- Task handler returns error to Engine
- Engine triggers rollback
- No partial package created

**Rationale:**
- Builtin functions are required for package validity
- Errors indicate system issue, not user error
- Rollback ensures consistency

## Function Execution Characteristics

### Idempotency

**Render pipeline:**
- Should be idempotent (functions should produce same output for same input)
- Not enforced by Porch
- Function author responsibility

**Builtin functions:**
- Idempotent by design
- Safe to execute multiple times

### Isolation

**Function execution:**
- Functions execute in isolated environments (pods, containers)
- No access to Porch internals
- Only see package resources

**Resource access:**
- Functions receive package resources as input
- Functions return modified resources as output
- No direct filesystem or network access (except function runner)

### Performance

**Render pipeline:**
- Can be slow (function pod startup, image pull)
- Executed on every package modification
- Cached function images improve performance

**Builtin functions:**
- Fast (in-process execution)
- No external dependencies
- Minimal overhead

## Function Runtime Configuration

Function runtime is configured at Porch server startup:

### Runtime Options

**RunnerOptions:**
- Namespace-specific configuration
- Function runner service endpoint
- Image pull secrets
- Resource limits

**Runtime types:**
- **Builtin**: For built-in functions only
- **gRPC**: For external function runner service
- **Multi**: Chains builtin + gRPC

**Configuration:**
- Set during task handler initialization
- Passed from Engine
- Used for all function executions

### Function Runner Service

For gRPC runtime:

```
Task Handler
        ↓
  gRPC Runtime
        ↓
  Function Runner Service
        ↓
  • Pod-based execution
  • Image caching
  • Resource management
```

**Function runner responsibilities:**
- Pull function images
- Execute functions in pods
- Manage pod lifecycle
- Return function results

**For details on function runner, see [Function Runner](../../function-runner).**
