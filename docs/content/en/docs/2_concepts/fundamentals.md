---
title: "Porch Fundamentals"
type: docs
weight: 2
description: |
  The fundamental topics necessary to understand Porch as "package orchestration" on a conceptual level.
---

## Core Concepts

This section introduces some core concepts of Porch's package orchestration:

* ***Package***: A package, in Porch, is specifically a [kpt package](https://kpt.dev/book/02-concepts/#packages) - a
  collection of related YAML files including one or more **[KRM resources](https://github.com/kubernetes/design-proposals-archive/blob/main/architecture/resource-management.md)**
  and a [Kptfile](https://kpt.dev/reference/schema/kptfile).
  {{% alert title="N.B." color="warning" %}}

  There is no such thing as a "Porch Package" - rather, **Porch stores and orchestrates kpt packages**.

  {{% /alert %}}

* ***Repository***: This is a version-control [repository](#repositories) used to store packages. For example, a
  [Git](https://git-scm.org/) or (experimentally) [OCI](https://github.com/opencontainers/image-spec/blob/main/spec.md)
  repository.

* ***Package Revision***: This refers to the state of a package as of a specific version. Packages are sequentially
[versioned](#package-revisions) such that multiple versions of the same package may exist in a repository. Each successive
version is considered a *package revision*.

* ***Lifecycle***: This refers to a package revision's current stage in the process of its orchestration by Porch. A package
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

* ***Functions***: specifically, [KRM functions](https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md).
  Functions can be added to a package's kptfile [pipeline](https://kpt.dev/book/04-using-functions/#declarative-function-execution)
  in the course of modifying a package revision in *Draft* state. Porch runs the pipeline on the package contents, mutating
  or validating the KRM resource files.

* ***Package Variant*** and ***Package Variant Set***: these Kubernetes objects represent higher levels of package revision
  automation. Package variants can be used to automatically track an upstream package (at a specific revision) and manage
  cloning it to one or several downstream packages, as well as preparing new downstream package revisions when a new revision
  of the upstream package is published. Package variant sets enable the same behaviour for package variants themselves.
  Use of package variants involves advanced concepts worthy of their own separate document:
  [Package Variants]({{% relref "/docs/5_architecture_and_components/controllers/pkg-variant-controllers.md" %}})


In addition, some terms may be used with specific qualifiers, frequently enough to count them as sub-concepts:

* ***Upstream package revision***: a package revision of an ***upstream package*** may be *cloned*, producing a new,
***downstream package*** and associated package revision. The downstream package maintains a link (URL) to the upstream
package revision from which it was cloned. [More details]({{% relref "/docs/5_architecture_and_components/relevant_old_docs/extracted_from_old_porch_concepts.md#package-relationships---upstream-and-downstream" %}})

* ***Deployment repository***: a repository can be designated as a deployment repository. Package revisions in *Published*
state in a deployment repository are considered [deployment-ready]({{% relref "/docs/2_concepts/theory.md#deployment-mechanism" %}}).

* ***Package revision workspace***, or `workspaceName`: a user-defined string and element of package revision names automatically
assembled by Porch. Used to uniquely identify a package revision while in *Draft* state, especially to distinguish between
multiple drafts undergoing concurrent development. **N.B.**: a package revision workspace does not refer to any distinct
"folder" or "space", but only to the in-development draft. The same workspace name may be assigned to multiple package
revisions **of different packages** and **does not of itself indicate any connection between the packages**.

## Core Concepts Elaborated

Some of the concepts, briefly introduced above, bear examination in greater detail.

### Repositories

{{% alert title="Note" color="primary" %}}

Currently, Porch primarily integrates with Git repositories. OCI support is available, but it is experimental and possibly
unstable.

{{% /alert %}}

A *Porch repository* represents Porch's connection to a Git repository containing kpt packages. It allows a Porch user to
read existing packages from Git, and to author new or existing packages with Porch's create, clone, and modify operations.
Once a repository is *registered* (created in Porch), Porch performs an initial read operation against it, scanning its
file structure to discover kpt packages, and building a local cache to improve the performance of subsequent operations.

Any repositories registered must be capable of storing the following minimum data and metadata:
* kpt packages' file contents.
* Package versions.
* Sufficient metadata associated with the package to capture:
  * Package dependency relationships (upstream - downstream).
  * Package lifecycle state (draft, proposed, published).
  * Package purpose (base package).
  * Customer-defined attributes (optionally).

### Package Revisions

In a manner similar to Git commits, Porch allows the user to modify packages (including creating or cloning new ones) on
a basis of incremental releases. A new version of a package is not released immediately, but starts out as a draft, allowing
the user to develop it in safety before it is proposed and published as a standalone version of the package. Porch enables
this by modelling each successive version (whether published or still under development) as a *package revision*.

Package revisions are sequentially versioned using a simple integer sequence. This enables the following important capabilities:

* Compare any two versions of a package to establish "newer than", equal, or "older than" relationships.
* Automatically assign new version numbers on publication.
* [Optimistic concurrency](https://en.wikipedia.org/wiki/Optimistic_concurrency_control) of package revision development,
  by comparing version numbers.
* Identify the latest (most recently published) package revision.
* A simple versioning model which easily supports automation.

Porch's get/list operations provide these versions to the user in a package revision's `revision` field.

#### Latest Package Revision

The "latest" package revision is the one most recently published, corresponding to the numerically-greatest revision number.
For additional ease of use, the PackageRevision resource type applies a Kubernetes label to the latest package revision
when read using the `porchctl` or `kubectl` CLI: `kpt.dev/latest-revision: "true"`
