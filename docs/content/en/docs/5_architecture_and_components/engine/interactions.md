---
title: "Interactions"
type: docs
weight: 3
description: How the Engine interacts with other components
---

This section describes how the Engine interacts with other Porch components, what it depends on, and what depends on it.

## Component Interaction Overview

```
                    ┌──────────────────────────────────────┐
                    │          API Server                  │
                    │  - Receives HTTP requests            │
                    │  - Authenticates/authorizes          │
                    │  - Calls Engine operations           │
                    └──────────────┬───────────────────────┘
                                   │
                                   │ CaDEngine interface
                                   ▼
                    ┌──────────────────────────────────────┐
                    │            Engine                    │
                    │  - Orchestrates operations           │
                    │  - Enforces business rules           │
                    │  - Manages lifecycle                 │
                    └──────────────┬───────────────────────┘
                                   │
                ┌──────────────────┼──────────────────┐
                │                  │                  │
                ▼                  ▼                  ▼
        ┌───────────────┐  ┌──────────────┐  ┌──────────────┐
        │     Cache     │  │Task Handler  │  │   Watcher    │
        │               │  │              │  │   Manager    │
        └───────┬───────┘  └──────┬───────┘  └──────────────┘
                │                 │
                │                 │
                ▼                 ▼
        ┌───────────────┐  ┌──────────────┐
        │  Repository   │  │   Function   │
        │   Adapters    │  │   Runtime    │
        │  (Git/OCI)    │  │              │
        └───────────────┘  └──────┬───────┘
                                  │
                                  ▼
                           ┌──────────────┐
                           │  Function    │
                           │   Runner     │
                           │  (gRPC)      │
                           └──────────────┘
```

## Dependencies (What the Engine Needs)

### 1. Cache

**Interface**: `cachetypes.Cache`

**What Engine uses it for**:
- Opening repositories
- Accessing cached package data
- Avoiding repeated Git/OCI operations

**Key operations**:
```go
// Open a repository
repo, err := engine.cache.OpenRepository(ctx, repositorySpec)

// Repository then provides:
// - ListPackageRevisions()
// - CreatePackageRevisionDraft()
// - UpdatePackageRevision()
// - DeletePackageRevision()
// - ClosePackageRevisionDraft()
```

**Interaction pattern**:
1. Engine receives operation request
2. Engine calls `cache.OpenRepository()`
3. Cache returns Repository interface
4. Engine calls operations on Repository
5. Repository handles Git/OCI operations
6. Results flow back to Engine

**Why this design**:
- Cache abstracts storage details (CR vs DB)
- Repository abstracts repo type (Git vs OCI)
- Engine doesn't need to know about Git commands or OCI APIs

**Configuration**: Injected via `WithCache()` option

### 2. Task Handler

**Interface**: `task.TaskHandler`

**What Engine uses it for**:
- Executing package tasks (init, clone, edit, update, render)
- Applying mutations to package content
- Coordinating with Function Runtime

**Key operations**:
```go
// Apply task during package creation
err := engine.taskHandler.ApplyTask(ctx, draft, repositoryObj, packageRevision, packageConfig)

// Apply mutations during package update
err := engine.taskHandler.DoPRMutations(ctx, repoPr, oldObj, newObj, draft)

// Apply resource mutations
renderStatus, err := engine.taskHandler.DoPRResourceMutations(ctx, pr, draft, oldRes, newRes)
```

**Interaction pattern**:
1. Engine creates draft package
2. Engine calls Task Handler with task specification
3. Task Handler executes task (init/clone/edit/update)
4. Task Handler may call Function Runtime for rendering
5. Task Handler updates draft with results
6. Engine closes draft (commits)

**Task types handled**:
- **Init**: Create new empty package
- **Clone**: Copy package from upstream
- **Edit**: Modify package content
- **Update**: Merge upstream changes
- **Patch**: Apply patches to resources
- **Eval**: Execute function pipeline

**Configuration**: Created automatically, configured via Engine options

### 3. Function Runtime

**Interface**: `fn.FunctionRuntime`

**What Engine uses it for**:
- Executing KRM functions to transform packages
- Rendering packages (applying function pipeline)

**Key operations**:
```go
// Get runner for specific function
runner, err := runtime.GetRunner(ctx, function)

// Execute function
err := runner.Run(inputReader, outputWriter)
```

**Interaction pattern**:
1. Task Handler needs to execute function
2. Task Handler asks Function Runtime for runner
3. Runtime returns appropriate runner (built-in or gRPC)
4. Task Handler executes function via runner
5. Function transforms package resources
6. Results returned to Task Handler
7. Task Handler updates package

**Runtime types**:

**Built-in Runtime**:
- Executes functions directly in Engine process
- Fast, no network overhead
- Limited to pre-compiled functions (apply-replacements, set-namespace, starlark)

**gRPC Runtime**:
- Delegates to Function Runner service
- Spawns function pods
- Supports any function image
- Network overhead

**Multi-Runtime**:
- Tries built-in first
- Falls back to gRPC if not found
- Best of both worlds

**Configuration**: Injected via `WithBuiltinFunctionRuntime()`, `WithGRPCFunctionRuntime()`, or `WithFunctionRuntime()` options

### 4. Watcher Manager

**Interface**: `WatcherManager`

**What Engine uses it for**:
- Notifying clients of package changes
- Managing watch registrations
- Providing real-time updates

**Key operations**:
```go
// Notify watchers of change
sent := engine.watcherManager.NotifyPackageRevisionChange(watch.Modified, packageRevision)
```

**Interaction pattern**:
1. Client registers watch via API Server
2. API Server calls Engine's WatcherManager
3. WatcherManager stores watcher
4. Engine performs operation (create/update/delete)
5. Engine calls `NotifyPackageRevisionChange()`
6. WatcherManager sends events to matching watchers
7. Clients receive real-time notifications

**Event types**:
- **ADDED**: Package created
- **MODIFIED**: Package updated
- **DELETED**: Package deleted

**Configuration**: Injected via `WithWatcherManager()` option

### 5. Credential Resolver

**Interface**: `repository.CredentialResolver`

**What Engine uses it for**:
- Resolving Git/OCI credentials
- Authenticating to private repositories

**Key operations**:
```go
// Resolve credentials for repository
credentials, err := resolver.ResolveCredential(ctx, namespace, secretRef)
```

**Interaction pattern**:
1. Task Handler needs to access private repository
2. Task Handler asks Credential Resolver for credentials
3. Resolver fetches credentials from Kubernetes Secrets
4. Credentials passed to Repository Adapter
5. Repository Adapter uses credentials for Git/OCI operations

**Credential types**:
- Basic auth (username/password)
- Bearer token
- GCP Workload Identity

**Configuration**: Injected via `WithCredentialResolver()` option

### 6. Reference Resolver

**Interface**: `repository.ReferenceResolver`

**What Engine uses it for**:
- Resolving package references for clone/upgrade
- Finding upstream packages

**Key operations**:
```go
// Resolve package reference
packageRevision, err := resolver.ResolveReference(ctx, namespace, ref)
```

**Interaction pattern**:
1. User specifies upstream package reference in clone/upgrade task
2. Task Handler asks Reference Resolver to resolve reference
3. Resolver finds the referenced package
4. Task Handler uses resolved package as source

**Configuration**: Injected via `WithReferenceResolver()` option

### 7. User Info Provider

**Interface**: `repository.UserInfoProvider`

**What Engine uses it for**:
- Getting current user information
- Recording who performed operations

**Key operations**:
```go
// Get user info from context
userInfo, err := provider.GetUserInfo(ctx)
```

**Interaction pattern**:
1. Engine receives operation request with context
2. Engine asks User Info Provider for user details
3. Provider extracts user from Kubernetes authentication
4. User info recorded in Git commits, audit logs

**Configuration**: Injected via `WithUserInfoProvider()` option

## Dependents (What Needs the Engine)

### 1. API Server

**Component**: `pkg/registry/porch/`

**How it uses Engine**:
- Delegates all package operations to Engine
- Translates HTTP requests to Engine calls
- Translates Engine responses to HTTP responses

**Operations delegated**:
```go
// Create PackageRevision
repoPkgRev, err := apiServer.cad.CreatePackageRevision(ctx, repositoryObj, newPr, parent)

// Update PackageRevision
repoPkgRev, err := apiServer.cad.UpdatePackageRevision(ctx, version, repositoryObj, repoPr, oldObj, newObj, parent)

// Delete PackageRevision
err := apiServer.cad.DeletePackageRevision(ctx, repositoryObj, pr2Del)

// List PackageRevisions
revs, err := apiServer.cad.ListPackageRevisions(ctx, repositorySpec, filter)

// Update PackageRevisionResources
repoPkgRev, renderStatus, err := apiServer.cad.UpdatePackageResources(ctx, repositoryObj, pr2Update, oldRes, newRes)
```

**Why this design**:
- API Server focuses on HTTP/Kubernetes concerns
- Engine focuses on business logic
- Clear separation of concerns
- Easy to test independently

### 2. Controllers

**Components**: PackageVariant Controller, PackageVariantSet Controller

**How they use Engine**:
- Controllers don't directly use Engine
- Controllers create/update PackageRevision resources via Kubernetes API
- API Server delegates to Engine
- Indirect usage through Kubernetes API

**Pattern**:
```
Controller → Kubernetes API → API Server → Engine
```

**Why this design**:
- Controllers are standard Kubernetes controllers
- They work with Kubernetes resources
- Don't need to know about Engine internals

### 3. CLI (porchctl)

**Component**: `pkg/cli/commands/`

**How it uses Engine**:
- CLI doesn't directly use Engine
- CLI creates/updates resources via Kubernetes API
- API Server delegates to Engine
- Indirect usage through Kubernetes API

**Pattern**:
```
porchctl → Kubernetes API → API Server → Engine
```

**Why this design**:
- CLI is a standard Kubernetes client
- Works with any Kubernetes cluster running Porch
- Don't need to link Engine code into CLI

## Interaction Sequences

### Creating a Package

```
User/Client
    │
    │ kubectl apply -f packagerevision.yaml
    ▼
API Server
    │
    │ packageRevisions.Create()
    ▼
Engine
    │
    ├─▶ Cache.OpenRepository()
    │       │
    │       └─▶ Returns Repository
    │
    ├─▶ Repository.CreatePackageRevisionDraft()
    │       │
    │       └─▶ Returns Draft
    │
    ├─▶ TaskHandler.ApplyTask()
    │       │
    │       ├─▶ FunctionRuntime.GetRunner()
    │       │       │
    │       │       └─▶ Returns Runner (built-in or gRPC)
    │       │
    │       ├─▶ Runner.Run()
    │       │       │
    │       │       └─▶ Executes function
    │       │
    │       └─▶ Updates Draft
    │
    ├─▶ Draft.UpdateLifecycle()
    │
    ├─▶ Repository.ClosePackageRevisionDraft()
    │       │
    │       └─▶ Commits to Git/OCI
    │
    ├─▶ WatcherManager.NotifyPackageRevisionChange()
    │       │
    │       └─▶ Sends ADDED events to watchers
    │
    └─▶ Returns PackageRevision
        │
        ▼
API Server
    │
    │ Returns HTTP 201 Created
    ▼
User/Client
```

### Updating Package Resources

```
User/Client
    │
    │ kubectl apply -f packagerevisionresources.yaml
    ▼
API Server
    │
    │ packageRevisionResources.Update()
    ▼
Engine
    │
    ├─▶ Cache.OpenRepository()
    │       │
    │       └─▶ Returns Repository
    │
    ├─▶ Repository.UpdatePackageRevision()
    │       │
    │       └─▶ Returns Draft
    │
    ├─▶ TaskHandler.DoPRResourceMutations()
    │       │
    │       ├─▶ Applies resource changes
    │       │
    │       ├─▶ FunctionRuntime.GetRunner()
    │       │       │
    │       │       └─▶ Returns Runner
    │       │
    │       ├─▶ Runner.Run()
    │       │       │
    │       │       └─▶ Renders package
    │       │
    │       └─▶ Returns RenderStatus
    │
    ├─▶ Repository.ClosePackageRevisionDraft()
    │       │
    │       └─▶ Commits to Git/OCI
    │
    └─▶ Returns PackageRevision + RenderStatus
        │
        ▼
API Server
    │
    │ Returns HTTP 200 OK
    ▼
User/Client
```

## Configuration and Initialization

The Engine is configured via functional options during initialization:

```go
// In API Server startup
engine, err := engine.NewCaDEngine(
    // Cache for repository access
    engine.WithCache(cache),
    
    // Function runtimes
    engine.WithBuiltinFunctionRuntime(imagePrefix),
    engine.WithGRPCFunctionRuntime(grpcOptions),
    
    // Credential resolution
    engine.WithCredentialResolver(credentialResolver),
    
    // Reference resolution
    engine.WithReferenceResolver(referenceResolver),
    
    // User info
    engine.WithUserInfoProvider(userInfoProvider),
    
    // Watch management
    engine.WithWatcherManager(watcherManager),
    
    // Retry configuration
    engine.WithRepoOperationRetryAttempts(retryAttempts),
)
```

**Why functional options**:
- Flexible configuration
- Easy to add new dependencies
- Clear what's being configured
- Testable (can inject mocks)

## Summary

The Engine interacts with:

**Dependencies (what it needs)**:
- **Cache**: Repository access and caching
- **Task Handler**: Package operation execution
- **Function Runtime**: KRM function execution
- **Watcher Manager**: Real-time notifications
- **Credential Resolver**: Authentication
- **Reference Resolver**: Package reference resolution
- **User Info Provider**: User identification

**Dependents (what needs it)**:
- **API Server**: Delegates all operations
- **Controllers**: Indirect via Kubernetes API
- **CLI**: Indirect via Kubernetes API

**Interaction patterns**:
- Engine orchestrates, components execute
- Clear interfaces between components
- Dependency injection for flexibility
- Separation of concerns for maintainability

The Engine sits at the center of Porch's architecture, coordinating all components to provide reliable, consistent package lifecycle management.
