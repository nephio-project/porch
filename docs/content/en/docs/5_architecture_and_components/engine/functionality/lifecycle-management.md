---
title: "Lifecycle Management"
type: docs
weight: 3
description: |
  Detailed architecture of package lifecycle state machine and enforcement.
---

## Overview

The Engine enforces a strict lifecycle state machine for package revisions that governs their mutability, visibility, and progression from draft to published state. The lifecycle system ensures that packages follow a controlled workflow from creation through approval to deployment, with appropriate constraints at each stage.

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Lifecycle Management System                │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │  State Machine   │      │   Validation     │         │
│  │                  │ ───> │   & Enforcement  │         │
│  │  • Draft         │      │                  │         │
│  │  • Proposed      │      │  • Creation      │         │
│  │  • Published     │      │  • Transition    │         │
│  │  • Deletion      │      │  • Mutation      │         │
│  └──────────────────┘      └──────────────────┘         │
│           │                         │                   │
│           └────────┬────────────────┘                   │
│                    ↓                                    │
│          ┌──────────────────┐                           │
│          │  State-Based     │                           │
│          │  Constraints     │                           │
│          │                  │                           │
│          │  • Mutability    │                           │
│          │  • Operations    │                           │
│          │  • Audit Trail   │                           │
│          └──────────────────┘                           │
└─────────────────────────────────────────────────────────┘
```

## Lifecycle State Machine

The lifecycle state machine defines four states that a package revision can be in:

### State Definitions

![Package Lifecycle Workflow](/static/images/porch/flowchart.drawio.svg)

**Draft:**
- Initial state for newly created package revisions
- Fully mutable - all operations allowed
- Work-in-progress state
- Can be modified, updated, or deleted freely
- Default state when no lifecycle specified

**Proposed:**
- Indicates package is ready for review and approval
- Still mutable like Draft
- Signals intent to publish
- Used in approval workflows and GitOps processes
- Can be reverted to Draft for rework

**Published:**
- **Immutable content** - no task or resource modifications allowed
- Production-ready, deployed state
- Only metadata and lifecycle can be updated
- Represents approved, stable package revision
- Cannot revert to Draft or Proposed

**DeletionProposed:**
- Marks package for removal
- Considered "published" for lifecycle checks
- Can revert to Published if deletion cancelled
- Final state before actual deletion
- Maintains audit trail during deletion process

### Published State Definition

The engine treats both Published and DeletionProposed states as "published" for lifecycle checks. A package is considered published if its lifecycle is either Published or DeletionProposed.

**Why DeletionProposed is considered published:**
- Maintains immutability during deletion process
- Prevents modifications to packages being removed
- Ensures consistency in dependency checks
- Preserves audit trail until final deletion

## State Transitions

The lifecycle state machine enforces specific allowed transitions:

### Allowed Transitions

```
Draft ──────────────────> Proposed
  │                          │
  │                          │
  └──────────> Published <───┘
                   │
                   ↓
            DeletionProposed
                   │
                   ↓
            [Actual Deletion]
```

**Forward transitions:**
- Draft → Proposed (submit for review)
- Draft → Published (direct publish, skip review)
- Proposed → Published (approve)
- Published → DeletionProposed (mark for deletion)

**Backward transitions:**
- Proposed → Draft (return for rework)
- DeletionProposed → Published (cancel deletion)

**Forbidden transitions:**
- Published → Draft (cannot unpublish)
- Published → Proposed (cannot unpublish)
- Any state → Draft (except from Proposed)

### Transition Validation

**Creation validation:**

```
CreatePackageRevision
        ↓
  Check Lifecycle
        ↓
  Empty? ──Yes──> Set to Draft
        │
        No
        ↓
  Draft/Proposed? ──Yes──> Allow
        │
        No
        ↓
  Published/DeletionProposed? ──Yes──> Reject
```

**Process:**
1. **Empty lifecycle**: Defaults to Draft
2. **Draft or Proposed**: Allowed for creation
3. **Published or DeletionProposed**: Rejected with error
4. **Invalid value**: Rejected with error

**Update validation:**

```
UpdatePackageRevision
        ↓
  Check Old Lifecycle
        ↓
  Draft/Proposed? ──Yes──> Allow Full Update
        │
        No
        ↓
  Published/DeletionProposed? ──Yes──> Metadata Only
        ↓
  Check New Lifecycle
        ↓
  Valid Transition? ──Yes──> Apply
        │
        No
        ↓
  Reject
```

**Process:**
1. **Check current state**: Determines allowed operations
2. **Draft/Proposed**: Full update workflow (draft-commit)
3. **Published/DeletionProposed**: Metadata-only update
4. **Validate new lifecycle**: Ensure transition is valid
5. **Apply or reject**: Based on validation results

## State-Based Constraints

Each lifecycle state has different constraints on what operations are allowed:

### Mutability Matrix

| Operation | Draft | Proposed | Published | DeletionProposed |
|-----------|-------|----------|-----------|------------------|
| Add Tasks | ✓ | ✓ | ✗ | ✗ |
| Update Res | ✓ | ✓ | ✗ | ✗ |
| Update Meta | ✓ | ✓ | ✓ | ✓ |
| Update Life | ✓ | ✓ | ✓* | ✓* |
| Delete | ✓ | ✓ | ✓** | ✓** |

\* Only to DeletionProposed (Published) or Published (DeletionProposed)  
\*\* Typically transitions to DeletionProposed first

### Draft State Constraints

**Allowed operations:**
- Add/modify tasks
- Update package resources
- Update metadata (labels, annotations, finalizers)
- Change lifecycle to Proposed or Published
- Delete package revision

**Workflow:**
```
Draft Update Request
        ↓
  Open Draft
        ↓
  Apply Mutations
        ↓
  Update Lifecycle
        ↓
  Close Draft
        ↓
  Return Updated PR
```

### Proposed State Constraints

**Allowed operations:**
- Same as Draft state
- Signals readiness for approval
- Can revert to Draft
- Can advance to Published

**Workflow:**
```
Proposed Update Request
        ↓
  Open Draft
        ↓
  Apply Mutations
        ↓
  Update Lifecycle
        ↓
  Close Draft
        ↓
  Return Updated PR
```

**Note:** Draft and Proposed have identical mutability - the difference is semantic (intent to publish).

### Published State Constraints

**Allowed operations:**
- Update metadata only (labels, annotations, finalizers, owner references)
- Change lifecycle to DeletionProposed
- Delete (typically after transitioning to DeletionProposed)

**Forbidden operations:**
- Add or modify tasks
- Update package resources
- Change lifecycle to Draft or Proposed

**Workflow:**
```
Published Update Request
        ↓
  Check Operation Type
        ↓
  Metadata? ──Yes──> Update Directly
        │
        No
        ↓
  Lifecycle? ──Yes──> Validate Transition
        │
        No
        ↓
  Reject (Immutable)
```

### DeletionProposed State Constraints

**Allowed operations:**
- Update metadata (labels, annotations, finalizers)
- Change lifecycle to Published (cancel deletion)
- Delete package revision

**Forbidden operations:**
- Add or modify tasks
- Update package resources
- Change lifecycle to Draft or Proposed

**Workflow:**
```
DeletionProposed Update
        ↓
  Check Operation Type
        ↓
  Metadata? ──Yes──> Update Directly
        │
        No
        ↓
  Lifecycle to Published? ──Yes──> Allow
        │
        No
        ↓
  Reject (Immutable)
```

## Validation and Enforcement

The engine enforces lifecycle rules through multiple validation layers:

### Creation Validation

```
CreatePackageRevision
        ↓
  Validate Lifecycle
        ↓
  Empty? ──Yes──> Default to Draft
        │
        No
        ↓
  Draft/Proposed? ──Yes──> Continue
        │
        No
        ↓
  Return Error
```

**Validation checks:**
1. **Lifecycle value**: Must be empty, Draft, or Proposed
2. **Cannot create Published**: Packages must progress through Draft/Proposed
3. **Cannot create DeletionProposed**: Invalid initial state
4. **Default to Draft**: If no lifecycle specified

**Error messages:**
- "cannot create a package revision with lifecycle value 'Final'" (Published/DeletionProposed)
- "unsupported lifecycle value: {value}" (invalid value)

### Update Validation

```
UpdatePackageRevision
        ↓
  Check Current Lifecycle
        ↓
  Draft/Proposed? ──Yes──> Full Update Path
        │
        No
        ↓
  Published/DeletionProposed? ──Yes──> Metadata Only Path
        │
        No
        ↓
  Return Error
```

**Validation checks:**
1. **Current lifecycle**: Determines allowed operations
2. **New lifecycle**: Must be valid value
3. **Transition validity**: Checked against allowed transitions
4. **Resource version**: Optimistic locking check

**Error messages:**
- "invalid original lifecycle value: {value}"
- "invalid desired lifecycle value: {value}"
- Optimistic lock error if resource version mismatch

### Resource Update Validation

```
UpdatePackageResources
        ↓
  Check Lifecycle
        ↓
  Draft? ──Yes──> Allow Update
        │
        No
        ↓
  Proposed/Published/DeletionProposed? ──Yes──> Reject
```

**Validation checks:**
1. **Must be Draft**: Only Draft packages can have resources updated
2. **Resource version**: Optimistic locking check
3. **Proposed rejected**: Even though mutable, explicit resource updates not allowed
4. **Published rejected**: Immutable content

**Error messages:**
- "cannot update a package revision with lifecycle value {value}; package must be Draft"
- Optimistic lock error if resource version mismatch

### Optimistic Locking

```
Update Request
        ↓
  Extract Resource Version
        ↓
  Empty? ──Yes──> Reject
        │
        No
        ↓
  Compare with Current
        ↓
  Match? ──Yes──> Proceed
        │
        No
        ↓
  Return Conflict Error
```

**Locking mechanism:**
1. **Resource version required**: All updates must provide current version
2. **Version comparison**: New version must match current version
3. **Conflict on mismatch**: Returns HTTP 409 Conflict
4. **Client retry**: Client must re-read and retry with latest version

**Error message:**
- "the object has been modified; please apply your changes to the latest version and try again"

### Finalizer Handling

```
Update Request
        ↓
  Check Deletion Timestamp
        ↓
  Set? ──No──> Normal Update
        │
       Yes
        ↓
  Check Finalizers
        ↓
  Empty? ──Yes──> Delete Instead
        │
        No
        ↓
  Normal Update
```

**Finalizer behavior:**
1. **Deletion timestamp set**: Package in terminating state
2. **Last finalizer removed**: Triggers actual deletion
3. **Finalizers remain**: Update proceeds normally
4. **Metadata updated**: Finalizers stored in metadata

## Audit and Tracking

The lifecycle system maintains an audit trail of package evolution:

### Audit Fields

**PublishedBy:**
- Records user who published the package
- Set when lifecycle transitions to Published
- Extracted from Kubernetes request context
- Stored in PackageRevision status

**PublishedAt:**
- Timestamp when package was published
- Set when lifecycle transitions to Published
- Stored in PackageRevision status
- Used for tracking approval timing

**Tasks:**
- Append-only list of operations performed
- Each task represents a modification
- First task indicates creation method (init, clone, edit, upgrade)
- Subsequent tasks show evolution history
- Stored in PackageRevision spec

**Resource Version:**
- Kubernetes resource version for optimistic locking
- Incremented on each update
- Used to prevent concurrent modification conflicts
- Managed by Kubernetes API server

### Audit Trail Flow

```
Package Creation
        ↓
  Task: Init/Clone/Edit
        ↓
  Lifecycle: Draft
        ↓
Modifications
        ↓
  Tasks: [Additional tasks]
        ↓
  Lifecycle: Proposed
        ↓
Approval
        ↓
  Lifecycle: Published
  PublishedBy: user@example.com
  PublishedAt: 2025-01-27T10:00:00Z
        ↓
Deletion Request
        ↓
  Lifecycle: DeletionProposed
        ↓
Actual Deletion
        ↓
  Package Removed
```

### Tracking Benefits

**Compliance:**
- Who approved packages for production
- When packages were approved
- What changes were made

**Debugging:**
- Package evolution history
- Task execution sequence
- Lifecycle transition timeline

**Rollback:**
- Identify previous stable versions
- Understand changes between versions
- Revert to known-good states

**Governance:**
- Enforce approval workflows
- Audit package modifications
- Track package lineage
