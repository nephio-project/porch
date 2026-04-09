---
title: "Repositories"
type: docs
weight: 2
description: |
  Understanding repositories in Porch: connecting Porch to Git and OCI storage backends.
---

## What is a Repository in Porch?

A **Repository** is a Kubernetes resource that connects Porch to a Git repository where kpt packages are stored. When you
register a repository with Porch, you're creating a Repository resource that tells Porch:
- Where the storage backend is located (Git URL)
    - including a particular branch within the repository - the deployment branch
- How to authenticate (via references to a Kubernetes Secret containing the credentials)
- Whether it's a deployment repository (packages ready for deployment)
- How often to sync/refresh the package revision cache

Repositories can be accessed using the short name `repo` for convenience (e.g., `kubectl get repo`).

## Repository Types

Porch supports two storage backends:

### Git Repositories

Git repositories are the primary and fully-supported storage backend for Porch:

- Packages are stored as kpt packages in a Git branch, each in a directory with a Kptfile
- When using the CR cache, Draft and Proposed package revisions are stored in temporary branches (e.g., `drafts/my-package/v1`,
  `proposed/my-package/v1`)
- Published package revisions are committed to the registerd deployment branch
- The full Git history is preserved for audit and rollback
- Standard Git authentication (username/password, SSH keys, tokens) is used

### OCI Repositories (Experimental)

OCI (Open Container Initiative) registries provide an alternative storage backend:

- Packages stored as OCI artifacts (similar to container images)
- Experimental support - may be unstable
- Useful for environments already using OCI registries

{{% alert title="Note" color="warning" %}}
OCI repository support is **experimental** and **may be unstable**. Git repositories are recommended for production use.
{{% /alert %}}

## Deployment vs Blueprint Repositories

Repositories can be designated as either deployment or blueprint repositories via the `spec.deployment` field. This field
is merely an indicator - Porch does not treat deployment repositories differently from blueprint repositories in any respect.

**Blueprint repositories** (`deployment: false`):
- Store reusable template packages
- Contain packages to be cloned and customized
- Are typically maintained by platform teams
- Examples: shared infrastructure patterns, application templates

**Deployment repositories** (`deployment: true`):
- Store deployment-ready packages
- Contain published packages, ready for GitOps tools (like Flux) to deploy
- Typically target specific environments or clusters
- Examples: prod-cluster-configs, dev-environment-packages

## The Repository-Package Relationship

When a Repository resource is registered with Porch, Porch automatically conducts [package revision discovery]({{% relref "package-revisions#package-revision-discovery" %}}):
- Scans the storage backend for kpt packages (directories containing a Kptfile)
- Creates PackageRevision resources for each discovered package
- Maintains a cache of package metadata for performance
- Periodically syncs to detect new or updated packages

This means the Repository resource acts as the bridge between Porch's Kubernetes API and the actual Git repository where
kpt package files are stored.

## Immutability

{{% alert title="Important" color="warning" %}}
The following fields are **immutable** after repository creation and cannot be modified:
- `spec.type`
- `spec.git.repo`
- `spec.git.branch`
- `spec.git.directory`
- `spec.oci.registry`

To change these fields, you must delete and recreate the Repository resource.
{{% /alert %}}

This immutability ensures consistency and prevents accidental changes to the fundamental location and type of the repository.

## Repository Status

The Repository resource maintains status information that reflects the current state of synchronization and the repository's health:

**Condition Fields:**
- `conditions`: Standard Kubernetes conditions indicating repository health and readiness

**Sync Tracking:**
- `lastFullSyncTime`: Timestamp of the last successful full sync
- `nextFullSyncTime`: Scheduled time for the next full sync
- `observedGeneration`: Generation of the spec that was last observed (used to detect spec changes)
- `observedRunOnceAt`: The `runOnceAt` value that was last processed (prevents duplicate one-time syncs)

**Repository Information:**
- `packageCount`: Number of packages discovered in the repository
- `gitCommitHash`: Current Git commit hash for Git repositories (empty for OCI repositories)

These status fields are updated by the Repository Controller and provide observability into sync operations. See the [Repository Controller documentation]({{% relref "/docs/5_architecture_and_components/controllers/repository-controller" %}}) for details on how the controller manages repository synchronization.

### kubectl Output

When viewing repositories with `kubectl get repo`, you'll see several useful columns including Branch and Age:

```bash
$ kubectl get repo
NAME        TYPE   CONTENT      SYNC SCHEDULE   DEPLOYMENT   READY   ADDRESS                           BRANCH   AGE
my-repo     git    Package      0 */1 * * *     true         True    https://github.com/org/repo.git   main     5d
```

## Sync Configuration

The `spec.sync` field controls how and when Porch synchronizes with the repository:

**Periodic Synchronization:**
- `schedule`: A cron expression for scheduled syncs (e.g., `*/10 * * * *` to sync every 10 minutes)

**One-Time Synchronization:**
- `runOnceAt`: A timestamp to trigger a one-time cache sync outside the periodic schedule. This is tracked via `observedRunOnceAt` in the status to prevent redundant syncs.

See [Repository Sync Configuration]({{% relref "/docs/6_configuration_and_deployments/configurations/repository-sync" %}}) for detailed sync configuration options.

## Key Points

- **Repository** is a Kubernetes resource, not the Git/OCI storage itself
- It tells Porch where to find kpt packages and how to access them
- Git repositories are production-ready; OCI support is experimental
- Deployment repositories should contain packages that are ready for deployment
- Porch automatically discovers packages, parses their history, and exposes them as package revisions
