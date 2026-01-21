---
title: "Workspace"
type: docs
weight: 4
description: |
  Understanding workspaces: how Porch isolates concurrent work on packages.
---

## What is a Workspace?

A **workspace** is a named, isolated environment for working on a package revision in Porch. Each Draft or Proposed PackageRevision has a unique workspace name that identifies the specific set of changes being made to a package.

Workspaces enable:
- Multiple users to work on different changes to the same package simultaneously
- Isolation of work-in-progress changes from published packages
- Clear identification of what changes a PackageRevision contains
- Tracking of package revision history and lineage

## How Workspaces Work

When you create a new Draft PackageRevision, you specify a workspace name. This workspace name:

- **Must be unique** within a package (no two PackageRevisions of the same package can share a workspace name)
- **Identifies the changes** contained in that PackageRevision
- **Maps to Git branches** (for Git repositories) in the format `drafts/<package-path>/<workspace-name>`
- **Persists through lifecycle** transitions (Draft → Proposed → Published)

## Workspaces and Lifecycle

Workspaces persist throughout a PackageRevision's lifecycle stages (Draft → Proposed → Published). The workspace name remains constant, providing continuity as the package moves through different stages. For details on lifecycle stages and transitions, see [Package Lifecycle]({{% relref "/docs/2_concepts/package-lifecycle" %}}).

## Workspace Names

Workspace names should be:
- **Descriptive**: Indicate what changes the PackageRevision contains (e.g., `add-monitoring`, `update-v2`, `fix-rbac`)
- **Unique**: No other PackageRevision of the same package can use the same workspace name
- **Valid**: Follow Kubernetes naming conventions (lowercase alphanumeric, hyphens allowed)

Common patterns:
- Feature-based: `add-feature-x`, `enable-tls`
- Version-based: `v1`, `v2`, `v3` (often auto-generated)
- User-based: `alice-changes`, `bob-update`
- Purpose-based: `security-patch`, `config-update`

## Workspaces and Git

In Git repositories, workspaces map directly to Git branches:

**Draft packages**: Stored in branches named `drafts/<package-path>/<workspace-name>`
- Example: `drafts/my-app/add-monitoring`

**Proposed packages**: Stored in branches named `proposed/<package-path>/<workspace-name>`
- Example: `proposed/my-app/add-monitoring`

**Published packages**: Committed to the main branch and tagged
- The workspace name is preserved in the PackageRevision metadata
- The Git branch is deleted after publication

This Git mapping provides:
- Native Git workflow integration
- Branch-based isolation
- Standard Git operations (diff, merge, rebase) on package changes
- Audit trail through Git history

## Multiple Workspaces on the Same Package

Multiple users can work on the same package simultaneously by using different workspaces:

```
Package: my-app

Workspace: add-monitoring (Draft)
  - User Alice adding Prometheus configs

Workspace: update-dependencies (Draft)
  - User Bob updating container images

Workspace: v3 (Published)
  - Current production version
```

Each workspace is isolated. Changes in one workspace don't affect others until they're published.

## Workspace and Revision Numbers

Workspaces and revision numbers serve different purposes:

**Workspace name**: Identifies work-in-progress changes (Draft/Proposed)
- Temporary identifier
- User-defined
- Describes the changes

**Revision number**: Identifies published versions
- Permanent identifier
- Auto-assigned by Porch (v1, v2, v3, ...)
- Indicates version sequence

When a PackageRevision is published, it receives a revision number, but the workspace name is preserved for historical tracking.

## Key Points

- Workspace = named, isolated environment for package changes
- Each Draft/Proposed PackageRevision has a unique workspace name
- Workspace names must be unique within a package
- Workspaces map to Git branches (drafts/ or proposed/ prefixes)
- Multiple workspaces enable concurrent work on the same package
- Workspace names are preserved after publication for tracking
- Workspace names should be descriptive and follow naming conventions
