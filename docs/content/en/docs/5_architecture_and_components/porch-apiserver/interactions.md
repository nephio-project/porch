---
title: "Interactions"
type: docs
weight: 4
description: |
  How the Porch API server interacts with other components.
---

## Overview

The Porch API Server acts as the integration point between Kubernetes clients and Porch's internal components. It translates Kubernetes API requests into Engine operations, manages watch streams, and coordinates background synchronization tasks.

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Porch API Server                           │
│                                                         │
│  ┌──────────────┐      ┌──────────────┐      ┌──────┐   │
│  │   Clients    │ ───> │     REST     │ ───> │Engine│   │
│  │   (kubectl,  │      │   Storage    │      │      │   │
│  │    kpt, etc) │      │              │      │      │   │
│  └──────────────┘      └──────────────┘      └──────┘   │
│         ↑                      │                │       │
│         │                      ↓                ↓       │
│         │              ┌──────────────┐      ┌──────┐   │
│         └──────────────│   Watcher    │      │Cache │   │
│                        │   Manager    │      │      │   │
│                        └──────────────┘      └──────┘   │
└─────────────────────────────────────────────────────────┘
```

## Engine Integration

The API Server delegates all package operations to the CaD Engine:

### Request Translation Pattern

```
Kubernetes API Request
        ↓
  REST Storage Handler
        ↓
  Strategy Validation
        ↓
  Engine Method Call
        ↓
  • CreatePackageRevision
  • UpdatePackageRevision
  • DeletePackageRevision
  • ListPackageRevisions
        ↓
  Engine Response
        ↓
  Convert to API Object
        ↓
  Return to Client
```

**Translation characteristics:**
- REST storage translates API operations to Engine calls
- Strategies validate before Engine invocation
- Engine returns repository objects
- REST storage converts to Kubernetes API objects
- Errors propagated back to client

### Operation Mapping

**Create operations:**
- CreatePackageRevision → Engine package revision creation
- CreatePackage → Engine package creation

**Read operations:**
- GetPackageRevision → Engine package revision listing (filtered by name)
- ListPackageRevisions → Engine package revision listing
- GetPackage → Engine package listing (filtered by name)
- ListPackages → Engine package listing

**Update operations:**
- UpdatePackageRevision → Engine package revision update
- UpdatePackageRevisionResources → Engine package resources update

**Delete operations:**
- DeletePackageRevision → Engine package revision deletion

**Watch operations:**
- WatchPackageRevisions → Engine cache watch for package revisions

### Context Propagation

**Context flow:**
- Client request includes Kubernetes request context
- REST storage extracts user info from context
- Context passed to Engine for all operations
- Engine uses context for:
  - Cancellation and timeouts
  - User info for audit trails (PublishedBy)
  - Tracing and logging

## Cache Integration

The API Server interacts with the Cache through the Engine:

### Repository Access Pattern

```
API Request
        ↓
  REST Storage
        ↓
  Engine
        ↓
  Cache.OpenRepository
        ↓
  Repository Operations
        ↓
  Cache Response
        ↓
  Engine Response
        ↓
  API Response
```

**Access characteristics:**
- API Server never directly accesses Cache
- All cache operations through Engine
- Engine manages repository lifecycle
- Cache provides repository abstractions

### Background Synchronization

See [Functionality](functionality.md#repository-synchronization) for detailed sync process and configuration.

**Integration pattern:**
- API Server starts background sync goroutines at startup
- Discovers repositories via Kubernetes client
- Delegates sync operations to Cache through Engine
- Propagates watch notifications from Cache to clients

## Kubernetes API Integration

The API Server integrates with the Kubernetes API aggregation layer:

### API Aggregation Pattern

```
Kubernetes API Server
        ↓
  API Aggregation Layer
        ↓
  Porch API Server
        ↓
  • porch.kpt.dev/v1alpha1
  • config.porch.kpt.dev/v1alpha1
        ↓
  REST Storage Handlers
```

**Aggregation characteristics:**
- Porch API Server registered as aggregated API
- Kubernetes API Server proxies requests to Porch
- Authentication and authorization handled by Kubernetes
- Porch API Server receives authenticated requests

### RBAC Integration

**Authorization flow:**
- Kubernetes API Server enforces RBAC policies
- Porch API Server receives authorized requests
- User info available in request context
- Porch enforces additional business rules

**RBAC resources:**
- PackageRevision: get, list, watch, create, update, delete
- PackageRevisionResources: get, list, watch, update
- Package: get, list, watch, create, delete

### Client Integration

**Client types:**
- **kubectl**: Standard Kubernetes CLI
- **kpt**: Package management CLI
- **Porchctl**: Porch-specific CLI
- **Custom controllers**: Automation and workflows

**Client operations:**
- CRUD operations on Porch resources
- Watch streams for real-time updates
- Approval workflows (lifecycle transitions)
- Package content updates

## Watch Stream Management

See [Functionality](functionality.md#watch-stream-management) for detailed watch lifecycle and event delivery.

The API Server integrates watch streams between clients and Engine:

### Watch Integration Pattern

```
Client Watch Request
        ↓
  REST Storage.Watch()
        ↓
  Engine.ObjectCache().WatchPackageRevisions()
        ↓
  WatcherManager.Watch()
        ↓
  Register Watcher
        ↓
  Return Watch Interface
        ↓
  Client Receives Events:
        ↓
  • Added
  • Modified
  • Deleted
```

**Watch integration:**
- Clients subscribe via standard Kubernetes watch API
- REST storage delegates to Engine's WatcherManager
- Events filtered and delivered through component chain
- Automatic cleanup on client disconnect

### Event Delivery

**Event sources:**
- Package revision creation (Added events)
- Package revision updates (Modified events)
- Package revision deletion (Deleted events)
- Background sync changes (all event types)

## Background Job Coordination

See [Functionality](functionality.md#background-operations) for detailed background operation implementation.

The API Server coordinates background operations across components:

### Repository Sync Coordination

```
API Server Startup
        ↓
  Start Background Goroutine
        ↓
  For Each Repository:
        ↓
    Create SyncManager
        ↓
    Start Periodic Sync
        ↓
    Sync Triggers Cache Update
        ↓
    Cache Sends Notifications
        ↓
    WatcherManager Delivers Events
        ↓
    Clients Receive Updates
```

**Integration flow:**
- API Server manages background goroutine lifecycle
- Discovers repositories via Kubernetes API
- Coordinates sync operations through Cache
- Delivers notifications through WatcherManager to clients

### Cleanup Coordination

See [Functionality](functionality.md#resource-cleanup) for detailed cleanup operations.

**Integration flow:**
- Repository deletion detected via Kubernetes API
- Cleanup coordinated through Engine and Cache
- Notifications propagated to active watchers

## Error Handling

See [Functionality](functionality.md#error-handling) for detailed error handling within the API Server.

The API Server translates errors across component boundaries:

### Engine Error Translation

**Error types:**
- **Validation errors**: Translated to 400 Bad Request
- **Not found errors**: Translated to 404 Not Found
- **Conflict errors**: Translated to 409 Conflict
- **Internal errors**: Translated to 500 Internal Server Error

**Translation pattern:**
- Engine returns typed errors
- REST storage translates to Kubernetes status
- Status includes error message and details
- Client receives standard Kubernetes error response

### Watch Error Handling

**Integration error handling:**
- Registration errors from WatcherManager returned to client
- Delivery errors trigger stream closure and cleanup
- Automatic cleanup on client disconnection

### Background Job Errors

**Error types:**
- Repository sync failures
- Cache operation failures
- Kubernetes API errors

**Handling strategy:**
- Errors logged with context
- Sync retried on next cycle
- Repository condition updated with error
- Operations continue for other repositories

## Concurrency and Safety

See [Functionality](functionality.md#concurrency-control) for detailed concurrency mechanisms.

The API Server coordinates concurrent operations across components:

### Request Concurrency

**Integration patterns:**
- Request concurrency managed through Engine
- Optimistic locking enforced at API Server and Engine boundary
- Watch streams isolated per client
- Background jobs coordinate through Cache


