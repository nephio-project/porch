---
title: "Repositories"
type: docs
weight: 2
description: |
  Understanding repositories in Porch: connecting Porch to Git and OCI storage backends.
---

## What is a Repository in Porch?

A **Repository** is a Kubernetes resource that connects Porch to a Git repository or OCI registry where kpt packages are stored. When you register a repository with Porch, you're creating a Repository resource that tells Porch:

- Where the storage backend is located (Git URL or OCI registry address)
- How to authenticate (via Secret references)
- Whether it's a deployment repository (packages ready for deployment)
- How often to sync/refresh the package cache

## Repository Types

Porch supports two storage backends:

### Git Repositories

Git repositories are the primary and fully-supported storage backend for Porch:

- Packages stored as directories with Kptfile in a Git branch
- Draft packages stored in temporary branches (e.g., `drafts/my-package/v1`)
- Published packages committed to the main branch
- Full Git history preserved for audit and rollback
- Standard Git authentication (username/password, SSH keys, tokens)

### OCI Repositories (Experimental)

OCI (Open Container Initiative) registries provide an alternative storage backend:

- Packages stored as OCI artifacts (similar to container images)
- Experimental support - may be unstable
- Useful for environments already using OCI registries

{{% alert title="Note" color="warning" %}}
OCI repository support is experimental. Git repositories are recommended for production use.
{{% /alert %}}

## Deployment vs Blueprint Repositories

Repositories can be designated as either deployment or blueprint repositories via the `spec.deployment` field:

**Blueprint repositories** (`deployment: false`):
- Store reusable template packages
- Packages are meant to be cloned and customized
- Typically maintained by platform teams
- Examples: shared infrastructure patterns, application templates

**Deployment repositories** (`deployment: true`):
- Store deployment-ready packages
- Published packages are ready for GitOps tools (like Config Sync) to deploy
- Typically represent specific environments or clusters
- Examples: prod-cluster-configs, dev-environment-packages

## The Repository-Package Relationship

When a Repository resource is registered with Porch:
- Porch scans the storage backend for kpt packages (directories containing a Kptfile)
- Creates PackageRevision resources for each discovered package
- Maintains a cache of package metadata for performance
- Periodically syncs to detect new or updated packages

This means the Repository resource acts as the bridge between Porch's Kubernetes API and the actual Git/OCI storage where kpt package files live.

## Key Points

- **Repository** is a Kubernetes resource, not the Git/OCI storage itself
- It tells Porch where to find kpt packages and how to access them
- Git repositories are production-ready; OCI support is experimental
- Deployment repositories indicate packages are deployment-ready
- Porch automatically discovers packages and creates PackageRevision resources for them
