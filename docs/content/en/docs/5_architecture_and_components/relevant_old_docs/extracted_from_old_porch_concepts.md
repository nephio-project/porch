---
title: "[### Extracted from Old Porch Concepts Document ###]"
type: docs
weight: 4
---

### Package Relationships - Upstream and Downstream

kpt packages support the concept of ***upstream*** and ***downstream*** relationships. When a package is cloned from another,
the new package (the downstream package) maintains an upstream link to the specific package revision from which it was cloned.
If a new revision of the upstream package is published, the upstream link can be used to upgrade the downstream package.

### High-Level CaD Architecture

At the high level, the CaD functionality comprises:

* A generic (i.e. not task-specific) package orchestration service implementing:
  * package revision authoring and lifecycle management.
  * package repository management.

* [porchctl]({{% relref "/docs/7_cli_api/porchctl.md" %}}) - a Git-native, schema-aware, extensible client-side
  tool for managing KRM packages in Porch.
* A GitOps-based deployment mechanism (for example, [Config Sync](https://cloud.google.com/anthos-config-management/docs/config-sync-overview)
  or [FluxCD](https://fluxcd.io/)), which distributes and deploys configuration, and provides observability of the status
  of deployed resources.
* A task-specific UI supporting repository management, package discovery, authoring, and lifecycle.

![CaD Core Architecture](/static/images/porch/CaD-Core-Architecture.svg)

### Porch Architecture

Porch consists of several microservices, designed to be hosted in a [Kubernetes](https://kubernetes.io/) cluster.

The overall architecture is shown below, including additional components external to Porch (Kubernetes API server and deployment
mechanism).

![Porch Architecture](/static/images/porch/Porch-Architecture.drawio.svg)

In addition to satisfying requirements highlighted above, the focus of the architecture is to:

* establish clear components and interfaces.
* support low latency in package authoring operations.

The primary Porch components are:

#### Porch Server

The Porch server is implemented as a [Kubernetes extension API server](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/apiserver-aggregation/) which works with the Kubernetes API
aggregation layer. The benefits of this approach are:

* seamless integration with the well-defined Kubernetes API style
* availability of generated clients for use in code
* integration with existing Kubernetes ecosystem and tools such as  `kubectl` CLI,
  [RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
* avoids requirement to open another network port to access a separate endpoint running inside k8s cluster
  * this is a distinct advantage over GRPC which was initially considered as an alternative approach

The Porch server serves the primary Kubernetes
resources required for basic package authoring and lifeycle management, including:

* For each package revision (see [Package Revisions]({{% relref "/docs/2_concepts/package-revisions.md" %}})):
  * `PackageRevision` - represents the *metadata* of the package revision stored in a repository.
  * `PackageRevisionResources` - represents the *file contents* of the package revision.
    {{% alert color="primary" %}}
  Note that each package revision is represented by a *pair* of resources, each presenting a different view
  (or [representation](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#differing-representations))
  of the same underlying package revision.
    {{% /alert %}}
* A `Repository` [custom resource](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), which supports repository registration.

The **Porch server** itself includes the following key components:

* The *aggregated API server*, which implements the integration into the main Kubernetes API server and 
  serves API requests for the `PackageRevision` and `PackageRevisionResources` resources.
* Package orchestration *engine*, which implements the package lifecycle operations and package mutation workflows.
* *CaD Library*, which implements specific package manipulation algorithms such as package rendering (evaluation of
  package's function *pipeline*), initialization of a new package, etc. The CaD Library is a fork of `kpt` to allow Porch
  to reuse the `kpt` algorithms and fulfil its overarching use case to be "kpt as a service".
* *Package cache*, which enables:
  * local caching to allow package lifecycle and content manipulation operations to be executed within the Porch server
    with minimal latency.
  * abstracting package operations upward so they can be used without having to take account of the underlying storage
    repository software mechanism (Git or OCI).
* *Repository adapters* for Git and OCI, which implement the specific logic of interacting with each repository type.
* *Function Runner runtime*, which evaluates individual [KRM functions][functions] (or delegates to the dedicated
  [function runner](#function-runner)), incorporating a multi-tier cache of functions to support low-latency evaluation.

#### Function Runner

The **Function Runner** is a separate microservice responsible for evaluating [KRM functions][functions]. It exposes
a [GRPC](https://grpc.io/) endpoint which enables evaluating a specified kpt function on a provided configuration package.

GRPC was chosen for the function runner service because the [benefits of an API server](#porch-server) that prompted its use
for the Porch server do not apply in this case. The function runner is an internal microservice, an implementation detail not exposed
to external callers, which makes GRPC perfectly suitable.

The function runner maintains a cache of functions to support low-latency function evaluation. It achieves this through
two mechanisms available to it for evaluation of a function:

* The **Executable Evaluation** mechanism executes the function directly inside the `function-runner` pod through shell-based
  invocation of a function's binary executable. This applies only to a selected subset of popular functions, whose binaries
  are baked into the `function-runner` image itself at compile-time to form a sort of pre-cache.
* The **Pod Evaluation** mechanism is the fallback when the invoked function is not one of those packaged in the `function-runner`
  image for the Executable Evaluation approach. The `function-runner` pod spawns a separate *function pod*, based on the
  image of the invoked function, along with a corresponding front-end service. Once the pod and service are ready, the
  exposed GRPC endpoint is invoked to evaluate the function with the package contents as input. Once a function pod completes
  evaluation and returns the result to the `function-runner` pod, the function pod is kept in existence temporarily so
  it can be re-used quickly as a cache hit. After a pre-configured period of disuse (default 30 minutes), the function
  runner terminates the function pod and its service, to recreate them from the start on the next invocation of that function.

#### Repository registration

At repository registration, customers must be able to specify details needed to store packages in an appropriate location
in the repository. For example, registration of a Git repository must accept a URL or directory path to locate the repository,
a branch and a directory to narrow down the location of packages, and any credentials needed to read from and/or write to
the repository

A successful repository registration results in the creation of a Repository custom resource, a *Repository object*. This
is not to be confused with, for example, the remote Git repository - the Porch repository only stores the details Porch
uses to interact with the Git repository.

{{% alert title="Note" color="primary" %}}

A user role with sufficient permissions can register a repository at practically any URL, including repositories containing
packages authored by third parties. Since the contents of the registered repositories become discoverable, a customer
registering a third-part repository must be aware of the implications and trust the contents thereof.

{{% /alert %}}


#### CaD Library

The [kpt](https://kpt.dev/) CLI already implements the fundamental package manipulation algorithms in order to provide its command line user experience:

* [kpt pkg init](https://kpt.dev/reference/cli/pkg/init/) - create a bare-bones, valid, kpt package.
* [kpt pkg get](https://kpt.dev/reference/cli/pkg/get/) - create a downstream package by cloning an upstream package;
  set up the upstream reference of the downstream package.
* [kpt pkg update](https://kpt.dev/reference/cli/pkg/update/) - update the downstream package with changes from new
  version of upstream, 3-way merge.
* [kpt fn eval](https://kpt.dev/reference/cli/fn/eval/) - evaluate a KRM function on a package.
* [kpt fn render](https://kpt.dev/reference/cli/fn/render/) - render the package by executing the function pipeline of
  the package and its nested packages.
* [kpt fn source](https://kpt.dev/reference/cli/fn/source/) and [kpt fn sink](https://kpt.dev/reference/cli/fn/sink/) -
  read package from local disk as a `ResourceList` and write package represented as `ResourcesList` into local disk.

The same set of primitive operations form the foundational building blocks of the package orchestration service. Further,
Porch combines these blocks into higher-level operations (for example, Porch renders packages automatically on changes;
future versions will support bulk operations such as upgrade of multiple packages, etc.).

A longer-term goal is to refactor kpt and Porch to extract the package manipulation operations into a reusable CaD Library, which will consumed by both the kpt CLI and Porch to allow them equal reuse of the same operations:
* create a valid empty package (init).
* clone a package and add upstream pointers (get).
* perform 3-way merge (upgrade).
* render - core package rendering algorithm using a pluggable function evaluator to support:
  * function evaluation via Docker (as used by kpt CLI).
  * function evaluation via an RPC to a service or appropriate function sandbox.
  * high-performance evaluation of trusted, built-in, functions without sandbox.
* heal configuration (restore comments after lossy transformation).

This approach will allow leveraging the investment already made into the high-quality package manipulation operations, maintain functional parity between the kpt CLI and Porch, and allow dependencies to be abstracted away which differ between CLI and Porch (most notably the dependency on Docker for function evaluation and on the local file system for package rendering).


<!-- Reference links -->
[functions]: https://kpt.dev/book/02-concepts/#functions