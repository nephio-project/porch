---
title: "Package Revisions"
type: docs
weight: 3
description: |
  Understanding package revisions: versioning and the working unit in Porch.
---

## What is a Package Revision?

A **PackageRevision** is the actual working unit in Porch. While users often say they're "working with a package", they're actually working with a specific revision of that package.

Think of it like Git commits:
- A Git repository contains a project
- Each commit is a specific version of that project
- You work with commits, not the abstract "project"

Similarly in Porch:
- A Repository contains kpt packages
- Each PackageRevision is a specific version of a kpt package
- You work with PackageRevisions, not the abstract "package"

## Revision Numbering

PackageRevisions use simple integer sequence versioning (v1, v2, v3, etc.):

- **v1**: First published revision
- **v2**: Second published revision
- **v3**: Third published revision
- And so on...

This simple scheme enables:
- Easy comparison (v3 is newer than v2)
- Automatic version assignment on publication
- Optimistic concurrency control (detect conflicting edits)
- Simple automation without parsing semantic versions

## Draft vs Published Revisions

PackageRevisions exist in different lifecycle stages:

**Draft revisions**:
- Work-in-progress versions
- Not yet assigned a revision number
- Identified by workspace name instead (e.g., `my-package-v1` where `v1` is the workspace)
- Can be modified freely
- Not visible to downstream consumers

**Published revisions**:
- Finalized, immutable versions
- Assigned an integer revision number (v1, v2, v3)
- Cannot be modified (must create new draft to make changes)
- Visible to downstream consumers
- Can be cloned or deployed

## The Latest Revision

The "latest" PackageRevision is the most recently published revision (highest revision number). Porch marks it with a Kubernetes label:

```
kpt.dev/latest-revision: "true"
```

This label makes it easy to:
- Query for the latest revision using label selectors
- Automatically track the newest version of a kpt package
- Build automation that follows the latest published revision

## PackageRevision Identity

A PackageRevision is uniquely identified by:
- **Repository name**: Which Repository resource it belongs to
- **Package name**: The kpt package directory name
- **Revision number** (if published): v1, v2, v3, etc.
- **Workspace name** (if draft): User-defined identifier for the draft

Example PackageRevision names:
- `blueprints-abc123-v1` - Draft in workspace `v1` of package `abc123` in repository `blueprints`
- `blueprints-abc123-v2` - Published revision v2 of package `abc123` in repository `blueprints`

## Key Points

- **PackageRevision** is the working unit in Porch, not "package"
- Each PackageRevision represents a specific version of a kpt package
- Simple integer versioning (v1, v2, v3) enables automation
- Draft revisions use workspace names; published revisions use revision numbers
- The latest revision is marked with `kpt.dev/latest-revision: "true"` label
- PackageRevisions are immutable once published
