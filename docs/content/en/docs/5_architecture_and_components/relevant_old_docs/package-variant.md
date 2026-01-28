---
title: "Package Variant Controller"
type: docs
weight: 3
description: 
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

## Overview

When deploying workloads across large fleets of clusters, it is often necessary to modify the
workload configuration for a specific cluster. Additionally, these workloads may evolve over time
with security or other patches that require updates. in general, and [Package Orchestration]({{% relref "/docs/2_concepts/relevant_old_docs/package-orchestration.md" %}}) in particular, can assist in this.
However, they are still centered around a manual, one-by-one hydration and configuration of a
workload.

This proposal introduces a number of concepts and a set of resources for automating the creation
and lifecycle management of the package variants. These are designed to address several different
dimensions of scalability:

- the number of different workloads for a given cluster
- the number of clusters across which the workloads are deployed
- the different types or characteristics of the clusters
- the complexity of the organizations deploying the workloads
- changes to those workloads over time

For further information, see the following links:

- [Package Orchestration]({{% relref "/docs/2_concepts/relevant_old_docs/package-orchestration.md" %}})
- [#3347](https://github.com/GoogleContainerTools/kpt/issues/3347) Bulk package creation
- [#3243](https://github.com/GoogleContainerTools/kpt/issues/3243) Support bulk package upgrades
- [#3488](https://github.com/GoogleContainerTools/kpt/issues/3488) Porch: BaseRevision controller aka Fan Out
  controller - but more
- [Managing Package
  Revisions](https://docs.google.com/document/d/1EzUUDxLm5jlEG9d47AQOxA2W6HmSWVjL1zqyIFkqV1I/edit?usp=sharing)
- [Porch UpstreamPolicy Resource
  API](https://docs.google.com/document/d/1OxNon_1ri4YOqNtEQivBgeRzIPuX9sOyu-nYukjwN1Q/edit?usp=sharing&resourcekey=0-2nDYYH5Kw58IwCatA4uDQw)

## Core concepts

For this solution, the workloads are represented by packages. A package is a more general concept,
being an arbitrary bundle of resources, and is therefore sufficient to solve the problem that was
stated originally.

The idea here is to introduce a *PackageVariant* resource that manages the derivation of a variant
of a package from the original source package, and to manage the evolution of that variant over
time. This effectively automates the human-centered process for variant creation that might be used
with *kpt*, and allows you to do the following:

- Clone an upstream package locally.
- Make changes to the local package, setting values in the resources and executing KRM functions.
- Push the package to a new repository and tag it as a new version.

Similarly, the *PackageVariant* can manage the process of updating a package when a new version of
the upstream package is published. In the human-centered workflow, a user uses the `kpt pkg update`
to pull in changes to their derivative package. When using a *PackageVariant* resource, the change
is made to the upstream specification in the resource, and the controller proposes a new draft
package reflecting the outcome of the `kpt pkg update`.

Automating this process opens up the possibility of performing systematic changes that tie back to
the different dimensions of scalability. We can use data about the specific variant we are creating
to look up an additional context in the Porch cluster, and copy that information into the variant.
That context is a well-structured resource, not simply a set of key/value pairs. The KRM functions
within the package can interpret the resource, modifying other resources in the package accordingly.
The context can come from multiple sources that vary differently along those dimensions of
scalability. For example, one piece of information may vary by region, another by individual site,
another by cloud provider, and another based on whether we are deploying to development, staging,
or production. By using the resources in the Porch cluster as our input model, we can represent this
complexity in a manageable model that is reused across many packages, rather than scattered in
package-specific templates or key/value pairs without any structure. The KRM functions, also reused
across packages, but configured as needed for the specific package, are used to interpret the
resources within the package. This decouples the authoring of the packages, the creation of the
input model, and the deploy time use of that input model within the packages, thereby allowing
those activities to be performed by different teams or organizations.

The mechanism described above is referred to as configuration injection. Configuration injection
enables the dynamic, context-aware creation of variants. Another way to think about it is as a
continuous reconciliation, much like other Kubernetes controllers. In this case, the inputs are a
parent package *P* and a context *C* (which may be a collection of many independent resources),
with the output being the derived package *D*. When a new version of *C* is created by updates to
in-cluster resources, we get a new revision of *D*, customized according to the updated context.
Similarly, the user (or an automation) can monitor for new versions of *P*. When a new version
arrives, the PackageVariant can be updated to point to that new version. This results in a newly
proposed draft *D*, updated to reflect the upstream changes. This will be explained in more
detail below.

This proposal also introduces a way of “fanning out”, or creating multiple PackageVariant resources
declaratively based on a list or selector with the PackageVariantSet resource. This is combined with
the injection mechanism to enable the generation of large sets of variants that are specialized for
a particular target repository, cluster, or other resource.

## Basic package cloning

The *PackageVariant* resource controls the creation and lifecycle of a variant of a package. That
is, it defines the original (upstream) package, the new (downstream) package, and the changes, or
mutations, that need to be made to transform the upstream package into the downstream package. It
also allows the user to specify the policies around the adoption, deletion, and update of package
revisions that are under the control of the package variant controller.

The clone operation is shown in *Figure 1*.

| ![Figure 1: Basic package cloning](/static/images/porch/packagevariant-clone.png) | ![Legend](/static/images/porch/packagevariant-legend.png) |
| :---: | :---: |
| *Figure 1: Basic package cloning* | *Legend* |

{{% alert title="Note" color="primary" %}}

*Proposals* and *approvals* are not handled by the package variant controller. They are left to
other types of controller. The exception to this is the proposal to delete (there is no such thing
as a draft deletion). This is performed by the package variant controller, depending on the
specified deletion policy.

{{% /alert %}}

### PackageRevision metadata

The package variant controller utilizes Porch APIs. This means that it is not just performing a
clone operation, but is also creating a Porch *PackageRevision* resource. In particular, this
resource can contain Kubernetes metadata that is not a part of the package, as stored in the
repository.

Some of this metadata is necessary for the management of the *PackageRevision* by the package
variant controller, for example, the owner reference that indicates which *PackageVariant* created
the *PackageRevision*. This metadata is not under the user's control. However, the *PackageVariant*
resource does make the annotations and labels of the *PackageRevision* available as
values that the user may control during the creation of the *PackageRevision*. This can assist in
additional automation workflows.

## Introducing variance

Since cloning by itself is not particularly interesting, the *PackageVariant* resource also allows
you to control the various ways of mutating the original package to create the variant.

### Package context[^porch17]

Every *kpt* package that is fetched with `--for-deployment` contains a ConfigMap called
*kptfile.kpt.dev*. Analogously, when Porch creates a package in a deployment repository, it creates
a ConfigMap, if it does not already exist. *Kpt* (or Porch) automatically adds a key name to the
ConfigMap data, with the value of the package name. This ConfigMap can then be used as input to the
functions in the *kpt* function pipeline.

This process also holds true for the package revisions created via the package variant controller.
Additionally, the author of the *PackageVariant* resource can specify additional key-value pairs to
insert into the package context, as shown in *Figure 2*.

| ![Figure 2: Package context mutation](/static/images/porch/packagevariant-context.png) |
| :---: |
| *Figure 2: Package context mutation* |

While this is convenient, it can easily be misused, leading to over-parameterization. The preferred
approach is configuration injection, as described below, since it allows inputs to adhere to a
well-defined, reusable schema, rather than simple key/value pairs.

### Kptfile function pipeline editing[^porch18]

In the manual workflow, one of the ways in which packages are edited is by running KRM functions
imperatively. The *PackageVariant* offers a similar capability, by allowing the user to add
functions to the beginning of the downstream package *Kptfile* mutators pipeline. These functions
then execute before the functions present in the upstream pipeline. This method is not exactly the
same as running functions imperatively, because they are also run in every subsequent execution of
the downstream package function pipeline. However, it can achieve the same goals.

Consider, for example, an upstream package that includes a Namespace resource. In many
organizations, the deployer of the workload may not have the permissions to provision cluster-scoped
resources such as namespaces. This means that they would not be able to use this upstream package
without removing the Namespace resource (assuming that they only have access to a pipeline that
deploys with constrained permissions). By adding a function that removes Namespace resources, and
a call to set-namespace, they can take advantage of the upstream package.

Similarly, the *Kptfile* pipeline editing feature provides an easy mechanism for the deployer to
create and set the namespace, if their downstream package application pipeline allows it, as seen in
*Figure 3*.[^setns]

| ![Figure 3: KRM function pipeline editing](/static/images/porch/packagevariant-function.png) |
| :---: |
| *Figure 3: Kptfile function pipeline editing* |

### Configuration injection[^porch18]

Adding values to the package context or functions to the pipeline works for configurations that are
under the control of the creator of the *PackageVariant* resource. However, in more advanced use
cases, it may be necessary to specialize the package based on other contextual information. This
comes into play in particular when the user deploying the workload does not have direct control
over the context in which it is being deployed. For example, one part of the organization may manage
the infrastructure - that is, the cluster in which the workload is being deployed - and another part
the actual workload. It would be desirable to be able to pull in the inputs specified by the
infrastructure team automatically, based on the cluster to which the workload is deployed, or
possibly the region in which the cluster is deployed.

To facilitate this, the package variant controller can "inject" configuration directly into the
package. This means it uses information specific to this instance of the package to look up a
resource in the Porch cluster and copy that information into the package. The package has to be
ready to receive this information. Therefore, there is a protocol that is used to facilitate this:

- Packages may contain resources annotated with *kpt.dev/config-injection*
- These resources are often also *config.kubernetes.io/local-config* resources, as they are likely
  to be used only by the local functions as input. However, this is not mandatory.
- The package variant controller looks for any resource in the Kubernetes cluster that matches the
  Group, Version, and Kind of the package resource, and satisfies the injection selector.
- The package variant controller copies the specification field from the matching in-cluster
  resource to the in-package resource, or the data field, in the case of a ConfigMap.

| ![Figure 4: Configuration injection](/static/images/porch/packagevariant-config-injection.png) |
| :---: |
| *Figure 4: Configuration injection* |

{{% alert title="Note" color="primary" %}}

Because the data is being injected from the Kubernetes cluster, this data can also be monitored for
changes. For each resource that is injected, the package variant controller establishes a
Kubernetes “watch” on the resource (or on the collection of such resources). A change to that
resource results in a new draft package with the updated configuration injected.

{{% /alert %}}

There are a number of additional details that will be described in the detailed design below, along
with the specific API definition.

## Lifecycle management

### Upstream changes

The package variant controller allows you to specify an upstream package revision to clone.
Alternatively, you can specify a floating tag[^notimplemented].

If you specify an upstream revision, then the downstream will not be changed unless the
*PackageVariant* resource itself is modified to point to a new revision. That is, the user must
edit the *PackageVariant* and change the upstream package reference. When that is done, the package
variant controller updates any existing draft package under its ownership by performing the
equivalent of a `kpt pkg update`. This updates the downstream so that it is based on the new
upstream revision. If a draft does not exist, then the package variant controller creates a new
draft based on the current published downstream, and applies the `kpt pkg update`. This updated
draft must then be proposed and approved, as with other package changes.

If a floating tag is used, then explicit modification of the *PackageVariant* is not required.
Rather, when the floating tag is moved to a new tagged revision of the upstream package, the package
revision controller will notice and automatically propose an update to that revision. For example,
the upstream package author may designate three floating tags: stable, beta, and alpha. The upstream
package author can move these tags to specific revisions, and any *PackageVariant* resource tracking
them will propose updates to their downstream packages.

### Adoption and deletion policies

When a *PackageVariant* resource is created, it has a particular repository and package name as the
downstream. The adoption policy determines whether or not the package variant controller takes over
an existing package with that name, in that repository.

Analogously, when a *PackageVariant* resource is deleted, a decision must be made about whether or
not to delete the downstream package. This is controlled by the deletion policy.

## Fanning out of variant generation[^pvsimpl]

When used with a single package, the package variant controller mostly helps to handle the time
dimension: that is, producing new versions of a package as the upstream changes, or as injected
resources are updated. It can also be useful for automating common, systematic changes that are
made when bringing an external package into an organization, or an organizational package into a
team repository.

This is useful, but not particularly compelling by itself. More interesting is when we use the
*PackageVariant* as a primitive for automations that act on other dimensions of scale. This means
writing controllers that emit *PackageVariant* resources. For example, we can create a controller
that instantiates a *PackageVariant* for each developer in our organization, or we can create a
controller to manage the *PackageVariant*s across environments. The ability not only to clone a
package, but also to make systematic changes to that package, enables flexible automation.

The workload controllers in Kubernetes are a useful analogy. In Kubernetes, there are different
workload controllers, such as Deployment, StatefulSet, and DaemonSet. These all ultimately result
in pods. However, the decisions as to what kind of pods to create, how to schedule them across the
nodes, how to configure the pods, and how to manage them as changes take place, differ with each
workload controller. Similarly, we can build different controllers to handle the different ways in
which we want to generate the *PackageRevisions*. The *PackageVariant* resource provides a
convenient primitive for all of these controllers, allowing them to leverage a range of well-defined
operations to mutate the packages as needed.

A common requirement is the ability to generate multiple variants of a package based on a simple
list of an entity. Examples include the following:

- Generating package variants to spin up development environments for each developer in an
  organization.
- Instantiating the same package, with minor configuration changes, across a fleet of clusters.
- Instantiating the packages for each customer.

The package variant set controller is designed to meet this common need. The controller consumes
and outputs the *PackageVariant* resources. The *PackageVariantSet* defines the following:

- the upstream package
- the targeting criteria
- a template for generating one *PackageVariant* per target

Three types of targeting are supported:

- an explicit list of repositories and package names
- a label selector for the repository objects
- an arbitrary object selector

The rules for generating a *PackageVariant* are associated with a list of targets using a template.
This template can have explicit values for various *PackageVariant* fields, or it can use
[Common Expression Language (CEL)](https://github.com/google/cel-go) expressions to specify the
field values.

*Figure 5* shows an example of the creation of *PackageVariant* resources based on the explicit
list of repositories. In this example, for the *cluster-01* and *cluster-02* repositories, no
template is defined for the resulting *PackageVariant*s. It simply takes the defaults. However, for
*cluster-03*, a template is defined to change the downstream package name to *bar*.

| ![Figure 5: PackageVariantSet with the repository list](/static/images/porch/packagevariantset-target-list.png) |
| :---: |
| *Figure 5: PackageVariantSet with the repository list* |

It is also possible to target the same package to a repository more than once, using different
names. This is useful if, for example, the package is used for provisioning namespaces and you
would like to provision multiple namespaces in the same cluster. It is also useful if a repository
is shared across multiple clusters. In *Figure 6*, two *PackageVariant* resources for creating the
*foo* package in the *cluster-01* repository are generated, one for each listed package name. Since
no *packageNames* field is listed for *cluster-02*, only one instance is created for that
repository.

| ![Figure 6: PackageVariantSet with the package list](/static/images/porch/packagevariantset-target-list-with-packages.png) |
| :---: |
| *Figure 6: PackageVariantSet with the package list* |

*Figure 7* shows an example that combines a repository label selector with configuration injectors
that differ according to the target. The template for the *PackageVariant* includes a CEL expression
for one of the injectors, so that the injection varies systematically according to the attributes of
the target.

| ![Figure 7: PackageVariantSet with the repository selector](/static/images/porch/packagevariantset-target-repo-selector.png) |
| :---: |
| *Figure 7: PackageVariantSet with the repository selector* |

## Detailed design

### PackageVariant API

The Go types below define the *PackageVariantSpec*.

```go
type PackageVariantSpec struct {
        Upstream   *Upstream   `json:"upstream,omitempty"`
        Downstream *Downstream `json:"downstream,omitempty"`

        AdoptionPolicy AdoptionPolicy `json:"adoptionPolicy,omitempty"`
        DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`

        Labels      map[string]string `json:"labels,omitempty"`
        Annotations map[string]string `json:"annotations,omitempty"`

        PackageContext *PackageContext     `json:"packageContext,omitempty"`
        Pipeline       *kptfilev1.Pipeline `json:"pipeline,omitempty"`
        Injectors      []InjectionSelector `json:"injectors,omitempty"`
}

type Upstream struct {
        Repo     string `json:"repo,omitempty"`
        Package  string `json:"package,omitempty"`
        Revision string `json:"revision,omitempty"`
}

type Downstream struct {
        Repo    string `json:"repo,omitempty"`
        Package string `json:"package,omitempty"`
}

type PackageContext struct {
        Data       map[string]string `json:"data,omitempty"`
        RemoveKeys []string          `json:"removeKeys,omitempty"`
}

type InjectionSelector struct {
        Group   *string `json:"group,omitempty"`
        Version *string `json:"version,omitempty"`
        Kind    *string `json:"kind,omitempty"`
        Name    string  `json:"name"`
}

```

#### Basic specification fields

The Upstream and Downstream fields specify the source package, and the destination repository and
package name. The Repo fields refer to the names of the Porch Repository resources in the same
namespace as the *PackageVariant* resource. The Downstream field does not contain a revision,
because the package variant controller only creates the draft packages. The revision of the eventual *PackageRevision* resource is determined by Porch at the time of approval.

The Labels and Annotations fields list the metadata to include in the created *PackageRevision*.
These values are set only at the time a draft package is created. They are ignored for subsequent
operations, even if the *PackageVariant* itself has been modified. This means users are free to
change these values on the *PackageRevision*. The package variant controller will not touch them
again.

The AdoptionPolicy controls how the package variant controller behaves if it finds an existing
*PackageRevision* draft matching the Downstream field. If the status of the AdoptionPolicy is
*adoptExisting*, then the package variant controller takes ownership of the draft, associating it
with this *PackageVariant*. This means that it will begin to reconcile the draft, as if it had
created it in the first place. If the status of the AdoptionPolicy is *adoptNone* (this is the
default setting), then the package variant controller simply ignores any matching drafts that were
not created by the controller.

The DeletionPolicy controls how the package variant controller behaves with respect to the
*PackageRevisions* that package variant controller created when the *PackageVariant* resource itself
was deleted. The *delete* value (the default value) deletes the *PackageRevision*, potentially
removing it from a running cluster, if the downstream package has been deployed. The *orphan* value
removes the owner references and leaves the *PackageRevisions* in place.

#### Package context injection

*PackageVariant* resource authors may specify key-value pairs in the spec.packageContext.data field
of the resource. These key-value pairs are automatically added to the data of the *kptfile.kpt.dev*
ConfigMap, if it exists.

Specifying the key name is invalid and must fail the validation of the *PackageVariant*. This key
is reserved for *kpt* or Porch to set to the package name. Similarly, the package-path is reserved
and will result in an error.

The spec.packageContext.removeKeys field can also be used to specify a list of keys that the package
variant controller should remove from the data field of the *kptfile.kpt.dev* ConfigMap.

When creating or updating a package, the package variant controller ensures the following:

- The *kptfile.kpt.dev* ConfigMap exists. If it does not exist, then the package variant controller
  will fail the ConfigMap.
- All of the key-value pairs in the spec.packageContext.data exist in the data field of the
  ConfigMap.
- None of the keys listed in spec.packageContext.removeKeys exists in the ConfigMap.

{{% alert title="Note" color="primary" %}}

If a user adds a key via the *PackageVariant*, then changes the *PackageVariant* to not add that key
anymore, then it will not be removed automatically, unless the user also lists the key in the
removeKeys list. This avoids the need to track which keys were added by the *PackageVariant*.

Similarly, if a user manually adds a key in the downstream that is also listed in the removeKeys
field, then the package variant controller will remove that key the next time it needs to update
the downstream package. There will be no attempt to coordinate “ownership” of these keys.

{{% /alert %}}

If, for some reason, the controller cannot modify the ConfigMap, then this is considered to be an
error and will prevent the generation of the draft. This will result in the Ready condition being
set to *False*.

#### Editing the Kptfile function pipeline

The *PackageVariant* resource creators may specify a list of KRM functions to add to the beginning
of the *Kptfile's* pipeline. These functions are listed in the spec.pipeline field, which is a
[Pipeline](https://github.com/GoogleContainerTools/kpt/blob/cf1f326486214f6b4469d8432287a2fa705b48f5/pkg/api/kptfile/v1/types.go#L236), just as in the *Kptfile*. The user can therefore prepend both validators
and mutators.

Functions added in this way are always added to the *beginning* of the *Kptfile* pipeline. To enable
the management of the list on subsequent reconciliations, functions added by the package variant
controller use the Name field of the
[Function](https://github.com/GoogleContainerTools/kpt/blob/cf1f326486214f6b4469d8432287a2fa705b48f5/pkg/api/kptfile/v1/types.go#L283). In the *Kptfile*, each function is named as the dot-delimited
concatenation of the *PackageVariant*, the name of the *PackageVariant* resource, the function name
as specified in the pipeline of the *PackageVariant* resource (if present), and the positional
location of the function in the array.

For example, if the *PackageVariant* resource contains the following:

```yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: PackageVariant
metadata:
  namespace: default
  name: my-pv
spec:
  ...
  pipeline:
    mutators:
    - image: gcr.io/kpt-fn/set-namespace:v0.1
      configMap:
        namespace: my-ns
      name: my-func
    - image: gcr.io/kpt-fn/set-labels:v0.1
      configMap:
        app: foo
```

then the resulting *Kptfile* will have the following two entries prepended to its mutators list:

```yaml
  pipeline:
    mutators:
    - image: gcr.io/kpt-fn/set-namespace:v0.1
      configMap:
        namespace: my-ns
      name: PackageVariant.my-pv.my-func.0
    - image: gcr.io/kpt-fn/set-labels:v0.1
      configMap:
        app: foo
      name: PackageVariant.my-pv..1
```

This allows the controller, during subsequent reconciliations, to identify the functions within its
control, remove them all, and add them again, based on its updated content. Including the
*PackageVariant* name enables chains of *PackageVariants* to add functions, as long as the user is
careful about their choice of resource names and avoids conflicts.

If, for some reason, the controller cannot modify the pipeline, then this is considered to be an
error and should prevent the generation of the draft. This will result in the Ready condition being
set to *False*.

#### Configuration injection details

As described [above](#configuration-injection), configuration injection is a process whereby
in-package resources are matched to in-cluster resources, and the specifications of the in-cluster
resources are copied to the in-package resource.

Configuration injection is controlled by a combination of in-package resources with annotations, and
injectors (also known as *injection selectors*) defined on the *PackageVariant* resource. Package
authors control the injection points they allow in their packages, by flagging specific resources as
*injection points* with an annotation. Creators of the *PackageVariant* resource specify how to map
in-cluster resources to those injection points using the injection selectors. Injection selectors
are defined in the spec.injectors field of the *PackageVariant*. This field is an ordered array of
structs containing a group, version, kind (GVK) tuple as separate fields, and a name. Only the name
is required. To identify a match, all fields present must match the in-cluster object, and all *GVK*
fields present must match the in-package resource. In general, the name will not match the
in-package resource. This is discussed in more detail below.

The annotations, along with the GVK of the annotated resource, allow a package to “advertise” the
injections it can accept and understand. These injection points effectively form a configuration API
for the package. The injection selectors provide a way for the *PackageVariant* author to specify
the inputs for those APIs from the possible values in the management cluster. If we define the APIs
carefully, they can be used across many packages. Since they are KRM resources, we can apply
versioning and schema validation to them as well. This creates a more maintainable, automatable set
of APIs for package customization than simple key/value pairs.

As an example, we can define a GVK that contains service endpoints that many applications use. In
each application package, we then include an instance of the resource. We can call this resource,
for example, *service-endpoints*. We then configure a function to propagate the values from this
resource to other resources within our package. As those endpoints may vary by region, we can create
in our Porch cluster an instance of this GVK for each region: *useast1-service-endpoints*, *useast2-service-endpoints*, *uswest1-service-endpoints*, and so on. When we instantiate the
*PackageVariant* for a cluster, we want to inject the resource corresponding to the region in which
the cluster exists. Therefore, for each cluster we will create a *PackageVariant* resource pointing
to the upstream package, but with injection selector name values that are specific to the region for
that cluster.

It is important to understand that the name of the in-package resource and that of the in-cluster
resource need not match. In fact, it would be an unusual coincidence if they did match. The names in
the package are the same across the *PackageVariants* using that upstream, but we want to inject
different resources for each *PackageVariant*. In addition, we do not want to change the name in the
package, because it likely has meaning within the package and will be used by the functions in the
package. Also, different owners control the names of the in-package and in-cluster resources. The
names in the package are in the control of the package author. The names in the cluster are in the
control of whomever populates the cluster (for example, an infrastructure team). The selector is the
glue between them, and is in control of the *PackageVariant* resource creator.

The GVK, however, has to be the same for the in-package resource and the in-cluster resource. This
is because the GVK tells us the API schema for the resource. Also, the namespace of the in-cluster
object needs to be the same as that of the *PackageVariant* resource. Otherwise, we could leak
resources from those namespaces to which our *PackageVariant* user does not have access.

With this in mind, the injection process works as follows:

1. The controller examines all the in-package resources, looking for those that have an annotation
   named *kpt.dev/config-injection*, with either of the following values:
   - *required*
   - *optional*
   These are called injection points. It is the responsibility of the package author to define these
   injection points, and to specify which are required and which are optional. Optional injection
   points are a way of specifying default values.   
2. For each injection point, a condition is created *in the downstream PackageRevision*, with the
   ConditionType set to the dot-delimited concatenation of the config.injection, with the in-package
   resource kind and name, and the value set to *False*.

   {{% alert title="Note" color="primary" %}}
      
   Since the package author controls the name of the resource, the kind and the name are sufficient
   to identify the injection point. This ConditionType is called the "injection point
   ConditionType".

   {{% /alert %}}

3. For each required injection point, the injection point ConditionType is added to the
   *PackageRevision*  readinessGates by the package variant controller. The ConditionTypes of the
   optional injection points must not be added to the readinessGates by the package variant
   controller. However, other actors may do so at a later date, and the package variant controller
   should not remove them on subsequent reconciliations. Also, this relies on the readinessGates
   gating publishing the package to a *deployment* repository, but not gating publishing to a
   blueprint repository.
4. The injection processing proceeds as follows. For each injection point, the following is the
   case:

   - The controller identifies all in-cluster objects in the same namespace as the *PackageVariant*
     resource, with the GVK matching the injection point (the in-package resource). If the
     controller is unable to load these objects (for example, there are none and the CRD is not
     installed), then the injection point ConditionType will be set to *False*, with a message
     indicating the error. Processing then proceeds to the next injection point.

     {{% alert title="Note" color="primary" %}}

     For optional injection, this may be an acceptable outcome. Therefore, it does not interfere
     with the overall generation of the draft.

     {{% /alert %}}

   - The controller looks through the list of injection selectors in order and checks if any of the
     in-cluster objects match the selector. If there is an in-cluster object that matches, then that
     in-cluster object is selected and processing of the list of injection selectors ceases.

     {{% alert title="Note" color="primary" %}}

     The namespace is set according to the *PackageVariant* resource. The GVK is set according to
     the in-package resource. Each selector requires a name. Therefore, one match at most is
     possible for any given selector.

     Additionally, *all fields present in the selector* must match the in-cluster resource. Only
     the *GVK fields present in the selector* must match the in-package resource.

     {{% /alert %}}

   - If no in-cluster object is selected, then the injection point ConditionType is set to *False*,
     with a message that no matching in-cluster resource was found. Processing proceeds to the next
     injection point.

   - If a matching in-cluster object is selected, then it is injected as follows:

     - For the ConfigMap resources, the data field from the in-cluster resource is copied to the
       data field of the in-package resource (the injection point), overwriting it.
     - For the other resource types, the specification field from the in-cluster resource is copied
       to the specification field of the in-package resource (the injection point), overwriting it.
     - An annotation with the name *kpt.dev/injected-resource-name* and the value set to the name
       of the in-cluster resource is added (or overwritten) in the in-package resource.

If, for some reason, the overall injection cannot be completed, or if either of the problems set
out below exists in the upstream package, then it is considered to be an error and should prevent
the generation of the draft. The two possible problems are the following:

   - There is a resource annotated as an injection point which, however, has an invalid annotation
     value (that is, a value other than *required* or *optional*).
   - There are ambiguous condition types, due to conflicting GVK and name values. If this is the
     case, then these must be disambiguated in the upstream package.

This results in the Ready condition being set to *False*.

{{% alert title="Note" color="primary" %}}

Whether or not all the required injection points are fulfilled does not affect the *PackageVariant*
conditions. It only affects the *PackageRevision* conditions.

{{% /alert %}}

**A Further note on selectors**

By allowing the use, and not just name, of the GVK in the selector, more precision in the selection
is enabled. This is a way to constrain the injections that are performed. That is, if the package
has 10 different objects with a config-injection annotation, then the *PackageVariant* could say it
only wants to replace certain GVKs, thereby allowing better control.

Consider, for example, if the cluster contains the following resources:

- GVK1 foo
- GVK1 bar
- GVK2 foo
- GVK2 bar

If we could define injection selectors based only on their names, it would be impossible to ever
inject one GVK with *foo* and another with *bar*. Instead, by using the GVK, we can accomplish this
with a list of selectors, such as the following:

 - GVK1 foo
 - GVK2 bar

That said, often a name is sufficiently unique when combined with the in-package resource GVK.
Therefore, making the selector GVK optional is more convenient. This allows a single injector to
apply to multiple injection points with different GVKs.

#### Order of mutations

During creation, the first step the controller takes is to clone the upstream package to create the
downstream package.

For the update, first note that changes to the downstream *PackageRevision* can be triggered for the
following reasons:

1. The *PackageVariant* resource is updated. This could change any of the options for introducing
   variance, or could also change the upstream package revision referenced.
2. A new revision of the upstream package has been selected, due to a floating tag change, or due
   to a force retagging of the upstream.
3. An injected in-cluster object has been updated.

The downstream *PackageRevision* may have been updated by humans or other automation actors since
creation. Therefore, we cannot simply recreate the downstream *PackageRevision* from scratch when a
change occurs. Instead, the controller must maintain the later edits by performing the equivalent
of a `kpt pkg update`, in the case of changes to the upstream, for any reason. Any other changes
require a reapplication of the *PackageVariant* functionality. With this in mind, we can see that
the controller performs mutations on the downstream package in the following order, for both
creation and update:

1. Create (via clone) or update (via `kpt pkg update` equivalent):

   - This is carried out by the Porch server, not directly by the package variant controller.
   - This means that Porch runs the *Kptfile* pipeline after clone or update.

2. The package variant controller applies configured mutations:

   - Package context injections
   - *Kptfile* KRM function pipeline additions/changes
   - Config injection

3. The package variant controller saves the *PackageRevision* and the *PackageRevisionResources*:

   - The Porch server executes the *Kptfile* pipeline.

The package variant controller mutations edit the resources (including the *Kptfile*) according to
the contents of the *PackageVariant* and the injected in-cluster resources. However, they cannot
affect one another. The results of these mutations throughout the rest of the package are manifested
by the execution of the *Kptfile* pipeline during the *save* operation.

#### PackageVariant status

The PackageVariant sets the following status conditions:

 - **Stalled**
   The PackageVariant sets this condition to *True* if there has been a failure that likely requires
   intervention by the user.
 - **Ready**
   The PackageVariant sets this condition to *True* if the last reconciliation has successfully
   produced an up-to-date draft.

The *PackageVariant* resource also contains a DownstreamTargets field. This field contains a list of
downstream *Draft* and *Proposed* *PackageRevisions* owned by this *PackageVariant* resource, or the
latest published *PackageRevision*, if there are none in the *Draft* or *Proposed* state. Typically,
there is only a single draft, but the use of the *adopt* value for the AdoptionPolicy could result
in multiple drafts being owned by the same *PackageVariant*.

### PackageVariantSet API[^pvsimpl]

The Go types below define the `PackageVariantSetSpec`.

```go
// PackageVariantSetSpec defines the desired state of PackageVariantSet
type PackageVariantSetSpec struct {
        Upstream *pkgvarapi.Upstream `json:"upstream,omitempty"`
        Targets  []Target            `json:"targets,omitempty"`
}

type Target struct {
        // Exactly one of Repositories, RepositorySeletor, and ObjectSelector must be
        // populated
        // option 1: an explicit repositories and package names
        Repositories []RepositoryTarget `json:"repositories,omitempty"`

        // option 2: a label selector against a set of repositories
        RepositorySelector *metav1.LabelSelector `json:"repositorySelector,omitempty"`

        // option 3: a selector against a set of arbitrary objects
        ObjectSelector *ObjectSelector `json:"objectSelector,omitempty"`

        // Template specifies how to generate a PackageVariant from a target
        Template *PackageVariantTemplate `json:"template,omitempty"`
}
```

At the highest level, a *PackageVariantSet* is just an upstream and a list of targets. For each
target, there is a set of criteria for generating a list, and a set of rules (a template) for
creating a *PackageVariant* from each list entry.

Since the template is optional, let us start with describing the different types of targets, and how
the criteria in each target is used to generate a list that seeds the *PackageVariant* resources.

The target structure must include one of three different ways of generating the list. The first is
a simple list of repositories and package names for each of these repositories[^repo-pkg-expr]. The
package name list is required for uses cases in which you want to repeatedly instantiate the same
package in a single repository. For example, if a repository represents the contents of a cluster,
you may want to instantiate a namespace package once for each namespace, with a name matching the
namespace.

The following example shows how to use the repositories field:

```yaml
apiVersion: config.porch.kpt.dev/v1alpha2
kind: PackageVariantSet
metadata:
  namespace: default
  name: example
spec:
  upstream:
    repo: example-repo
    package: foo
    revision: v1
  targets:
  - repositories:
    - name: cluster-01
    - name: cluster-02
    - name: cluster-03
      packageNames:
      - foo-a
      - foo-b
      - foo-c
    - name: cluster-04
      packageNames:
      - foo-a
      - foo-b
```

In the following case, the *PackageVariant* resources are created for each of the pairs of
downstream repositories and package names:

| Repository | Package name |
| ---------- | ------------ |
| cluster-01 | foo          |
| cluster-02 | foo          |
| cluster-03 | foo-a        |
| cluster-03 | foo-b        |
| cluster-03 | foo-c        |
| cluster-04 | foo-a        |
| cluster-04 | foo-b        |

All of the *PackageVariants* in the above list have the same upstream.

The second criteria targeting is via a label selector against the Porch repository objects, along
with a list of package names. These packages are instantiated in each matching repository. As in the
first example, not listing a package name defaults to one package, with the same name as the
upstream package. Suppose, for example, we have the following four repositories defined in our Porch
cluster:

| Repository | Labels                                |
| ---------- | ------------------------------------- |
| cluster-01 | region=useast1, env=prod, org=hr      |
| cluster-02 | region=uswest1, env=prod, org=finance |
| cluster-03 | region=useast2, env=prod, org=hr      |
| cluster-04 | region=uswest1, env=prod, org=hr      |

If we create a *PackageVariantSet* with the following specificattion:

```yaml
spec:
  upstream:
    repo: example-repo
    package: foo
    revision: v1
  targets:
  - repositorySelector:
      matchLabels:
        env: prod
        org: hr
  - repositorySelector:
      matchLabels:
        region: uswest1
      packageNames:
      - foo-a
      - foo-b
      - foo-c
```

then the *PackageVariant* resources will be created with the following repository and package names:

| Repository | Package name |
| ---------- | ------------ |
| cluster-01 | foo          |
| cluster-03 | foo          |
| cluster-04 | foo          |
| cluster-02 | foo-a        |
| cluster-02 | foo-b        |
| cluster-02 | foo-c        |
| cluster-04 | foo-a        |
| cluster-04 | foo-b        |
| cluster-04 | foo-c        |

The third possibility allows the use of *arbitrary* resources in the Porch cluster as targeting
criteria. The objectSelector looks like this:

```yaml
spec:
  upstream:
    repo: example-repo
    package: foo
    revision: v1
  targets:
  - objectSelector:
      apiVersion: krm-platform.bigco.com/v1
      kind: Team
      matchLabels:
        org: hr
        role: dev
```

The object selector works in the same way as the repository selector - in fact, the repository
selector is equivalent to the object selector, with the apiVersion and kind values set to point to
the Porch repository resources. That is, the repository name comes from the object name, and the
package names come from the listed package names. In the description of the template, we will see
how to derive different repository names from the objects.

#### PackageVariant template

As discussed earlier, the list entries generated by the target criteria result in *PackageVariant*
entries. If no template is specified, then the *PackageVariant* default is used, along with the
downstream repository name and the package name, as described in the previous section. The template
allows the user to have control over all the values in the resulting *PackageVariant*. The template
API is shown below.

```go
type PackageVariantTemplate struct {
  // Downstream allows overriding the default downstream package and repository name
  // +optional
  Downstream *DownstreamTemplate `json:"downstream,omitempty"`

  // AdoptionPolicy allows overriding the PackageVariant adoption policy
  // +optional
  AdoptionPolicy *pkgvarapi.AdoptionPolicy `json:"adoptionPolicy,omitempty"`

  // DeletionPolicy allows overriding the PackageVariant deletion policy
  // +optional
  DeletionPolicy *pkgvarapi.DeletionPolicy `json:"deletionPolicy,omitempty"`

  // Labels allows specifying the spec.Labels field of the generated PackageVariant
  // +optional
  Labels map[string]string `json:"labels,omitempty"`

  // LabelsExprs allows specifying the spec.Labels field of the generated PackageVariant
  // using CEL to dynamically create the keys and values. Entries in this field take precedent over
  // those with the same keys that are present in Labels.
  // +optional
  LabelExprs []MapExpr `json:"labelExprs,omitempty"`

  // Annotations allows specifying the spec.Annotations field of the generated PackageVariant
  // +optional
  Annotations map[string]string `json:"annotations,omitempty"`

  // AnnotationsExprs allows specifying the spec.Annotations field of the generated PackageVariant
  // using CEL to dynamically create the keys and values. Entries in this field take precedent over
  // those with the same keys that are present in Annotations.
  // +optional
  AnnotationExprs []MapExpr `json:"annotationExprs,omitempty"`

  // PackageContext allows specifying the spec.PackageContext field of the generated PackageVariant
  // +optional
  PackageContext *PackageContextTemplate `json:"packageContext,omitempty"`

  // Pipeline allows specifying the spec.Pipeline field of the generated PackageVariant
  // +optional
  Pipeline *PipelineTemplate `json:"pipeline,omitempty"`

  // Injectors allows specifying the spec.Injectors field of the generated PackageVariant
  // +optional
  Injectors []InjectionSelectorTemplate `json:"injectors,omitempty"`
}

// DownstreamTemplate is used to calculate the downstream field of the resulting
// package variants. Only one of Repo and RepoExpr may be specified;
// similarly only one of Package and PackageExpr may be specified.
type DownstreamTemplate struct {
  Repo        *string `json:"repo,omitempty"`
  Package     *string `json:"package,omitempty"`
  RepoExpr    *string `json:"repoExpr,omitempty"`
  PackageExpr *string `json:"packageExpr,omitempty"`
}

// PackageContextTemplate is used to calculate the packageContext field of the
// resulting package variants. The plain fields and Exprs fields will be
// merged, with the Exprs fields taking precedence.
type PackageContextTemplate struct {
  Data           map[string]string `json:"data,omitempty"`
  RemoveKeys     []string          `json:"removeKeys,omitempty"`
  DataExprs      []MapExpr         `json:"dataExprs,omitempty"`
  RemoveKeyExprs []string          `json:"removeKeyExprs,omitempty"`
}

// InjectionSelectorTemplate is used to calculate the injectors field of the
// resulting package variants. Exactly one of the Name and NameExpr fields must
// be specified. The other fields are optional.
type InjectionSelectorTemplate struct {
  Group   *string `json:"group,omitempty"`
  Version *string `json:"version,omitempty"`
  Kind    *string `json:"kind,omitempty"`
  Name    *string `json:"name,omitempty"`

  NameExpr *string `json:"nameExpr,omitempty"`
}

// MapExpr is used for various fields to calculate map entries. Only one of
// Key and KeyExpr may be specified; similarly only on of Value and ValueExpr
// may be specified.
type MapExpr struct {
  Key       *string `json:"key,omitempty"`
  Value     *string `json:"value,omitempty"`
  KeyExpr   *string `json:"keyExpr,omitempty"`
  ValueExpr *string `json:"valueExpr,omitempty"`
}

// PipelineTemplate is used to calculate the pipeline field of the resulting
// package variants.
type PipelineTemplate struct {
  // Validators is used to caculate the pipeline.validators field of the
  // resulting package variants.
  // +optional
  Validators []FunctionTemplate `json:"validators,omitempty"`

  // Mutators is used to caculate the pipeline.mutators field of the
  // resulting package variants.
  // +optional
  Mutators []FunctionTemplate `json:"mutators,omitempty"`
}

// FunctionTemplate is used in generating KRM function pipeline entries; that
// is, it is used to generate Kptfile Function objects.
type FunctionTemplate struct {
  kptfilev1.Function `json:",inline"`

  // ConfigMapExprs allows use of CEL to dynamically create the keys and values in the
  // function config ConfigMap. Entries in this field take precedent over those with
  // the same keys that are present in ConfigMap.
  // +optional
  ConfigMapExprs []MapExpr `json:"configMapExprs,omitempty"`
}
```

To make this complex structure more comprehensible, the first thing to notice is that many fields
have a plain version and an Expr version. The plain version is used when the value is static across
all the *PackageVariants*. The Expr version is used when the value needs to vary across the
*PackageVariants*.

Let us consider a simple example. Suppose we have a package for provisioning namespaces that is
called *base-ns*. We would like to instantiate this several times in the *cluster-01* repository.
We could do this with the following *PackageVariantSet*:

```yaml
apiVersion: config.porch.kpt.dev/v1alpha2
kind: PackageVariantSet
metadata:
  namespace: default
  name: example
spec:
  upstream:
    repo: platform-catalog
    package: base-ns
    revision: v1
  targets:
  - repositories:
    - name: cluster-01
      packageNames:
      - ns-1
      - ns-2
      - ns-3
```

This will produce three *PackageVariant* resources with the same upstream, all with the same
downstream repository, and each with a different downstream package name. If we also want to set
some labels identically across the packages, we can do this with the template.labels field:

```yaml
apiVersion: config.porch.kpt.dev/v1alpha2
kind: PackageVariantSet
metadata:
  namespace: default
  name: example
spec:
  upstream:
    repo: platform-catalog
    package: base-ns
    revision: v1
  targets:
  - repositories:
    - name: cluster-01
      packageNames:
      - ns-1
      - ns-2
      - ns-3
    template:
      labels:
        package-type: namespace
        org: hr
```

The resulting *PackageVariant* resources include labels in their specification, and are identical,
apart from their names and the downstream.package:

```yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: PackageVariant
metadata:
  namespace: default
  name: example-aaaa
spec:
  upstream:
    repo: platform-catalog
    package: base-ns
    revision: v1
  downstream:
    repo: cluster-01
    package: ns-1
  labels:
    package-type: namespace
    org: hr
---
apiVersion: config.porch.kpt.dev/v1alpha1
kind: PackageVariant
metadata:
  namespace: default
  name: example-aaab
spec:
  upstream:
    repo: platform-catalog
    package: base-ns
    revision: v1
  downstream:
    repo: cluster-01
    package: ns-2
  labels:
    package-type: namespace
    org: hr
---

apiVersion: config.porch.kpt.dev/v1alpha1
kind: PackageVariant
metadata:
  namespace: default
  name: example-aaac
spec:
  upstream:
    repo: platform-catalog
    package: base-ns
    revision: v1
  downstream:
    repo: cluster-01
    package: ns-3
  labels:
    package-type: namespace
    org: hr
```

When using other targeting means, the use of the Expr fields becomes more probable, since we have
more possible sources for the different field values. The Expr values are all
[Common Expression Language (CEL)](https://github.com/google/cel-go) expressions, rather than static
values. This allows the user to construct values based on the various fields of the targets.
Consider again the RepositorySelector example, where we have these repositories in the cluster.

| Repository | Labels                                |
| ---------- | ------------------------------------- |
| cluster-01 | region=useast1, env=prod, org=hr      |
| cluster-02 | region=uswest1, env=prod, org=finance |
| cluster-03 | region=useast2, env=prod, org=hr      |
| cluster-04 | region=uswest1, env=prod, org=hr      |

If we create a *PackageVariantSet* with the following specification, then we can use the Expr fields
to add labels to the *PackageVariantSpecs* (and therefore to the resulting *PackageRevisions* later)
that vary according to the cluster. We can also use this to diversify the injectors defined for each
*PackageVariant*, resulting in each *PackageRevision* having different resources injected. The
following specification results in three *PackageVariant* resources, one for each repository, with
the *env=prod* and *org=hr* labels.

```yaml
spec:
  upstream:
    repo: example-repo
    package: foo
    revision: v1
  targets:
  - repositorySelector:
      matchLabels:
        env: prod
        org: hr
    template:
      labelExprs:
        key: org
        valueExpr: "repository.labels['org']"
      injectorExprs:
        - nameExpr: "repository.labels['region'] + '-endpoints'"
```

The labels and injectors fields of the *PackageVariantSpec* are different for each of the
*PackageVariants*, as determined by the use of the Expr fields in the template, as shown here:

```yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: PackageVariant
metadata:
  namespace: default
  name: example-aaaa
spec:
  upstream:
    repo: example-repo
    package: foo
    revision: v1
  downstream:
    repo: cluster-01
    package: foo
  labels:
    org: hr
  injectors:
    name: useast1-endpoints
---
apiVersion: config.porch.kpt.dev/v1alpha1
kind: PackageVariant
metadata:
  namespace: default
  name: example-aaab
spec:
  upstream:
    repo: example-repo
    package: foo
    revision: v1
  downstream:
    repo: cluster-03
    package: foo
  labels:
    org: hr
  injectors:
    name: useast2-endpoints
---
apiVersion: config.porch.kpt.dev/v1alpha1
kind: PackageVariant
metadata:
  namespace: default
  name: example-aaac
spec:
  upstream:
    repo: example-repo
    package: foo
    revision: v1
  downstream:
    repo: cluster-04
    package: foo
  labels:
    org: hr
  injectors:
    name: uswest1-endpoints
```

Since the injectors are different for each *PackageVariant*, each of the resulting
*PackageRevisions* has different resources injected.

When CEL expressions are evaluated, they have an environment associated with them. That is, there
are certain objects that are accessible within the CEL expression. For CEL expressions used in the *PackageVariantSet* template field, the following variables are available:

| CEL variable   | Variable contents                                            |
| -------------- | ------------------------------------------------------------ |
| repoDefault    | The default repository name based on the targeting criteria. |
| packageDefault | The default package name based on the targeting criteria.    |
| upstream       | The upstream *PackageRevision*.                              |
| repository     | The downstream repository.                                   |
| target         | The target object (details vary. See below).                 |

There is one expression that is an exception to the above table. Since the repository value
corresponds to the downstream repository, we must first evaluate the downstream.repoExpr expression
to find that repository. Therefore, for this expression only, *repository* is not a valid variable.

There is one other variable that is available across all the CEL expressions: the target variable.
This variable has a meaning that varies depending on the type of target, as follows:

| Target type         | Target variable contents contents                       |
| ----------------------------------------------------------------------------- |
| Repo/package list   | A struct that has two fields: repo and package, as with |
|                     | the repoDefault and the packageDefault values.          |
| Repository selector | The repository selected by the selector. Although not   |
|                     | recommended, this can be different from the repository  | 
|                     | value, which can be altered with the downstream.repo or |
|                     | the downstream.repoExpr.                                | 
| Object selector     | The Object selected by the selector.                    |

For the various resource variables - upstream, repository, and target - arbitrary access to all the
fields of the object could lead to security concerns. Therefore, only a subset of the data is
available for use in CEL expressions, specifically, the following fields: name, namespace, labels,
and annotations.

Given the minor quirk with the repoExpr, it may be helpful to state the processing flow for the
template evaluation:

1. The upstream *PackageRevision* is loaded. It must be in the same namespace as the
   *PackageVariantSet*[^multi-ns-reg].
2. The targets are determined.
3. For each target, the following is the case:

   1. The CEL environment is prepared with repoDefault, packageDefault, upstream, and the target
      variables.
   2. The downstream repository is determined and loaded, as follows:

      - If present, the downstream.repoExpr is evaluated using the CEL environment. The result is
        used as the downstream repository name.
      - If the downstream.repo is set, then this is used as the downstream repository name.
      - If neither the downstream.repoExpr nor the downstream.repo is present, then the default
        repository name, based on the target, is used (that is, the same value as the repoDefault
        variable).
      - The resulting downstream repository name is used to load the corresponding repository
        object in the same namespace as the *PackageVariantSet*.

   3. The downstream repository is added to the CEL environment.
   4. All other CEL expressions are evaluated.

4. If any of the resources, such as the upstream *PackageRevision* or the downstream repository,
   are not found or otherwise fail to load, then the processing stops and a failure condition is
   raised. Similarly, if a CEL expression cannot be properly evaluated, due to syntax or other
   issues, then the processing stops and a failure condition is raised.

#### Other considerations

It seems convenient to automatically inject the *PackageVariantSet* targeting resource. However, it
is better to require the package to advertise the ways in which it accepts injections (that is, the
GVKs that it understands), and only inject those. This keeps the separation of concerns cleaner. The
package does not build in an awareness of the context in which it expects to be deployed. For
example, a package should not accept a Porch repository resource just because that happens to be the
targeting mechanism. That would make the package unusable in other contexts.

#### PackageVariantSet status

The *PackageVariantSet* status uses the following conditions:

 - Stalled is set to *True*, if there has been a failure that likely requires user intervention.
 - Ready is set to *True*, if the last reconciliation has successfully reconciled all the targeted
   *PackageVariant* resources.

## Future considerations
- As an alternative to the floating tag proposal, it may instead be desirable to have a separate tag
  tracking controller that can update the PV and PVS resources, to tweak their upstream as the tag
  moves.
- Installing a collection of packages across a set of clusters, or performing the same mutations to
  each package in a collection, is only supported by creating multiple *PackageVariant*/
  *PackageVariantSet* resources. These are options to consider for the following use cases:

  - Upstreams listing multiple packages.
  - Label the selector against *PackageRevisions*. This does not seem particularly useful, as
    *PackageRevisions* are highly reusable and would probably be composed in many different ways.
  - A *PackageRevisionSet* resource that simply contains a list of upstream structures and could be
    used as an upstream. This is functionally equivalent to the upstreams option, except this list
    is reusable across resources.
  - Listing multiple *PackageRevisionSets* in the upstream is also desirable. 
  - Any or all of the above use cases could be implemented in the *PackageVariant* or
    *PackageVariantSet*, or both.

## Footnotes

[^porch17]: Implemented in Porch v0.0.17.
[^porch18]: Available in Porch v0.0.18.
[^notimplemented]: Proposed here, but not yet implemented in Porch v0.0.18.
[^setns]: As of writing, the set-namespace function does not have a *create* option. This should be
  added, in order to avoid the user needing also to use the `upsert-resource` function. Such common
  operations should be simple for users.
[^pvsimpl]: This document describes *PackageVariantSet* v1alpha2, which will be available from
  Porch v0.0.18 onwards. In Porch v0.0.16 and 17, the v1alpha1 implementation is available, but it
  is a somewhat different API, which does not support CEL or any injection. It is focused only on
  fan-out targeting, and uses a [slightly different targeting API](https://github.com/nephio-project/porch/blob/main/controllers/packagevariants/api/v1alpha1/packagevariant_types.go).
[^repo-pkg-expr]: This is not exactly correct. As we will see later in the template discussion, the
  repository and package names listed are just defaults for the template. They can be further
  manipulated in the template to reference different downstream repositories and package names. The
  same is true for the repositories selected via the `repositorySelector` option. However, this can
  be ignored for now.
[^multi-ns-reg]: Note that the same upstream repository can be registered in multiple namespaces
  without any problems. This simplifies access controls, avoiding the need for cross-namespace
  relationships between the repositories and other Porch resources.
