---
title: "Disaster Recovery in Porch"
type: docs
weight: 9
description: Descriptions of scenarios where one or more Porch data stores are lost or corrupted, with or without backups.
---

## Introduction

This document describes the impact on Porch when one or more data stores fails.

Porch has a relatively complex data storage model, such that it essentially acts as a mediator between sets of data stored
in several locations:
* the Kubernetes **cluster control plane** on which Porch is installed, including Porch's **custom resources**
* the **storage repositories** in which package resources are stored (backing the Porch Repository objects). For more details
  on how Porch interacts with repositories, see the documentation on Repositories and Repository Adapters
        <!-- TODO: add links to Repositories and Repository Adapters documentation once each is merged -->
        <!-- TODO: Repositories will be ../2_concepts/repositories.md -->
        <!-- TODO: Repository Adapters will be ../2_concepts/repositories.md -->
  * **Git** repositories are Porch's primary and fully-supported storage backend
* and the contents of the **package revision cache** (which, depending on the cache option configured at install-time, may
  be either **incorporated in the cluster's control plane** (with the CR cache) or stored in **a separate SQL database**
  (with the DB cache)).
  A more detailed explanation of the package revision cache and the different cache options can be found in the Architecture
  and Components section: [Cache]({{% relref "/docs/5_architecture_and_components/package-cache/_index.md" %}})

Porch's data storage operations are covered in significantly greater depth in the [Architecture and Components section]({{% relref "/docs/5_architecture_and_components/_index.md" %}}).

Each data store serves as the source of truth for different elements of Porch's data structure:
* custom resource objects on Kubernetes control plane:
    * Porch repositories (Repository objects)
    * package variants (and by extension package variant sets)
    * **(if the CR cache is configured)** "work-in-progress" package revisions whose lifecycle stage is "Draft", "Proposed",
      or "DeletionProposed"
* Git repositories:
    * package revisions (Kpt package file contents and directory structures)
* Package revision cache:
    * Kubernetes-related metadata for package revisions (e.g. labels and annotations)
    * **(if the DB cache is configured)** "work-in-progress" package revisions whose lifecycle stage is "Draft", "Proposed",
      or "DeletionProposed"

## Backup strategy

Implementing a regular backup strategy for each data store will minimise data loss in the event of a disaster scenario.
Restoring from a valid point-in-time backup will guarantee recovery of Porch state at least at the point the backup was
made.

### Back up Porch custom resources

Porch custom resources - `Repositories`, `PackageVariants`, `PackageVariantSets`, and the CR cache's `PackageRevs` - are
stored in the Kubernetes cluster. They can be backed up as such - for example, by exporting them as YAML manifests or by
taking a snapshot of the cluster's etcd store. If Porch is itself being managed by a GitOps platform such as ConfigSync
or FluxCD, the GitOps platform is expected to have its own record of the custom resources, conducting its own reconciliation
operations to save and restore custom resources.

{{% alert title="N.B." color="primary" %}}

`PackageRevisions` and `PackageRevisionResources` are not actual custom resources and **should not be backed up as such**.
Their data and metadata are brought together by the Porch API server from separate sources in Git and the package revision
cache.

{{% /alert %}}
{{% alert title="N.B." color="primary" %}}

In the absence of an overarching GitOps solution, the exact mechanism for backing up cluster resources may vary between
Kubernetes distributions. Consult the documentation for your specific distribution!

{{% /alert %}}

### Back up Git repositories

The Git repositories contain the file content of the Kpt packages managed by Porch. The distributed nature of Git version
control means backing up a few repositories can be as simple as using Git from a secure machine: `git clone` to back up
and `git push` to restore. However, repositories in larger numbers or containing many large Kpt packages may require more
specialized approaches to back up the Git servers themselves

{{% alert title="N.B." color="primary" %}}

The exact mechanism for backing up Git servers may vary depending on your choice of Git software. Consult the documentation
for your specific Git service!

{{% /alert %}}

{{% alert title="Caution!" color="warning" %}}

The Git repository is the source of truth for package revision files. Even if a package revision's data is present in the
cache, this is not enough for Porch to restore a package revision that is not already present in Git.

{{% /alert %}}

### Back up DB cache database

If the DB cache option is selected at install time, the package revision cache is stored in an SQL database. Porch recommends
backing up the entire contents of this database on a regular basis, as it contains all data for unpublished package revisions
(in lifecycle stages "Draft", "Proposed", or "DeletionProposed")

{{% alert title="N.B." color="primary" %}}

The exact mechanism for backing up the database may vary depending on your choice of SQL software. Consult the documentation
for your specific database!

{{% /alert %}}

## Disaster scenarios

This section gives details of tested scenarios with varying combinations of backup, wipe, and restore routines for different
data stores.

{{% alert title="In case of package revision cache corruption" color="warning" %}}

- If the CR cache is configured and becomes corrupted, it can be recreated from the Git repositories. Porch recommends
  restarting the `porch-server` microservice to trigger this process (which will entail a decrease in quality of service
  until all repositories are deemed Ready).
- If the DB cache is configured and becomes corrupted, it **must be restored from a valid database backup**. If a valid
  backup is not available, data loss will ensue: "Draft" and "Proposed" package revisions will be lost completely, and "DeletionProposed" package revisions will be reverted to "Published".

  Porch recommends backing up the entire contents of the DB cache database
  on a regular basis.

{{% /alert %}}


### 1. Complete disaster

Kubernetes cluster is lost with all nodes; Git repositories are lost; DB cache database is lost.

#### Data backed up:
- Porch custom resources
- Git repository contents
- DB cache database contents

#### Data stores lost:
- Kubernetes control plane: entire Kubernetes cluster deleted
- Git repositories: Git server deleted and recreated empty of data
- DB cache database:
    - SQL script used to drop all Porch tables
    - PostgreSQL server deleted and recreated empty of data

#### Restoration steps:
1. Recreate Kubernetes cluster
2. Reinstall Porch with DB cache pointed to empty database server
   {{% alert title="Compatibility" color="warning" %}}

   To ensure data compatibility, backup must be restored into the DB cache of the same version of Porch.

   **In step 2, ensure Porch is reinstalled with the same version as before the cluster was lost!**
   {{% /alert %}}
3. Restore backed-up repository contents to Git server
4. Restore backed-up database contents to PostgreSQL server
5. Perform GitOps reconciliation, gradually (in batches of 20) re-creating all backed-up Porch Repository objects
    1. For each batch, wait until all Repository objects have condition with type "Ready" and status set "True"

#### Expected data loss

None - complete recovery of state at time data was backed up.

With backups of all data stores, Porch recovers all data.


### 2. Kubernetes cluster loss

Kubernetes cluster is lost with all nodes; Git repositories and DB cache database remain safe.

#### Data backed up:
- Porch custom resources

#### Data stores lost:
- Kubernetes control plane: entire Kubernetes cluster deleted

#### Restoration steps:
1. Recreate Kubernetes cluster
2. Reinstall Porch with DB cache pointed to same (still-existing) PostgreSQL server
   {{% alert title="Compatibility" color="warning" %}}

   To ensure data compatibility, backup must be restored into the DB cache of the same version of Porch.

   **In step 2, ensure Porch is reinstalled with the same version as before the cluster was lost!**
   {{% /alert %}}
3. Perform GitOps reconciliation, gradually (in batches of 20) re-creating all backed-up Porch Repository objects
    1. For each batch, wait until all Repository objects have condition with type "Ready" and status set "True"

#### Expected data loss

None - complete recovery of state at time of cluster loss.

Through using Git as the source of truth, we might expect Porch to automatically delete any state that only exists in the
cache - e.g., package revisions in "Draft" lifecycle stage. However, the connection between Porch and Git is represented by the Repository
objects, so until they are recreated in the GitOps reconciliation, Porch does not know the state of Git to overwrite the
cached state.


### 3. Porch microservices restarted

All Porch pods (by default, all in the "porch-system" namespace) are ungracefully restarted (e.g. by forcible pod deletion
with grace-period 0).

#### Data backed up:
- Porch custom resources
- Git repository contents
- DB cache database contents

#### Data stores lost:
- None
- Porch will immediately begin to re-sync all repositories, resulting in a **decrease in quality of service** until all
  repositories are deemed Ready
    - **Porch API will be unavailable** to perform operations on package revisions
        - `get` or `list` operations can be used to monitor Porch for API availability and repository status

#### Restoration steps:
1. Wait until all Porch pods return to Ready state
2. Wait until all Repository objects have condition with type "Ready" and status set "True"
    1. GitOps reconciliation is unnecessary in this case since the Repository objects are unchanged
3. List package revisions periodically, monitoring results until state stabilises

#### Expected data loss

None - no data stores were impacted, but only Porch's ability to manage them, allowing for full recovery.

In a representative testing environment, recovery takes **less than 5 minutes** for 115 Repository objects with a `4GiB`
memory limit applied to the `porch-server` microservice

{{% alert title="However:" color="warning" %}}

Recovery may take longer depending on:
- CPU and memory resources allotted to the Porch pods
    - lower resource limits => slower to sync
- number of Repository objects to re-sync
- physical size of Git repositories to reconnect to
- performance characteristics of the Kubernetes cluster

The `porch-server` pod, in particular, is responsible for fetching the state of all Git repositories referred to by Repository
objects. It may become unresponsive and be restarted by Kubernetes several times before eventually achieving package revision
consistency. Given sufficiently low resource limits, it may enter a crash loop and be unable to achieve even eventual consistency.

{{% /alert %}}


### 4. DB cache loss with backup

Kubernetes cluster and Git repositories remain safe; DB cache database is lost.

{{% alert title="N.B." color="primary" %}}
Applicable only to Porch with DB cache configured.
{{% /alert %}}

#### Data backed up:
- DB cache database contents

#### Data stores lost:
- DB cache database:
    - SQL script used to drop all Porch tables
    - PostgreSQL server deleted and recreated empty of data

#### Restoration steps:
1. Restore backed-up database contents to PostgreSQL server
2. Perform GitOps reconciliation, gradually (in batches of 20) re-creating all backed-up Porch Repository objects
    1. For each batch, wait until all Repository objects have condition with type "Ready" and status set "True"

#### Expected data loss

None - complete recovery of state at time DB cache database was backed up.

With a backup of the cache database, Porch recovers all data.

### 5. DB cache loss without backup

Kubernetes cluster and Git repositories remain safe; DB cache database is lost without a valid backup.

{{% alert title="N.B." color="primary" %}}
Applicable only to Porch with DB cache configured.
{{% /alert %}}

#### Data backed up:
- None

#### Data stores lost:
- DB cache database:
    - SQL script used to drop all Porch tables
    - PostgreSQL server deleted and recreated empty of data

#### Restoration steps:
1. Perform GitOps reconciliation, gradually (in batches of 20) re-creating all backed-up Porch Repository objects
    1. Repository objects will fail to become ready, as crucial data is missing from the wiped cache database
2. Restart the `porch-server` microservice:
    1. ```bash
        kubectl -n porch-system delete pod --selector app=porch-server --force --grace-period 0 
       ```
    2. Porch will immediately begin to re-sync all repositories, resulting in a **decrease in quality of service** until
       all repositories are deemed Ready
3. Wait until all Repository objects have condition with type "Ready" and status set "True"
4. List package revisions periodically, monitoring results until state stabilises

#### Expected data loss

All "work in progress" on package revisions lost:
- package revisions in "Draft" or "Proposed" lifecycle stages - complete loss
- package revisions in "DeletionProposed" lifecycle stage are reverted to "Published" lifecycle stage
    - a similar effect to the use of `porchctl rpkg reject`

{{% alert title="Warning" color="warning" %}}
Having a valid backup of the DB cache database avoids this situation. Porch highly recommends making regular backups.
{{% /alert %}}
