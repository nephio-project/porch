---
title: "Request Flow"
type: docs
weight: 4
description: Example flow showing how the Engine processes a package creation request
---

This section provides a detailed walkthrough of what happens when a user creates a new package revision. This example demonstrates how the concepts from Design Rationale, Functionality, and Interactions come together in practice.

{{% alert title="Note" color="primary" %}}

This section provides representative examples of how the Engine processes requests internally. These examples illustrate the typical patterns and interactions - they are not meant to exhaustively document every possible operation.

Understanding one or two key flows (like creating a package) provides insight into how all Engine operations work, as they follow similar patterns.

{{% /alert %}}

## User Perspective

```bash
# User creates a PackageRevision with init task
kubectl apply -f - <<EOF
apiVersion: porch.kpt.dev/v1alpha1
kind: PackageRevision
metadata:
  namespace: default
spec:
  packageName: my-package
  workspaceName: v1
  repositoryName: blueprints
  lifecycle: Draft
  tasks:
  - type: Init
    init:
      description: "My new package"
EOF
```

**What the user expects**:
- A new package created in the `blueprints` repository
- Package named `my-package` with workspace `v1`
- Package in Draft lifecycle state
- Package contains basic structure (Kptfile, README)

## Complete Flow

### Step 1: API Server Receives Request

**Component**: API Server

**What happens**:
1. User submits package creation request via kubectl
2. Kubernetes API Server receives the request
3. Request is routed to Porch API Server (via API aggregation)
4. Porch API Server handles the package revision creation

**Checks performed**:
- Authentication (via Kubernetes)
- Authorization (RBAC check)
- Schema validation (OpenAPI schema)
- Admission webhooks (validation/mutation)

**Result**: Request is validated and forwarded to Engine for processing

### Step 2: Engine Validates Request

**Component**: Engine

**What happens**:
1. Engine receives package creation request
2. Validates lifecycle value
3. Builds package configuration
4. Validates task list

**Validations performed**:

**Lifecycle validation**:
- Empty lifecycle → defaults to `Draft`
- `Draft` or `Proposed` → allowed
- `Published` or `DeletionProposed` → rejected (cannot create directly)
- Invalid values → rejected with error

**Task validation**:
- Multiple tasks → rejected (only one task allowed during creation)
- No tasks → defaults to `Init` task with package description
- Single task → validated based on task type

**Result**: Request validated, ready to proceed

### Step 3: Engine Opens Repository

**Component**: Engine → Cache

**What happens**:
1. Engine requests repository from Cache
2. Cache checks if repository is already open
3. If not, Cache opens repository (Git or OCI)
4. Cache returns Repository interface

**Repository types**:
- **Git**: Opens Git repository, fetches latest
- **OCI**: Opens OCI registry connection

**Result**: Repository interface ready for operations

### Step 4: Engine Validates Workspace Name

**Component**: Engine

**What happens**:
1. Engine lists existing package revisions in same repository
2. Checks if any have the same workspace name
3. Validates package name format

**Validation process**:
1. Get existing revisions in repository
2. Filter revisions to same package name
3. Verify no duplicate workspace names
4. Reject if workspace name already exists

**Why this matters**:
- Workspace name is used to generate PackageRevision name
- Must be unique within a package
- Prevents naming conflicts

**Result**: Workspace name validated as unique

### Step 5: Engine Validates Task Type

**Component**: Engine

**What happens**:
1. Engine checks task type (Init, Clone, Copy, Upgrade)
2. Performs task-specific validation

**For Init task**:
- No special validation needed

**For Clone task**:
- Ensures package doesn't already exist
- Clone can only create the first revision of a package
- Subsequent revisions must use Copy or Upgrade

**For Upgrade task**:
- Verifies all source revisions are Published
- Ensures stable upstream sources for upgrade operations

**Result**: Task validated, ready to execute

### Step 6: Engine Validates Package Path

**Component**: Engine

**What happens**:
1. Engine checks if package path overlaps with existing packages
2. Prevents one package being subdirectory of another

**Why this matters**:
- Prevents ambiguity in repository structure
- Avoids conflicts between packages
- Ensures clean package boundaries

**Result**: Package path validated

### Step 7: Repository Creates Draft

**Component**: Repository (Git or OCI adapter)

**What happens**:
1. Engine requests draft creation from Repository
2. Repository creates draft package revision
3. Draft is a working copy, not yet committed

**For Git repository**:
- Creates new branch (workspace name)
- Initializes package directory
- Returns draft object

**For OCI repository**:
- Creates temporary working directory
- Initializes package structure
- Returns draft object

**Result**: Draft package revision created, ready for modifications

### Step 8: Engine Sets Up Rollback

**Component**: Engine

**What happens**:
1. Engine defines rollback mechanism
2. Rollback will be called if any subsequent step fails

**Rollback mechanism**:
- Closes the draft
- Deletes the package revision
- Logs warnings if cleanup fails

**Why this matters**:
- Ensures atomic operations
- Prevents partial package creation
- Keeps repository in consistent state

**Result**: Rollback mechanism ready

### Step 9: Task Handler Applies Task

**Component**: Task Handler

**What happens**:
1. Engine delegates task execution to Task Handler
2. Task Handler determines task type (Init in this case)
3. Task Handler executes Init task

**For Init task**:
1. Creates Kptfile with package metadata
2. Creates README.md with description
3. Writes files to draft

**Init task creates**:
- `Kptfile`: Package metadata with name and description
- `README.md`: Basic documentation template

If task fails, rollback is called to clean up the draft.

**Result**: Package initialized with basic structure

### Step 10: Engine Updates Lifecycle

**Component**: Engine → Draft

**What happens**:
1. Engine updates draft lifecycle state
2. Draft lifecycle set to Draft (or Proposed if specified)

If update fails, rollback is called to clean up.

**Result**: Package lifecycle set correctly

### Step 11: Repository Closes Draft (Commits)

**Component**: Repository

**What happens**:
1. Engine requests draft closure from Repository
2. Repository commits draft to storage

**For Git repository**:
- Commits changes to branch
- Pushes to remote
- Returns PackageRevision

**For OCI repository**:
- Packages content
- Pushes to registry
- Returns PackageRevision

**Note**: If this step fails, rollback is not attempted (would likely fail again). The draft may remain in the repository and will be cleaned up by garbage collection.

**Result**: Package committed to repository

### Step 12: Engine Notifies Watchers

**Component**: Engine → Watcher Manager

**What happens**:
1. Repository sync detects new package
2. Cache updates
3. Watcher Manager sends ADDED events to registered watchers

**Note**: This happens during repository synchronization, not directly in the create flow.

**Who receives events**:
- kubectl watch commands
- Controllers watching PackageRevisions
- Other clients with active watches

**Result**: Watchers notified of new package

### Step 13: Engine Returns Result

**Component**: Engine → API Server

**What happens**:
1. Engine returns PackageRevision to API Server
2. API Server converts to HTTP response
3. Response sent to kubectl

**Result**: PackageRevision returned to user

### Step 14: User Receives Response

**Component**: kubectl

**What happens**:
1. kubectl receives HTTP 201 Created
2. kubectl displays created PackageRevision

**Output**:
```
packagerevision.porch.kpt.dev/blueprints-my-package-v1 created
```

**User can now**:
- View package: `kubectl get packagerevision blueprints-my-package-v1`
- Edit package: `kubectl edit packagerevision blueprints-my-package-v1`
- View resources: `kubectl get packagerevisionresources blueprints-my-package-v1`

## Error Scenarios

### Scenario 1: Workspace Name Already Exists

**When**: Step 4 (Validate Workspace Name)

**Error**:
```
Error from server: package revision workspaceNames must be unique; 
package revision with name my-package in repo blueprints with 
workspaceName v1 already exists
```

**Resolution**: User must choose different workspace name

### Scenario 2: Task Execution Fails

**When**: Step 9 (Apply Task)

**What happens**:
1. Task Handler fails to apply task
2. Engine calls rollback
3. Draft is deleted
4. Error returned to user

**Error**:
```
Error from server: failed to apply init task: <specific error>
```

**Resolution**: User must fix task specification and retry

### Scenario 3: Git Push Fails

**When**: Step 11 (Close Draft)

**What happens**:
1. Repository fails to push to Git
2. Error returned to Engine
3. Engine returns error to API Server
4. Draft may remain in repository (cleaned up by garbage collection)

**Error**:
```
Error from server: failed to close package revision draft: 
failed to push to remote: <git error>
```

**Resolution**: Check Git credentials, network, repository permissions

## Performance Considerations

**What affects Engine performance**:

**Validation overhead** (Steps 2-6) - **LOW impact**:
- Workspace name uniqueness check (lists existing packages)
- Package path overlap validation
- Task type validation
- Generally fast, in-memory operations

**Orchestration complexity** - **LOW impact**:
- Number of components to coordinate (Cache, Task Handler, Repository, Watcher Manager)
- Rollback setup and teardown
- Minimal overhead compared to delegated operations

**Delegated operations** - **Impact varies by component**:
- Repository operations (Git/OCI) - handled by Repository Adapters
- Function execution - handled by Function Runner
- Cache access - handled by Cache
- *Engine orchestrates these but doesn't control their performance*

**Key insight**: Engine itself is fast. Performance bottlenecks are typically in the components it delegates to (Repository for Git operations, Function Runner for KRM functions, Cache for repository access).

## Summary

Creating a package involves:

1. **API Server**: Receives request, validates, delegates to Engine
2. **Engine**: Orchestrates entire operation
3. **Cache**: Opens repository
4. **Engine**: Validates workspace name, task type, package path
5. **Repository**: Creates draft
6. **Engine**: Sets up rollback
7. **Task Handler**: Applies init task
8. **Engine**: Updates lifecycle
9. **Repository**: Commits draft
10. **Watcher Manager**: Notifies watchers
11. **Engine**: Returns result
12. **API Server**: Returns HTTP response
13. **User**: Receives created package

**Key features demonstrated**:
- **Validation** (from Functionality): Multiple validation layers ensure correctness
- **Rollback** (from Design Rationale): Atomic operations maintain consistency
- **Component Interaction** (from Interactions): Engine orchestrates Cache, Task Handler, Repository, Watcher Manager
- **Design Patterns** (from Design Rationale): Optimistic locking, dependency injection, separation of concerns

This flow demonstrates how the Engine orchestrates multiple components to provide reliable, consistent package creation.
