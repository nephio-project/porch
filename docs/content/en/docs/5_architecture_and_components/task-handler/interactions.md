---
title: "Task Handler Interactions"
type: docs
weight: 4
description: |
  How the Task Handler integrates with other components.
---

## Engine Invocation

The Task Handler is invoked by the Engine through three main integration points that correspond to different package operations.

### ApplyTask Integration

**When invoked:**
- During package revision creation (CreatePackageRevision)
- After draft created but before lifecycle update
- For all task types (init, clone, edit, upgrade)

**Invocation flow:**
```
Engine
      ↓
CreatePackageRevision
      ↓
Create Draft
      ↓
ApplyTask(draft, repo, pr, config)
      ↓
Task Handler
      ↓
Execute Task + Render
      ↓
Return Modified Draft
      ↓
Engine
      ↓
Update Lifecycle
      ↓
Close Draft
```

**Parameters passed:**
- **Draft**: Mutable workspace for modifications
- **Repository**: Repository CR specification for context
- **PackageRevision**: Contains task specification in Spec.Tasks
- **PackageConfig**: Package path, name, workspace, upstream reference

**Return values:**
- **Error**: If task execution or render fails
- **Success**: Draft modified in-place, no explicit return value

**Error handling:**
- Task errors trigger rollback in Engine
- Draft cleaned up automatically
- Error propagated to client
- No partial package revisions created

### DoPRMutations Integration

**When invoked:**
- During package revision updates (UpdatePackageRevision)
- After draft opened from existing package revision
- Only for Draft lifecycle packages (Proposed/Published skip)

**Invocation flow:**
```
Engine
      ↓
UpdatePackageRevision
      ↓
Open Draft from Existing
      ↓
DoPRMutations(repoPR, oldObj, newObj, draft)
      ↓
Task Handler
      ↓
Patch Kptfile + Render
      ↓
Return Modified Draft
      ↓
Engine
      ↓
Update Lifecycle
      ↓
Close Draft
```

**Parameters passed:**
- **Repository PackageRevision**: Current package revision from repository
- **Old PackageRevision**: Previous spec for comparison
- **New PackageRevision**: Desired spec with changes
- **Draft**: Mutable workspace already containing current content

**Return values:**
- **Error**: If Kptfile patching or render fails
- **Success**: Draft modified in-place

**Mutation operations:**
- Patch Kptfile with readiness gates and conditions
- Execute render pipeline
- No new tasks applied (tasks are append-only, handled separately)

### DoPRResourceMutations Integration

**When invoked:**
- During package resource updates (UpdatePackageResources)
- After draft opened from existing package revision
- Only for Draft lifecycle packages

**Invocation flow:**
```
Engine
      ↓
UpdatePackageResources
      ↓
Open Draft from Existing
      ↓
DoPRResourceMutations(pr, draft, oldRes, newRes)
      ↓
Task Handler
      ↓
Replace Resources + Render
      ↓
Return RenderStatus
      ↓
Engine
      ↓
Close Draft (no lifecycle change)
```

**Parameters passed:**
- **PackageRevision**: Current package revision for context
- **Draft**: Mutable workspace for modifications
- **Old PackageRevisionResources**: Previous resource content
- **New PackageRevisionResources**: Desired resource content

**Return values:**
- **RenderStatus**: Function execution results (always returned)
- **Error**: If resource replacement or render fails

**Special handling:**
- RenderStatus returned even on error
- Contains function execution details
- Used by Engine to populate API response
- Render errors don't prevent status return

### Integration Characteristics

**Synchronous execution:**
- All invocations are synchronous
- Task Handler blocks until completion
- Engine waits for return
- No async callbacks or events

**Draft-based workflow:**
- Engine provides draft workspace
- Task Handler modifies draft in-place
- Engine closes draft after success
- Draft cleanup on errors

**Error propagation:**
- Errors returned directly to Engine
- Engine handles rollback and cleanup
- Task Handler doesn't manage draft lifecycle
- Clear separation of concerns

## Function Runtime Integration

The Task Handler integrates with function runtimes to execute KRM functions during render operations.

### Function Runtime Configuration

**Runtime types:**
- **Builtin Runtime**: Executes built-in functions in-process
- **gRPC Runtime**: Calls external function runner service
- **Multi Runtime**: Chains builtin and gRPC runtimes together

**Configuration flow:**
```
Engine Initialization
      ↓
Create Task Handler
      ↓
SetRuntime(functionRuntime)
      ↓
Task Handler Configured
      ↓
Used for All Function Executions
```

**Runtime selection:**
- Configured at Porch server startup
- Single runtime used for all operations
- Cannot be changed without restart
- Passed from Engine during initialization

### Render Pipeline Execution

**Execution flow:**
```
Task Handler
      ↓
Create Renderer
      ↓
Write Resources to Filesystem
      ↓
Execute Render
      ↓
Function Runtime
      ↓
Read Kptfile Pipeline
      ↓
For Each Function:
      ↓
  Execute Function
      ↓
  Collect Results
      ↓
Return Results
      ↓
Task Handler
      ↓
Read Rendered Resources
      ↓
Return Resources + RenderStatus
```

**Function runtime responsibilities:**
- Read Kptfile pipeline configuration
- Execute mutator functions in sequence
- Execute validator functions in sequence
- Collect function results and errors
- Return overall execution status

**Task Handler responsibilities:**
- Prepare filesystem with package resources
- Create renderer with runtime configuration
- Invoke render operation
- Convert results to Porch API format
- Read rendered resources from filesystem

### Builtin Function Execution

**Execution flow:**
```
Task Handler
      ↓
Create Builtin Mutation
      ↓
Create KIO Pipeline
      ↓
Builtin Runtime
      ↓
Execute Function
      ↓
Transform Resources
      ↓
Return Transformed Resources
      ↓
Task Handler
      ↓
Continue Workflow
```

**Builtin runtime characteristics:**
- In-process execution (no external service)
- Fast and deterministic
- No pod startup overhead
- Used for package-context generator

### gRPC Runtime Integration

**When gRPC runtime configured:**
```
Task Handler
      ↓
Execute Render
      ↓
gRPC Runtime
      ↓
Call Function Runner Service
      ↓
Function Runner
      ↓
Create Function Pod
      ↓
Execute Function in Pod
      ↓
Return Results
      ↓
gRPC Runtime
      ↓
Return to Task Handler
```

**gRPC runtime characteristics:**
- External function runner service
- Pod-based function execution
- Image pull and caching
- Higher latency but more flexible

**For details on function runner service, see [Function Runner](../function-runner).**

### Runner Options Resolution

**Namespace-specific configuration:**
```
Task Handler
      ↓
Get Draft Namespace
      ↓
Resolve Runner Options
      ↓
RunnerOptionsResolver
      ↓
Return Namespace-Specific Config:
      ↓
  • Function runner endpoint
  • Image pull secrets
  • Resource limits
      ↓
Task Handler
      ↓
Create Renderer with Options
```

**Configuration includes:**
- Function runner service endpoint
- Image pull secrets for private registries
- Resource limits for function pods
- Timeout settings

## Package Resource Manipulation

The Task Handler manipulates package resources through the draft interface and filesystem operations.

### Draft Interface Usage

**Draft operations:**
- **UpdateResources**: Write transformed resources to draft
- **GetResources**: Read current resources from draft
- **GetPackageRevision**: Get package revision metadata
- **UpdateLifecycle**: Change lifecycle state (used by Engine, not Task Handler)

**Resource update flow:**
```
Task Handler
      ↓
Transform Resources
      ↓
draft.UpdateResources(resources)
      ↓
Draft
      ↓
Write to Repository Workspace
      ↓
Return Success
      ↓
Task Handler
      ↓
Continue Workflow
```

**Resource read flow:**
```
Task Handler
      ↓
draft.GetResources()
      ↓
Draft
      ↓
Read from Repository Workspace
      ↓
Return Resources
      ↓
Task Handler
      ↓
Process Resources
```

### Filesystem Operations

**In-memory filesystem:**
- Task Handler uses in-memory filesystem for transformations
- Resources written to virtual filesystem
- kpt operations performed on filesystem
- Resources read back after transformations

**Filesystem workflow:**
```
Task Handler
      ↓
Create In-Memory FS
      ↓
Write Resources to FS
      ↓
Perform Transformations:
      ↓
  • Init package
  • Clone from upstream
  • Merge changes
  • Execute functions
      ↓
Read Resources from FS
      ↓
Return Transformed Resources
```

**Benefits:**
- No disk I/O required
- Fast operations
- Isolated from other operations
- Easy cleanup

### Resource Format Handling

**Resource representations:**
- **API format**: Porch API types (PackageRevisionResources)
- **Repository format**: Map of filename to content
- **RNode format**: kyaml YAML nodes for manipulation
- **Filesystem format**: Files in directory structure

**Conversion flow:**
```
API Format (PackageRevisionResources)
      ↓
Repository Format (map[string]string)
      ↓
Filesystem Format (files in directory)
      ↓
RNode Format (YAML nodes)
      ↓
Transformations Applied
      ↓
RNode Format (transformed)
      ↓
Filesystem Format (updated files)
      ↓
Repository Format (map[string]string)
      ↓
API Format (PackageRevisionResources)
```

**Conversion responsibilities:**
- Task Handler converts between formats as needed
- Uses kyaml library for YAML manipulation
- Preserves comments during transformations
- Maintains file structure and organization

### Upstream Package Fetching

**PackageFetcher integration:**
```
Task Handler
      ↓
Clone/Edit/Upgrade Task
      ↓
Create PackageFetcher
      ↓
Fetch Upstream Package
      ↓
PackageFetcher
      ↓
Resolve Reference
      ↓
Open Repository (via Cache)
      ↓
Get Package Revision
      ↓
Return Package Revision
      ↓
Task Handler
      ↓
Extract Resources
      ↓
Continue Transformation
```

**PackageFetcher configuration:**
- **RepoOpener**: Opens repositories through cache
- **ReferenceResolver**: Resolves package references to repositories
- **CredentialResolver**: Resolves authentication credentials

**Used by:**
- **Clone task**: Fetch upstream package from registered repository
- **Edit task**: Fetch source package from same repository
- **Upgrade task**: Fetch three package revisions (old upstream, new upstream, local)

### Git Operations

**Git clone for upstream packages:**
```
Task Handler
      ↓
Clone Task (Git source)
      ↓
Create Temp Directory
      ↓
Open Git Repository
      ↓
Credential Resolver
      ↓
Resolve Git Credentials
      ↓
Git Operations
      ↓
Clone Repository
      ↓
Checkout Ref
      ↓
Extract Package
      ↓
Return Resources
      ↓
Task Handler
      ↓
Cleanup Temp Directory
```

**Git operation characteristics:**
- Temporary directory for clone
- Credential resolution for authentication
- Retry logic for transient failures
- Cleanup after extraction

### Comment Healing

**Preserving user comments:**
```
Task Handler
      ↓
Replace Resources Mutation
      ↓
Read Old Resources as RNodes
      ↓
Read New Resources as RNodes
      ↓
For Each Resource:
      ↓
  Match by namespace/name/kind/apiVersion
      ↓
  Copy Comments from Old to New
      ↓
  Skip ytt Templates (#@, #!)
      ↓
Write Healed Resources
      ↓
Return Updated Resources
```

**Comment healing characteristics:**
- Preserves user documentation
- Maintains inline comments
- Skips templated resources (ytt)
- Best-effort operation (doesn't fail on errors)
