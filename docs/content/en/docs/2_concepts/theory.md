---
title: "Theoretical Concepts"
type: docs
weight: 3
description: |
  The principles and theories behind Porch; Porch as a service which provides kpt workflows, implementing a "configuration
  as data" approach to management of kpt packages.
---

## Configuration as Data (CaD)

CaD is an approach to the management of configuration: namely, configuration data for infrastructure, policy, services,
applications, etc. To treat configuration as data means implementing the following principles:

* Making configuration data the source of truth, stored separately from the live state.
* Using a uniform, serializable data model to represent the configuration.
* Separating the data (including, if applicable, packages or bundles of data), from code that applies or acts on the
  configuration.
* Abstracting the configuration file structure and storage from operations that act on the configuration data. Clients
  manipulating the configuration data do not need to interact directly with the storage (such as git, container images,
  etc.).

![CaD Overview](/static/images/porch/CaD-Overview.svg)

### Key principles

A system based on CaD should observe the following key principles:

* *Decouple configuration abstractions* from collections of configuration data.
* Represent *abstractions of configuration generators* as data with schemas, as with other configuration data.
* *Separate the configuration data from its schemas*.
  * Rely on the schema information to distinguish between data structures and other versions/variations within the schema.
* Separate the *actuation* (reconciliation of configuration data with live state) from the *intermediate processing*
  (validation and transformation) of the configuration data.
  * Actuation should be conducted according to the declarative data model.
* Prefer *transforming configuration data* to generating it wholesale, especially for value propagation
  * except in the case of dramatic changes (for example, an expansion by 10x or more).
* *Decouple generation of transformation input data* from propagation.
* *Link the live state back to the configuration as source of truth*.

## Package Orchestration - Porch

Having established the basics of a very generic CaD architecture, the remainder of the document will focus
on **Porch** - the Package Orchestration service.

Package Orchestration - "Porch" for short - is "[kpt](https://kpt.dev/)-as-a-service". It provides opinionated,
Kubernetes-based interfaces to manage and orchestrate kpt configuration packages, allowing the use of standard Kubernetes
controller techniques to automate:
* package management, including [CRUD](https://en.wikipedia.org/wiki/Create,_read,_update_and_delete) operations, content
  manipulation, and lifecycle operations
* connection to package repositories, with automatic discovery of packages contained in them
* [WYSIWYG](https://en.wikipedia.org/wiki/WYSIWYG) package authoring
* evaluation of [KRM functions](https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md)
  on package contents
* package version control, with a proposal/approval workflow around publishing new package versions
* package customization with guardrails

To cement the role of Porch as part of the Nephio CaD implementation, it covers:

* [Repository Management](#repository-management)
* [Package Discovery](#package-discovery)
* [Package Authoring](#package-authoring) and Lifecycle

The following section expands more on each of these areas. The term *client* used in these sections can be either a person
interacting with the API (e.g., through a web application or a command-line tool), or an automated agent or process.

### Rationale behind Porch

The benefits of Configuration as Data are already available in CLI form, using kpt and the KRM function ecosystem, which
includes a kpt-hosted [function catalog](https://catalog.kpt.dev/function-catalog/). YAML files can be created and
organised into packages using any editor with YAML support. However, a WYSIWYG user experience of package management is
not yet available which can support broader package lifecycle management and necessary development guardrails.

Porch enables development of such a user experience. It enables workflows similar to those supported by the kpt CLI, offering
them as-a-service over an API and CLI which provide lifecycle management of kpt packages, package authoring with guardrails,
a proposal/approval workflow, package deployment, and more.

### Repository Management

Porch's repository management functionality enables the client to manage Porch repositories:

* Register (create) and unregister (delete) repositories.
  * A repository may be registered as a *deployment* repository to indicate that it contains deployment-ready packages.
* Discover (read) and update registered repositories.
  * Since Porch repositories are Kubernetes objects, the update operation may be used to add arbitrary metadata, in the
    form of annotations or labels, for the benefit of applications or customers.

Git repository integration is available, with limited experimental support for OCI.

### Package Discovery

Porch's package discovery functionality enables the client to read package data:

* List package revisions in registered repositories.
  * Sort and filter based on package metadata (labels) or a selection of field values.
  * To improve performance and latency, package revisions are automatically discovered and cached in Porch upon repository
    registration.
  * Porch's repository-synchronisation then polls the repository at a user-customizable interval to keep the cache up to
    date.
* Retrieve details of an individual package revision.
* Discover upstream packages with new latest revisions to which their downstream packages can be upgraded.
* Identify deployment-ready packages that are available to be deployed by the chosen deployment software.

### Package Authoring

Porch's package lifecycle management enables the client to orchestrate packages and package revisions:

* Create a *draft* package revision in any of the following ways:
  * Create an empty draft 'from scratch' (`porchctl rpkg init`).
  * Clone an upstream package (`porchctl rpkg clone`) from either a registered upstream repository or from an unregistered
    repository accessible by URL.
  * Edit an existing package (`porchctl rpkg edit`).

* Retrieve the contents of a package's files for local review or editing (`porchctl rpkg pull`).

* Manage approval status of a package revision:
  * Propose a *Draft* package for publication, moving it to *Proposed* status.
  * Reject a *Proposed* package, setting it back to *Draft* status.
  * Approve a *Proposed* package, releasing it to *Published* status.

* Update the package contents of a draft package revision by pushing an edited local copy to the draft (`porchctl rpkg push`).
  Example edits:
  * Add, modify, or delete resources in the package.
  * Add, modify, or delete the KRM functions in the pipeline in the package's `kptfile`.
    * e.g.: mutator functions to transform the [KRM resources](https://github.com/kubernetes/design-proposals-archive/blob/main/architecture/resource-management.md)
      in the package contents; validator functions to enforce validation.
  * Add, modify, or delete a sub-package.

* Guard against pushing invalid package changes:
  * As part of the `porchctl rpkg push` operation, Porch renders the kpt package, running the pipeline.
  * If the pipeline encounters a failure, error, or validation violation, Porch refuses to update the package contents.

* Perform bulk operations using package variants, such as:
  * Assisted/automated update (upgrade, rollback) of groups of packages matching specific criteria (e.g. base package has
    a new version; specific base package version has a vulnerability and should be rolled back).
  * Proposed change validation (pre-validating change that adds a validator function to a base package).

* Delete an existing package or package revision.

#### Authoring & Latency

An important goal of Porch is to support building of task-specific UIs. In order for Porch to sustain a quick turnaround
of operations, package authors must ensure their packages allow the innermost authoring loop (depicted below) to execute
quickly in the following areas:
* Low-latency execution of mutations and transformations on the package contents.
* Low-latency rendering of the package's KRM function pipeline.

![Inner Loop](/static/images/porch/Porch-Inner-Loop.svg)

#### Authoring & Access Control

Using Kubernetes Roles and RoleBindings, a user can apply role-based access control to limit the operations an actor (other
user, service account) can perform. For example, access can be segregated to restrict who can:

* register and unregister repositories.
* create a new draft package revision and propose it for publication.
* approve (or reject) the a proposed package revision.
* clone packages from a specific upstream repository.
* perform bulk operations (using package variants, scripts, user-developed client, etc.) such as rolling out upgrade of
  downstream packages, including rollouts across multiple downstream repositories.
