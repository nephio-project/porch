---
title: "Package Lifecycle"
type: docs
weight: 5
description: |
  Understanding package lifecycle stages and workflow in Porch.
---

## What is Package Lifecycle?

The **lifecycle** of a PackageRevision refers to its current stage in Porch's orchestration process. A PackageRevision moves through different stages from creation to publication, with approval gates between stages.

## Lifecycle Stages

### Draft

A PackageRevision in Draft stage is:
- Work-in-progress
- Being actively edited
- Not ready for deployment
- Stored in a temporary Git branch (e.g., `drafts/package-name/workspace`)

Operations allowed:
- Modify package contents
- Add/remove KRM functions
- Update Kptfile

### Proposed

A PackageRevision in Proposed stage is:
- Ready for review
- Awaiting approval
- Immutable (cannot be edited)
- Still in a temporary Git branch

This is an intermediate stage between Draft and Published, allowing team review before finalization.

### Published

A PackageRevision in Published stage is:
- Approved and finalized
- Assigned a revision number (v1, v2, v3, etc.)
- Committed to the main Git branch
- Ready for deployment or cloning
- Immutable (cannot be modified)

To make changes to a Published revision, you must create a new Draft based on it.

### DeletionProposed

A PackageRevision in DeletionProposed stage is:
- Marked for deletion
- Awaiting approval for removal
- Still exists in the repository

This intermediate stage allows review before permanently deleting a PackageRevision.

## Lifecycle Workflow

The typical flow of a PackageRevision through lifecycle stages:

![Package Lifecycle Workflow](/static/images/porch/flowchart.drawio.svg)

**Normal workflow:**
1. **Draft** → User creates or edits a package
2. **Proposed** → User proposes changes for review
3. **Review** → Team reviews the changes
4. **Published** → Changes are approved and committed

**Rejection workflow:**
- If changes are rejected during review, the PackageRevision returns to Draft stage
- User makes required modifications
- Process repeats until approval

**Deletion workflow:**
- Published PackageRevision → DeletionProposed
- Review → If approved, PackageRevision is deleted
- If rejected, remains Published

## State Transitions

Valid state transitions:
- Draft → Proposed
- Proposed → Published (on approval)
- Proposed → Draft (on rejection)
- Published → DeletionProposed
- DeletionProposed → Deleted (on approval)
- DeletionProposed → Published (on rejection)

Invalid transitions:
- Draft → Published (must go through Proposed)
- Published → Draft (must create new Draft)

## Key Points

- Lifecycle stages control when a PackageRevision can be modified or deployed
- Proposed and DeletionProposed are approval gates
- Only Draft revisions can be modified
- Published revisions are immutable and deployment-ready
- The workflow ensures review and approval before changes are finalized
