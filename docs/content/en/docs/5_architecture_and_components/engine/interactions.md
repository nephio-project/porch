---
title: "Interactions"
type: docs
weight: 3
description: How the Engine interacts with other components
---

This section describes how the Engine interacts with other Porch components, what it depends on, and what depends on it.

## Component Interaction Overview

![Engine Architecture](/static/images/porch/engine-component-interaction.drawio.svg)

## Dependencies (What the Engine Needs)

### 1. Cache

**Purpose**: Repository access and caching

**What Engine uses it for**:
- Opening repositories
- Accessing cached package data
- Avoiding repeated Git/OCI operations

**Key operations**:
- Open a repository by specification
- Repository provides list, create, update, delete operations
- Repository handles Git/OCI operations transparently

**Interaction pattern**:
1. Engine receives operation request
2. Engine opens repository via cache
3. Cache returns repository interface
4. Engine calls operations on repository
5. Repository handles Git/OCI operations
6. Results flow back to Engine

**Why this design**:
- Cache abstracts storage details (CR vs DB)
- Repository abstracts repo type (Git vs OCI)
- Engine doesn't need to know about Git commands or OCI APIs

### 2. Task Handler

**Purpose**: Package task execution

**What Engine uses it for**:
- Executing package tasks (init, clone, edit, update, render)
- Applying mutations to package content
- Coordinating with Function Runtime

**Key operations**:
- Apply task during package creation
- Apply mutations during package update
- Apply resource mutations with rendering

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

### 3. Function Runtime

**Purpose**: KRM function execution

**What Engine uses it for**:
- Executing KRM functions to transform packages
- Rendering packages (applying function pipeline)

**Key operations**:
- Get runner for specific function
- Execute function with input/output

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

### 4. Watcher Manager

**Purpose**: Real-time change notifications

**What Engine uses it for**:
- Notifying clients of package changes
- Managing watch registrations
- Providing real-time updates

**Key operations**:
- Notify watchers of package changes (added, modified, deleted)

**Interaction pattern**:
1. Client registers watch via API Server
2. API Server calls Engine's WatcherManager
3. WatcherManager stores watcher
4. Engine performs operation (create/update/delete)
5. Engine notifies WatcherManager of change
6. WatcherManager sends events to matching watchers
7. Clients receive real-time notifications

**Event types**:
- **ADDED**: Package created
- **MODIFIED**: Package updated
- **DELETED**: Package deleted

### 5. Credential Resolver

**Purpose**: Authentication for private repositories

**What Engine uses it for**:
- Resolving Git/OCI credentials
- Authenticating to private repositories

**Key operations**:
- Resolve credentials from Kubernetes Secrets

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

### 6. Reference Resolver

**Purpose**: Package reference resolution

**What Engine uses it for**:
- Resolving package references for clone/upgrade
- Finding upstream packages

**Key operations**:
- Resolve package reference to actual package revision

**Interaction pattern**:
1. User specifies upstream package reference in clone/upgrade task
2. Task Handler asks Reference Resolver to resolve reference
3. Resolver finds the referenced package
4. Task Handler uses resolved package as source

### 7. User Info Provider

**Purpose**: User identification for audit trails

**What Engine uses it for**:
- Getting current user information
- Recording who performed operations

**Key operations**:
- Extract user info from request context

**Interaction pattern**:
1. Engine receives operation request with context
2. Engine asks User Info Provider for user details
3. Provider extracts user from Kubernetes authentication
4. User info recorded in Git commits, audit logs

## Dependents (What Needs the Engine)

### 1. API Server

**Component**: API Server REST handlers

**How it uses Engine**:
- Delegates all package operations to Engine
- Translates HTTP requests to Engine calls
- Translates Engine responses to HTTP responses

**Operations delegated**:
- Create PackageRevision
- Update PackageRevision
- Delete PackageRevision
- List PackageRevisions
- Update PackageRevisionResources

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

## Configuration and Initialization

The Engine is configured via options during initialization:

**Configuration options**:
- Cache for repository access
- Function runtimes (built-in, gRPC, or both)
- Credential resolver for authentication
- Reference resolver for package references
- User info provider for audit trails
- Watcher manager for real-time notifications
- Retry configuration for repository operations

**Why this approach**:
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
