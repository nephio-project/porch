---
title: "Using the Porch CLI tool"
type: docs
weight: 3
description:
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

## Setting up the porchctl CLI

The Porch CLI uses the `porchctl` command.
To use it locally, [download](https://github.com/nephio-project/porch/releases/tag/dev), unpack and add it to your PATH.

{{% alert title="Note" color="primary" %}}

Installation of Porch, including its prerequisites, is covered in a [dedicated document]({{% relref "/docs/6_configuration_and_deployments/relevant_old_docs/install-porch.md" %}}).

{{% /alert %}}

*Optional*: Generate the autocompletion script for the specified shell to add to your sh profile.

```
porchctl completion bash
```

The `porchctl` command is an administration command for acting on Porch *Repository* (repo) and *PackageRevision* (rpkg)
CRs.

The commands for administering repositories are:

| Command               | Description                        |
| --------------------- | -----------------------------------|
| `porchctl repo get`   | List registered repositories.      |
| `porchctl repo reg`   | Register a package repository.     |
| `porchctl repo unreg` | Unregister a repository.           |
| `porchctl repo sync`  | schedules one-time synchronization.|     |

The commands for administering package revisions are:

| Command                        | Description                                                                                      |
| ------------------------------ | ------------------------------------------------------------------------------------------------ |
| `porchctl rpkg approve`        | Approve a proposal to publish a package revision.                                                |
| `porchctl rpkg clone`          | Create a clone of an existing package revision.                                                  |
| `porchctl rpkg copy`           | Create a new package revision from an existing one.                                              |
| `porchctl rpkg del`            | Delete a package revision.                                                                       |
| `porchctl rpkg get`            | List package revisions in registered repositories.                                               |
| `porchctl rpkg init`           | Initializes a new package revision in a repository.                                              |
| `porchctl rpkg propose`        | Propose that a package revision should be published.                                             |
| `porchctl rpkg propose-delete` | Propose deletion of a published package revision.                                                |
| `porchctl rpkg pull`           | Pull the content of the package revision.                                                        |
| `porchctl rpkg push`           | Push resources to a package revision.                                                            |
| `porchctl rpkg reject`         | Reject a proposal to publish or delete a package revision.                                       |
| `porchctl rpkg upgrade`        | Update a downstream package revision to a more recent revision of its upstream package revision. |

## Using the porchctl CLI

### Guide prerequisites
* [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)

Make sure that your `kubectl` context is set up for `kubectl` to interact with the correct Kubernetes instance (see
[installation instructions]({{% relref "/docs/6_configuration_and_deployments/relevant_old_docs/install-porch.md" %}}) guide for details).

To check whether `kubectl` is configured with your Porch cluster (or local instance), run:

```bash
kubectl api-resources | grep porch
```

You should see the following API resources listed:

```bash
repositories                  config.porch.kpt.dev/v1alpha1          true         Repository
packagerevisionresources      porch.kpt.dev/v1alpha1                 true         PackageRevisionResources
packagerevisions              porch.kpt.dev/v1alpha1                 true         PackageRevision
```

## Porch Resources

Porch server manages the following resources:

1. `repositories`: a repository (Git or OCI) can be registered with Porch to support discovery or management of KRM
   configuration packages in those repositories.
2. `packagerevisions`: a specific revision of a KRM configuration package managed by Porch in one of the registered
   repositories. This resource represents a _metadata view_ of the KRM configuration package.
3. `packagerevisionresources`: this resource represents the contents of the configuration package (KRM resources
   contained in the package)

{{% alert title="Note" color="primary" %}}

`packagerevisions` and `packagerevisionresources` represent different _views_ of the same underlying KRM
configuration package. `packagerevisions` represents the package metadata, and `packagerevisionresources` represents the
package content. The matching resources share the same `name` (as well as API group and version:
`porch.kpt.dev/v1alpha1`) and differ in resource kind (`PackageRevision` and `PackageRevisionResources` respectively).

{{% /alert %}}


## Repository Registration

To use Porch with a Git repository, you will need:

* A Git repository for your blueprints. An otherwise empty repository with an
  initial commit works best. The initial commit is required to establish the
  `main` branch.
* If the repository requires authentication you will require either
  - A [Personal Access Token](https://github.com/settings/tokens) (when using GitHub repository) for Porch to authenticate
    with the repository if the repository. Porch requires the 'repo' scope.
  - Basic Auth credentials for Porch to authenticate with the repository.

To use Porch with an OCI repository ([Artifact Registry](https://console.cloud.google.com/artifacts) or
[Google Container Registry](https://cloud.google.com/container-registry)), first make sure to:

* Enable [workload identity](https://cloud.google.com/kubernetes-engine/docs/concepts/workload-identity) for Porch
* Assign appropriate roles to the Porch workload identity service account
  (`iam.gke.io/gcp-service-account=porch-server@$(GCP_PROJECT_ID).iam.gserviceaccount.com`)
  to have appropriate level of access to your OCI repository.

Use the `porchctl repo register` command to register your repository with Porch.

```bash
# Unauthenticated Repositories
porchctl repo register --namespace default https://github.com/platkrm/test-blueprints.git --name=test-blueprints --sync-schedule="*/10 * * * *"
porchctl repo register --namespace default https://github.com/nephio-project/catalog --name=oai --directory=workloads/oai
porchctl repo register --namespace default https://github.com/nephio-project/catalog --name=infra --directory=infra --deployment=true --sync-schedule="*/10 * * * *"
```

# Authenticated Repositories

## Basic Auth

```bash
GITHUB_USERNAME=<your github username>
GITHUB_TOKEN=<GitHub Personal Access Token>

$ porchctl repo register \
  --namespace default \
  --repo-basic-username=${GITHUB_USERNAME} \
  --repo-basic-password=${GITHUB_TOKEN} \
  https://github.com/${GITHUB_USERNAME}/blueprints.git \
  --sync-schedule="*/10 * * * *"
```

## Workload Identity

```bash
$ porchctl repo register \
  --namespace default \
  --repo-workload-identity \
  https://github.com/example/private-blueprints.git \
  --sync-schedule="0 */2 * * *"
```

For more details on configuring authenticated repositories see [Authenticating to Remote Git Repositories]({{% relref "/docs/6_configuration_and_deployments/relevant_old_docs/git-authentication-config.md" %}}).

The command line flags supported by `porchctl repo register` are:

* `--directory` - Directory within the repository where to look for packages.
* `--branch` - Branch in the repository where finalized packages are committed (defaults to `main`).
* `--name` - Name of the package repository Kubernetes resource. If unspecified, will default to the name portion (last
  segment) of the repository URL (`blueprint` in the example above)
* `--description` - Brief description of the package repository.
* `--deployment` - Boolean value; If specified, repository is a deployment repository; published packages in a
  deployment repository are considered deployment-ready.
* `--repo-basic-username` - Username for repository authentication using basic auth.
* `--repo-basic-password` - Password for repository authentication using basic auth.
* `--repo-workload-identity` - Use workload identity for authentication.
* `--sync-schedule` - Cron expression for periodic repository synchronization (e.g., "*/10 * * * *" for every 10 minutes).

Additionally, common `kubectl` command line flags for controlling aspects of
interaction with the Kubernetes apiserver, logging, and more (this is true for
all `porchctl` CLI commands which interact with Porch).

Use the `porchctl repo get` command to query registered repositories:

```bash
$ porchctl repo get -A
NAMESPACE    NAME              TYPE   CONTENT   SYNC SCHEDULE   DEPLOYMENT   READY   ADDRESS
default      oai               git    Package                                True    https://github.com/nephio-project/catalog
default      test-blueprints   git    Package   */10 * * * *                 True    https://github.com/platkrm/test-blueprints.git
default      infra             git    Package   */10 * * * *    true         True    https://github.com/nephio-project/catalog
```

The `porchctl <group> get` commands support common `kubectl`
[flags](https://kubernetes.io/docs/reference/kubectl/cheatsheet/#formatting-output) to format output, for example
`porchctl repo get --output=yaml`.

The command `porchctl repo unregister` can be used to unregister a repository:

```bash
$ porchctl repo unregister test-blueprints --namespace default
```
## Repository Sync Command

The command `porchctl repo sync` can be used to schedule one-time synchronization of repositories:

```bash
# Sync specific repository (schedules 1-minute delayed sync)
$ porchctl repo sync test-blueprints --namespace default

# Sync multiple repositories
$ porchctl repo sync repo1 repo2 repo3 --namespace default

# Sync all repositories in namespace
$ porchctl repo sync --all --namespace default

# Sync all repositories across all namespaces
$ porchctl repo sync --all --all-namespaces

# Schedule sync with custom delay
$ porchctl repo sync my-repo --run-once 5m
$ porchctl repo sync my-repo --run-once 2h30m

# Schedule sync at specific time
$ porchctl repo sync my-repo --run-once "2024-01-15T14:30:00Z"
```

### Sync Command Flags
- `--all`: Sync all repositories in namespace
- `--all-namespaces`: Include all namespaces
- `--run-once`: Schedule one-time sync (duration or RFC3339 timestamp)
- `--namespace`: Target namespace

### Sync Behavior
- Minimum delay: 1 minute from command execution
- Updates `spec.sync.runOnceAt` field in Repository CR
- Independent of existing periodic sync schedule
- Past timestamps automatically adjusted to minimum delay

## Package Discovery And Introspection

The `porchctl rpkg` command group contains commands for interacting with package revisions managed by the Package Orchestration
service. the `r` prefix used in the command group name stands for 'remote'.

The `porchctl rpkg get` command list the package revisions in registered repositories:

```bash
$ porchctl rpkg get -A
NAMESPACE    NAME                                                           PACKAGE                                     WORKSPACENAME   REVISION   LATEST   LIFECYCLE          REPOSITORY
default      infra.infra.baremetal.bmh-template.main                        infra/baremetal/bmh-template                main            -1         false    Published          infra
default      infra.infra.capi.cluster-capi.main                             infra/capi/cluster-capi                     main            -1         false    Published          infra
default      infra.infra.capi.cluster-capi.v2.0.0                           infra/capi/cluster-capi                     v2.0.0          -1         false    Published          infra
default      infra.infra.capi.cluster-capi.v3.0.0                           infra/capi/cluster-capi                     v3.0.0          -1         false    Published          infra
default      infra.infra.capi.vlanindex.main                                infra/capi/vlanindex                        main            -1         false    Published          infra
default      infra.infra.capi.vlanindex.v2.0.0                              infra/capi/vlanindex                        v2.0.0          -1         false    Published          infra
default      infra.infra.capi.vlanindex.v3.0.0                              infra/capi/vlanindex                        v3.0.0          -1         false    Published          infra
default      infra.infra.gcp.nephio-blueprint-repo.main                     infra/gcp/nephio-blueprint-repo             main            -1         false    Published          infra
default      infra.infra.gcp.nephio-blueprint-repo.v1                       infra/gcp/nephio-blueprint-repo             v1              1          true     Published          infra
default      infra.infra.gcp.nephio-blueprint-repo.v2.0.0                   infra/gcp/nephio-blueprint-repo             v2.0.0          -1         false    Published          infra
default      infra.infra.gcp.nephio-blueprint-repo.v3.0.0                   infra/gcp/nephio-blueprint-repo             v3.0.0          -1         false    Published          infra
default      oai.workloads.oai.oai-ran-operator.main                        workloads/oai/oai-ran-operator              main            -1         false    Published          oai
default      oai.workloads.oai.oai-ran-operator.v1                          workloads/oai/oai-ran-operator              v1              1          true     Published          oai
default      oai.workloads.oai.oai-ran-operator.v2.0.0                      workloads/oai/oai-ran-operator              v2.0.0          -1         false    Published          oai
default      oai.workloads.oai.oai-ran-operator.v3.0.0                      workloads/oai/oai-ran-operator              v3.0.0          -1         false    Published          oai
default      oai.workloads.oai.pkg-example-cucp-bp.main                     workloads/oai/pkg-example-cucp-bp           main            -1         false    Published          oai
default      oai.workloads.oai.pkg-example-cucp-bp.v1                       workloads/oai/pkg-example-cucp-bp           v1              1          true     Published          oai
default      oai.workloads.oai.pkg-example-cucp-bp.v2.0.0                   workloads/oai/pkg-example-cucp-bp           v2.0.0          -1         false    Published          oai
default      oai.workloads.oai.pkg-example-cucp-bp.v3.0.0                   workloads/oai/pkg-example-cucp-bp           v3.0.0          -1         false    Published          oai
default      oai.workloads.oai.pkg-example-cuup-bp.main                     workloads/oai/pkg-example-cuup-bp           main            -1         false    Published          oai
default      test-blueprints.basens.main                                    basens                                      main            -1         false    Published          test-blueprints
default      test-blueprints.basens.v1                                      basens                                      v1              1          false    Published          test-blueprints
default      test-blueprints.basens.v2                                      basens                                      v2              2          false    Published          test-blueprints
default      test-blueprints.basens.v3                                      basens                                      v3              3          true     Published          test-blueprints
default      test-blueprints.empty.main                                     empty                                       main            -1         false    Published          test-blueprints
default      test-blueprints.empty.v1                                       empty                                       v1              1          true     Published          test-blueprints
porch-demo   porch-test.basedir.subdir.subsubdir.edge-function.inadir       basedir/subdir/subsubdir/edge-function      inadir          0          false    Draft              porch-test
porch-demo   porch-test.basedir.subdir.subsubdir.network-function.dirdemo   basedir/subdir/subsubdir/network-function   dirdemo         0          false    Draft              porch-test
porch-demo   porch-test.network-function.innerhome                          network-function                            innerhome       2          true     Published          porch-test
porch-demo   porch-test.network-function.innerhome3                         network-function                            innerhome3      0          false    Proposed           porch-test
porch-demo   porch-test.network-function.innerhome4                         network-function                            innerhome4      0          false    Draft              porch-test
porch-demo   porch-test.network-function.main                               network-function                            main            -1         false    Published          porch-test
porch-demo   porch-test.network-function.outerspace                         network-function                            outerspace      1          false    DeletionProposed   porch-test
```

The `NAME` column gives the kubernetes name of the package revision resource. Names are of the form:

**repository.([pathnode.]*)package.workspace**

1. The first part (up to the first dot) is the **repository** that the package revision is in.
1. The second (optional) part is zero or more **pathnode** nodes, identifying the path of the package.
1. The second last part (between the second last and last dots) is the **package** that the package revision is in.
1. The last part (after the last dot) is the **workspace** of the package revision, which uniquely identifies the package revision in the package.

From the listing above, the package revision with the name `test-blueprints.basens.v3` is in a repository called `test-blueprints`. It is in the root of that
repository because there are no **pathnode** entries in its name. It is in a package called `basens` and its workspace name is `v3`.

The package revision with the name `porch-test.basedir.subdir.subsubdir.edge-function.inadir` is in the repo `porch-test`. It has a path of
`basedir/subdir/subsubdir`. The package name is `edge-function` and its workspace name is `inadir`.

The entire name must comply with the constraints on DNS Subdomain Names
specified in [kubernetes rules for naming objects and IDs](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/).
The name must:

- contain no more than 253 characters
- contain only lowercase alphanumeric characters, '-' or '.'
- start with an alphanumeric character
- end with an alphanumeric character

Each part of the name must comply with the constraints on RFC 1123 label names
specified in [kubernetes rules for naming objects and IDs](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/).
Each part of the name must:

- contain at most 63 characters
- contain only lowercase alphanumeric characters or '-'
- start with an alphanumeric character
- end with an alphanumeric character

The `PACKAGE` column contains the package name of a pacakge revision. Of course, all package revisions in a package have the same package name. The
package name includes the path to the directory containing the package if the package is not in the root directory of the repo. For example, in the listing above
the packages `basedir/subdir/subsubdir/edge-function` and `basedir/subdir/subsubdir/network-function` are in the directory `basedir/subdir/subsubdir`. The
`basedir/subdir/subsubdir/network-function` and `network-function` packages are different packages because they are in different directories.

The `REVISION` column indicates the revision of the package.
- Revisions of `1` or greater indicate released package revisions. When a package revision is `Published` it is assigned the next
  available revision number, starting at `1`. In the listing above, the `porch-test.network-function.innerhome` revision of package `network-function`
  has a revision of `2` and is the latest revision of the package. The `porch-test.network-function.outerspace` revision of the package has a
  revision of `1`. If the `porch-test.network-function.innerhome3` revision is published, it will be assigned a revision of `3` and will become
  the latest package revision.
- Package revisions that are not published (package revisions with a lifecycle status of `Draft` or `Proposed`) have a revision number of `0`. There can be many
  revisions of a package with revision `0` as is shown with revisions `porch-test.network-function.innerhome3` and `porch-test.network-function.innerhome4`
  of package `network-function` above.
- Placeholder package revisions that point at the head of a git branch or tag have a revision number of `-1`

The `LATEST` column indicates whether the package revision is the latest among the revisions of the same package. In the
output above, `3` is the latest revision of `basens` package and `1` is the latest revision of `empty` package.

The `LIFECYCLE` column indicates the lifecycle stage of the package revision, one of: `Draft`, `Proposed`, `Published` or `DeletionProposed`.

The `WORKSPACENAME` column indicates the workspace name of a package revision. The workspace name is selected by a user when a draft
package revision is created. The workspace name must be unique among package revisions in the same package. A user is free to
select any workspace name that complies with the constraints on DNS Subdomain Names specified in
[kubernetes rules for naming objects and IDs](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/).

{{% alert title="Scope of WORKSPACENAME" color="primary" %}}
The scope of a workspace name is restricted to its package and it is merely a string that identifies a package revision within a package.
The workspace name `V1` on the `empty` package has no relation to the workspace name `V1` on the `basens` package listed above.
A user has simply decided to use the same workspace name on two separate package revisions.
{{% /alert %}}

{{% alert title="Setting WORKSPACENAME and REVISION from repositories" color="primary" %}}
When Porch connects to a repository, it scans the branches and tags of the Git repository for package revisions. It descends the directory tree of the repo
looking for files called `Kptfile`. When it finds a Kptfile in a directory, Porch knows that it has found a kpt package and it does not search any child directories
of this directory. Porch then examines all branches and tags that have references to that package and finds package revisions using the following rules:
1. Look for a commit message of the form `kpt:{"package":"<package>","workspaceName":"<workspace>","revision":"<revision>"}` at the tip of the branch/tag and
   set the workspace name and revision from the commit message. The commit message `kpt:{"package":"network-function","workspaceName":"outerspace","revision":"1"}`
   is used to set the workspace name to `outerspace` and the revision to `1` in the case of the `porch-test.network-function.outerspace`
   package revision in the listing above.
2. If 1. fails, and if the reference is of the form `<package>.v1`, set the workspace name to `v1` and the revision to `1` as is the case for the
   `oai.workloads.oai.oai-ran-operator.v1` package revision in the listing above.
3. If 2. fails, set the workspace name to the branch or tag name, and the revision to `-1`, as is the case for the `infra.infra.gcp.nephio-blueprint-repo.v3.0.0`
   package revision in the listing above. The workspace name is set to the branch name `v3.0.0`, and the revision is set to `-1`.
{{% /alert %}}

## Package Revision Filtering

Simple filtering of package revisions by name (substring) and revision (exact match) is supported by the CLI using
`--name`, `--revision` and `--workspace` flags:

```bash
$ porchctl -n porch-demo rpkg get --name network-function
NAME                                    PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.network-function.dirdemo     network-function   dirdemo         1          false    Published   porch-test
porch-test.network-function.innerhome   network-function   innerhome       2          true     Published   porch-test
porch-test.network-function.main        network-function   main            -1         false    Published   porch-test

$ porchctl -n porch-demo rpkg get --revision 1
NAME                                                         PACKAGE                                   WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.basedir.subdir.subsubdir.edge-function2.diredge   basedir/subdir/subsubdir/edge-function2   diredge         1          true     Published   porch-test
porch-test.edge-function2.diredgeab                          edge-function2                            diredgeab       1          true     Published   porch-test
porch-test.edge-function.diredge                             edge-function                             diredge         1          true     Published   porch-test
porch-test.network-function3.outerspace                      network-function3                         outerspace      1          true     Published   porch-test
porch-test.network-function.dirdemo                          network-function                          dirdemo         1          false    Published   porch-test

$ porchctl -n porch-demo rpkg get --workspace outerspace
NAME                                      PACKAGE             WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.network-function2.outerspace   network-function2   outerspace      0          false    Draft       porch-test
porch-test.network-function3.outerspace   network-function3   outerspace      1          true     Published   porch-test
```

You can also filter package revisions using the `kubectl` CLI with the `--selector` and `--field-selector` flags under the same conventions as for other KRM objects.

The `--selector` flag can be used to filter on one or more [metadata labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#list-and-watch-filtering):
```bash
$ kubectl get packagerevisions --show-labels --selector 'kpt.dev/latest-revision=true'
NAME                        PACKAGE   WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY        LABELS
test-blueprints.basens.v3   basens    v3              3          true     Published   test-blueprints   kpt.dev/latest-revision=true
test-blueprints.empty.v1    empty     v1              1          true     Published   test-blueprints   kpt.dev/latest-revision=true
```

The `--field-selector` flag can be used to filter on one or more package revision [fields](https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/).

### Supported Fields

As per Kubernetes convention, the `--field-selector` flag supports a subset of the PackageRevision resource type's fields:
- `metadata.name`
- `metadata.namespace`
- `spec.revision`
- `spec.packageName`
- `spec.repository`
- `spec.workspaceName`
- `spec.lifecycle`

{{% alert title="Note" color="primary" %}}

 The `spec.versions[*].selectableFields` field is not available for the PackageRevision resource type. Changing the fields supported by `--field-selector` requires editing Porch's source code and rebuilding the porch-server microservice.

{{% /alert %}}

For example:
```bash
$ kubectl get packagerevisions --show-labels --field-selector 'spec.repository==oai'
NAME                        PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY   LABELS
oai.database.main           database           main            -1         false    Published   oai          <none>
oai.oai-amf.main            oai-amf            main            -1         false    Published   oai          <none>
oai.oai-ausf.main           oai-ausf           main            -1         false    Published   oai          <none>
oai.oai-cp-operators.main   oai-cp-operators   main            -1         false    Published   oai          <none>
oai.oai-nrf.main            oai-nrf            main            -1         false    Published   oai          <none>
oai.oai-repository.main     oai-repository     main            -1         false    Published   oai          <none>
oai.oai-smf.main            oai-smf            main            -1         false    Published   oai          <none>
oai.oai-udm.main            oai-udm            main            -1         false    Published   oai          <none>
oai.oai-udr.main            oai-udr            main            -1         false    Published   oai          <none>
oai.oai-upf-edge.main       oai-upf-edge       main            -1         false    Published   oai          <none>
oai.oai-up-operators.main   oai-up-operators   main            -1         false    Published   oai          <none>
```

{{% alert title="Note" color="primary" %}}

Due to the restrictions of Porch's internal caching behavior, the `--field-selector` flag supports only the `=` and `==` operators. **The `!=` operator is not supported.**

{{% /alert %}}

The common `kubectl` [flags that control output format](https://kubernetes.io/docs/reference/kubectl/#output-options) are available as well:

```bash
$ porchctl rpkg get -n porch-demo porch-test.network-function.innerhome -o yaml
apiVersion: porch.kpt.dev/v1alpha1
kind: PackageRevision
metadata:
  labels:
    kpt.dev/latest-revision: "true"
  name: porch-test.network-function.innerhome
  namespace: porch-demo
spec:
  lifecycle: Published
  packageName: network-function
  repository: porch-test
  revision: 2
  workspaceName: innerhome
...
```

The `porchctl rpkg pull` command can be used to read the package revision resources.

The command can be used to print the package revision resources as `ResourceList` to `stdout`, which enables
[chaining](https://kpt.dev/book/04-using-functions/#chaining-functions-using-the-unix-pipe)
evaluation of functions on the package revision pulled from the Package Orchestration server.

```bash
$ porchctl rpkg pull -n porch-demo porch-test.network-function.innerhome
apiVersion: config.kubernetes.io/v1
kind: ResourceList
items:
- apiVersion: ""
  kind: KptRevisionMetadata
  metadata:
    name: porch-test.network-function.innerhome
    namespace: porch-demo
...
```

One of the driving motivations for the Package Orchestration service is enabling
WYSIWYG authoring of packages, including their contents, in highly usable UIs.
Porch therefore supports reading and updating package *contents*.

In addition to using a [UI](https://kpt.dev/guides/namespace-provisioning-ui/) with Porch, we
can change the package contents by pulling the package from Porch onto the local
disk, make any desired changes, and then pushing the updated contents to Porch.

```bash
$ porchctl rpkg pull -n porch-demo porch-test.network-function.innerhome ./innerhome

$ find innerhome

./innerhome
./innerhome/.KptRevisionMetadata
./innerhome/README.md
./innerhome/Kptfile
./innerhome/package-context.yaml
```

The command downloaded the `innerhome/v1` package revision contents and saved
them in the `./innerhome` directory. Now you will make some changes.

First, note that even though Porch updated the namespace name (in
`namespace.yaml`) to `innerhome` when the package was cloned, the `README.md`
was not updated. Let's fix it first.

Open the `README.md` in your favorite editor and update its contents, for
example:

```
# innerhome

## Description
kpt package for provisioning Innerhome namespace
```

In the second change, add a new mutator to the `Kptfile` pipeline. Use the
[set-labels](https://catalog.kpt.dev/function-catalog/set-labels/v0.1/) function which will add
labels to all resources in the package. Add the following mutator to the
`Kptfile` `pipeline` section:

```yaml
  - image: gcr.io/kpt-fn/set-labels:v0.1.5
    configMap:
      color: orange
      fruit: apple
```

The whole `pipeline` section now looks like this:

```yaml
pipeline:
  mutators:
  - image: gcr.io/kpt-fn/set-namespace:v0.4.1
    configPath: package-context.yaml
  - image: gcr.io/kpt-fn/apply-replacements:v0.1.1
    configPath: update-rolebinding.yaml
  - image: gcr.io/kpt-fn/set-labels:v0.1.5
    configMap:
      color: orange
      fruit: apple
```

Save the changes and push the package contents back to the server:

```sh
# Push updated package contents to the server
$ porchctl rpkg push -n porch-demo porch-test.network-function.innerhome ./innerhome
```

Now, pull the contents of the package revision again, and inspect one of the
configuration files.

```sh
# Pull the updated package contents to local drive for inspection:
$ porchctl rpkg pull -n porch-demo porch-test.network-function.innerhome ./updated-innerhome

# Inspect updated-innerhome/namespace.yaml
$ cat updated-innerhome/namespace.yaml 

apiVersion: v1
kind: Namespace
metadata:
  name: innerhome
  labels:
    color: orange
    fruit: apple
spec: {}
```

The updated namespace now has new labels! What happened?

Whenever a package is updated during the authoring process, in case current functions
of the pipline were changed or a new function was added to the pipeline list,
Porch automatically re-renders the package to make sure that all mutators and validators are
executed. So when we added the new `set-labels` mutator, as soon as we pushed
the updated package contents to Porch, Porch re-rendered the package and
the `set-labels` function applied the labels we requested (`color: orange` and
`fruit: apple`).

## Authoring Packages

Several commands in the `porchctl rpkg` group support package authoring:

* `init` - Initializes a new package revision in the target repository.
* `clone` - Creates a clone of a source package revision in the target repository.
* `copy` - Creates a new package revision from an existing one.
* `push` - Pushes package revision resources into a remote package.
* `del` - Deletes one or more package revisions in registered repositories.

The `porchctl rpkg init` command can be used to initialize a new package revision. Porch server will create and
initialize a new package revision (as a draft) and save it in the specified repository.

```bash
$ porchctl rpkg init new-package --repository=porch-test --workspace=my-workspace -n porch-demo
porch-test.new-package.my-workspace created

$ porchctl rpkg get -n porch-demo porch-test.new-package.my-workspace
NAME                                  PACKAGE       WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.new-package.my-workspace   new-package   my-workspace    0          false    Draft       porch-test
```

The new package revision is created in the `Draft` lifecycle stage. This is true also for all commands that create new package
revision (`init`, `clone` and `copy`).

Additional flags supported by the `porchctl rpkg init` command are:

* `--repository` - Repository in which the package revision will be created.
* `--workspace` - Workspace of the new package revision.
* `--description` -  Short description of the package revision.
* `--keywords` - List of keywords for the package revision.
* `--site` - Link to page with information about the package revision.


Use `porchctl rpkg clone` command to create a _downstream_ package revision by cloning an _upstream_ package revision. You can find out more about the _upstream_ and _downstream_ sections of the `Kptfile` in a [Getting a Package](https://kpt.dev/book/03-packages/#getting-a-package).

```bash
$ porchctl rpkg clone porch-test.new-package.my-workspace new-package-clone --repository=porch-deployment -n porch-demo
porch-deployment.new-package-clone.v1 created

# Confirm the package revision was created
porchctl rpkg get porch-deployment.new-package-clone.v1 -n porch-demo
NAME                                    PACKAGE             WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-deployment.new-package-clone.v1   new-package-clone   v1              0          false    Draft       porch-deployment
```

{{% alert title="Note" color="primary" %}}
 A cloned package must be created in a repository in the same namespace as
 the source package. Cloning a package with the Package Orchestration Service
 retains a reference to the upstream package revision in the clone, and
 cross-namespace references are not allowed. Package revisions in repositories
 in other namespaces can be cloned using a reference directly to the underlying
 oci or git repository as described below.
{{% /alert %}}

`porchctl rpkg clone` can also be used to clone package revisions that are in repositories not registered with Porch, for
example:

```bash
$ porchctl rpkg clone \
  https://github.com/nephio-project/catalog.git cloned-pkg-example-ue-bp \
  --directory=workloads/oai/pkg-example-ue-bp \
  --ref=main \
  --repository=porch-deployment \
  --namespace=porch-demo
porch-deployment.cloned-pkg-example-ue-bp.v1 created

# Confirm the package revision was created
$ porchctl rpkg get -n porch-demo porch-deployment.cloned-pkg-example-ue-bp.v1
NAME                                           PACKAGE                    WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-deployment.cloned-pkg-example-ue-bp.v1   cloned-pkg-example-ue-bp   v1              0          false    Draft       porch-deployment
```

The flags supported by the `porchctl rpkg clone` command are:

* `--directory` - Directory within the upstream repository where the upstream
  package revision is located.
* `--ref` - Ref in the upstream repository where the upstream package revision is
  located. This can be a branch, tag, or SHA.
* `--repository` - Repository to which package revision will be cloned (downstream
  repository).
* `--workspace` - Workspace to assign to the downstream package revision.

The `porchctl rpkg copy` command can be used to create a new revision of an existing package. It is a means to
modifying an already published package revision.

```bash
$ porchctl rpkg copy porch-test.network-function.innerhome --workspace=great-outdoors -n porch-demo
porch-test.network-function.great-outdoors created

# Confirm the package revision was created
$ porchctl rpkg get porch-test.network-function.great-outdoors -n porch-demo
NAME                                         PACKAGE            WORKSPACENAME    REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.network-function.great-outdoors   network-function   great-outdoors   0          false    Draft       porch-test
```
Unlike `clone` of a package which establishes the upstream-downstream
relationship between the respective packages, and updates the `Kptfile`
to reflect the relationship, the `copy` command does *not* change the
upstream-downstream relationships. The copy of a package shares the same
upstream package as the package from which it was copied. Specifically,
in this case both packages have identical contents,
including upstream information, and differ in revision only.

The `porchctl rpkg pull` and `porchctl rpkg push` commands can be used to update the resources (package revision contents) of a package _draft_:

```bash
$ porchctl rpkg pull porch-test.network-function.great-outdoors ./great-outdoors -n porch-demo

# Make edits using your favorite YAML editor, for example adding a new resource
$ cat <<EOF > ./great-outdoors/config-map.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: example-config-map
data:
  color: green
EOF

# Push the updated contents to the Package Orchestration server, updating the
# package revision contents.
$ porchctl rpkg push porch-test.network-function.great-outdoors ./great-outdoors -n porch-demo

# Confirm that the remote package revision now includes the new ConfigMap resource
$ porchctl rpkg pull porch-test.network-function.great-outdoors -n porch-demo
apiVersion: config.kubernetes.io/v1
kind: ResourceList
items:
...
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: example-config-map
    annotations:
      config.kubernetes.io/index: '0'
      internal.config.kubernetes.io/index: '0'
      internal.config.kubernetes.io/path: 'config-map.yaml'
      config.kubernetes.io/path: 'config-map.yaml'
  data:
    color: green
...
```
Package revision can be deleted using `porchctl rpkg del` command:

```bash
# Delete package revision
$ porchctl rpkg del porch-test.network-function.great-outdoors -n porch-demo
porch-test.network-function.great-outdoors deleted
```

## Package Lifecycle and Approval Flow

Authoring is performed on the package revisions in the _Draft_ lifecycle stage. Before a package revision can be deployed, copied or
cloned, it must be _Published_. The approval flow is the process by which the package revision is advanced from _Draft_ state
through _Proposed_ state and finally to _Published_ lifecycle stage.

The commands used to manage package revision lifecycle stages include:

* `propose` - Proposes to finalize a package revision draft
* `approve` - Approves a proposal to finalize a package revision.
* `reject`  - Rejects a proposal to finalize a package revision

In the [Authoring Packages](#authoring-packages) section above we created several _draft_ package revisions and in this section
we will create proposals for publishing some of them.

```bash
# List package revisions to identify relevant drafts:
$ porchctl rpkg get
NAME                                           PACKAGE                    WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
...
porch-deployment.cloned-pkg-example-ue-bp.v1   cloned-pkg-example-ue-bp   v1              0          false    Draft       porch-deployment
porch-deployment.new-package-clone.v1          new-package-clone          v1              0          false    Draft       porch-deployment
porch-test.network-function2.outerspace        network-function2          outerspace      0          false    Draft       porch-test
porch-test.network-function3.innerhome5        network-function3          innerhome5      0          false    Draft       porch-test
porch-test.network-function3.innerhome6        network-function3          innerhome6      0          false    Draft       porch-test
porch-test.new-package.my-workspace            new-package                my-workspace    0          false    Draft       porch-test
...

# Propose two package revisions to be be published
$ porchctl rpkg propose \
  porch-deployment.new-package-clone.v1 \
  porch-test.network-function3.innerhome6 \
  -n porch-demo

porch-deployment.new-package-clone.v1 proposed
porch-test.network-function3.innerhome6 proposed

# Confirm the package revisions are now Proposed
$ porchctl rpkg get
NAME                                           PACKAGE                    WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
...
porch-deployment.cloned-pkg-example-ue-bp.v1   cloned-pkg-example-ue-bp   v1              0          false    Draft       porch-deployment
porch-deployment.new-package-clone.v1          new-package-clone          v1              0          false    Proposed    porch-deployment
porch-test.network-function2.outerspace        network-function2          outerspace      0          false    Draft       porch-test
porch-test.network-function3.innerhome5        network-function3          innerhome5      0          false    Draft       porch-test
porch-test.network-function3.innerhome6        network-function3          innerhome6      0          false    Proposed    porch-test
porch-test.new-package.my-workspace            new-package                my-workspace    0          false    Draft       porch-test
```

At this point, a person in _platform administrator_ role, or even an automated process, will review and either approve
or reject the proposals. To aid with the decision, the platform administrator may inspect the package revision contents using the
commands above, such as `porchctl rpkg pull`.

```bash
# Approve a proposal to publish a package revision
$ porchctl rpkg approve porch-deployment.new-package-clone.v1 -n porch-demo
porch-deployment.new-package-clone.v1 approved

# Reject a proposal to publish a package revision
$ porchctl rpkg reject porch-test.network-function3.innerhome6 -n porch-demo
porch-test.network-function3.innerhome6 no longer proposed for approval
```

Now the user can confirm lifecycle stages of the package revisions:

```bash
# Confirm package revision lifecycle stages after approvals:
$ porchctl rpkg get
NAME                                           PACKAGE                    WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
...
porch-deployment.cloned-pkg-example-ue-bp.v1   cloned-pkg-example-ue-bp   v1              0          false    Draft       porch-deployment
porch-deployment.new-package-clone.v1          new-package-clone          v1              1          true     Published   porch-deployment
porch-test.network-function2.outerspace        network-function2          outerspace      0          false    Draft       porch-test
porch-test.network-function3.innerhome5        network-function3          innerhome5      0          false    Draft       porch-test
porch-test.network-function3.innerhome6        network-function3          innerhome6      0          false    Draft       porch-test
porch-test.new-package.my-workspace            new-package                my-workspace    0          false    Draft       porch-test
```

Observe that the rejected proposal returned the package revision back to _Draft_ lifecycle stage. The package revision whose
proposal was approved is now in _Published_ state.

An approved pacakge revision cannot be directly deleted, it must first be proposed for deletion.

```bash
porchctl rpkg propose-delete -n porch-demo porch-deployment.new-package-clone.v1

# Confirm package revision lifecycle stages after deletion proposed:
$ porchctl rpkg get
NAME                                           PACKAGE                    WORKSPACENAME   REVISION   LATEST   LIFECYCLE          REPOSITORY
...
porch-deployment.cloned-pkg-example-ue-bp.v1   cloned-pkg-example-ue-bp   v1              0          false    Draft              porch-deployment
porch-deployment.new-package-clone.v1          new-package-clone          v1              1          true     DeletionProposed   porch-deployment
porch-test.network-function2.outerspace        network-function2          outerspace      0          false    Draft              porch-test
porch-test.network-function3.innerhome5        network-function3          innerhome5      0          false    Draft              porch-test
porch-test.network-function3.innerhome6        network-function3          innerhome6      0          false    Draft              porch-test
porch-test.new-package.my-workspace            new-package                my-workspace    0          false    Draft              porch-test
```

At this point, a person in _platform administrator_ role, or even an automated process, will review and either approve
or reject the deletion.

```bash
porchctl rpkg reject -n porch-demo porch-deployment.new-package-clone.v1

# Confirm package revision deletion has been rejected:
$ porchctl rpkg get
NAME                                           PACKAGE                    WORKSPACENAME   REVISION   LATEST   LIFECYCLE          REPOSITORY
...
porch-deployment.cloned-pkg-example-ue-bp.v1   cloned-pkg-example-ue-bp   v1              0          false    Draft       porch-deployment
porch-deployment.new-package-clone.v1          new-package-clone          v1              1          true     Published   porch-deployment
porch-test.network-function2.outerspace        network-function2          outerspace      0          false    Draft       porch-test
porch-test.network-function3.innerhome5        network-function3          innerhome5      0          false    Draft       porch-test
porch-test.network-function3.innerhome6        network-function3          innerhome6      0          false    Draft       porch-test
porch-test.new-package.my-workspace            new-package                my-workspace    0          false    Draft       porch-test
```

The package revision can again be proposed for deletion.

```bash
porchctl rpkg propose-delete -n porch-demo porch-deployment.new-package-clone.v1

# Confirm package revision lifecycle stages after deletion proposed:
$ porchctl rpkg get
NAME                                           PACKAGE                    WORKSPACENAME   REVISION   LATEST   LIFECYCLE          REPOSITORY
...
porch-deployment.cloned-pkg-example-ue-bp.v1   cloned-pkg-example-ue-bp   v1              0          false    Draft              porch-deployment
porch-deployment.new-package-clone.v1          new-package-clone          v1              1          true     DeletionProposed   porch-deployment
porch-test.network-function2.outerspace        network-function2          outerspace      0          false    Draft              porch-test
porch-test.network-function3.innerhome5        network-function3          innerhome5      0          false    Draft              porch-test
porch-test.network-function3.innerhome6        network-function3          innerhome6      0          false    Draft              porch-test
porch-test.new-package.my-workspace            new-package                my-workspace    0          false    Draft              porch-test
```

At this point, a person in _platform administrator_ role, or even an automated process, decides to proceed with the deletion.

```bash
porchctl rpkg delete -n porch-demo porch-deployment.new-package-clone.v1

# Confirm package revision is deleted:
$ porchctl rpkg get
NAME                                           PACKAGE                    WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
...
porch-deployment.cloned-pkg-example-ue-bp.v1   cloned-pkg-example-ue-bp   v1              0          false    Draft       porch-deployment
porch-test.network-function2.outerspace        network-function2          outerspace      0          false    Draft       porch-test
porch-test.network-function3.innerhome5        network-function3          innerhome5      0          false    Draft       porch-test
porch-test.network-function3.innerhome6        network-function3          innerhome6      0          false    Draft       porch-test
porch-test.new-package.my-workspace            new-package                my-workspace    0          false    Draft       porch-test
```

## Package Upgrade


The flags supported by the `porchctl rpkg upgrade` command are:

* `--revision` - (*Optional*) The revision number of the upstream package that the target
  downstream package revision should be upgraded to.
  The corresponding revision must be published. If not set, the latest will be chosen.
* `--workspace` - The workspace name of the newly created package revision.
* `--strategy` - The strategy to use for the upgrade.
Options: `resource-merge` (*default*), `fast-forward`, `force-delete-replace`, `copy-merge`.

The `porchctl rpkg upgrade` command can be used to create a new revision which upgrades a published downstream to a more recent published revision of its upstream package.

```bash
# upgrade repository.package.v1 package to the latest of its upstream, using resource-merge strategy
$ porchctl rpkg upgrade repository.package.1 --workspace=2

# upgrade repository.package.v1 package to revision v3 of its upstream, using resource-merge strategy
$ porchctl rpkg upgrade repository.package.1 --workspace=2 --revision=3

# upgrade repository.package.v1 package to revision v3 of its upstream, using copy-merge strategy
$ porchctl rpkg upgrade repository.package.1 --workspace=2 --revision=3 --strategy=copy-merge
```
