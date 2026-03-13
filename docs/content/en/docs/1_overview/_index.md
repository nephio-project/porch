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

With Porch, you can manage packages through Kubernetes resources (PackageRevision, Repository) instead of direct Git operations. Use kubectl, client-go, or any Kubernetes tooling, or Porch's `porchctl` CLI utility. Packages move through lifecycle stages (Draft → Proposed → Published) with **approval workflows** and explicit approval gates, which helps prevent accidental publication of unreviewed changes. **Automatic package discovery** lets you register a Git or OCI repository once; Porch then discovers all packages in it, so there is no need for manual inventory management. **Function execution** applies KRM functions to transform and validate packages; functions run in isolated containers and their results are recorded in package history. **Package cloning and upgrades** let you clone packages from upstream sources and upgrade them automatically when new upstream versions are published, with a three-way merge preserving local customizations. **GitOps integration** means all changes are committed to Git with full history, and Porch works with [Flux](https://fluxcd.io/), [Config Sync](https://docs.cloud.google.com/kubernetes-engine/config-sync/docs/how-to/installing-config-sync), and other GitOps deployment tools. **Multi-repository orchestration** lets you manage packages across multiple Git and OCI repositories from a single control plane, and controllers can automate cross-repository operations. **Collaboration and governance** allow multiple users and automation to work on packages at the same time, with draft revisions providing workspace isolation before publication. **Repository synchronization** lets Porch detect changes made directly in Git (outside Porch) and update its cache, and it supports both Porch-managed and externally-managed packages. Packages remain **standard kpt packages**, with no vendor lock-in or Porch-specific DSL in the kpt package content.

## What Porch is not

Porch is not a deployment tool. It manages packages in repositories but does not deploy them to clusters. That's the job of Flux, ArgoCD,  Config Sync, or similar GitOps tools. Porch prepares and publishes packages to git; gitops deployment tools consume them.

Porch does not replace kpt; it uses kpt internally for package operations and function execution—effectively "kpt-as-a-service," exposing a Kubernetes API on top of kpt. It manages package content and lifecycle in repositories and does not deploy packages; deployment to clusters is handled by separate GitOps tools that watch those repositories. Exclusive control is not required, since packages can be edited through Porch or directly in Git, and the system synchronizes with external changes and supports mixed workflows. It does not provide a package registry either, as it manages packages in your existing Git and OCI repositories rather than acting as a separate package storage system. Porch works with standard kpt package layouts and does not enforce a specific repository structure, so you control how your repositories are organized.

## How Porch fits in the ecosystem

Porch is a key component in configuration-as-data workflows. With **Package authoring**, users create and edit packages through Porch's Kubernetes API. Using **Package transformation**, KRM functions validate and transform package contents. Approved packages are published to deployment repositories using **Package publication**. **Package deployment** is done with the GitOps deployment system (Flux etc), which deploys published packages to clusters. Controllers (PackageVariant, PackageVariantSet) take care of **Package automation** operations.

Porch sits between package authors and deployment tools, providing the orchestration layer that was previously handled by manual Git operations or custom scripts.

## Architecture

Porch consists of four main deployable components. Porch Server is a Kubernetes aggregated apiserver that exposes the PackageRevision and Repository APIs and includes the Engine (orchestration logic), Cache (repository content), and Repository Adapters (Git/OCI abstraction). A separate gRPC service, the Function Runner, runs KRM functions in containers and can execute both functions supplied by Porch and externally developed function images. Kubernetes controllers automate package operations: the PackageVariant controller clones and updates packages, and the PackageVariantSet controller manages sets of package variants. Repository content is cached for performance via a storage backend; the server supports either a CR-based cache (using Kubernetes custom resources) or a PostgreSQL-based cache for larger deployments.
