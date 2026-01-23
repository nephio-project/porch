---
title: "API Reference"
type: docs
weight: 2
description: API reference documentation for Porch resources
---

# API Reference

## Packages
- [porch.kpt.dev/v1alpha1](#porchkptdevv1alpha1)
- [config.porch.kpt.dev/v1alpha1](#configporchkptdevv1alpha1)

---

## porch.kpt.dev/v1alpha1

### Resource Types
- [PackageRevision](#packagerevision)
- [PackageRevisionResources](#packagerevisionresources)
- [PorchPackage](#porchpackage)

#### Condition

_Appears in:_
- [PackageRevisionStatus](#packagerevisionstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ |  |  |  |
| `status` _[ConditionStatus](#conditionstatus)_ |  |  |  |
| `reason` _string_ |  |  |  |
| `message` _string_ |  |  |  |


#### ConditionStatus

_Underlying type:_ _string_

_Appears in:_
- [Condition](#condition)

| Field | Description |
| --- | --- |
| `True` |  |
| `False` |  |
| `Unknown` |  |


#### Field

Field references a field in a resource

_Appears in:_
- [ResultItem](#resultitem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `path` _string_ | Path is the field path. This field is required. |  |  |
| `currentValue` _string_ | CurrentValue is the current field value |  |  |
| `proposedValue` _string_ | ProposedValue is the proposed value of the field to fix an issue. |  |  |


#### File

File references a file containing a resource

_Appears in:_
- [ResultItem](#resultitem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `path` _string_ | Path is relative path to the file containing the resource.<br />This field is required. |  |  |
| `index` _integer_ | Index is the index into the file containing the resource<br />(i.e. if there are multiple resources in a single file) |  |  |


#### GitLock

GitLock is the resolved locator for a package on Git.

_Appears in:_
- [UpstreamLock](#upstreamlock)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repo` _string_ | Repo is the git repository that was fetched.<br />e.g. 'https://github.com/kubernetes/examples.git' |  |  |
| `directory` _string_ | Directory is the sub directory of the git repository that was fetched.<br />e.g. 'staging/cockroachdb' |  |  |
| `ref` _string_ | Ref can be a Git branch, tag, or a commit SHA-1 that was fetched.<br />e.g. 'master' |  |  |
| `commit` _string_ | Commit is the SHA-1 for the last fetch of the package.<br />This is set by kpt for bookkeeping purposes. |  |  |


#### GitPackage

_Appears in:_
- [UpstreamPackage](#upstreampackage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repo` _string_ | Address of the Git repository, for example:<br />  `https://github.com/GoogleCloudPlatform/blueprints.git` |  |  |
| `ref` _string_ | `Ref` is the git ref containing the package. Ref can be a branch, tag, or commit SHA. |  |  |
| `directory` _string_ | Directory within the Git repository where the packages are stored. A subdirectory of this directory containing a Kptfile is considered a package. |  |  |
| `secretRef` _[SecretRef](#secretref)_ | Reference to secret containing authentication credentials. Optional. |  |  |


#### NameMeta

NameMeta contains name information.

_Appears in:_
- [ResourceIdentifier](#resourceidentifier)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the metadata.name field of a Resource |  |  |
| `namespace` _string_ | Namespace is the metadata.namespace field of a Resource |  |  |


#### OciPackage

OciPackage describes a repository compatible with the Open Container Registry standard.

_Appears in:_
- [UpstreamPackage](#upstreampackage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `image` _string_ | Image is the address of an OCI image. |  |  |


#### OriginType

_Underlying type:_ _string_

_Appears in:_
- [UpstreamLock](#upstreamlock)


#### PackageCloneTaskSpec

_Appears in:_
- [Task](#task)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `upstreamRef` _[UpstreamPackage](#upstreampackage)_ | `Upstream` is the reference to the upstream package to clone. |  |  |


#### PackageEditTaskSpec

_Appears in:_
- [Task](#task)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `sourceRef` _[PackageRevisionRef](#packagerevisionref)_ |  |  |  |


#### PackageInitTaskSpec

PackageInitTaskSpec defines the package initialization task.

_Appears in:_
- [Task](#task)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `subpackage` _string_ | `Subpackage` is a directory path to a subpackage to initialize. If unspecified, the main package will be initialized. |  |  |
| `description` _string_ | `Description` is a short description of the package. |  |  |
| `keywords` _string array_ | `Keywords` is a list of keywords describing the package. |  |  |
| `site` _string_ | `Site is a link to page with information about the package. |  |  |


#### PackageMergeStrategy

_Underlying type:_ _string_

_Appears in:_
- [PackageUpgradeTaskSpec](#packageupgradetaskspec)

| Field | Description |
| --- | --- |
| `resource-merge` |  |
| `fast-forward` |  |
| `force-delete-replace` |  |
| `copy-merge` |  |


#### PackageRevision

PackageRevision represents a specific revision of a package.

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `porch.kpt.dev/v1alpha1` | | |
| `kind` _string_ | `PackageRevision` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PackageRevisionSpec](#packagerevisionspec)_ |  |  |  |
| `status` _[PackageRevisionStatus](#packagerevisionstatus)_ |  |  |  |


#### PackageRevisionLifecycle

_Underlying type:_ _string_

_Appears in:_
- [PackageRevisionSpec](#packagerevisionspec)

| Field | Description |
| --- | --- |
| `Draft` |  |
| `Proposed` |  |
| `Published` |  |
| `DeletionProposed` |  |


#### PackageRevisionRef

PackageRevisionRef is a reference to a package revision.

_Appears in:_
- [PackageEditTaskSpec](#packageedittaskspec)
- [PackageUpgradeTaskSpec](#packageupgradetaskspec)
- [UpstreamPackage](#upstreampackage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | `Name` is the name of the referenced PackageRevision resource. |  |  |


#### PackageRevisionResources

PackageRevisionResources contains the actual file contents of a package revision.

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `porch.kpt.dev/v1alpha1` | | |
| `kind` _string_ | `PackageRevisionResources` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PackageRevisionResourcesSpec](#packagerevisionresourcesspec)_ |  |  |  |
| `status` _[PackageRevisionResourcesStatus](#packagerevisionresourcesstatus)_ |  |  |  |


#### PackageRevisionResourcesSpec

PackageRevisionResourcesSpec represents resources (as ResourceList serialized as yaml string) of the PackageRevision.

_Appears in:_
- [PackageRevisionResources](#packagerevisionresources)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `packageName` _string_ | PackageName identifies the package in the repository. |  |  |
| `workspaceName` _string_ | WorkspaceName identifies the workspace of the package. |  |  |
| `revision` _integer_ | Revision identifies the version of the package. |  |  |
| `repository` _string_ | RepositoryName is the name of the Repository object containing this package. |  |  |
| `resources` _object (keys:string, values:string)_ | Resources are the content of the package. |  |  |


#### PackageRevisionResourcesStatus

PackageRevisionResourcesStatus represents state of the rendered package resources.

_Appears in:_
- [PackageRevisionResources](#packagerevisionresources)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `renderStatus` _[RenderStatus](#renderstatus)_ | RenderStatus contains the result of rendering the package resources. |  |  |


#### PackageRevisionSpec

PackageRevisionSpec defines the desired state of PackageRevision

_Appears in:_
- [PackageRevision](#packagerevision)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `packageName` _string_ | PackageName identifies the package in the repository. |  |  |
| `repository` _string_ | RepositoryName is the name of the Repository object containing this package. |  |  |
| `workspaceName` _string_ | WorkspaceName is a short, unique description of the changes contained in this package revision. |  |  |
| `revision` _integer_ | Revision identifies the version of the package. |  |  |
| `parent` _[ParentReference](#parentreference)_ | Parent references a package that provides resources to us |  |  |
| `lifecycle` _[PackageRevisionLifecycle](#packagerevisionlifecycle)_ |  |  |  |
| `tasks` _[Task](#task) array_ | The task slice holds zero or more tasks that describe the operations performed on the packagerevision. |  |  |
| `readinessGates` _[ReadinessGate](#readinessgate) array_ |  |  |  |


#### PackageRevisionStatus

PackageRevisionStatus defines the observed state of PackageRevision

_Appears in:_
- [PackageRevision](#packagerevision)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `upstreamLock` _[UpstreamLock](#upstreamlock)_ | UpstreamLock identifies the upstream data for this package. |  |  |
| `publishedBy` _string_ | PublishedBy is the identity of the user who approved the packagerevision. |  |  |
| `publishTimestamp` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#time-v1-meta)_ | PublishedAt is the time when the packagerevision were approved. |  |  |
| `deployment` _boolean_ | Deployment is true if this is a deployment package (in a deployment repository). |  |  |
| `conditions` _[Condition](#condition) array_ |  |  |  |


#### PackageSpec

PackageSpec defines the desired state of Package

_Appears in:_
- [PorchPackage](#porchpackage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `packageName` _string_ | PackageName identifies the package in the repository. |  |  |
| `repository` _string_ | RepositoryName is the name of the Repository object containing this package. |  |  |


#### PackageStatus

PackageStatus defines the observed state of Package

_Appears in:_
- [PorchPackage](#porchpackage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `latestRevision` _integer_ | LatestRevision identifies the package revision that is the latest published package revision belonging to this package. |  |  |


#### PackageUpgradeTaskSpec

_Appears in:_
- [Task](#task)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `oldUpstreamRef` _[PackageRevisionRef](#packagerevisionref)_ | `OldUpstream` is the reference to the original upstream package revision. |  |  |
| `newUpstreamRef` _[PackageRevisionRef](#packagerevisionref)_ | `NewUpstream` is the reference to the new upstream package revision. |  |  |
| `localPackageRevisionRef` _[PackageRevisionRef](#packagerevisionref)_ | `LocalPackageRevisionRef` is the reference to the local package revision. |  |  |
| `strategy` _[PackageMergeStrategy](#packagemergestrategy)_ | Defines which strategy should be used to update the package. |  |  |


#### ParentReference

ParentReference is a reference to a parent package

_Appears in:_
- [PackageRevisionSpec](#packagerevisionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the parent PackageRevision |  |  |


#### PorchPackage

PorchPackage represents a package in a repository.

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `porch.kpt.dev/v1alpha1` | | |
| `kind` _string_ | `PorchPackage` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PackageSpec](#packagespec)_ |  |  |  |
| `status` _[PackageStatus](#packagestatus)_ |  |  |  |


#### ReadinessGate

_Appears in:_
- [PackageRevisionSpec](#packagerevisionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditionType` _string_ |  |  |  |


#### RenderStatus

RenderStatus represents the result of performing render operation on a package resources.

_Appears in:_
- [PackageRevisionResourcesStatus](#packagerevisionresourcesstatus)
- [TaskResult](#taskresult)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `error` _string_ |  |  |  |


#### RepositoryType

_Underlying type:_ _string_

_Appears in:_
- [UpstreamPackage](#upstreampackage)

| Field | Description |
| --- | --- |
| `git` |  |
| `oci` |  |


#### ResourceIdentifier

ResourceIdentifier contains the information needed to uniquely identify a resource in a cluster.

_Appears in:_
- [ResultItem](#resultitem)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the metadata.name field of a Resource |  |  |
| `namespace` _string_ | Namespace is the metadata.namespace field of a Resource |  |  |


#### ResultItem

ResultItem defines a validation result

_Appears in:_
- [Result](#result)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `message` _string_ | Message is a human readable message. This field is required. |  |  |
| `severity` _string_ | Severity is the severity of this result |  |  |
| `resourceRef` _[ResourceIdentifier](#resourceidentifier)_ | ResourceRef is a reference to a resource. |  |  |
| `field` _[Field](#field)_ | Field is a reference to the field in a resource this result refers to |  |  |
| `file` _[File](#file)_ | File references a file containing the resource this result refers to |  |  |
| `tags` _object (keys:string, values:string)_ | Tags is an unstructured key value map stored with a result |  |  |


#### SecretRef

_Appears in:_
- [GitPackage](#gitpackage)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the secret. The secret is expected to be located in the same namespace as the resource containing the reference. |  |  |


#### Task

_Appears in:_
- [PackageRevisionSpec](#packagerevisionspec)
- [TaskResult](#taskresult)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[TaskType](#tasktype)_ |  |  |  |
| `init` _[PackageInitTaskSpec](#packageinittaskspec)_ |  |  |  |
| `clone` _[PackageCloneTaskSpec](#packageclonetaskspec)_ |  |  |  |
| `edit` _[PackageEditTaskSpec](#packageedittaskspec)_ |  |  |  |
| `upgrade` _[PackageUpgradeTaskSpec](#packageupgradetaskspec)_ |  |  |  |


#### TaskType

_Underlying type:_ _string_

_Appears in:_
- [Task](#task)

| Field | Description |
| --- | --- |
| `init` |  |
| `clone` |  |
| `edit` |  |
| `upgrade` |  |
| `render` |  |
| `push` |  |
| `` |  |


#### UpstreamLock

UpstreamLock is a resolved locator for the last fetch of the package.

_Appears in:_
- [PackageRevisionStatus](#packagerevisionstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[OriginType](#origintype)_ | Type is the type of origin. |  |  |
| `git` _[GitLock](#gitlock)_ | Git is the resolved locator for a package on Git. |  |  |


#### UpstreamPackage

UpstreamRepository repository may be specified directly or by referencing another Repository resource.

_Appears in:_
- [PackageCloneTaskSpec](#packageclonetaskspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[RepositoryType](#repositorytype)_ | Type of the repository (i.e. git, OCI). If empty, `upstreamRef` will be used. |  |  |
| `git` _[GitPackage](#gitpackage)_ | Git upstream package specification. Required if `type` is `git`. |  |  |
| `oci` _[OciPackage](#ocipackage)_ | OCI upstream package specification. Required if `type` is `oci`. |  |  |
| `upstreamRef` _[PackageRevisionRef](#packagerevisionref)_ | UpstreamRef is the reference to the package from a registered repository. |  |  |

---

## config.porch.kpt.dev/v1alpha1

Package v1alpha1 contains API Schema definitions for the v1alpha1 API group

### Resource Types
- [Repository](#repository)


#### GitRepository

GitRepository describes a Git repository.

_Appears in:_
- [RepositorySpec](#repositoryspec)
- [UpstreamRepository](#upstreamrepository)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `repo` _string_ | Address of the Git repository, for example:<br />  `https://github.com/GoogleCloudPlatform/blueprints.git` |  |  |
| `branch` _string_ | Name of the branch containing the packages. Finalized packages will be committed to this branch (if the repository allows write access). If unspecified, defaults to "main". | main | MinLength: 1 <br /> |
| `createBranch` _boolean_ | CreateBranch specifies if Porch should create the package branch if it doesn't exist. |  |  |
| `directory` _string_ | Directory within the Git repository where the packages are stored. A subdirectory of this directory containing a Kptfile is considered a package. If unspecified, defaults to root directory. |  |  |
| `secretRef` _[SecretRef](#secretref)_ | Reference to secret containing authentication credentials. |  |  |
| `author` _string_ | Author to use for commits |  |  |
| `email` _string_ | Email to use for commits |  |  |


#### OciRepository

OciRepository describes a repository compatible with the Open Container Registry standard.

_Appears in:_
- [RepositorySpec](#repositoryspec)
- [UpstreamRepository](#upstreamrepository)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `registry` _string_ | Registry is the address of the OCI registry |  |  |
| `secretRef` _[SecretRef](#secretref)_ | Reference to secret containing authentication credentials. |  |  |


#### Repository

Repository represents a Git or OCI repository containing packages.

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `config.porch.kpt.dev/v1alpha1` | | |
| `kind` _string_ | `Repository` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[RepositorySpec](#repositoryspec)_ |  |  |  |
| `status` _[RepositoryStatus](#repositorystatus)_ |  |  |  |


#### RepositoryContent

_Underlying type:_ _string_

_Appears in:_
- [RepositorySpec](#repositoryspec)

| Field | Description |
| --- | --- |
| `Package` |  |


#### RepositoryRef

RepositoryRef identifies a reference to a Repository resource.

_Appears in:_
- [UpstreamRepository](#upstreamrepository)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Repository resource referenced. |  |  |


#### RepositorySpec

RepositorySpec defines the desired state of Repository

_Appears in:_
- [Repository](#repository)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `description` _string_ | User-friendly description of the repository |  |  |
| `deployment` _boolean_ | The repository is a deployment repository; final packages in this repository are deployment ready. |  |  |
| `type` _[RepositoryType](#repositorytype)_ | Type of the repository (i.e. git, OCI) |  |  |
| `content` _[RepositoryContent](#repositorycontent)_ | The Content field is deprecated, please do not specify it in new manifests. | Package |  |
| `sync` _[RepositorySync](#repositorysync)_ | Repository sync/reconcile details |  |  |
| `git` _[GitRepository](#gitrepository)_ | Git repository details. Required if `type` is `git`. Ignored if `type` is not `git`. |  |  |
| `oci` _[OciRepository](#ocirepository)_ | OCI repository details. Required if `type` is `oci`. Ignored if `type` is not `oci`. |  |  |


#### RepositoryStatus

RepositoryStatus defines the observed state of Repository

_Appears in:_
- [Repository](#repository)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#condition-v1-meta) array_ | Conditions describes the reconciliation state of the object. |  |  |


#### RepositorySync

_Appears in:_
- [RepositorySpec](#repositoryspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `runOnceAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#time-v1-meta)_ | Value in metav1.Time format to indicate when the repository should be synced once outside the periodic cron based reconcile loop. |  |  |
| `schedule` _string_ | Cron value to indicate when the repository should be synced periodically. Example: `*/10 * * * *` to sync every 10 minutes. |  |  |


#### RepositoryType

_Underlying type:_ _string_

_Appears in:_
- [RepositorySpec](#repositoryspec)
- [UpstreamRepository](#upstreamrepository)

| Field | Description |
| --- | --- |
| `git` |  |
| `oci` |  |


#### SecretRef

_Appears in:_
- [GitRepository](#gitrepository)
- [OciRepository](#ocirepository)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the secret. The secret is expected to be located in the same namespace as the resource containing the reference. |  |  |
