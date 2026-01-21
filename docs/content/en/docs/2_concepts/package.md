---
title: "Packages"
type: docs
weight: 1
description: |
  Understanding packages in Porch: the relationship between kpt packages and Porch's PackageRevision API.
---

{{% alert title="Important" color="warning" %}}

There is no such thing as a "Porch package". When people say they are creating, editing, or upgrading a package in Porch, they are manipulating a **PackageRevision** resource and its paired **PackageRevisionResources** resource. These are Porch's Kubernetes API interpretation of kpt package contents stored in Git or OCI repositories.

{{% /alert %}}

## What is a Package in Porch?

In Porch, a **package** is represented through Kubernetes API resources, not as files you directly manipulate. When you work with Porch, you interact with:

- **PackageRevision**: A Kubernetes resource containing the kpt package metadata (name, repository, revision number, lifecycle stage, workspace, tasks)
- **PackageRevisionResources**: A companion resource containing the actual kpt package content (YAML files as key-value pairs)

These resources surface [kpt packages](https://kpt.dev/book/03-packages/) stored in Git or OCI repositories as Kubernetes-native objects.

## The Relationship: kpt Packages and Porch

**kpt packages** are the underlying storage format:
- A directory containing a `Kptfile` and one or more KRM (Kubernetes Resource Model) YAML files
- Stored in Git repositories (branches, tags, commits) or OCI registries
- Can be manipulated directly with the [kpt CLI](https://kpt.dev/)
- Follow the [kpt package specification](https://kpt.dev/book/03-packages/)

**Porch packages** are the API layer:
- PackageRevision resources that expose kpt packages through the Kubernetes API
- Enable declarative package lifecycle management (Draft → Proposed → Published)
- Support operations like init, clone, edit, upgrade through Kubernetes-style interactions
- Automatically sync changes between the API resources and the underlying Git/OCI storage

## Example: Creating and Viewing a Package

When you create a "package" with `porchctl`:

```bash
porchctl rpkg init my-package --repository=blueprints --workspace=v1
```

Porch creates:
1. A **PackageRevision** resource with metadata (name, repository, workspace, lifecycle=Draft)
2. A **PackageRevisionResources** resource with initial content (Kptfile + any starter resources)
3. A Git branch in the `blueprints` repository containing the kpt package files

When you get "packages" in Porch:

```bash
porchctl rpkg get
```

You see **PackageRevision** resources, not raw Git files. Each row represents a PackageRevision with its metadata.

When you pull a "package" contents in Porch:

```bash
porchctl rpkg pull my-package-v1 ./local-dir
```

Porch fetches the **PackageRevisionResources** content and writes it as kpt package files locally.

## Key Distinction

- **Storage layer**: kpt packages in Git/OCI (Kptfile + YAML files)
- **API layer**: PackageRevision + PackageRevisionResources in Kubernetes
- **User interaction**: You work with PackageRevision resources through `porchctl` or `kubectl`
- **Compatibility**: The underlying kpt packages remain standard and can be used with kpt CLI

This separation allows Porch to add orchestration capabilities (lifecycle management, approval workflows, automation) on top of standard kpt packages without vendor lock-in.
