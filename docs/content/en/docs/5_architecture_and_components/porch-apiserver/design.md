---
title: "Design"
type: docs
weight: 2
description: |
  Design patterns and architecture of the Porch API server.
---

## REST Storage Interface

See [Functionality](functionality.md#rest-storage-implementation) for detailed CRUD operations and storage implementations.

The Porch API Server implements Kubernetes' REST storage interface to provide custom storage backends for Porch resources. Unlike standard Kubernetes resources that store data in etcd, Porch resources delegate to the Engine which manages package data in Git repositories through the Cache.

**Storage interface characteristics:**
- Implements standard Kubernetes storage.Interface
- Provides CRUD operations (Create, Get, List, Update, Delete)
- Supports Watch for real-time change notifications
- Delegates all operations to CaD Engine
- No direct etcd storage - packages stored in Git

**Storage implementations:**
- **packageRevisions**: Manages PackageRevision resources
- **packageRevisionResources**: Manages PackageRevisionResources (package content)
- **packages**: Manages Package resources

## Strategy Pattern

See [Functionality](functionality.md#validation-strategies) for detailed validation rules and processes.

The API Server uses Kubernetes' strategy pattern to customize resource behavior:

### Validation Strategy

**Purpose**: Validates resource specifications before persistence

**Validation types:**
- **Create validation**: Ensures required fields present, lifecycle constraints enforced
- **Update validation**: Validates resource version, lifecycle transitions, immutability rules
- **Status validation**: Validates status subresource updates

### Admission Strategy

**Purpose**: Applies admission control policies and defaults

**Admission operations:**
- **PrepareForCreate**: Sets defaults, generates names, initializes status
- **PrepareForUpdate**: Validates resource version, enforces immutability
- **Canonicalize**: Normalizes resource representation

### Table Conversion Strategy

**Purpose**: Converts resources to table format for kubectl display

**Table conversion:**
- Defines columns for kubectl output (Name, Package, Workspace, Revision, Lifecycle)
- Extracts values from resource specifications
- Formats data for human-readable display
- Supports both list and individual resource views

## API Groups

The Porch API Server registers two API groups with Kubernetes:

### porch.kpt.dev API Group (Aggregated API)

**Resources:**
- **PackageRevision**: Represents a specific revision of a package
- **PackageRevisionResources**: Contains the actual resource content of a package revision
- **Package**: Represents a package across all its revisions

**Versions:**
- v1alpha1: Current version with all resources

**Characteristics:**
- Primary API group for package management
- All resources namespaced
- Served via Kubernetes API aggregation (not CRDs)
- Supports full CRUD and Watch operations
- Integrates with Engine for all operations
- Uses custom REST storage (Git-backed, not etcd)

### config.porch.kpt.dev API Group (CRDs)

**Resources:**
- **Repository**: Configures Git repositories for package storage
- **PackageRev**: Internal metadata resource for tracking package revisions

**Versions:**
- v1alpha1: Current version for all resources
- v1alpha2: Additional version for PackageVariantSet

**Characteristics:**
- Configuration API group for repository management
- All resources are namespaced
- Implemented as standard Kubernetes CRDs
- Managed by separate controllers, not directly by API server
- Stored in etcd (standard Kubernetes CRD storage)
- PackageRev is an internal resource used for metadata tracking

## Background Operations

See [Functionality](functionality.md#background-operations) for detailed implementation of background operations including repository synchronization and resource cleanup.

## Design Decisions

### REST Storage vs etcd

**Decision**: Implement custom REST storage that delegates to Engine instead of using etcd.

**Rationale:**
- Package data naturally lives in Git repositories
- etcd not suitable for large package content
- Engine provides necessary abstraction over Git
- Enables draft-commit workflow for package modifications

**Alternatives considered:**
- **Store in etcd**: Would require duplicating package content, large storage overhead
- **Hybrid approach**: Metadata in etcd, content in Git - adds complexity

**Trade-offs:**
- Custom storage more complex than standard etcd
- Enables Git-native package management
- Better scalability for large packages

### Strategy-Based Validation

**Decision**: Use Kubernetes strategy pattern for validation and admission control.

**Rationale:**
- Follows Kubernetes conventions
- Separates validation logic from storage logic
- Enables reuse across different storage implementations
- Provides consistent validation behavior

**Alternatives considered:**
- **Validation in Engine**: Would duplicate validation logic
- **Webhook-based validation**: Adds network overhead and complexity

**Trade-offs:**
- Strategy pattern adds abstraction layer
- Provides clean separation of concerns
- Enables testing validation independently

### Watch via WatcherManager

**Decision**: Implement watch streams using Engine's WatcherManager.

**Rationale:**
- Engine knows when package revisions change
- WatcherManager provides efficient fan-out to multiple watchers
- Avoids polling or etcd watch overhead
- Enables real-time notifications

**Alternatives considered:**
- **etcd watch**: Would require storing all data in etcd
- **Polling**: Inefficient and high latency

**Trade-offs:**
- Custom watch implementation more complex
- Provides efficient real-time updates
- Scales to many concurrent watchers

### Background Job Pattern

**Decision**: Run repository synchronization and cleanup as background goroutines.

**Rationale:**
- Sync operations are long-running and periodic
- Should not block API requests
- Enables concurrent sync of multiple repositories
- Provides automatic cache refresh

**Alternatives considered:**
- **Sync on demand**: Would add latency to API requests
- **External controller**: Adds deployment complexity

**Trade-offs:**
- Background jobs add complexity to server lifecycle
- Provides better user experience (no sync delays)
- Enables automatic cache consistency

### Dependency Injection

**Decision**: Configure Engine, Cache, and clients through dependency injection.

**Rationale:**
- Enables testing with mock implementations
- Provides flexible configuration
- Separates construction from usage
- Supports different deployment scenarios

**Alternatives considered:**
- **Global singletons**: Hard to test and configure
- **Service locator**: Hides dependencies

**Trade-offs:**
- Requires explicit wiring during initialization
- Provides clear dependency graph
- Enables flexible testing and configuration
