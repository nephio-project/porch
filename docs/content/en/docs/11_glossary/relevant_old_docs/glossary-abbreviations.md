---
title: Glossary and Abbreviations
description: 
weight: 10
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

We use many terms in our Nephio discussions, coming from different domains
including telecommunications, Kubernetes, configuration management, and our own
[Nephio-specific](#nephio-related-abbreviations) terms. This glossary is intended to help clarify our usage of
these terms.


## Config
See [Configuration](#configuration).

## Config Injection
See [Injector](#injector).

## Configuration
In Nephio, this *usually* refers to the Kubernetes resources used to provision
and manage network functions, their underlying infrastructure, and their
internal operation. Unfortunately this is a very general term and often is
overloaded with multiple meanings.

Sometimes, folks will say *network config* or *workload config* to refer to the
internal configuration of the network functions. Consider that most network
functions today cannot be directly configured via Kubernetes resources. Instead,
they are configured via a proprietary configuration file, netconf, or even an
API. In that case, those terms usually refer to this proprietary configuration
language rather than Kubernetes resources. It is a goal for Nephio to help
vendors enable KRM-based management of this internal configuration, to allow
leveraging all the techniques we are building for KRM-based configuration (this
is part of the "Kubernetes Everywhere" principle).

As a community, we should try to use a common set of terminology for different types of configuration. See
[docs#4](https://github.com/nephio-project/nephio/issues/266).

## Configuration as Data
Configuration as Data is an approach to management of configuration (incl. configuration of infrastructure, policy,
services, applications, etc.) which:

* makes configuration data the source of truth, stored separately from the live state
* uses a uniform, serializable data model to represent configuration
* separates code that acts on the configuration from the data and from packages / bundles of the data
* abstracts configuration file structure and storage from operations that act upon the configuration data; clients
  manipulating configuration data don’t need to directly interact with storage (git, container images)


## Controller
This term comes from Kubernetes where
[controller](https://kubernetes.io/docs/reference/glossary/?fundamental=true#term-controller) is defined as a control
loop that watches the intended and actual state of the cluster, and attempts to make changes as needed to make the
actual state match the intended state. More specifically, this typically refers to software that processes Kubernetes
Resources residing in the Kubernetes API server, and either transforms them into new resources, or calls to other APIs
that change the state of some entity external to the API server. For example, `kubelet` itself is a controller that
processes Pod resources to create and manage containers on a Node.

*See also*: [Operator](#operator), [Injector](#injector), [KRM
function](#krm-function), [Specializer](#specializer)

## Controller Manager
This term comes from Kubernetes and refers to an executable that bundles many
[controllers](#controller) into one binary.

*See also*: [Controller](#controller), [Operator](#operator)

## CR
See [Custom Resource](#custom-resource).

## CRD
See [Custom Resource Definition](#custom-resource-definition).

## Custom Resource
A Custom Resource (CR) is a resource in a Kubernetes API server that has a
Group/Version/Kind. It was added to the API server via a
[Custom Resource Definition](#custom-resource-definition). The
relationship between a CR and a CRD is analogous to that of an object and a
class in Object-Oriented Programming; the CRD defines the schema, and the CR is
a particular instance.

Note that it is common for people to say "CRD" when in fact they mean "CR", so
be sure to ask for clarification if necessary.

*See also*: [Custom Resource Definition](#custom-resource-definition)

## Custom Resource Definition
A [Custom Resource
Definition (CRD)](https://kubernetes.io/docs/reference/glossary/?fundamental=true#term-CustomResourceDefinition)
is a built-in Kubernetes resource used to define custom resources
within a Kubernetes API server. It is used to extend the functionality
of a Kubernetes API server by adding new resource types. The CRD, identified by
its Group/Version/Kind, defines the schema associated with the resource, as well
as the resource API endpoints.

Note that it is common for people to say "CRD" when in fact they mean "CR", so
be sure to ask for clarification if necessary.

*See also*: [Custom Resource](#custom-resource)

## Dehydration
See [Hydration](#hydration).

## DRY
This is a common software engineering term that stands for [Don't Repeat
Yourself](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself).  DRY attempts
to reduce repetition in software development. In the Kubernetes configuration
management context, a good example is a Helm chart, which attempts to abstract
the particular manifests for a given workload. A kpt package that is not yet
ready to deploy is also an example of a DRY artifact. In general, any sort of
"template" or "blueprint" is usually an attempt to capture some repeatable
pattern, following this principle.

*See also*: [Hydration](#hydration), [WET](#wet)

## Fanout
This term refers to the process of taking a package and customizing it across a
series of targets. It is a type of [Variant Generation](#variant-generation) but
more specific than that term. It is also an application of the [DRY](#dry)
principle.

Some examples:
 * A script that loops through an array, feeding values into Helm and rendering
   individually specialized manifests for each entry in the array.
 * The PackageDeployment controller from the ONE Summit 2022 Workshop uses a
   label selector to identify target clusters, then clones a kpt package for
   each, creating one package revision per cluster.
 * The PackageVariantSet controller in Porch can be used to clone a package
   across a set of repositories, or can create multiple clones of the same
   package with different names in a single repository, based on arbitrary
   object selectors.

*See also*: [Hydration](#hydration), [Variant](#variant), [Variant Generation](#variant-generation)

## Hydration
A play on [DRY](#dry) and [WET](#wet), this is the process by which a DRY
artifact becomes ready for deployment. A familiar example is rendering a Helm
chart. A lot of the effort in the configuration management aspects of Nephio are
spent on making the hydration process scalable, collaborative, and manageable in
Day 2 and beyond, all of which are challenges with current techniques.

Hydration may be *out-of-place*, where the source material (e.g., the Helm
chart), is separate from the output of the hydration process (the manifests).
This is probably the most familiar type of hydration, used by Helm and
Kustomize, for example. Think of it as a pipeline with an input artifact, input
values, and output artifacts.

Hydration may also be *in-place*, where modifications are directly written to
the manifests in question. There is no separate input artifact and output
artifact. Rather, you may have a starting artifact, some operations you perform
on that artifact to achieve your goal, but you store the results of those
operations directly in the same artifact. Utilization of a version control
system such as Git is critical in this case. This is the kind of hydration we
typically use when operating on kpt packages.

With out-of-place hydration, the author of the template has to figure out,
upfront, all the possible outcomes of the hydration process. Then, they have to
make available inputs to the pipeline in order to make all of those different
outcomes achievable. This leads to "over-parameterization" - where effectively
every option possible in the outputs becomes an option in the input. At that
point, you have mostly *moved* complexity rather than *reduced* complexity.
In-place hydration can help with the over-parameterization, as values that are
rarely changed by users can be edited in-place.

While related, *DRY* and *WET* are not exactly the same concepts as *in-place* and
*out-of-place* hydration. The former two refer to principles, whereas the latter
two are more about the operational pipeline.

Note that occasionally people say "dehydration" when they mean "hydration",
likely due to the fact that "dehydration" is a more familiar word in common
language. Please offer folks some leeway in this, especially since we have many
non-native English speakers.

*See also*: [DRY](#dry), [WET](#wet)

## Injection
See [Injector](#injector).

## Injector
We introduced this term during the Nephio [ONE Summit 2022
Workshop](https://github.com/nephio-project/one-summit-22-workshop#packages).
However, it has been renamed to [specializer](#specializer).

There is still the concept of an injector, but it is limited to the
PackageVariant and PackageVariantSet controllers. This process allows the author
of the PackageVariant(Set) to configure the controller to pull in a resource
from the management cluster, and copy it into the package. This allows us to
combine upstream ([DRY](#dry)) configuration with cluster-specific configuration
based upon the target cluster.

## kpt
[Kpt](https://kpt.dev) is an open source tool for managing bundles of Kubernetes
resource configurations, called kpt [packages](#package), using the
[Configuration as Data](#configuration-as-data) methodology.

The `kpt` command-line tool allows pulling, pushing, cloning and otherwise
managing packages stored in version control repositories (Git or OCI), as well
as execution of [KRM functions](#krm-function) to perform consistent and
repeatable modifications to package resources.

[Porch](#porch) provides these package management, manipulation, and lifecycle
operations in a Kubernetes-based API, allowing automation of these operations
using standard Kubernetes controller techniques.

## kpt Function
See [KRM Function](#krm-function).

## KRM
See [Kubernetes Resource Model](#kubernetes-resource-model).

## KRM Function
A [KRM
Function](https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md)
is an executable that takes Kubernetes resources as inputs, and produces
Kubernetes resources as outputs. The function may add, remove, or modify the
input resources to produce the outputs. This is similar to a Unix pipeline, but
with KRM on the input and output, rather than simple streams.

Generally, best practices suggest KRM functions be hermetic (that is, they do
not access the outside world).

In terms of the specification linked above, Kustomize, kpt, and Porch are all
*orchestrators*.

*See also*: [Controller](#controller), [kpt](#kpt), [Porch](#porch)

## Kubernetes Resource Model
The [Kubernetes Resource Model
(KRM)](https://github.com/kubernetes/design-proposals-archive/blob/main/architecture/resource-management.md)
is the underlying declarative, intent-based API model and machinery for
Kubernetes. It is the general name for what you likely think of when you hear
"Kubernetes API". Additional background:
* [Kubernetes API Overview](https://kubernetes.io/docs/concepts/overview/kubernetes-api/)
* [Kubernetes API
  Concepts](https://kubernetes.io/docs/reference/using-api/api-concepts/)

## Manifest
A file (or files) containing a representation of resources. Typically YAML
files, but it could also be JSON or some other format.

## Mutation
The act of changing the configuration. There are different processes that can be
used for mutation, including controllers, specializers, KRM functions, web
hooks, and manual in-place edits.

*See also*: [Validation](#validation)

## Operator
An operator is a software component - usually a collection of one or more
[controller managers](#controller-manager) - that manages a particular type of
workload. For example, a set of Kubernetes controllers to manage MySQL instances
would be an operator.

Speaking loosely, [controller](#controller) and operator are often used
interchangeably, though an operator always refers to code managing CRs rather
than Kubernetes built-in types.

See [CNFs and
Operators](https://docs.google.com/document/d/1Le8TUgr0dXix7fvq7BqMSY3rgeEwaxW7mEf9G72itBI/edit?usp=sharing)
for a thorough discussion.

## Package
Generically, a logical grouping of Kubernetes resources or templated resources,
for example representing a particular workload or network function installation.

For kpt packages, this specifically means well-formed Kubernetes resources along
with a Kptfile. See the kpt [package
documentation](https://kpt.dev/book/02-concepts/01-packages).

This could also refer to a Helm chart, though generally we mean "kpt package"
when we say "package".

## Package Revision
This specifically refers to the Porch `PackageRevision` resource. Porch adds
opinionated versioning and lifecycle management to packages, beyond what the
baseline `kpt` CLI expects. See the [Porch
documentation](https://kpt.dev/book/08-package-orchestration/04-package-authoring)
for more information.

## Porch

[Porch](https://kpt.dev/book/08-package-orchestration/) is "kpt-as-a-service",
providing opinionated package management, manipulation, and lifecycle
operations in a Kubernetes-based API. This allows automation of these
operations using standard Kubernetes controller techniques.

Short for **P**ackage **Orch**estration.

*See also*: [kpt](#kpt)

## Resource

A [Kubernetes
term](https://kubernetes.io/docs/reference/using-api/api-concepts/#standard-api-terminology)
referring to a specific object stored in the API server,
although we also use it to refer to the external representation of that object
(for example text in a YAML file).

Also see [REST](https://en.wikipedia.org/wiki/Representational_state_transfer).

## Specializer
This refers to a software component that runs in the Nephio Management cluster,
and could be considered a type of [controller](#controller). However, it
specifically watches for `PackageRevision` resources in a Draft state, and
checks for the conditions on those resources. When it finds
unsatisfied conditions of the type it handles, the specializer will
[mutate](#mutation) (modify) the Draft package by adding or
changing resources.

For example, the IPAM specializer monitors package revision drafts for unresolved
IP address claims. When it sees one, it takes information from the claim and
uses it to allocate an IP address from the IP address management system. It
writes the result back into the draft package, where a KRM function can process
the result and copy ([propagate](#value-propagation)) it to the correct
resources in the package.

## Validation
The act of verifying that the configuration is syntactical correct, and that it
matches a set of rules (or policies). Those rules or policies may be for
internal consistency (e.g., matching Deployment and Service label selectors),
or they may be organizationally related (e.g., all Deployments must contain a
label indicating cost allocation center).

## Value Propagation
The same value in a configuration is often used in more than one place. *Value
propagation* is the technique of setting or generating the value once, and then
copying (or propagating) it to different places in the configuration. For
example, setting a Helm value in the *values.yaml* file, and then having it used
in multiple places across different resources.

## Variant
A *variant* is an modified version of a package. Sometimes it is the output of
the hydration process, particularly when using out-of-place hydration. For
example, if you use the same Helm chart with different inputs to create
per-cluster workloads, you are generating variants.

In Nephio, we use kpt packages to help keep an association between a package and
the variants of that package. When you clone a kpt package, an association is
maintained with the upstream package. Every deployable variant of a package is a
clone of the original, upstream package. This assists greatly in Day 2
operations; when you update the original package, you can identify all variants
and merge the updates from the upstream into the downstream. This behavior is
automated via the PackageVariant controller.

## Variant Generation
The process of creating [variants](#variant), typically in an automated way.
Variants could be created across different dimensions - for example, you could
create a package per cluster. Alternatively, you may create a variant per
environment - for example, development, staging, and production variants.

Different methods may be warranted depending on the reason for your variants. In
the ONE Summit 2022 Workshop, the PackageDeployment controller generated
variants based upon the target clusters. The Porch PackageVariantSet allows more
general-purpose generation of variants, based upon an explicitly list, a label
selector on repositories, or an arbitrary object selector. As we develop Nephio,
we may build new types of variant generators, and may even compose them (for
example, to produce variants that are affected by both environment and cluster).

## WET

This term, which we use as an acronym for "Write Every Time", comes from
[software engineering](https://en.wikipedia.org/wiki/Don%27t_repeat_yourself), and is a somewhat pejorative term in
contrast to [DRY](#dry). However, in the context of *configuration-as-data*, rather than *code*, the idea of storing the
configuration as fully-formed data enables automation and the use of data-management techniques to manage the
configuration at scale.

*See also*: [DRY](#dry), [Hydration](#hydration)

## Workload

A workload is any application running on Kubernetes, including network
functions.

## Nephio Related Abbreviations

* PV: Package Variant
* PVS: Package Variant Set

## 5G 3GPP Related Abbreviations

* NF: Network Function
* AMF: Access and Mobility Management Function
* SMF: Session Management Function
* UPF: User Plane Function
* AUSF: Authentication Server Function
* NRF: Network Repository Function
* UDR: Unified Data Repository
* UDM: Unified Data Management
* DNN: Data Network Name

## Kubernetes Networking

* NAD: Network Attachment Definition
