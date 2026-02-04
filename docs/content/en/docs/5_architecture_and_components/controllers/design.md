---
title: "Design"
type: docs
weight: 1
description: |
  Architecture and design patterns of the Porch controllers.
---

## Architecture

The Controllers follow a [**standard Kubernetes controller pattern**](https://kubernetes.io/docs/concepts/architecture/controller/#controller-pattern)
using the controller-runtime framework:

```
┌─────────────────────────────────────────────────────┐
│                 Controller Manager                  │
│                                                     │
│    ┌──────────────────┐     ┌──────────────────┐    │
│    │ PackageVariant   │     │PackageVariantSet │    │
│    │   Reconciler     │     │   Reconciler     │    │
│    │                  │     │                  │    │
│    │  Watch/Reconcile │     │  Watch/Reconcile │    │
│    │  Loop            │     │  Loop            │    │
│    └────────┬─────────┘     └────────┬─────────┘    │
│              │                       │              │
│              └───────────┬───────────┘              │
│                          ↓                          │
│                 ┌─────────────────┐                 │
│                 │  Shared Client  │                 │
│                 │  (Porch API)    │                 │
│                 └─────────────────┘                 │
└─────────────────────────────────────────────────────┘
```

**Key architectural patterns:**

### Controller-Runtime Framework

Both controllers use the [Kubernetes controller-runtime library](https://pkg.go.dev/sigs.k8s.io/controller-runtime),
which provides them with several Kubernetes-standard features, such as an interface to watch the resources they manage
and a client to interact with the rest of the cluster.

### Reconciliation Loop Pattern

Standard Kubernetes reconciliation pattern:

```
  Event Trigger
       ↓
  Watch Detects Change
       ↓
  Enqueue Request
       ↓
  Reconcile(ctx, request)
       ↓
  ┌────┴────┐
  │         │
Read      Compute
State      Desired
  │         State
  │         │
  └────┬────┘
       ↓
  Apply Changes
       ↓
  Update Status
       ↓
  Return (Requeue if needed)
```

**Reconciliation characteristics:**
- Idempotent - can be called multiple times safely
- Level-triggered - works from current state, not events
- Error handling - returns error to trigger requeue with backoff
- Status updates - deferred to ensure they happen even on errors

### Hierarchical Controller Pattern

PackageVariantSet and PackageVariant have a parent-child relationship:

```
PackageVariantSet (Parent)
        ↓
  Creates/Manages
        ↓
PackageVariant (Child)
        ↓
  Creates/Manages
        ↓
PackageRevision (Grandchild)
```

**Ownership chain:**
- PackageVariantSet owns PackageVariant (via OwnerReference)
- PackageVariant owns PackageRevision (via OwnerReference)
- Kubernetes garbage collection handles cascading deletion
- Child watches trigger parent reconciliation

## PackageVariant Controller Design

### Core Reconciliation Strategy

The PackageVariant controller implements a **continuous synchronization pattern** between upstream and downstream packages:

```
Upstream Package
       ↓
 Monitor Changes
       ↓
  Detect Drift
       ↓
  ┌────┴────┐
  ↓         ↓
Upgrade   Edit
  ↓         ↓
  └────┬────┘
       ↓
 Apply Mutations
       ↓
Downstream Package
```

**Design principles:**
- **Declarative**: User declares desired upstream/downstream relationship
- **Reactive**: Automatically responds to upstream changes
- **Mutation-based**: Applies systematic transformations to packages
- **Approval-driven**: All changes to the downstream must go through Porch draft/proposal/approval workflow before being
  published

### Upstream Change Detection

**UpstreamLock mechanism:**
- Porch stores Git ref in PackageRevision status
- Controller compares current downstream's UpstreamLock with desired upstream
- Mismatch triggers upgrade draft creation
- Enables automatic version tracking

**Change types:**
- **Upstream version change**: Create upgrade draft (three-way merge)
- **Mutation spec change**: Create edit draft (copy and reapply)
- **No change**: No action needed

### Draft Management Strategy

**Draft types and their purpose:**

- **Clone**: Initial package creation from upstream
- **Edit**: New revision from existing (for mutation changes)
- **Upgrade**: Three-way merge (for upstream version changes)

**Draft workflow:**
- Controller creates draft PackageRevision via Porch API
- Applies mutations (package context, pipeline, injections)
- Saves draft for human/automation approval
- Does not auto-publish (separation of concerns)

### Adoption and Deletion Policies

**Policy-based ownership:**

- **Adoption**: Controls whether to take ownership of pre-existing packages
  - `adoptNone`: Only manage self-created packages
  - `adoptExisting`: Take ownership of matching packages

- **Deletion**: Controls cleanup behavior when PackageVariant deleted
  - `delete`: Remove owned PackageRevisions
  - `orphan`: Leave PackageRevisions but remove ownership

**Design rationale:**
- Enables gradual migration to controller-managed packages
- Supports different organizational policies
- Prevents accidental deletion of important packages

## PackageVariantSet Controller Design

### Fan-Out Pattern

PackageVariantSet implements a **declarative fan-out pattern** for bulk package creation:

```
  Single Upstream
        ↓
  Target Selector
        ↓
   ┌────┴───┬────────┐
   ↓        ↓        ↓
Target1  Target2  Target3
   ↓        ↓        ↓
   └────┬───┴────────┘
        ↓
Multiple PackageVariants
        ↓
Multiple Downstream Packages
```

**Design principles:**
- **Declarative targeting**: Specify targets via selectors, not explicit lists
- **Template-based**: Single template generates multiple PackageVariants
- **Dynamic**: Automatically adjusts as targets change
- **CEL-powered**: Expressions enable context-aware customization

### Target Unrolling Strategy

**Three targeting mechanisms:**

1. **Repository List**: Explicit list of repositories and package names
2. **Repository Selector**: Label selector against Repository CRs
3. **Object Selector**: Label selector against arbitrary Kubernetes objects

**Unrolling process:**
- Each target produces exactly one PackageVariant
- Target information becomes available in CEL environment
- Template evaluated per-target for customization

### Template Evaluation Design

**CEL expression environment:**

Provides controlled access to:
- Target information (repo, package, object)
- Upstream PackageRevision metadata
- Downstream Repository metadata

**Security by design:**
- Only name, namespace, labels, annotations accessible
- Prevents leaking sensitive data from cluster
- Namespace isolation enforced

**Evaluation order:**
1. Load upstream PackageRevision
2. Determine all targets
3. For each target:
   - Evaluate downstream.repoExpr (if present)
   - Load downstream Repository
   - Evaluate all other expressions
   - Generate PackageVariant spec

### Desired State Reconciliation

**Set-based reconciliation:**

```
Desired Set: {A, B, C}
Existing Set: {B, C, D}

Actions:
  Create: A
  Update: B, C
  Delete: D
```

**Identifier-based tracking:**
- Identifier: `{pvsName}-{repo}-{package}`
- Hashed if exceeds Kubernetes name limit
- Enables stable tracking across reconciliations

**Owner reference pattern:**
- PackageVariantSet owns all generated PackageVariants
- Label for efficient querying: `config.porch.kpt.dev/packagevariantset`
- Kubernetes garbage collection handles cleanup

## Design Trade-offs

### Watch Strategy

**Broad watches vs targeted watches:**

- **Current**: Watch all PackageRevisions in namespace
- **Trade-off**: More reconciliations, simpler logic
- **Alternative**: Targeted watches with complex filtering
- **Decision**: Simplicity over efficiency (acceptable for typical scale)

### Mutation Application

**Controller-applied vs function-applied:**

- **Package context**: Controller directly modifies ConfigMap
- **Pipeline functions**: Controller prepends to Kptfile
- **Config injection**: Controller injects then functions process
- **Rationale**: Controller handles orchestration, functions handle transformation

### Draft vs Auto-Publish

**Draft workflow vs automatic publishing:**

- **Current**: Controller creates drafts, requires approval
- **Trade-off**: Manual step, but safer
- **Alternative**: Auto-publish with rollback
- **Decision**: Safety and auditability over automation

### CEL Security Model

**Limited access vs full access:**

- **Current**: Only name, namespace, labels, annotations
- **Trade-off**: Less flexible, more secure
- **Alternative**: Full object access
- **Decision**: Security over flexibility
