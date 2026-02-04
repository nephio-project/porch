---
title: "PackageVariantSet Reconciliation"
type: docs
weight: 2
description: |
  Detailed architecture of PackageVariantSet reconciliation flows and set-based management.
---

## Overview

The PackageVariantSet controller implements a declarative fan-out pattern where a single upstream package is automatically instantiated across multiple downstream targets using template-based generation with CEL expressions. It manages bulk creation of PackageVariant CRs based on target selectors and ensures the desired set of PackageVariants matches the actual set through set-based reconciliation.

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│        PackageVariantSet Reconciliation System          │
│                                                         │
│      ┌──────────────────┐     ┌──────────────────┐      │
│      │  Target          │     │   Template       │      │
│      │  Unrolling       │ ──> │   Evaluation     │      │
│      │                  │     │                  │      │
│      │  • Repository    │     │  • CEL Exprs     │      │
│      │    List          │     │  • Context       │      │
│      │  • Repository    │     │  • Dynamic       │      │
│      │    Selector      │     │    Config        │      │
│      │  • Object        │     │                  │      │
│      │    Selector      │     │                  │      │
│      └──────────────────┘     └──────────────────┘      │
│               │                         │               │
│               └────────┬────────────────┘               │
│                        ↓                                │
│                  ┌──────────────────┐                   │
│                  │   Set-Based      │                   │
│                  │  Reconciliation  │                   │
│                  │                  │                   │
│                  │  • Desired Set   │                   │
│                  │  • Existing Set  │                   │
│                  │  • Create/Update │                   │
│                  │  • Delete        │                   │
│                  └──────────────────┘                   │
└─────────────────────────────────────────────────────────┘
```

## Reconciliation Flow

The controller follows a structured reconciliation process:

### Main Reconciliation Loop

```
Reconcile Triggered
        ↓
  Get PackageVariantSet
        ↓
  List PackageRevisions
        ↓
  List Repositories
        ↓
  Validate Spec
        ↓
  Valid? ──No──> Set Stalled=True, return
        │
       Yes
        ↓
  Find Upstream PR
        ↓
  Found? ──No──> Set Stalled=True, return
        │
       Yes
        ↓
  Unroll Downstream Targets
        ↓
  Success? ──No──> Set Stalled=True, return
        │
       Yes
        ↓
  Set Stalled=False
        ↓
  Ensure PackageVariants
        ↓
  Success? ──No──> Set Stalled=True, return
        │
       Yes
        ↓
  Set Ready=True
        ↓
  Update Status
        ↓
  Return
```

**Process characteristics:**
- **Deferred status update**: Status updated at end regardless of success/failure
- **Early validation**: Validation errors prevent further processing
- **Upstream dependency**: Requires upstream PackageRevision to exist
- **Target unrolling**: Converts target selectors to concrete downstream contexts
- **Set reconciliation**: Ensures desired PackageVariants match actual

## Target Unrolling

The controller converts target selectors into concrete downstream contexts:

### Unrolling Process

```
Unroll Downstream Targets
        ↓
  For Each Target in Spec.Targets:
        ↓
    Repositories specified? ──Yes──> Explicit Unroll
        │                                   ↓
        No                            For each Repository:
        ↓                                   ↓
    RepositorySelector? ──Yes──> Query Repos     For each PackageName:
        │                           ↓                   ↓
        No                    For each match:     Create pvContext
        ↓                           ↓
    ObjectSelector? ──Yes──> Query Objects
        │                           ↓
        No                    For each match:
        ↓                           ↓
    Error                     Create pvContext
        ↓
  Return all pvContexts
```

**pvContext structure:**
- **template**: PackageVariantTemplate from target
- **repoDefault**: Default repository name
- **packageDefault**: Default package name
- **object**: Matched object (for ObjectSelector only)

### Repository List Unrolling

**When used:**
- Explicit list of repositories and package names
- Most direct targeting mechanism

**Unrolling logic:**
```
Target.Repositories = [
  {Name: "repo-1", PackageNames: ["pkg-a", "pkg-b"]},
  {Name: "repo-2", PackageNames: ["pkg-c"]}
]
        ↓
Unrolls to 3 pvContexts:
  • repo-1/pkg-a
  • repo-1/pkg-b
  • repo-2/pkg-c
```

**Characteristics:**
- Each repository can specify multiple package names
- If PackageNames empty, defaults to upstream package name
- Creates one pvContext per repository-package combination
- Most predictable and explicit

### Repository Selector Unrolling

**When used:**
- Label-based selection of Repository CRs
- Dynamic discovery of target repositories

**Unrolling logic:**
```
Target.RepositorySelector = {
  MatchLabels: {env: "prod"}
}
        ↓
Query Repositories in namespace
        ↓
Filter by label selector
        ↓
For each matching Repository:
        ↓
  Create pvContext:
    • repoDefault = Repository.Name
    • packageDefault = upstream.Package
    • object = nil
```

**Characteristics:**
- Automatically discovers repositories with matching labels
- Package name defaults to upstream package name
- Dynamic - adjusts as repositories added/removed
- Internally converted to ObjectSelector with Repository GVK

### Object Selector Unrolling

**When used:**
- Label-based selection of arbitrary Kubernetes objects
- Most flexible targeting mechanism

**Unrolling logic:**
```
Target.ObjectSelector = {
  APIVersion: "v1",
  Kind: "ConfigMap",
  LabelSelector: {MatchLabels: {cluster: "edge"}}
}
        ↓
Query objects of specified GVK
        ↓
Filter by label selector
        ↓
For each matching object:
        ↓
  Create pvContext:
    • repoDefault = object.Name
    • packageDefault = upstream.Package
    • object = object (full unstructured)
```

**Characteristics:**
- Can select any Kubernetes object type
- Object metadata available in CEL expressions
- Repository name defaults to object name
- Package name defaults to upstream package name
- Most flexible but requires careful configuration

**Empty results handling:**
- Warning logged if no objects match selector
- No error returned (may be temporary condition)
- Reconciliation continues with empty target list

## Template Evaluation

The controller evaluates CEL expressions to generate PackageVariant specs:

### Evaluation Process

```
Render PackageVariant Spec
        ↓
  Build Base Inputs
        ↓
  • repoDefault, packageDefault
  • upstream (PackageRevision)
  • target (matched object)
        ↓
  Evaluate Downstream.RepoExpr
        ↓
  Load Repository object
        ↓
  Add repository to inputs
        ↓
  Evaluate Downstream.PackageExpr
        ↓
  Evaluate LabelExprs
        ↓
  Evaluate AnnotationExprs
        ↓
  Evaluate PackageContext.DataExprs
        ↓
  Evaluate PackageContext.RemoveKeyExprs
        ↓
  Evaluate Injector.NameExpr
        ↓
  Evaluate Pipeline.ConfigMapExprs
        ↓
  Return PackageVariant Spec
```

**Evaluation order:**
1. **Base inputs**: repoDefault, packageDefault, upstream, target
2. **Downstream repo**: Evaluated first (needed to load Repository object)
3. **Repository object**: Loaded and added to CEL environment
4. **All other expressions**: Evaluated with full context

### CEL Environment

**Available variables:**
- **repoDefault** (string): Default repository name from target unrolling
- **packageDefault** (string): Default package name from target unrolling
- **upstream** (dynamic): Upstream PackageRevision object
- **repository** (dynamic): Downstream Repository object
- **target** (dynamic): Matched target object (ObjectSelector only)

**Object structure (security-limited):**
- **name**: Object name
- **namespace**: Object namespace
- **labels**: Object labels map
- **annotations**: Object annotations map

**Security rationale:**
- Only metadata fields accessible
- Prevents leaking sensitive data from cluster
- Namespace isolation enforced
- No access to spec or status fields

### Expression Types

**Static vs Dynamic:**
- **Static fields**: Direct values (repo, package, labels, annotations)
- **Expression fields**: CEL expressions (repoExpr, packageExpr, labelExprs, annotationExprs)
- **Precedence**: Expression results override static values

**Expression evaluation examples:**

**Downstream repository:**
- Static: `downstream.repo: "prod-repo"`
- Dynamic: `downstream.repoExpr: "target.labels.environment + '-repo'"`

**Package naming:**
- Static: `downstream.package: "my-package"`
- Dynamic: `downstream.packageExpr: "upstream.name + '-' + target.name"`

**Label generation:**
- Static: `labels: {env: prod}`
- Dynamic: `labelExprs: [{keyExpr: "'cluster'", valueExpr: "target.name"}]`

**Package context injection:**
- Static: `packageContext.data: {key: value}`
- Dynamic: `packageContext.dataExprs: [{keyExpr: "'region'", valueExpr: "repository.labels.region"}]`

**Function configuration:**
- Static: `pipeline.mutators[0].configMap: {namespace: prod}`
- Dynamic: `pipeline.mutators[0].configMapExprs: [{keyExpr: "'namespace'", valueExpr: "target.labels.ns"}]`

### Map Expression Overlay

**Overlay pattern:**
```
Static Map + Expression Map = Result Map
        ↓
  Copy static entries
        ↓
  Evaluate each MapExpr
        ↓
  • Evaluate keyExpr or use key
  • Evaluate valueExpr or use value
        ↓
  Overlay onto result map
        ↓
  Return merged map
```

**Characteristics:**
- Static entries copied first
- Expression entries evaluated and overlaid
- Expression entries can override static entries
- Empty result returns nil (not empty map)

## Set-Based Reconciliation

The controller reconciles desired PackageVariants with existing PackageVariants:

### Reconciliation Strategy

```
Ensure PackageVariants
        ↓
  List Existing PackageVariants
        ↓
  • Filter by owner label
  • Build existingMap by identifier
        ↓
  For Each Downstream Target:
        ↓
    Render PackageVariant Spec
        ↓
    Generate Identifier
        ↓
    Create PackageVariant Object
        ↓
    Add to desiredMap
        ↓
  Compare existingMap vs desiredMap
        ↓
  ┌────┴────┬────────┬─────────┐
  ↓         ↓        ↓         ↓
Only     Both     Only      
Existing          Desired
  ↓         ↓        ↓
Delete   Update   Create
```

**Set operations:**
- **Existing only**: PackageVariant no longer needed → Delete
- **Both**: PackageVariant exists and needed → Update spec
- **Desired only**: PackageVariant needed but missing → Create

### Identifier Generation

**Identifier format:**
```
{pvsName}-{downstreamRepo}-{downstreamPackage}
```

**Example:**
- PVS name: `my-pvs`
- Downstream repo: `prod-repo`
- Downstream package: `my-package`
- Identifier: `my-pvs-prod-repo-my-package`

**Characteristics:**
- Stable across reconciliations
- Unique per downstream repo/package combination
- Used to match existing with desired PackageVariants

### Name Generation

**Name generation logic:**
```
Generate PackageVariant Name
        ↓
  Identifier length ≤ 63? ──Yes──> Use identifier as-is
        │
        No
        ↓
  Truncate and hash:
        ↓
  • Take first 54 characters
  • Compute SHA1 hash of full identifier
  • Take first 8 hex characters of hash
  • Format: {identifier[:54]}-{hash[:8]}
```

**Rationale:**
- Kubernetes names limited to 63 characters
- Long identifiers truncated with hash suffix
- Hash ensures uniqueness even after truncation
- Stable names across reconciliations

**Example:**
- Short identifier: `my-pvs-prod-repo-my-package` (28 chars) → Used as-is
- Long identifier: `very-long-packagevariantset-name-very-long-repo-name-very-long-package-name` (76 chars) → `very-long-packagevariantset-name-very-long-repo-name-v-a1b2c3d4`

### PackageVariant Creation

**Generated PackageVariant structure:**
```
PackageVariant:
  ObjectMeta:
    Name: Generated from identifier
    Namespace: Same as PackageVariantSet
    Labels:
      config.porch.kpt.dev/packagevariantset: {PVS UID}
    OwnerReferences:
      - PackageVariantSet (controller=true)
    Finalizers:
      - config.porch.kpt.dev/packagevariants
  Spec:
    Upstream: From PackageVariantSet
    Downstream: From template evaluation
    AdoptionPolicy: From template
    DeletionPolicy: From template
    Labels: From template evaluation
    Annotations: From template evaluation
    PackageContext: From template evaluation
    Pipeline: From template evaluation
    Injectors: From template evaluation
```

**Owner reference characteristics:**
- PackageVariantSet is controller owner
- Enables garbage collection
- Cascading deletion when PackageVariantSet deleted
- Label for efficient querying

### Update vs Create Logic

**Update existing PackageVariant:**
```
PackageVariant exists in both sets
        ↓
  Copy existing ObjectMeta
        ↓
  Replace Spec with desired Spec
        ↓
  Update via Kubernetes API
        ↓
  Return
```

**Update characteristics:**
- Only spec is updated
- Metadata (labels, annotations) preserved
- Owner references unchanged
- Finalizers unchanged

**Create new PackageVariant:**
```
PackageVariant only in desired set
        ↓
  Create full PackageVariant object
        ↓
  • Set owner reference
  • Set label for tracking
  • Set finalizer
        ↓
  Create via Kubernetes API
        ↓
  Return
```

**Delete obsolete PackageVariant:**
```
PackageVariant only in existing set
        ↓
  Delete via Kubernetes API
        ↓
  PackageVariant controller handles cleanup
        ↓
  Downstream packages handled per PV policy
```

**Deletion characteristics:**
- No longer in desired set
- Deleted via Kubernetes API
- PackageVariant controller handles cleanup
- Downstream packages handled per PackageVariant deletion policy

## Validation

The controller validates PackageVariantSet specs before processing:

### Validation Flow

```
Validate PackageVariantSet
        ↓
  Check Upstream
        ↓
  • Package specified?
  • Repo specified?
  • Revision or WorkspaceName specified?
        ↓
  Check Targets
        ↓
  • At least one target?
  • For each target:
    - Exactly one selector type?
    - Repository list not empty?
    - Repository names not empty?
    - Package names not empty?
    - ObjectSelector has APIVersion/Kind?
        ↓
  Check Template (if present)
        ↓
  • AdoptionPolicy valid?
  • DeletionPolicy valid?
  • Not both Repo and RepoExpr?
  • Not both Package and PackageExpr?
  • MapExpr not both Key and KeyExpr?
  • MapExpr not both Value and ValueExpr?
  • Injector has Name or NameExpr?
  • Function image not empty?
  • Function name no dots?
        ↓
  Return all errors
```

**Validation categories:**

**Upstream validation:**
- Upstream field must be present
- Package must be specified
- Repo must be specified
- Either Revision or WorkspaceName must be specified

**Targets validation:**
- At least one target must be specified
- Each target must specify exactly one of: Repositories, RepositorySelector, or ObjectSelector
- Repository list must not be empty if specified
- Repository names cannot be empty
- Package names cannot be empty
- ObjectSelector must have APIVersion and Kind

**Template validation:**
- AdoptionPolicy must be "adoptNone" or "adoptExisting"
- DeletionPolicy must be "delete" or "orphan"
- Cannot specify both Repo and RepoExpr in Downstream
- Cannot specify both Package and PackageExpr in Downstream
- Cannot specify both Key and KeyExpr in MapExpr
- Cannot specify both Value and ValueExpr in MapExpr
- Cannot specify both Name and NameExpr in Injectors
- Must specify either Name or NameExpr in Injectors
- Function image must not be empty
- Function name must not contain dots

**Validation failures:**
- Set Condition: Stalled=True, Ready=False
- Do NOT requeue (requires PackageVariantSet change)
- Error message includes all validation errors combined

## Error Handling

The controller handles errors at multiple stages:

### Validation Errors

```
Validate Spec
        ↓
  Errors Found? ──Yes──> Set Conditions
        │                      ↓
        No                Stalled=True
        ↓                 Ready=False
  Continue                     ↓
                          Do NOT Requeue
```

**Validation failures:**
- Set Stalled condition to True with error message
- Set Ready condition to False
- Do NOT requeue (requires PackageVariantSet spec change)
- Error message includes all validation errors combined

### Upstream Not Found Errors

```
Find Upstream PR
        ↓
  Not Found? ──Yes──> Set Conditions
        │                   ↓
        No              Stalled=True
        ↓               Ready=False
  Continue                  ↓
                       Do NOT Requeue
```

**Upstream not found:**
- Set Stalled condition to True with reason "UpstreamNotFound"
- Set Ready condition to False
- Do NOT requeue (watch will trigger when upstream appears)
- Watch on PackageRevisions triggers reconciliation when upstream appears

### Target Unroll Errors

```
Unroll Downstream Targets
        ↓
  Error? ──Yes──> Check Error Type
        │               ↓
        No        NoMatchError? ──Yes──> Stalled=True, "NoMatchingTargets"
        ↓               │
  Continue              No
                        ↓
                  Stalled=True, "UnexpectedError"
```

**Target unroll failures:**
- **NoMatchError**: CRD not found for ObjectSelector
  - Set Stalled=True with reason "NoMatchingTargets"
  - Do NOT requeue (requires CRD installation)
- **Other errors**: Unexpected errors
  - Set Stalled=True with reason "UnexpectedError"
  - Do NOT requeue (may require investigation)

### Reconciliation Errors

```
Ensure PackageVariants
        ↓
  Error? ──Yes──> Set Conditions
        │               ↓
        No          Stalled=True
        ↓           Ready=False
  Success               ↓
        ↓          Do NOT Requeue
  Ready=True
  Stalled=False
```

**Reconciliation failures:**
- Set Stalled condition to True with reason "UnexpectedError"
- Set Ready condition to False
- Do NOT requeue (may be transient, watch will trigger)
- Examples: API errors, network issues, Porch unavailable

### CEL Evaluation Errors

```
Evaluate CEL Expression
        ↓
  Error? ──Yes──> Return Error
        │               ↓
        No        Include field context
        ↓               ↓
  Continue        "template.downstream.repoExpr: {error}"
```

**CEL evaluation failures:**
- Returned during template rendering
- Include expression field name in error
- Include original CEL error details
- Set Stalled=True condition
- Requires expression fix

## Condition Management

**Condition types:**

**Stalled condition:**
- **True**: Validation error, upstream not found, or target unroll error (no progress possible)
- **False**: All validation passed, upstream found, targets unrolled (can make progress)
- **Reasons**: "ValidationError", "UpstreamNotFound", "NoMatchingTargets", "UnexpectedError", "Valid"

**Ready condition:**
- **True**: Successfully ensured all PackageVariants
- **False**: Error during reconciliation
- **Reasons**: "Reconciled", "UnexpectedError"

**Status update:**
- Deferred to end of reconciliation
- Updated even if reconciliation fails
- Provides visibility into controller state
- Used by users and automation to monitor progress

## Watch Configuration

The controller watches multiple resource types:

### Watch Setup

```
Controller Manager
        ↓
  Set up Watches
        ↓
  ┌─────┴─────┬─────────────┐
  ↓           ↓             ↓
Primary    Secondary    Secondary
Watch      Watch        Watch
  ↓           ↓             ↓
PackageVariantSet  PackageVariant  PackageRevision
```

**Watch configuration:**
- **Primary watch**: PackageVariantSet CRs (main resource being reconciled)
- **Secondary watch**: PackageVariant CRs (triggers reconciliation when child changes)
- **Secondary watch**: PackageRevision CRs (triggers reconciliation when upstream changes)

### Secondary Watch Mapping

**Mapping function:**
```
PackageVariant or PackageRevision Change
        ↓
  Map Function
        ↓
  List All PackageVariantSets
        ↓
  In Same Namespace
        ↓
  Enqueue All
        ↓
  Each PVS Reconciled
```

**Mapping rationale:**
- Simple implementation (no complex filtering)
- Ensures all PackageVariantSets see child changes
- Ensures all PackageVariantSets see upstream changes
- Acceptable performance for typical scale (hundreds of resources)
- Trade-off: More reconciliations, simpler logic

**Implications:**
- Broad watches ensure consistency
- May trigger unnecessary reconciliations
- Guarantees correctness over efficiency
- Suitable for typical deployment scales

## Design Patterns

### Fan-Out Pattern

```
Single Upstream Package
        ↓
  PackageVariantSet
        ↓
  Target Unrolling
        ↓
  ┌─────┴─────┬─────────┬─────────┐
  ↓           ↓         ↓         ↓
Target1    Target2   Target3   TargetN
  ↓           ↓         ↓         ↓
  └─────┬─────┴─────────┴─────────┘
        ↓
  Template Evaluation
        ↓
  ┌─────┴─────┬─────────┬─────────┐
  ↓           ↓         ↓         ↓
PV1         PV2       PV3       PVN
  ↓           ↓         ↓         ↓
  └─────┬─────┴─────────┴─────────┘
        ↓
Multiple Downstream Packages
```

**Pattern characteristics:**
- One-to-many relationship
- Declarative targeting
- Template-based customization
- Automatic synchronization

### Template Pattern

**Static + Dynamic configuration:**
- Static values provide defaults
- Dynamic expressions override defaults
- Expressions evaluated per-target
- Context-aware customization

**Expression precedence:**
- Static fields evaluated first
- Expression fields evaluated second
- Expression results override static values
- Enables flexible configuration

### Set-Based Reconciliation

**Desired state computation:**
- Compute complete desired set
- Compare with existing set
- Apply minimal changes
- Idempotent operations

**Benefits:**
- Declarative approach
- Handles additions, updates, deletions
- Consistent with Kubernetes patterns
- Easy to reason about

## Integration with PackageVariant Controller

The PackageVariantSet controller creates PackageVariant CRs, which are then reconciled by the PackageVariant controller:

### Two-Level Hierarchy

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

**Separation of concerns:**
- **PackageVariantSet**: Bulk creation, target selection, template evaluation
- **PackageVariant**: Individual package management, upstream tracking, mutation application
- **Clear boundaries**: Each controller has distinct responsibilities

**Reconciliation flow:**
```
PackageVariantSet Reconcile
        ↓
  Create/Update/Delete PackageVariant CRs
        ↓
PackageVariant Controller Watches
        ↓
  Reconcile Each PackageVariant
        ↓
  Create/Update PackageRevisions
```
