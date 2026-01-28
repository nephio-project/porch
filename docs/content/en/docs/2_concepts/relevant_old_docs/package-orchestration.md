---
title: "Package Orchestration"
type: docs
weight: 2
description: 
---

Customers who want to take advantage of the benefits of Configuration as Data
can do so today using the [kpt](https://kpt.dev) CLI and the kpt function ecosystem, including its
[functions catalog](https://catalog.kpt.dev/). Package authoring is possible using a variety of
editors with [YAML](https://yaml.org/) support. That said, a UI experience of
what-you-see-is-what-you-get (WYSIWYG) package authoring which supports a broader package lifecycle,
including package authoring with *guardrails*, approval workflows, package deployment, and more, is
not yet available.

The *Package Orchestration* (Porch) service is a part of the Nephio implementation of the
Configuration as Data approach. It offers an API and a CLI that enable you to build the UI
experience for supporting the configuration lifecycle.

## Core concepts

This section briefly describes core concepts of package orchestration:

***Package***: A package is a collection of related configuration files containing configurations
of [KRM][krm] **resources**. Specifically, configuration packages are [kpt packages](https://kpt.dev/book/02-concepts/#packages).
Packages are sequentially ***versioned***. Multiple versions of the same package may exist in a
([repository](#package-versioning)). A package may have a link (URL) to an
***upstream package*** (a specific version) ([from which it was cloned](#package-relationships)) . Packages go through three lifecycle stages: ***Draft***, ***Proposed***, and ***Published***:

  * ***Draft***: The package is being created or edited. The contents of the package can be
  modified; however, the package is not ready to be used (or deployed).
  * ***Proposed***: The author of the package has proposed that the package be published.
  * ***Published***: The changes to the package have been approved and the package is ready to be
  used. Published packages can be deployed or cloned.

***Repository***: The repository stores packages. [git][] and [OCI][oci] are two examples of a
([repository](#repositories)). A repository can be designated as a
***deployment repository***. *Published* packages in a deployment repository are considered to be
([deployment-ready](#deployment)).
***Functions***: Functions (specifically, [KRM functions][krm functions]) can be applied to
packages to mutate or validate the resources within them. Functions can be applied to a
package to create specific package mutations while editing a package draft. Functions can be added
to a package's Kptfile [pipeline][].

## Core components of the Configuration as Data (CAD) implementation

The core implementation of Configuration as Data, or *CaD Core*, is a set of components and APIs
which collectively enable the following:

* Registration of the repositories (Git, OCI) containing kpt packages and the discovery of packages.
* Management of package lifecycles. This includes the authoring, versioning, deletion, creation,
and mutations of a package draft, the process of proposing the package draft, and the publishing of
the approved package.
* Package lifecycle operations, such as the following:

  * The assisted or automated rollout of a package upgrade when a new version of the upstream
  package version becomes available (the three-way merge).
  * The rollback of a package to its previous version.

* The deployment of the packages from the deployment repositories, and the observability of their
deployment status.
* A permission model that allows role-based access control (RBAC).

### High-level architecture

At the high level, the Core CaD functionality consists of the following components:

* A generic (that is, not task-specific) package orchestration service implementing the following:

  * package repository management
  * package discovery, authoring, and lifecycle management

* The Porch CLI tool [porchctl]({{% relref "/docs/7_cli_api/relevant_old_docs/porchctl-cli-guide.md" %}}): this is a Git-native,
schema-aware, extensible client-side tool for managing KRM packages.
* A GitOps-based deployment mechanism (for example [configsync][]), which distributes and deploys
configurations, and provides observability of the status of the deployed resources.
* A task-specific UI supporting repository management, package discovery, authoring, and lifecycle.

![CaD Core Architecture](/static/images/porch/CaD-Core-Architecture.svg)

## CaD concepts elaborated

The concepts that were briefly introduced in **High-level architecture** are elaborated in more
detail in this section.

### Repositories

Porch and [configsync][] currently integrate with [git][] repositories. There is an existing design
that adds OCI support to kpt. Initially, the Package Orchestration service will prioritize
integration with [git][]. Support for additional repository types may be added in the future, as
required.

Requirements applicable to all repositories include the ability to store the packages and their
versions, and sufficient metadata associated with the packages to capture the following:

* package dependency relationships (upstream - downstream)
* package lifecycle state (draft, proposed, published)
* package purpose (base package)
* customer-defined attributes (optional)

At repository registration, the customers must be able to specify the details needed to store the
packages in appropriate locations in the repository. For example, registration of a Git repository
must accept a branch and a directory.

{{% alert title="Note" color="primary" %}}

A user role with sufficient permissions can register a package or a function repository, including
repositories containing functions authored by the customer, or by other providers. Since the
functions in the registered repositories become discoverable, customers must be aware of the
implications of registering function repositories and trust the contents thereof.

{{% /alert %}}

### Package versioning

Packages are versioned sequentially. The requirements are as follows:

* The ability to compare any two versions of a package as "newer than", "equal to", or "older than"
  the other.
* The ability to support the automatic assignment of versions.
* The ability to support the [optimistic concurrency][optimistic-concurrency] of package changes
  via version numbers.
* A simple model that easily supports automation.

A simple integer sequence is used to represent the package versions.

### Package relationships

The Kpt packages support the concept of ***upstream***. When one package is cloned from another,
the new package, known as the ***downstream*** package, maintains an upstream link to the version
of the package from which it was cloned. If a new version of the upstream package becomes available,
then the upstream link can be used to update the downstream package.

### Deployment

The deployment mechanism is responsible for deploying the configuration packages from a repository
and affecting the live state. Because the configuration is stored in standard repositories (Git,

and in the future OCI), the deployment component is pluggable. By default, [Config Sync](https://cloud.google.com/kubernetes-engine/enterprise/config-sync/docs/overview) is the
deployment mechanism used by CaD Core implementation. However, other deployment mechanisms can be
also used.

Some of the key attributes of the deployment mechanism and its integration within the CaD Core are
highlighted here:

* _Published_ packages in a deployment repository are considered to be ready to be deployed.
* configsync supports the deployment of individual packages and whole repositories. For Git
  specifically, that translates to a requirement to be able to specify the repository,
  branch/tag/ref, and directory when instructing configsync to deploy a package.
* _Draft_ packages need to be identified in such a way that configsync can easily avoid deploying
  them.
* configsync needs to be able to pin to specific versions of deployable packages, in order to
  orchestrate rollouts and rollbacks. This means it must be possible to get a specific version of a
  package.
* configsync needs to be able to discover when new versions are available for deployment.

## Package Orchestration (Porch)

Having established the context of the CaD Core components and the overall architecture, the
remainder of the document will focus on the Package Orchestration service, or **Porch** for short.

The role of the Package Orchestration service among the CaD Core components covers the following
areas:

* [Repository Management](#repository-management)
* [Package Discovery](#package-discovery)
* [Package Authoring](#package-authoring) and Lifecycle

In the next sections we will expand on each of these areas. The term _client_ used in these
sections can be either a person interacting with the user interface, such as a web application or a
command-line tool, or an automated agent or process.

### Repository management

The repository management functionality of the Package Orchestration service enables the client to
do the following:

* Register, unregister, and update the registration of the repositories, and discover registered
  repositories. Git repository integration will be available first, with OCI and possibly more
  delivered in the subsequent releases.
* Manage repository-wide upstream/downstream relationships, that is, designate the default upstream
  repositories from which the packages will be cloned.
* Annotate the repositories with metadata, such as whether or not each repository contains
  deployment-ready packages. Metadata can be application- or customer-specific.

### Package discovery

The package discovery functionality of the Package Orchestration service enables the client to do
the following:

* Browse the packages in a repository.
* Discover the configuration packages in the registered repositories, and sort and/or filter them
  based on the repository containing the package, package metadata, version, and package lifecycle
  stage (draft, proposed, and published).
* Retrieve the resources and metadata of an individual package, including the latest version, or
  any specific version or draft of a package, for the purpose of introspection of a single package,
  or for comparison of the contents of multiple versions of a package or related packages.
* Enumerate the _upstream_ packages that are available for creating (cloning) a _downstream_
  package.
* Identify the downstream packages that need to be upgraded after a change has been made to an
  upstream package.
* Identify all the deployment-ready packages in a deployment repository that are ready to be synced
  to a deployment target by configsync.
* Identify new versions of packages in a deployment repository that can be rolled out to a
  deployment target by configsync.

### Package authoring

The package authoring and lifecycle functionality of the package Orchestration service enables the
client to do the following:

* Create a package _draft_ via one of the following means:

  * An empty draft from scratch (`porchctl rpkg init`).
  * A clone of an upstream package (`porchctl rpkg clone`) from a registered upstream repository or
    from another accessible, unregistered repository.
  * Editing an existing package (`porchctl rpkg pull`).
  * Rolling back or restoring a package to any of its previous versions
    (`porchctl rpkg pull` of a previous version).

* Push changes to a package _draft_. In general, mutations include adding, modifying, and deleting
  any part of the package's contents. Specific examples include the following:

  * Adding, changing, or deleting package metadata (that is, some properties in the `Kptfile`).
  * Adding, changing, or deleting resources in the package.
  * Adding function mutators/validators to the package's pipeline.
  * Adding, changing, or deleting sub-packages.
  * Retrieving the contents of the package for arbitrary client-side mutations
    (`porchctl rpkg pull`).
  * Updating or replacing the package contents with new contents, for example, the results of
    client-side mutations by a UI (`porchctl rpkg push`).

* Rebase a package onto another upstream base package or onto a newer version of the same package
  (to assist with conflict resolution during the process of publishing a draft package).

* Get feedback during package authoring, and assistance in recovery from merge conflicts, invalid
  package changes, or guardrail violations.

* Propose that a _draft_ package be _published_.
* Apply arbitrary decision criteria, and by a manual or an automated action, approve or reject a
  proposal for _draft_ package to be _published_.
* Perform bulk operations, such as the following:

  * Assisted/automated updates (upgrades and rollbacks) of groups of packages matching specific
    criteria (for example, if a base package has new version or a specific base package version has
    a vulnerability and needs to be rolled back).
  * Proposed change validation (prevalidating changes that add a validator function to a base
    package).

* Delete an existing package.

#### Authoring and latency

An important aim of the Package Orchestration service is to support the building of task-specific
UIs. To deliver a low-latency user experience that is acceptable to UI interactions, the innermost
authoring loop depicted below requires the following:

* high-performance access to the package store (loading or saving a package) with caching
* low-latency execution of mutations and transformations of the package contents
* low-latency [KRM function][krm functions] evaluation and package rendering (evaluation of a
  package's function pipelines)

![Inner Loop](/static/images/porch/Porch-Inner-Loop.svg)

#### Authoring and access control

A client can assign actors (for example, persons, service accounts, and so on) to roles that
determine which operations they are allowed to perform, in order to satisfy the requirements of the
basic roles. For example, only permitted roles can do the following:

* Manipulate repository registration, and enforcement of repository-wide invariants and guardrails.
* Create a draft of a package and propose that the draft be published.
* Approve or reject a proposal to publish a draft package.
* Clone a package from a specific upstream repository.
* Perform bulk operations, such as rollout upgrade of downstream packages, including rollouts
  across multiple downstream repositories.

### Porch architecture

The Package Orchestration (**Porch**) service is designed to be hosted in a
[Kubernetes](https://kubernetes.io/) cluster.

The overall architecture is shown in the following figure. It also includes existing components,
such as the k8s apiserver and configsync.

![Porch Architecture](/static/images/porch/Porch-Architecture.svg)

In addition to satisfying the requirements highlighted above, the focus of the architecture was to
do the following:

* Establish clear components and interfaces.
* Support a low-latency package authoring experience required by the UIs.

The Porch architecture comprises three components:

* the Porch server
* the function runner
* the CaD Library

#### Porch server

The Porch server is implemented as a [Kubernetes extension API server][apiserver]. The benefits of
using the Kubernetes extension API server are as follows:

* A well-defined and familiar API style.
* The availability of generated clients.
* Integration with the existing Kubernetes ecosystem and tools, such as the `kubectl` CLI,
  [RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/).
* The Kubernetes extension API server removes the need to open another network port to access a
  separate endpoint running inside the k8s cluster. This is a clear advantage over Google Remote
  Procedure Calls (GRPC), which was considered as an alternative approach.

The resources implemented by Porch include the following:

* `PackageRevision`: This represents the _metadata_ of the configuration package revision stored in
  a _package_ repository.
* `PackageRevisionResources`: This represents the _contents_ of the package revision.

{{% alert title="Note" color="primary"%}}

Each configuration package revision is represented by a _pair_ of resources, each of which presents
a different view, or a [representation][] of the same underlying package revision.

{{% /alert %}}

Repository registration is supported by a `Repository` [custom resource][crds].

The **Porch server** itself comprises several key components, including the following:

* The *Porch aggregated apiserver*
  The *Porch aggregated apiserver* implements the integration into the main Kubernetes apiserver,
  and directly serves the API requests for the `PackageRevision`, `PackageRevisionResources`
  resources.
* The Package Orchestration *engine*
  The Package Orchestration *engine* implements the package lifecycle operations, and the package
  mutation workflows.
* The *CaD Library*
  The *CaD Library* implements specific package manipulation algorithms, such as package rendering
  (the evaluation of a package's function *pipeline*), the initialization of a new package, and so
  on. The CaD Library is shared with `kpt`, where it likewise provides the core package
  manipulation algorithms.
* The *package cache*
  The *package cache* enables both local caching, as well as the abstract manipulation of packages
  and their contents, irrespective of the underlying storage mechanism, such as Git, or OCI.
* The *repository adapters* for Git and OCI
  The *repository adapters* for Git and OCI implement the specific logic of interacting with those types of package
  repositories.
* The *function runtime*
  The *function runtime* implements support for evaluating the [kpt functions][functions] and the
  multitier cache of functions to support low-latency function evaluation.

#### Function runner

The **function runner** is a separate service that is responsible for evaluating the
[kpt functions][functions]. The function runner exposes a Google Remote Procedure Calls
([GRPC](https://grpc.io/)) endpoint, which enables the evaluation of a kpt function on the provided
configuration package.

The GRPC technology was chosen for the function runner service because the
[requirements](#grpc-api) that informed the choice of the KRM API for the Package Orchestration
service do not apply. The function runner is an internal microservice, an implementation detail not
exposed to external callers. This makes GRPC particularly suitable.

The function runner also maintains a cache of functions to support low-latency function evaluation.
It achieves this through two mechanisms that are available for the evaluation of a function.

The **Executable Evaluation** approach executes the function within the pod runtime through a
shell-based invocation of the function binary, for which the function binaries are bundled inside
the function runner image itself.

The **Pod Evaluation** approach is used when the invoked function is not available via the
Executable Evaluation approach, wherein the function runner pod starts the function pod that
corresponds to the invoked function, along with a front-end service. Once the pod and the service
are up and running, its exposed GRPC endpoint is invoked for function evaluation, passing the input 
package. For this mechanism, the function runner reads the list of functions and their images
supplied via a configuration file at startup, and spawns function pods, along with a corresponding
front-end service for each configured function. These function pods and services are terminated
after a preconfigured period of inactivity (the default is 30 minutes) by the function runner and
are recreated on the next invocation.

#### CaD Library

The [kpt](https://kpt.dev/) CLI already implements foundational package manipulation algorithms, in
order to provide the command line user experience, including the following:

* [kpt pkg init](https://kpt.dev/reference/cli/pkg/init/): this creates an empty, valid KRM package.
* [kpt pkg get](https://kpt.dev/reference/cli/pkg/get/): this creates a downstream package by
  cloning an upstream package. It sets up the upstream reference of the downstream package.
* [kpt pkg update](https://kpt.dev/reference/cli/pkg/update/): this updates the downstream package
  with changes from the new version of the upstream, three-way merge.
* [kpt fn eval](https://kpt.dev/reference/cli/fn/eval/): this evaluates a kpt function on a package.
* [kpt fn render](https://kpt.dev/reference/cli/fn/render/): this renders the package by executing
  the function pipeline of the package and its nested packages.
* [kpt fn source](https://kpt.dev/reference/cli/fn/source/) and
  [kpt fn sink](https://kpt.dev/reference/cli/fn/sink/): these read packages from a local disk as
  a `ResourceList` and write the packages represented as a `ResourcesList` into the local disk.

The same set of primitives form the building blocks of the package orchestration service. Further,
the Package Orchestration service combines these primitives into higher-level operations (for
example, package orchestrator renders the packages automatically on changes. Future versions will
support bulk operations, such as the upgrade of multiple packages, and so on).

The implementation of the package manipulation primitives in the kpt was refactored (with the
initial refactoring completed, and more to be performed as needed), in order to do the following:

* Create a reusable CaD library, usable by both the kpt CLI and the Package Orchestration service.
* Create abstractions for dependencies which differ between the CLI and Porch. Most notable are
  the dependency on Docker for function evaluation, and the dependency on the local file system for
  package rendering.

Over time, the CaD Library will provide the package manipulation primitives, to perform the
following tasks:

* Create a valid empty package (init).
* Update the package upstream pointers (get).
* Perform three-way merges (update).
* Render: using a core package rendering algorithm that uses a pluggable function evaluator, to
  support the following:

  * Function evaluation via Docker (used by kpt CLI).
  * Function evaluation via an RPC to a service or an appropriate function sandbox.
  * High-performance evaluation of trusted, built-in functions without a sandbox.

* Heal the configuration (restore comments after lossy transformation).

Both the kpt CLI and Porch will consume the library. This approach will allow the leveraging of the
investment already made into the high-quality package manipulation primitives, and enable
functional parity between the kpt CLI and the Package Orchestration service.

## User Guide

The Porch User Guide can be found in a dedicated document, via this link:
[document]({{% relref "/docs/4_tutorials_and_how-tos/" %}}).

## Open issues and questions

### Deployment rollouts and orchestration

__Not Yet Resolved__

Cross-cluster rollouts and orchestration of deployment activity. For example, a package deployed by
configsync in cluster A, and only on success, the same (or a different) package deployed by
configsync in cluster B.

## Alternatives considered

### GRPC API

The use of Google Remote Procedure Calls ([GRPC]()) was considered for the Porch API. The primary
advantages of implementing Porch as an extension of the Kubernetes apiserver are as follows:

* Customers would not have to open another port to their Kubernetes cluster and would be able to
  reuse their existing infrastructure.
* Customers could likewise reuse the existing Kubernetes tooling ecosystem.

<!-- Reference links -->
[krm]: https://github.com/kubernetes/design-proposals-archive/blob/main/architecture/resource-management.md
[functions]: https://kpt.dev/book/02-concepts/#functions
[krm functions]: https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md
[pipeline]: https://kpt.dev/book/04-using-functions/#declarative-function-execution
[Config Sync]: https://cloud.google.com/anthos-config-management/docs/config-sync-overview
[kpt]: https://kpt.dev/
[git]: https://git-scm.org/
[optimistic-concurrency]: https://en.wikipedia.org/wiki/Optimistic_concurrency_control
[apiserver]: https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/apiserver-aggregation/
[representation]: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#differing-representations
[crds]: https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/
[oci]: https://github.com/opencontainers/image-spec/blob/main/spec.md
