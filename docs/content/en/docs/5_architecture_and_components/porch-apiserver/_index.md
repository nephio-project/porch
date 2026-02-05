---
title: "Porch API Server"
type: docs
weight: 1
description: |
  The Kubernetes extension API server that provides Porch's API resources.
---

## What is the Porch API Server?

The **Porch API Server** is a Kubernetes extension API server that exposes Porch's aggregated API resources (PackageRevision, PackageRevisionResources, Package) and CRDs (Repository, PackageRev) through the Kubernetes API. It acts as the entry point for client interactions with Porch, translating Kubernetes API requests into operations on the underlying Engine and Cache.

The Porch API Server is responsible for:

- **API Resource Registration**: Registering Porch's Kubernetes resources with the Kubernetes API aggregation layer
- **Request Handling**: Processing CRUD operations (Create, Read, Update, Delete) and Watch requests for those resources
- **REST Storage Implementation**: Providing storage backends for each API resource type that delegate to the Engine
- **Validation and Admission**: Enforcing API validation rules and admission policies through strategies
- **Authentication and Authorization**: Integrating with Kubernetes RBAC for access control
- **Watch Stream Management**: Supporting real-time watch streams for resource changes
- **Background Operations**: Running periodic repository synchronization and maintenance tasks

## Role in the Architecture

The Porch API Server sits at the top of the Porch architecture, serving as the interface between Kubernetes clients and Porch's internal components:

```
┌────────────────────────────────────────────────────────────┐
│              Porch API Server                              │
│                                                            │
│  ┌──────────────────┐      ┌────────────────────────────┐  │
│  │   API Groups     │      │   REST Storage             │  │          
│  │                  │ ───> │   (Aggregated)             │  │          
│  │  • porch.kpt.dev │      │  • PackageRevision         │  │        
│  │                  │      │  • PackageRevisionResources│  │         
│  │                  │      │  • Package                 │  │          
│  └──────────────────┘      └────────────────────────────┘  │
│           │                         │                      │
│           │                         │                      │
│  ┌──────────────────┐               │                      │
│  │   CRDs (etcd)    │               │                      │
│  │                  │               │                      │
│  │  • Repository    │               │                      │
│  │  • PackageRev    │               │                      │
│  │  • PackageVarSet │               │                      │
│  └──────────────────┘               │                      │
│           │                         │                      │
│           └────────┬────────────────┘                      │
│                    ↓                                       │
│  ┌──────────────────┐      ┌──────────────────┐            │
│  │   Strategies     │      │   Background     │            │
│  │                  │      │   Jobs           │            │
│  │  • Validation    │      │                  │            │
│  │  • Admission     │      │  • Repo Sync     │            │
│  │  • Table Conv    │      │  • Cleanup       │            │
│  └──────────────────┘      └──────────────────┘            │
└────────────────────────────────────────────────────────────┘
                    ↓
         ┌──────────────────────┐
         │   CaD Engine         │
         │   (Orchestration)    │
         └──────────────────────┘
                    ↓
         ┌──────────────────────┐
         │   Package Cache      │
         │   (Repository Data)  │
         └──────────────────────┘
```

**Key architectural responsibilities:**

1. **Kubernetes API Integration**: Extends Kubernetes API with Porch-specific resources using API aggregation
2. **Request Translation**: Converts Kubernetes API requests into Engine operations (CreatePackageRevision, UpdatePackageRevision, etc.)
3. **Storage Abstraction**: Implements Kubernetes storage interface (REST storage) that delegates to Engine rather than etcd
4. **Strategy Pattern**: Uses Kubernetes strategy pattern for validation, admission control, and table conversion
5. **Watch Support**: Provides real-time watch streams by integrating with Engine's WatcherManager
6. **Background Processing**: Runs periodic tasks for repository synchronization and resource cleanup
7. **Multi-Tenancy**: Enforces namespace isolation and RBAC policies for secure multi-tenant operation

The API Server is instantiated once during Porch startup and configured with Engine, Cache, and client connections through dependency injection.
