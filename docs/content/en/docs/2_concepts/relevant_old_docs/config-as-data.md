---
title: "Configuration as Data"
type: docs
weight: 2
description: 
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>


## Configuration as data (CaD)

CaD is an approach to the management of configuration. It includes the configuration of
infrastructure, policy, services, applications, and so on. CaD performs the following actions:

* Making configuration data the source of truth, stored separately from the live state.
* Using a uniform, serializable data model to represent the configuration.
* Separating the code that acts on the configuration from the data and from packages/bundles of
  data.
* Abstracting the configuration file structure and storage from the operations that act on the
  configuration data. Clients manipulating the configuration data do not need to interact directly
  with the storage (such as git, container images, and so on).

![CaD Overview](/static/images/porch/CaD-Overview.svg)

### Key principles

A system based on CaD should observe the following key principles:

* Separate *handling of secret data* (credentials, certificates, etc.) out to a secret-focused storage system, such as
  ([cert-manager](https://cert-manager.io/)).
* Maintain a *versioned history of configuration changes* to bundles of related configuration data.
* Maintain *uniformity and consistency of the configuration format*, including type metadata, and rely on this to enable
  pattern-based operations on the configuration data (along the lines of [duck typing](https://en.wikipedia.org/wiki/Duck_typing)).
* *Separate the configuration data from its schemas*.
  * Rely on the schema information to:
    * define strongly-typed operations.
    * disambiguate data structures and other variations within the schema.
* *Decouple configuration abstractions* from collections of configuration data.
* Represent *abstractions of configuration generators* as data with schemas, as with other configuration data.
* Implement *get/list functionality* to find, filter, query, select, and/or validate:
  * configuration data.
  * code (functions) that can operate on the resource types that make up configuration data.
* Separate the *actuation* (reconciliation of configuration data with live state) from the *intermediate processing*
  (validation and transformation) of the configuration data.
  * Actuation should be conducted according to the declarative data model.
* Prefer *transforming configuration data* to generating it wholesale, especially for value propagation
  * except in the case of dramatic changes (for example, an expansion >10x).
* *Decouple generation of transformation input data* from propagation.
* Obtain deployment context inputs from *well-defined "provider context" objects*.
* *Identifiers and references* should be declarative.
* *Link the live state back to the configuration as source of truth*.

## Kubernetes Resouce Model configuration as data (KRM CaD)

The kpt implementation of the Configuration as Data approach ([kpt][kpt], [Config Sync][Config Sync], and [Package Orchestration][Porch])
is built on the foundation of the [Kubernetes Resource Model][krm] (KRM).

{{% alert title="Note" color="primary" %}}

KRM is not a hard requirement of CaD, just as Python or Go templates, or Jinja, are not specifically requirements for
[IaC](https://en.wikipedia.org/wiki/Infrastructure_as_code). However, the choice of a different fundamental format for
configuration data would necessitate the implementation of adapters for all types of infrastructure and applications
configured, including Kubernetes, CRDs, and GCP resources. Likewise, choosing another configuration format would require
the redesign of several of the configuration management mechanisms that have already been designed for KRM, such as three-way
merge, structural merge patch, schema descriptions, resource metadata, references, status conventions, and so on.

{{% /alert %}}


**KRM CaD**, then, is a specific approach to implementing *Configuration as Data*. It uses and builds on the following
existing concepts:

* [KRM][krm] as the configuration serialization data model.
* [Kptfile](https://kpt.dev/reference/schema/kptfile/) to store kpt package metadata.
* [ResourceList](https://kpt.dev/reference/schema/resource-list/) as a serialized package wire format.
* A kpt function with input → output in the form `ResourceList → ResultList` as the foundational, composable unit of code
  with which to conduct package manipulation.

  {{% alert title="Note" color="primary" %}}

  Other forms of code can also manipulate packages, such as UIs and custom algorithms not necessarily packaged and used
  as kpt functions.

  {{% /alert %}}


KRM CaD provides the following basic use cases:

* Create a new (empty) kpt package.
* Load a serialized package from a repository (as a ResourceList). Examples of a repository may be one or more of the
  following:
  * Local HDD
  * Git repository
  * OCI
  * Cloud storage
* Save a serialized package (as a ResourceList) to a package repository.
* Evaluate a function on a serialized package (ResourceList).
* [Render](https://kpt.dev/book/04-using-functions/#declarative-function-execution) a package (in the process evaluating
  the functions declared within the package itself).
* Create a new (empty) package.
* Fork (or clone) an existing package from one package repository (called *upstream*) to another (called *downstream*).
* Delete a package from a repository.
* Associate a version with a package's condition at a particular point in time.
  * Publish a package with an assigned version, guaranteeing the immutability of the package at that version.
* Incorporate changes from a newly-published version of an upstream package into a new version of a downstream package
  (three-way merge).
* Revert a package to a prior version.

### Configuration values

The CaD approach enables the following key values, which other configuration management approaches provide to a lesser
extent or not at all:

* Capabilities enabled by version-control (unlike imperative tools, such as UI and CLI, that directly modify the live
  state via APIs):
  * Versioning
  * Undo
  * Configuration history audits
  * Review/approval flows
  * Previewing before deployment
  * Validation and safety checks
  * Constraint-based policy enforcement
  * Disaster recovery
* *Detecting and remedying drift* in the live state via continuous reconciliation, whether by direct re-application or
  targeted mutations of the sources of truth.
* *Exporting state* to reusable blueprints without needing to create and manage templates manually.
* *Bulk-changing* configuration data across multiple sources of truth.
* *Configuration injection* to address horizontally-applied variations.
  * Maintaining the resulting configuration variants without needing to invest effort wither in parameterisation frameworks
    or in manually constructing and maintaining patches.
* *Merging* of multiple sources of truth.

* *Simplified configuration authoring* using a variety of sources and editing methods.
* *What-you-see-is-what-you-get (WYSIWYG) interaction* with the configuration using a simple data serialization formation,
  rather than a code-like format.
* *Layering of interoperable interface surfaces* (notably GUIs) over the declarative configuration mechanisms, rather than
  forcing choices between exclusive alternatives (exclusively, UI/CLI or IaC initially, followed by exclusively UI/CLI or
  exclusively IaC).
  * The ability to apply UX techniques to simplify configuration authoring and viewing.
* *Cooperative configuration editing* by human and automated agents (for example, for security remediation, which is usually
  implemented against live-state APIs).

* *Combination of multiple* independent configuration transformations.
* *Reusability of configuration transformation functions* and function code across multiple bodies of configuration data
  containing the same resource types.
  * Reducing the frequency of changes to the existing transformation code.
* *Language-agnostic implementation* of configuration transformations, including both programming and scripting approaches.
* *Separation of roles* - for example, between developer (configuration author) and non-developer (configuration user).
* *Improved security on sources of truth*, including access control, validation, and invariant enforcement.

#### Related articles

For more information about Configuration as Data and the Kubernetes Resource Model, visit the following links:

* [Rationale for kpt](https://kpt.dev/guides/rationale)
* [Understanding Configuration as Data](https://cloud.google.com/blog/products/containers-kubernetes/understanding-configuration-as-data-in-kubernetes)
  blog post
* [Kubernetes Resource Model](https://cloud.google.com/blog/topics/developers-practitioners/build-platform-krm-part-1-whats-platform)
  blog post series


## Core Components of Configuration as Data Implementation

The Package Orchestration CaD implementation consists of a set of components and APIs enabling the following broad use
cases:

* Register repositories (Git, OCI) containing kpt packages.
* Automatically discover existing packages in registered repositories.
* Manage package revision lifecycle, including:
  * Authoring and versioning of a package through creation, mutation, and deletion of package revision drafts.
  * A 2-step approval process where a draft package revision is first proposed for publishing, and only published on a
    second (approval) operation.
* Manage package lifecycle - operations such as:
  * Package upgrade - assisted or automated rollout of a downstream (cloned) package when a new revision of the upstream
    package is published.
  * Package rollback to a previous package revision.
* Deploy packages from deployment repositories and observe their deployment status.
* Role-based access control to Porch APIs via Kubernetes standard roles.

### Deployment mechanism

The deployment mechanism is responsible for deploying packages from a repository and affecting the live state. The "default"
deployment mechanism tested with the CaD implementation is [Config Sync][Config Sync], but since the configuration is stored
in repositories of standard types, the exact software used for deployment is less of a concern.

Here we highlight some key attributes of the deployment mechanism and its integration within the CaD paradigm:

* _Published_ packages in a deployment repository are considered ready to be deployed.
* _Draft_ packages need to be identified in such a way that Config Sync can easily avoid deploying them.
* Config Sync supports deploying individual packages and whole repositories. For Git specifically, this translates to a
  requirement to be able to specify repository, branch/tag/ref, and directory when instructing Config Sync to deploy a
  package.
* Config Sync needs to be able to pin to specific versions of deployable packages in order to orchestrate rollouts and
  rollbacks. This means it must be possible to *get* a specific package revision.
* Config Sync needs to be able to discover when new package versions are available for deployment.
