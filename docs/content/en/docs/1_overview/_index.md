---
title: "Overview"
type: docs
weight: -1
description: >
  Porch is a Kubernetes extension apiserver for managing the lifecycle of KRM configuration packages in Git and OCI repositories. It provides a Kubernetes-native API for package operations, enabling GitOps workflows with approval gates, automation, and collaboration.
---

Porch (Package Orchestration Server) was originally developed in the [kpt project](https://github.com/kptdev/kpt) and donated to [Nephio](https://nephio.org) in December 2023. The name "Porch" is short for "Package ORCHestration."

## Why you need Porch and what it can do

Managing configuration packages across multiple repositories and teams is challenging. You need to clone repositories, edit YAML files, run KRM functions, handle merge conflicts, and push changes - all while maintaining consistency, applying governance policies, and coordinating with GitOps deployment tools.

What if you could manage packages through a Kubernetes API instead? That's where Porch comes in.

Porch provides you with a Kubernetes-native way to manage [KRM configuration packages](https://kpt.dev/book/03-packages/). It handles the Git/OCI operations, applies transformations through [KRM functions](https://kpt.dev/book/04-using-functions/), enforces approval workflows, and keeps everything synchronized - all through standard Kubernetes resources.

Porch provides you with:

* **Kubernetes-native package management**  
  Manage packages through Kubernetes resources (PackageRevision, Repository) instead of direct Git operations. Use kubectl, client-go, or any Kubernetes tooling, or Porch's `porchctl` CLI utility.

* **Approval workflows**  
  Packages move through lifecycle stages (Draft → Proposed → Published) with explicit approval gates. Prevent accidental publication of unreviewed changes.

* **Automatic package discovery**  
  Register a Git or OCI repository once, and Porch automatically discovers all packages within it. No manual inventory management.

* **Function execution**  
  Apply KRM functions to transform and validate packages. Functions run in isolated containers with results tracked in package history.

* **Package cloning and upgrades**  
  Clone packages from upstream sources and automatically upgrade them when new upstream versions are published. Three-way merge handles local customizations.

* **GitOps integration**  
  All changes are committed to Git with full history. Works seamlessly with [Flux](https://fluxcd.io/), [Config Sync](https://docs.cloud.google.com/kubernetes-engine/config-sync/docs/how-to/installing-config-sync) and other GitOps deployment tools.

* **Multi-repository orchestration**  
  Manage packages across multiple Git and OCI repositories from a single control plane. Controllers can automate cross-repository operations.

* **Collaboration and governance**  
  Multiple users and automation can work on packages concurrently. Draft revisions provide workspace isolation before publication.

* **Repository synchronization**  
  Porch detects changes made directly in Git (outside Porch) and synchronizes its cache. Supports both Porch-managed and externally-managed packages.

* **Standard kpt packages**  
  Packages remain standard kpt packages. No vendor lock-in or Porch specific DSL "code" in kpt packages.

## What Porch is not

Porch is not a deployment tool. It manages packages in repositories but does not deploy them to clusters. That's the job of Flux, ArgoCD,  Config Sync, or similar GitOps tools. Porch prepares and publishes packages to git; gitops deployment tools consume them.

Porch:

* **Does not replace kpt**. Porch uses kpt internally for package operations and function execution. It's "kpt-as-a-service" - providing a Kubernetes API on top of kpt capabilities.

* **Does not deploy packages**. Porch manages package content and lifecycle in repositories. Deployment to clusters is handled by separate GitOps tools that watch those repositories.

* **Does not require exclusive control**. Packages can be edited through Porch or directly in Git. Porch synchronizes with external changes and supports mixed workflows.

* **Does not provide a package registry**. Porch manages packages in your existing Git and OCI repositories. It's not a separate package storage system.

* **Does not enforce a specific repository structure**. Porch works with standard kpt package layouts. You control your repository organization.

## How Porch fits in the ecosystem

Porch is a key component in configuration-as-data workflows:

1. **Package authoring**: Users create and edit packages through Porch's Kubernetes API
2. **Package transformation**: KRM functions validate and transform package contents
3. **Package publication**: Approved packages are published to deployment repositories
4. **Package deployment**: The GitOps deployment system (Flux etc) deploys published packages to clusters
5. **Package automation**: Controllers (PackageVariant, PackageVariantSet) automate package operations

Porch sits between package authors and deployment tools, providing the orchestration layer that was previously handled by manual Git operations or custom scripts.

## Architecture

Porch consists of four main deployable components:

* **Porch Server**: A Kubernetes aggregated apiserver that provides the PackageRevision and Repository APIs. It contains the Engine (orchestration logic), Cache (repository content), and Repository Adapters (Git/OCI abstraction).

* **Function Runner**: A separate gRPC service that executes KRM functions in containers. It can run both  functions supplied by Porch and externally developed function images.

* **Controllers**: Kubernetes controllers automate package operations. The PackageVariant controller clones and updates packages. The PackageVariantSet controller manages sets of package variants.

* **Cache**: A storage backend for caching of repository content for performance reasons. Porch supports a CR-based cache (using a Kubernetes custom resources) or a PostgreSQL-based cache for larger deployments.
