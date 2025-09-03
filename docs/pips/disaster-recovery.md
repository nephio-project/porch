# PIP: Disaster recovery

## Summary

Porch should have descriptions on how to recover from scenarios when one or more of it's datastores are lost, or there is only a point-in-time backup existing of them.

## Motivation

Porch has a relatively complex data storage model, where Porch is acting as a mediator between these various data stores, namely the K8S cluster's control plane, git backends, and the contents of the cache. In the event of a data store failure, there should be descriptions on how to recover and what data loss is expected to happen. 

## Goals

- Provide descriptions on how to recover from scenarios when one or more of it's datastores or runtime components are lost, or there is only a point-in-time backup existing of them. It should cater for the following scenarios:
  - K8s cluster's control plane data is lost.
  - K8s cluster's control plane data is restored to a previous point in time.
  - One or more of the git repositories is lost.
  - One or more of the git repositories is restored to a previous point in time.
  - Porch database cache is lost.
  - Porch database cache is restored to a previous point in time.
- Create a test suite to verify that the recovery process works as expected.
- Provide small amounts of implementation so that the recovery process works in an acceptable way.

### Non-goals

- Change the architecture of Porch to provide a more robust recovery mechanism.

## Proposal

### General rules

The testing and the implementation should strive to adhere to the following rules:

1. The source of truth for Repositories should be the k8s control plane, and the cache should align with it.
2. The source of truth for PackageRevisions should be the git repository that's backing a Repository and the cache should align with it. A caveat to this is that for draft and proposed package are not stored in the git repository in case the cache backend is the database.
3. The source of truth for k8s related metadata for PackageRevisions (labels, annotations) should be the cache. The existence of a metadata information should not be enough for the cache to produce an empty PackageRevision.
4. The source of truth for PackageVariants should be the k8s control plane.
5. The system should allow for gradual restoration of Repoistories and PackageVariants. The rationale for this is that gitops systems do not provide one single atomic syncronisation of all of the cluster state, but it's syncronised gradually.

### Scenario #1: Total disaster

1. K8s cluster, with all nodes is lost. Git repositories are lost. Database cache is lost.
2. Git repositories were persisted via a point-in-time backup
3. Porch database cache were persisted via a point-in-time backup (if exists)
4. Git repositories are restored to a previous point in time
5. Porch database cache is restored to a previous point in time
6. Gitops reconciliation is triggered to restore Repositories and PackageVariants

### Scenario #2: K8s cluster's disaster

1. K8s cluster, with all nodes is lost. Git repositories are persisted. Database cache is peristed.
2. Porch database cache were persisted via a point-in-time backup (if exists)
3. Porch database cache is restored to a previous point in time
4. Gitops reconciliation is triggered to restore Repositories and PackageVariants

### Scenario #3: Porch server restart

1. K8s cluster and Git repositories and Database cache is persisted.
2. All porch components get an ungraceful restart.

### Scenario #4: Porch database cache is lost

1. K8s cluster and Git repositories are persisted. Database cache is lost without a valid backup.
2. Porch database cache is restored to a previous point in time
3. Gitops reconciliation is triggered to restore Repositories and PackageVariants

### Scenario $5: Porch database cache is restored to a previous point in time

1. K8s cluster and Git repositories are persisted. Database cache is lost.
2. Porch database cache is restored to a previous point in time.
3. Gitops reconciliation is triggered to restore Repositories and PackageVariants

## Detailed design

TBD
