---
title: "Functionality"
type: docs
weight: 3
description: |
  Core functionality provided by the Porch API server.
---

## Overview

The Porch API Server provides the Kubernetes API interface for Porch resources. It implements custom REST storage backends that delegate to the Engine, enforces validation and admission policies through strategies, and manages real-time watch streams for clients.

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│              API Server Functionality                   │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │   REST Storage   │      │   Strategies     │         │
│  │                  │ ───> │                  │         │
│  │  • CRUD Ops      │      │  • Validation    │         │
│  │  • Watch Streams │      │  • Admission     │         │
│  │  • Engine Deleg  │      │  • Table Conv    │         │
│  └──────────────────┘      └──────────────────┘         │
│           │                         │                   │
│           └────────┬────────────────┘                   │
│                    ↓                                    │
│          ┌──────────────────┐                           │
│          │   Background     │                           │
│          │   Operations     │                           │
│          │                  │                           │
│          │  • Repo Sync     │                           │
│          │  • Cleanup       │                           │
│          └──────────────────┘                           │
└─────────────────────────────────────────────────────────┘
```

## REST Storage Implementation

See [Design](design.md#rest-storage-interface) for the design rationale behind custom REST storage.

The API Server implements custom REST storage for each Porch resource type:

### PackageRevision Storage

**CRUD operations:**
- **Create**: Validates spec, calls Engine to create package revision, returns created resource
- **Get**: Filters Engine package revision list by name, returns single resource
- **List**: Calls Engine to list package revisions with filters, returns resource list
- **Update**: Validates resource version, calls Engine to update package revision, returns updated resource
- **Delete**: Calls Engine to delete package revision, returns delete status

**Watch support:**
- Delegates to Engine cache for package revision watches
- Filters events based on watch criteria
- Delivers real-time change notifications
- Automatically cleans up on client disconnect

**Storage characteristics:**
- No etcd storage - delegates to Engine
- Engine manages package data in Git via Cache
- Supports all standard Kubernetes operations
- Implements storage.Interface fully

### PackageRevisionResources Storage

**Operations:**
- **Get**: Retrieves package content via Engine
- **List**: Lists resources for package revisions
- **Update**: Updates package content via Engine

**Content handling:**
- Resources stored as map of filename to content
- Supports large package content (not limited by etcd)
- Updates trigger render pipeline execution
- Returns RenderStatus with function results

**Storage characteristics:**
- Read-only for most operations
- Update only allowed on Draft packages
- Content retrieved on-demand (not cached in API server)
- Delegates to Engine for all operations

### Package Storage

**Operations:**
- **Get**: Filters Engine package list by name
- **List**: Calls Engine to list packages with filters
- **Create**: Calls Engine to create package
- **Delete**: Calls Engine to delete package (deletes all revisions)

**Package aggregation:**
- Represents package across all revisions
- Tracks latest revision
- Provides package-level metadata
- Enables package-level operations

### Function Storage

**Operations:**
- **Get**: Retrieves function from catalog
- **List**: Lists available functions

**Catalog integration:**
- Functions discovered from repositories
- Provides function metadata (image, description)
- Enables function discovery for clients
- Read-only resource (no create/update/delete)

## Validation Strategies

See [Design](design.md#validation-strategy) for the design rationale behind validation strategies.

Strategies enforce validation rules before Engine operations:

### Create Validation

**Validation rules:**
- Required fields present (package name, repository)
- Lifecycle constraints (cannot create Published/DeletionProposed)
- Task validation (maximum one task)
- Workspace name format
- Package path validity

**Validation process:**
- Strategy validation called before Engine operation
- Returns field errors for invalid specifications
- Prevents invalid resources from reaching Engine
- Provides clear error messages to clients

### Update Validation

**Validation rules:**
- Resource version required (optimistic locking)
- Lifecycle transition validity
- Immutability constraints (Published packages)
- Task append-only (cannot remove tasks)
- Metadata update restrictions

**Validation process:**
- Strategy validation called before Engine update operation
- Compares old and new objects
- Validates changes are allowed
- Returns field errors for invalid updates

### Status Validation

**Validation rules:**
- Status subresource updates validated separately
- Conditions format validation
- RenderStatus structure validation
- DownstreamTargets validation

**Validation process:**
- Strategy.ValidateStatusUpdate called for status updates
- Ensures status updates don't modify spec
- Validates status structure
- Returns field errors for invalid status

## Admission Control

See [Design](design.md#admission-strategy) for the design rationale behind admission control.

Strategies apply admission policies and defaults:

### PrepareForCreate

**Operations:**
- Generate name if not provided
- Set default lifecycle (Draft)
- Initialize status conditions
- Set creation timestamp
- Add default labels/annotations

**Admission process:**
- Called before validation
- Modifies resource in-place
- Ensures consistent initial state
- Prepares resource for Engine

### PrepareForUpdate

**Operations:**
- Validate resource version
- Enforce immutability rules
- Preserve status on spec updates
- Update modification timestamp
- Merge labels/annotations

**Admission process:**
- Called before validation
- Checks current vs desired state
- Enforces business rules
- Prepares resource for Engine

### Canonicalization

**Operations:**
- Normalize resource representation
- Remove redundant fields
- Apply default values
- Ensure consistent format

**Canonicalization process:**
- Called after validation
- Ensures consistent storage format
- Simplifies comparison operations
- Improves cache efficiency

## Table Conversion

See [Design](design.md#table-conversion-strategy) for the design rationale behind table conversion.

Strategies convert resources to table format for kubectl:

### Column Definitions

**PackageRevision columns:**
- Name: Resource name
- Package: Package name
- WorkspaceName: Workspace identifier
- Revision: Revision number
- Lifecycle: Current lifecycle state
- Repository: Source repository

**Table format:**
- Follows Kubernetes table conventions
- Supports sorting and filtering
- Provides human-readable output
- Consistent with kubectl expectations

### Conversion Process

**Conversion flow:**
- kubectl requests table format
- REST storage calls Strategy.ConvertToTable
- Strategy extracts column values
- Returns metav1.Table with rows
- kubectl formats for display

**Conversion characteristics:**
- Supports both list and individual resources
- Handles missing fields gracefully
- Provides consistent formatting
- Enables kubectl get commands

## Watch Stream Management

See [Interactions](interactions.md#watch-stream-management) for how watch streams integrate with other components.

The API Server provides real-time watch streams:

### Watch Registration

**Registration process:**
- Client sends watch request with filters
- REST storage calls Engine cache to watch package revisions
- WatcherManager registers watcher with filters
- Returns watch interface to client
- Client receives events as they occur

**Filter support:**
- Namespace filtering
- Label selectors
- Field selectors
- Resource version (resume from point)

### Event Delivery

**Event types:**
- **Added**: New package revision created
- **Modified**: Package revision updated
- **Deleted**: Package revision removed

**Delivery characteristics:**
- Events delivered in real-time
- Filtered based on watch criteria
- Ordered per resource
- Best-effort delivery (network failures may drop events)

### Watch Lifecycle

**Lifecycle stages:**
- **Registration**: Client subscribes with filters
- **Active**: Events delivered as changes occur
- **Disconnection**: Client closes connection or timeout
- **Cleanup**: WatcherManager removes watcher automatically

**Cleanup triggers:**
- Client disconnects
- Context cancellation
- Watch timeout
- Error during event delivery

## Background Operations

The API Server runs background tasks for maintenance:

### Repository Synchronization

**Sync process:**
- Background goroutine discovers repositories
- Creates SyncManager for each repository
- SyncManager triggers periodic sync
- Cache updates from Git repository
- Watch notifications sent to clients

**Sync configuration:**
- Default frequency set at server startup
- Per-repository frequency via Repository CR
- Manual sync via Repository CR update
- Cron schedule support

**Sync coordination:**
- One SyncManager per repository
- Syncs run independently
- Errors logged and reported in Repository status
- Automatic retry on next cycle

### Resource Cleanup

**Cleanup operations:**
- Remove PackageRev CRs for deleted repositories
- Clean up orphaned cache entries
- Remove stale watch registrations
- Garbage collect expired resources

**Cleanup triggers:**
- Repository deletion
- Cache eviction
- Watch disconnection
- Periodic maintenance

**Cleanup coordination:**
- Triggered by lifecycle events
- Runs asynchronously
- Ensures consistency
- Prevents resource leaks

## Performance Optimization

The API Server employs several optimization strategies:

### List Operation Optimization

**Optimization techniques:**
- Concurrent repository listing (configurable max concurrency)
- Per-repository timeout (prevents slow repos from blocking)
- Early termination on context cancellation
- Efficient filtering at Engine level

**Configuration:**
- MaxConcurrentLists: Maximum concurrent repository operations
- ListTimeoutPerRepository: Timeout per repository
- Prevents slow repositories from impacting overall performance

### Watch Stream Efficiency

**Efficiency mechanisms:**
- WatcherManager provides efficient fan-out
- Events filtered at source (not delivered then filtered)
- Automatic cleanup of inactive watchers
- No polling overhead

**Scalability:**
- Supports many concurrent watchers
- Minimal overhead per watcher
- Efficient event delivery
- Scales with number of clients

### Cache Integration

**Cache benefits:**
- Engine caches repository data
- Reduces Git operations
- Faster list operations
- Consistent performance

**Cache coordination:**
- Background sync keeps cache fresh
- Watch notifications on cache updates
- Automatic cache invalidation
- Transparent to API clients

## Error Handling

See [Interactions](interactions.md#error-handling) for error handling across component boundaries.

The API Server handles errors at multiple levels:

### Validation Errors

**Error handling:**
- Strategy validation returns field errors
- Converted to 400 Bad Request
- Includes field path and error message
- Client receives detailed error information

**Error examples:**
- "spec.lifecycle: cannot create Published package"
- "spec.tasks: maximum one task allowed"
- "metadata.name: invalid format"

### Engine Errors

**Error handling:**
- Engine returns typed errors
- REST storage translates to Kubernetes status
- Appropriate HTTP status code
- Error details in status message

**Error types:**
- NotFound → 404 Not Found
- Conflict → 409 Conflict
- Validation → 400 Bad Request
- Internal → 500 Internal Server Error

### Watch Errors

**Error handling:**
- Registration errors returned immediately
- Delivery errors close watch stream
- Client receives error event
- Automatic cleanup on error

**Error recovery:**
- Client can re-establish watch
- Resume from last resource version
- No data loss on transient errors
- Graceful degradation

## Concurrency Control

See [Interactions](interactions.md#concurrency-and-safety) for concurrency patterns across component interactions.

The API Server handles concurrent operations:

### Request Concurrency

**Concurrency characteristics:**
- Multiple clients can make requests concurrently
- Each request processed independently
- Engine provides concurrency control
- Optimistic locking prevents conflicts

**Concurrency patterns:**
- Read operations fully concurrent
- Write operations serialized per package (Engine mutex)
- Watch streams independent
- Background jobs concurrent

### Optimistic Locking

**Locking mechanism:**
- Clients provide resource version on updates
- API Server validates version before Engine call
- Engine compares with current version
- Conflict returned if mismatch
- Client must re-read and retry

**Locking benefits:**
- Prevents lost updates
- No distributed locks needed
- Scales well
- Standard Kubernetes pattern

### Watch Stream Safety

**Safety guarantees:**
- Each watch stream independent
- No shared state between watchers
- Thread-safe event delivery
- Automatic cleanup prevents leaks
