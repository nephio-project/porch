---
title: "Controllers"
type: docs
weight: 6
description: |
  Kubernetes controllers that automate package variant creation and management.
---

## What are the Controllers?

The **Controllers** are Kubernetes controllers that watch high-level Custom Resources and automatically create and manage PackageRevisions through the Porch API. They provide declarative, template-based automation for creating multiple package variants from a single upstream package.

The Controllers are responsible for:

- **PackageVariant Management**: Watching PackageVariant CRs and ensuring corresponding downstream PackageRevisions exist and stay synchronized with upstream changes
- **Bulk Variant Creation**: Watching PackageVariantSet CRs and automatically generating multiple PackageVariant CRs based on target selectors
- **Lifecycle Synchronization**: Detecting upstream package changes and creating new downstream package revisions (upgrades or edits) as needed
- **Adoption and Deletion Policies**: Managing ownership of existing packages and handling cleanup when PackageVariant CRs are deleted
- **Resource Injection**: Dynamically injecting configuration from in-cluster resources into generated packages
- **Template Evaluation**: Using CEL expressions to dynamically generate package metadata, labels, annotations, and configuration

## Role in the Architecture

The Controllers sit above the Porch API Server and act as automation clients:

```
┌─────────────────────────────────────────────────────┐
│              Controllers                            │
│                                                     │
│  ┌──────────────────┐     ┌──────────────────┐      │
│  │ PackageVariant   │     │PackageVariantSet │      │
│  │   Controller     │<────│   Controller     │      │
│  │                  │     │                  │      │
│  │ • Watch PV CRs   │     │ • Watch PVS CRs  │      │
│  │ • Create/Update  │     │ • Generate PV    │      │
│  │   PackageRevs    │     │   CRs            │      │
│  │ • Sync Upstream  │     │ • Target Select  │      │
│  └────────┬─────────┘     └──────────────────┘      │
│           │                                         │
└───────────┴─────────────────────────────────────────┘
            ↓
    Porch API Server
            ↓
    PackageRevisions
```

**Key architectural responsibilities:**

1. **Declarative Package Management**: Enables users to declare desired package variants rather than manually creating each PackageRevision

2. **Automation Layer**: Bridges the gap between high-level intent (PackageVariant/PackageVariantSet) and low-level operations (PackageRevision CRUD)

3. **Multi-Target Distribution**: PackageVariantSet controller enables creating variants across multiple repositories or for multiple targets from a single declaration

4. **Change Detection and Reconciliation**:
   - Watches upstream PackageRevisions for new published versions
   - Automatically creates upgrade or edit drafts when changes detected
   - Ensures downstream packages stay synchronized with upstream

5. **Template-Based Generation**: Uses templates with CEL expressions to dynamically generate package configuration based on target context

6. **Ownership Management**:
   - Tracks which PackageRevisions are owned by which PackageVariant
   - Supports adopting existing packages or creating new ones
   - Handles cleanup with configurable deletion policies (delete or orphan)

## Controller Types

### PackageVariant Controller

Manages individual package variants - one upstream package to one downstream package relationship. Creates downstream PackageRevisions (clones, upgrades, edits) and applies mutations (package context, pipeline functions, injections).

### PackageVariantSet Controller

Manages bulk creation of PackageVariant CRs based on target selectors. Evaluates target selectors (repositories, repository selector, object selector) and generates PackageVariant CRs for each matching target using CEL expression templates.

## Integration with Porch

The controllers are **clients** of the Porch API, not part of the Porch server. They run as a separate deployment using standard Kubernetes client-go to interact with Porch API, and can be enabled/disabled independently via `--reconcilers` flag.

**Controller runtime:**
- Built using controller-runtime framework
- Standard Kubernetes controller patterns (watch, reconcile, requeue)
- Leader election support (currently disabled)

The controllers are instantiated once during startup and run continuously, reconciling resources as they change.
