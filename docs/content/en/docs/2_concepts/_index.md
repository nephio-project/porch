---
title: "Porch Concepts"
type: docs
weight: 2
description: |
  The fundamental topics necessary to understand Porch as "package orchestration" on a conceptual level.
---

## Core Concepts

This section introduces some core concepts of Porch's package orchestration:

* ***[Package]({{% relref "package" %}})***: In Porch, a package is represented by a **PackageRevision** resource (metadata) paired with a
  **PackageRevisionResources** resource (content). These Kubernetes API resources surface [kpt packages](https://kpt.dev/book/03-packages/)
  stored in Git or OCI repositories. When you interact with Porch via `porchctl` or `kubectl`, you work with PackageRevision
  objects, not directly with the files in Git. See [kpt documentation](https://kpt.dev/book/03-packages/) for details on
  the underlying kpt package format.

* ***[Repository]({{% relref "repositories" %}})***: This is a version-control repository used to store packages. For example, a
  [Git](https://git-scm.org/) or (experimentally) [OCI](https://github.com/opencontainers/image-spec/blob/main/spec.md)
  repository.

* ***[Package Revision]({{% relref "package-revisions" %}})***: This refers to the state of a package as of a specific version. Packages are sequentially
versioned such that multiple versions of the same package may exist in a repository. Each successive
version is considered a *package revision*.

* ***[Lifecycle]({{% relref "package-lifecycle" %}})***: This refers to a package revision's current stage in the process of its orchestration by Porch. A package
revision may be in one of several lifecycle stages:
  * ***Draft*** - the package is being authored (created or edited). The package contents can be modified but the package
    revision is not ready to be used/deployed. Previously-published package revisions, reflecting earlier states of the
    package files, can still be deployed.
  * ***Proposed*** - intermediate state. The package's author has proposed that the package revision be published as a new
    version of the package with its files in the current state.
  * ***Published*** - the changes to the package have been approved and the package is ready to be used. Published packages
    may be deployed, cloned to a new package, or edited to continue development.
  * ***DeletionProposed*** - intermediate state. A user has proposed that this package revision be deleted from the
    repository.

* ***[Workspace]({{% relref "workspace" %}})***: A named, isolated environment for working on a package revision. Each Draft or Proposed
  PackageRevision has a unique workspace name that enables multiple users to work on different changes to the same package
  simultaneously. Workspaces map to Git branches and persist through lifecycle transitions.

* ***[Upstream and Downstream]({{% relref "upstream-downstream" %}})***: Package relationships where an upstream package serves as a source
  that can be cloned to create downstream packages. Downstream packages maintain a link to their upstream source and can be
  upgraded when new upstream versions are published.

* ***[Functions]({{% relref "functions" %}})***: Specifically, [KRM functions](https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md).
  Functions can be added to a package's kptfile [pipeline](https://kpt.dev/book/04-using-functions/#declarative-function-execution)
  in the course of modifying a package revision in *Draft* state. Porch runs the pipeline on the package contents, mutating
  or validating the KRM resource files.

* ***[Package Variant]({{% relref "package-variant" %}})*** and ***Package Variant Set***: These Kubernetes objects represent higher levels of package revision
  automation. Package variants can be used to automatically track an upstream package (at a specific revision) and manage
  cloning it to one or several downstream packages, as well as preparing new downstream package revisions when a new revision
  of the upstream package is published. Package variant sets enable the same behaviour for package variants themselves.

## Additional Terms

* ***Configuration as Data (CaD)***: The architectural approach that Porch implements. CaD treats configuration with the same
  rigor as application code: configuration data is the source of truth (stored separately from live state), uses a uniform
  serializable data model (KRM YAML), separates data from code that acts on it (functions transform, controllers apply),
  and abstracts storage from operations (clients use APIs, not direct Git/OCI access). Key principles include decoupling
  abstractions from data, separating actuation from processing, and preferring transformation over generation.

* ***Deployment repository***: A repository can be designated as a deployment repository (via `spec.deployment: true`).
  Package revisions in *Published* state in a deployment repository are considered deployment-ready and can be consumed
  by GitOps tools like Config Sync or Flux. See [Repositories]({{% relref "repositories" %}}) for details.
