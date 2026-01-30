---
title: "Validation & Business Rules"
type: docs
weight: 2
description: |
  Detailed architecture of validation and business rule enforcement in the Engine.
---

## Overview

The Engine enforces validation and business rules to ensure package revisions are created and modified correctly. These rules prevent invalid operations, enforce naming constraints, and maintain referential integrity across packages and revisions.

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│            Validation & Business Rules                  │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │   Pre-Operation  │      │   Constraint     │         │
│  │   Validation     │ ───> │   Enforcement    │         │
│  │                  │      │                  │         │
│  │  • Lifecycle     │      │  • Uniqueness    │         │
│  │  • Tasks         │      │  • Path Overlap  │         │
│  │  • Resources     │      │  • Clone Rules   │         │
│  └──────────────────┘      └──────────────────┘         │
│           │                         │                   │
│           └────────┬────────────────┘                   │
│                    ↓                                    │
│          ┌──────────────────┐                           │
│          │   Optimistic     │                           │
│          │    Locking       │                           │
│          │                  │                           │
│          │  • Resource Ver  │                           │
│          │  • Conflict Det  │                           │
│          └──────────────────┘                           │
└─────────────────────────────────────────────────────────┘
```

## Lifecycle Validation

The Engine validates lifecycle values during package revision creation and updates:

### Creation Lifecycle Validation

```
CreatePackageRevision
        ↓
  Check Lifecycle Value
        ↓
  Empty? ──Yes──> Default to Draft
        │
        No
        ↓
  Draft/Proposed? ──Yes──> Allow
        │
        No
        ↓
  Published/DeletionProposed? ──Yes──> Reject
        │
        No
        ↓
  Invalid Value? ──Yes──> Reject
```

**Validation rules:**
- **Empty lifecycle**: Defaults to Draft (most common case)
- **Draft allowed**: New package revisions can be created as Draft
- **Proposed allowed**: New package revisions can be created as Proposed
- **Published forbidden**: Cannot create package revisions directly in Published state
- **DeletionProposed forbidden**: Cannot create package revisions in DeletionProposed state
- **Invalid values**: Any other lifecycle value is rejected

**Error messages:**
- "cannot create a package revision with lifecycle value 'Final'" (for Published/DeletionProposed)
- "unsupported lifecycle value: {value}" (for invalid values)

**Rationale:**
- Packages must progress through Draft/Proposed before being published
- Prevents bypassing review/approval workflows
- Ensures all packages have a draft history

### Update Lifecycle Validation

```
UpdatePackageRevision
        ↓
  Check Current Lifecycle
        ↓
  Draft/Proposed? ──Yes──> Allow Full Update
        │
        No
        ↓
  Published/DeletionProposed? ──Yes──> Metadata Only
        │
        No
        ↓
  Invalid Value? ──Yes──> Reject
```

**Validation rules:**
- **Draft/Proposed**: Full updates allowed (tasks, resources, lifecycle, metadata)
- **Published/DeletionProposed**: Only metadata and lifecycle updates allowed
- **Invalid current lifecycle**: Operation rejected

**Error messages:**
- "invalid original lifecycle value: {value}"
- "invalid desired lifecycle value: {value}"

**Rationale:**
- Published packages are immutable (content cannot change)
- Draft/Proposed packages are mutable (work-in-progress)
- Lifecycle transitions must follow state machine rules (see [Lifecycle Management](lifecycle-management))

## Task Validation

The Engine validates tasks during package revision creation:

### Task Count Validation

```
CreatePackageRevision
        ↓
  Check Task Count
        ↓
  Count > 1? ──Yes──> Reject
        │
        No
        ↓
  Count == 0? ──Yes──> Default to Init
        │
        No
        ↓
  Count == 1? ──Yes──> Validate Task Type
```

**Validation rules:**
- **Maximum one task**: Only one task allowed during creation
- **Default task**: If no task specified, defaults to `init` task
- **Task type validation**: Task type must be valid (init, clone, edit, upgrade)

**Error messages:**
- "task list must not contain more than one task"

**Rationale:**
- Simplifies creation workflow (one operation at a time)
- Multiple tasks can be added later via updates
- Default init task provides sensible starting point

### Task Type Validation

**Valid task types:**
- **init**: Create new package from scratch
- **clone**: Copy package from upstream
- **edit**: Create new revision from existing package
- **upgrade**: Merge changes from new upstream version

**Task-specific validation:**
- Each task type has additional validation rules
- Validation delegated to task-specific validators
- See sections below for details

## Workspace Name Uniqueness

The Engine enforces workspace name uniqueness within a package:

### Uniqueness Check

```
CreatePackageRevision
        ↓
  List Existing Revisions
        ↓
  For Each Revision:
        ↓
    Same Package? ──No──> Continue
        │
       Yes
        ↓
    Same Workspace? ──Yes──> Reject
        │
        No
        ↓
  Continue
        ↓
  Allow Creation
```

**Validation process:**
1. **List all revisions** for the same package in the repository
2. **Check workspace names** of existing revisions
3. **Reject if duplicate** workspace name found
4. **Allow if unique** workspace name

**Error message:**
- "package revision workspaceNames must be unique; package revision with name {name} in repo {repo} with workspaceName {workspace} already exists"

**Rationale:**
- Workspace name used to generate Kubernetes object name
- Kubernetes object names must be unique within namespace
- Prevents naming conflicts and confusion

### Workspace Name Validation

**Additional validation:**
- Workspace name must be valid Kubernetes name
- Combined with repository and package to form object name
- Format: `{repo}-{path}-{package}-{workspace}`

## Clone Task Validation

The Engine validates clone tasks to prevent creating duplicate packages:

### Clone Constraint Check

```
Clone Task Validation
        ↓
  List Existing Revisions
        ↓
  For Each Revision:
        ↓
    Same Package? ──Yes──> Reject
        │
        No
        ↓
  Continue
        ↓
  Allow Clone
```

**Validation process:**
1. **List all revisions** in the repository
2. **Check for existing package** with same name
3. **Reject if package exists** (clone can only create new packages)
4. **Allow if package doesn't exist**

**Error message:**
- "`clone` cannot create a new revision for package {package} that already exists in repo {repo}; make subsequent revisions using `copy`"

**Rationale:**
- Clone is for creating NEW packages from upstream
- Existing packages should use `edit` or `copy` for new revisions
- Prevents accidental overwriting of existing packages

### Clone vs Edit/Copy

**When to use clone:**
- Creating a new package from upstream source
- Package doesn't exist in target repository
- First time bringing package into repository

**When to use edit/copy:**
- Creating new revision of existing package
- Package already exists in repository
- Iterating on existing package

## Upgrade Task Validation

The Engine validates upgrade tasks to ensure source revisions are published:

### Upgrade Source Validation

```
Upgrade Task Validation
        ↓
  Extract Source Revisions
        ↓
  • OldUpstream
  • NewUpstream
  • LocalPackageRevision
        ↓
  For Each Source:
        ↓
    Find Revision
        ↓
    Published? ──No──> Reject
        │
       Yes
        ↓
  Continue
        ↓
  Allow Upgrade
```

**Validation process:**
1. **Extract source revision references** from upgrade task spec:
   - OldUpstream: Previous upstream version
   - NewUpstream: New upstream version to upgrade to
   - LocalPackageRevision: Current local package revision
2. **Find each source revision** in repository
3. **Check lifecycle state** of each source
4. **Reject if any source not published**
5. **Allow if all sources published**

**Error message:**
- "all source PackageRevisions of upgrade task must be published, {name} is not"

**Rationale:**
- Upgrade performs three-way merge (old upstream, new upstream, local)
- Source revisions must be stable (published) for reliable merge
- Prevents upgrading from unstable draft versions

### Upgrade Source Requirements

**Required sources:**
- **OldUpstream**: The upstream version currently used
- **NewUpstream**: The upstream version to upgrade to
- **LocalPackageRevision**: The local package revision to upgrade

**All sources must be:**
- Published lifecycle state
- Accessible in repository
- Valid package revisions

## Package Path Overlap Validation

The Engine validates package paths to prevent nested packages:

### Path Overlap Check

```
CreatePackageRevision (init/clone)
        ↓
  Check if Package Creation
        ↓
  Yes? ──No──> Skip Check
        │
       Yes
        ↓
  List All Revisions
        ↓
  Check Path Overlaps
        ↓
  Overlapping? ──Yes──> Reject
        │
        No
        ↓
  Allow Creation
```

**Validation process:**
1. **Determine if package creation** (init or clone task)
2. **List all package revisions** in repository
3. **Check for path overlaps** with new package path
4. **Reject if overlap detected**
5. **Allow if no overlap**

**Path overlap rules:**
- Package path cannot be parent of existing package
- Package path cannot be child of existing package
- Prevents ambiguous package boundaries

**Example overlaps (rejected):**
- New: `networking/vpc`, Existing: `networking/vpc/subnets` (parent)
- New: `networking/vpc/subnets`, Existing: `networking/vpc` (child)

**Valid paths (allowed):**
- New: `networking/vpc`, Existing: `networking/firewall` (siblings)
- New: `apps/frontend`, Existing: `apps/backend` (siblings)

**Rationale:**
- Prevents nested package structures
- Maintains clear package boundaries
- Avoids confusion about package ownership

## Optimistic Locking

The Engine uses optimistic locking to prevent concurrent modification conflicts:

### Resource Version Check

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

**Validation process:**
1. **Extract resource version** from update request
2. **Reject if empty** (resource version required)
3. **Compare with current version** in repository
4. **Proceed if match** (no concurrent modification)
5. **Return conflict if mismatch** (concurrent modification detected)

**Error message:**
- "the object has been modified; please apply your changes to the latest version and try again"

**HTTP status:**
- 409 Conflict (when resource version mismatch)

### Optimistic Locking Flow

```
Client A                      Engine                    Client B
   ↓                             ↓                          ↓
Read PR (v1)                     ↓                      Read PR (v1)
   ↓                             ↓                          ↓
Modify Locally                   ↓                   Modify Locally
   ↓                             ↓                          ↓
Update (v1)  ──────────> Check Version (v1 == v1)           ↓
   ↓                             ↓                          ↓
   ↓                        Apply Update                    ↓
   ↓                             ↓                          ↓
   ↓                        Increment to v2                 ↓
   ↓                             ↓                          ↓
Success (v2) <────────────  Return Success                  ↓
   ↓                             ↓                          ↓
   ↓                             ↓                   Update (v1) ───>
   ↓                             ↓                          ↓
   ↓                        Check Version (v1 != v2)        ↓
   ↓                             ↓                          ↓
   ↓                        Return Conflict ─────────> Conflict!
   ↓                             ↓                          ↓
   ↓                             ↓                      Re-read (v2)
   ↓                             ↓                          ↓
   ↓                             ↓                   Reapply Changes
   ↓                             ↓                          ↓
   ↓                             ↓                   Retry Update (v2)
```

**Conflict resolution:**
1. **Client receives conflict error**
2. **Client re-reads latest version**
3. **Client reapplies changes** to latest version
4. **Client retries update** with new resource version

### Resource Version Management

**Resource version characteristics:**
- Managed by Kubernetes API server
- Opaque string (typically integer)
- Incremented on each update
- Used for optimistic concurrency control

**When resource version checked:**
- UpdatePackageRevision operations
- UpdatePackageResources operations
- Any operation that modifies package revision

**When resource version NOT checked:**
- CreatePackageRevision (new object, no version)
- DeletePackageRevision (deletion doesn't need version check)
- ListPackageRevisions (read-only operation)

## Validation Timing

The Engine performs validation at specific points in the operation lifecycle:

### Pre-Operation Validation

```
API Request
     ↓
Parse Request
     ↓
Validate Inputs
     ↓
  Valid? ──No──> Return Error (400 Bad Request)
     │
    Yes
     ↓
Open Repository
     ↓
Execute Operation
```

**Validation before operation:**
- Lifecycle values
- Task count and types
- Resource version presence
- Input format and structure

**Benefits:**
- Fail fast (before expensive operations)
- Clear error messages
- No side effects on failure

### Mid-Operation Validation

```
Operation Started
     ↓
Open Draft
     ↓
Validate Constraints
     ↓
  Valid? ──No──> Rollback + Return Error
     │
    Yes
     ↓
Apply Changes
     ↓
Close Draft
```

**Validation during operation:**
- Workspace name uniqueness
- Clone constraints
- Upgrade source states
- Package path overlaps

**Benefits:**
- Access to repository state
- Can check against existing data
- Rollback mechanism available

### Post-Operation Validation

**Validation after operation:**
- Currently minimal post-operation validation
- Repository adapters may perform additional checks
- Cache consistency checks

## Error Handling

The Engine returns specific errors for validation failures:

### Error Types

**Bad Request (400):**
- Invalid lifecycle values
- Invalid task types
- Missing required fields
- Malformed input

**Conflict (409):**
- Resource version mismatch (optimistic locking)
- Workspace name conflicts
- Package path overlaps

**Unprocessable Entity (422):**
- Clone task on existing package
- Upgrade task with unpublished sources
- Business rule violations

### Error Messages

**Error message format:**
- Clear description of what failed
- Context (package name, repository, etc.)
- Suggestion for resolution when applicable

**Examples:**
- "cannot create a package revision with lifecycle value 'Final'"
- "package revision workspaceNames must be unique; package revision with name {name} in repo {repo} with workspaceName {workspace} already exists"
- "`clone` cannot create a new revision for package {package} that already exists in repo {repo}; make subsequent revisions using `copy`"
- "all source PackageRevisions of upgrade task must be published, {name} is not"

## Validation Extension Points

The Engine's validation system can be extended:

### Future Validation Rules

**Potential additions:**
- Package naming conventions
- Resource size limits
- Dependency validation
- Security policy enforcement
- Custom admission webhooks

**Extension mechanisms:**
- Validation plugins
- Webhook integration
- Policy engine integration
- Custom validators

### Validation Configuration

**Currently:**
- Validation rules are hardcoded in Engine
- No configuration mechanism

**Future possibilities:**
- Configurable validation rules
- Repository-specific policies
- Organization-wide policies
- Validation rule versioning
