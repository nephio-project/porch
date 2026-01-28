---
title: "Package Transformation Tasks"
type: docs
weight: 1
description: |
  Detailed architecture of package transformation tasks (init, clone, edit, upgrade) and resource mutations.
---

## Overview

The Task Handler implements package transformation tasks that modify package content based on task type. Each task type has specific logic for creating or updating packages, and all tasks follow a common mutation pattern.

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│           Package Transformation System                 │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │  Task Types      │      │   Resource       │         │
│  │                  │      │   Mutations      │         │
│  │  • Init          │      │                  │         │
│  │  • Clone         │      │  • Replace       │         │
│  │  • Edit          │      │  • Heal Comments │         │
│  │  • Upgrade       │      │                  │         │
│  └──────────────────┘      └──────────────────┘         │
│           │                         │                   │
│           └────────┬────────────────┘                   │
│                    ↓                                    │
│          ┌──────────────────┐                           │
│          │    Mutation      │                           │
│          │    Interface     │                           │
│          │                  │                           │
│          │  apply(ctx,      │                           │
│          │    resources)    │                           │
│          └──────────────────┘                           │
└─────────────────────────────────────────────────────────┘
```

## Mutation Pattern

All package transformations implement a common mutation interface:

### Mutation Interface

All package transformations implement a common mutation interface:

**Interface contract:**
- **Input**: Accepts current package resources (may be empty for init/clone)
- **Output**: Returns transformed package resources and task result
- **Error**: Returns error if transformation fails
- **Stateless**: Pure transformation with no side effects

**Mutation implementations:**
- `initPackageMutation` - Creates new package from scratch
- `clonePackageMutation` - Copies package from upstream
- `editPackageMutation` - Creates revision from existing package
- `upgradePackageMutation` - Merges changes from new upstream
- `replaceResourcesMutation` - Replaces package resources

## Init Task

Creates a new package from scratch with Kptfile and README.

### Init Flow

```
Init Task Request
        ↓
  Create In-Memory FS
        ↓
  Initialize Package
        ↓
  • Create Kptfile
  • Create README
  • Create package-context (if subpackage)
        ↓
  Read Resources from FS
        ↓
  Return Package Resources
```

**Process:**
1. **Create in-memory filesystem** (virtual fs)
2. **Determine package path** (root or subpackage)
3. **Initialize package** with kpt initializer:
   - Create Kptfile with package metadata
   - Create README with description
   - Set keywords and site if provided
4. **Read resources** from filesystem
5. **Return** package resources + task result

**Init parameters:**
- **Package name**: Name for the package
- **Description**: Package description for README
- **Keywords**: Package keywords for Kptfile
- **Site**: Package site URL for Kptfile
- **Subpackage**: Optional subpackage path

**Generated files:**
- `Kptfile`: Package metadata and configuration
- `README.md`: Package documentation
- `package-context.yaml`: Context for subpackages (if applicable)

## Clone Task

Copies a package from an upstream source (registered repository, Git, or OCI).

### Clone Flow

```
Clone Task Request
        ↓
  Determine Source Type
        ↓
  ┌────┴────┬────────┬─────────┐
  ↓         ↓        ↓         ↓
Upstream  Git      OCI    (future)
  Ref
  ↓         ↓
Fetch     Clone
Package   Repo
  ↓         ↓
  └────┬────┴─────────────────┘
       ↓
  Get Package Resources
       ↓
  Update Kptfile Upstream
       ↓
  Deployment? ──Yes──> Generate Package Context
       │
       No
       ↓
  Ensure Merge Keys
       ↓
  Return Package Resources
```

**Process:**
1. **Determine source type** (upstreamRef, git, or oci)
2. **Fetch package** from source:
   - **UpstreamRef**: Fetch from registered repository via PackageFetcher
   - **Git**: Clone Git repository and extract package
   - **OCI**: (Not yet implemented)
3. **Get package resources** from fetched package
4. **Update Kptfile upstream** section with source information
5. **If deployment package**: Generate package-context.yaml
6. **Ensure merge keys** on resources (best effort)
7. **Return** package resources + task result

**Clone from registered repository:**
- Uses PackageFetcher to resolve and fetch package revision
- Retrieves upstream lock information
- Updates Kptfile with upstream and lock

**Clone from Git:**
- Creates temporary directory for clone
- Opens Git repository with credentials
- Fetches package at specified ref and directory
- Updates Kptfile with Git upstream information

**Clone parameters:**
- **Upstream source**: UpstreamRef, Git, or OCI specification
- **Package name**: Target package name
- **Is deployment**: Whether package is deployable instance
- **Package config**: Configuration for package-context generation

## Edit Task

Creates a new revision from an existing published package in the same repository.

### Edit Flow

```
Edit Task Request
        ↓
  Fetch Source Package
        ↓
  Validate Source
        ↓
  Same Package? ──No──> Reject
        │
       Yes
        ↓
  Published? ──No──> Reject
        │
       Yes
        ↓
  Get Package Resources
        ↓
  Return Package Resources
```

**Process:**
1. **Fetch source package** using PackageFetcher
2. **Validate source package**:
   - Must be from same package (same repository and package name)
   - Must be published (not draft or proposed)
3. **Get package resources** from source
4. **Return** package resources + task result

**Edit validation:**
- Source must be from same repository
- Source must be from same package
- Source must be published lifecycle state

**Error messages:**
- "source revision must be from same package {repo}/{package}"
- "source revision must be published"

**Use case:**
- Creating new revision of existing package
- Iterating on published package
- Making changes to stable package

## Upgrade Task

Performs a three-way merge to upgrade a package to a new upstream version.

### Upgrade Flow

```
Upgrade Task Request
        ↓
  Fetch Three Sources
        ↓
  • OldUpstream (previous version)
  • NewUpstream (target version)
  • LocalPackageRevision (current local)
        ↓
  Perform Three-Way Merge
        ↓
  • Base: OldUpstream
  • Theirs: NewUpstream
  • Ours: LocalPackageRevision
        ↓
  Update Kptfile Upstream
        ↓
  Ensure Merge Keys
        ↓
  Return Merged Resources
```

**Process:**
1. **Fetch three package revisions**:
   - **OldUpstream**: Previous upstream version (base)
   - **NewUpstream**: Target upstream version (theirs)
   - **LocalPackageRevision**: Current local package (ours)
2. **Perform three-way merge** using PackageUpdater:
   - Merge strategy: resource-merge or fast-forward
   - Resolves conflicts based on strategy
3. **Update Kptfile upstream** with new upstream lock
4. **Ensure merge keys** on resources (best effort)
5. **Return** merged resources + task result

**Three-way merge:**
- **Base**: OldUpstream (common ancestor)
- **Theirs**: NewUpstream (upstream changes)
- **Ours**: LocalPackageRevision (local changes)
- **Result**: Merged package with both upstream and local changes

**Merge strategies:**
- **resource-merge**: Merge resources field-by-field
- **fast-forward**: Replace with new upstream if no local changes

**Upgrade parameters:**
- **OldUpstream**: Previous upstream package revision reference
- **NewUpstream**: Target upstream package revision reference
- **LocalPackageRevision**: Current local package revision reference
- **Strategy**: Merge strategy (resource-merge or fast-forward)

## Resource Mutations

Replaces package resources with new content while preserving comments.

### Replace Resources Flow

```
Replace Resources Request
        ↓
  Heal Config
        ↓
  Read Old Resources
        ↓
  Read New Resources
        ↓
  Copy Comments
        ↓
  • Match by namespace/name/kind/apiVersion
  • Skip ytt templates (#@, #!)
        ↓
  Write Healed Resources
        ↓
  Return Updated Resources
```

**Process:**
1. **Read old resources** into RNodes (YAML nodes)
2. **Read new resources** into RNodes
3. **Copy comments** from old to new:
   - Match resources by namespace, name, kind, apiVersion
   - Skip ytt templates (contain #@ or #! comments)
   - Copy comments using kyaml comment utilities
4. **Write healed resources** to output
5. **Return** updated resources

**Comment healing:**
- Preserves user comments when resources updated
- Maintains documentation and annotations
- Skips templated resources (ytt)

**Use case:**
- UpdatePackageResources operation
- Direct resource content updates
- Preserving user documentation

## Upstream Package Fetching

Common pattern used by clone, edit, and upgrade tasks.

### PackageFetcher

```
Fetch Package Request
        ↓
  Resolve Reference
        ↓
  Open Repository
        ↓
  Get Package Revision
        ↓
  Return Package Revision
```

**PackageFetcher responsibilities:**
- Resolves package references to repositories
- Opens repositories through cache
- Fetches specific package revisions
- Returns package revision interface

**Used by:**
- **Clone**: Fetch upstream package from registered repository
- **Edit**: Fetch source package from same repository
- **Upgrade**: Fetch three package revisions (old upstream, new upstream, local)

**Configuration:**
- **RepoOpener**: Opens repositories through cache
- **ReferenceResolver**: Resolves package references
- **CredentialResolver**: Resolves authentication credentials

## Error Handling

Task transformations handle errors at multiple levels:

### Task-Level Errors

**Init errors:**
- Invalid package name
- Filesystem errors
- Kptfile creation errors

**Clone errors:**
- Invalid upstream source (none specified)
- Upstream package not found
- Git clone failures
- Authentication errors

**Edit errors:**
- Source package not found
- Source not from same package
- Source not published

**Upgrade errors:**
- Source package revisions not found
- Merge conflicts
- Invalid merge strategy

### Resource-Level Errors

**Replace resource errors:**
- Invalid YAML syntax
- Comment healing failures
- Resource matching errors

**Recovery:**
- Errors returned to Engine
- Engine triggers rollback
- No partial transformations persisted

## Transformation Characteristics

### Idempotency

**Not idempotent:**
- Tasks create new content each time
- Clone fetches latest upstream state
- Upgrade merges current state

**Implications:**
- Tasks should not be retried blindly
- Engine handles rollback on errors

### Isolation

**Transformation isolation:**
- Tasks work on in-memory resources
- No side effects until draft closed
- Multiple tasks can run concurrently (different packages)

### Validation

**Task-specific validation:**
- Init: Package name validity
- Clone: Upstream source validity
- Edit: Source package constraints
- Upgrade: Source revision published state

**Validation timing:**
- Some validation in Engine (before task execution)
- Some validation in task implementation (during execution)
