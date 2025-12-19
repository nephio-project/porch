---
title: "Working with Porch Repositories"
type: docs
weight: 2
description: A group of guides outlining how to interact with Porch repositories
---

## Prerequisites

- Porch deployed on a Kubernetes cluster [Setup Porch Guide]({{% relref "/docs/3_getting_started/installing-porch.md" %}}).
- **Porchctl** CLI tool installed [Setup Porchctl Guide]({{% relref "/docs/3_getting_started/installing-porchctl.md" %}}).
- **Kubectl** configured to access your cluster.
- A Git repository to register with Porch. If you need to create one, see [GitHub's Repository Guide](https://docs.github.com/en/repositories/creating-and-managing-repositories/quickstart-for-repositories).

---

## Understanding Repositories

Before Porch can manage packages, you must register repositories where those packages are stored. Repositories tell Porch:

- Where to find package blueprints
- Where to store deployment packages
- How to authenticate with the repository

Porch primarily supports **Git repositories** from providers like GitHub, GitLab, Gitea, Bitbucket, and other Git-compatible services.

**Repository Types by Purpose:**

- **Blueprint Repositories**: Contain upstream package templates and blueprints that can be cloned and customized. These are typically read-only sources of reusable configurations.
- **Deployment Repositories**: Store deployment-ready packages that are actively managed and deployed to clusters. Mark repositories as deployment repositories using the `--deployment` flag during registration.

---

## Repository Types

Porch primarily supports Git repositories for storing and managing packages. Git is the recommended and production-ready storage backend.

### Git Repositories

Git repositories are the primary and recommended type for use with Porch.

**Requirements:**

- Git repository with an initial commit (to establish main branch)
- For private repos: Personal Access Token or Basic Auth credentials

**Supported Git hosting services:**

- GitHub
- GitLab
- Gitea
- Bitbucket
- Any Git-compatible service

---

### OCI Repositories (Experimental)

{{% alert title="Warning" color="warning" %}}
OCI repository support is **experimental** and not actively maintained. Use at your own risk. This feature may have limitations, bugs, or breaking changes. For production deployments, use Git repositories.
{{% /alert %}}

Porch has experimental support for OCI (Open Container Initiative) repositories that store packages as container images. This feature is not recommended for production use.

---

## Troubleshooting

Common issues when working with repositories and their solutions:

**Repository shows READY: False?**

- Check repository URL is accessible
- Verify authentication credentials are correct
- Inspect repository conditions: `porchctl repo get <name> -n <namespace> -o yaml`
- Check Porch server logs for detailed errors

**Packages not appearing after registration?**

- Ensure repository has been synchronized (check SYNC SCHEDULE or trigger manual sync)
- Verify packages have valid Kptfile in repository
- Check repository directory configuration matches package location
- If re-registering a previously unregistered repository, packages in Git will reappear after sync

**Authentication failures?**

- For GitHub: Ensure Personal Access Token has `repo` scope
- For private repos: Verify credentials are correctly configured
- Check secret exists: `kubectl get secret <secret-name> -n <namespace>`

**Need to change repository configuration?**

- Repository settings (branch, directory, credentials) cannot be updated via porchctl
- Use `kubectl edit repository <name> -n <namespace>` to modify the Repository resource
- Alternatively, unregister and re-register the repository with new settings

**Sync not working?**

- Verify cron expression syntax is correct
- Check minimum 1-minute delay for manual syncs
- Inspect repository status for sync errors

---

## Key Concepts

Important terms and concepts for working with Porch repositories:

- **Repository**: A Git repository registered with Porch for package management. Repositories are namespace-scoped Kubernetes resources.
- **Blueprint Repository**: Contains upstream package templates that can be cloned and customized. Typically used as read-only sources.
- **Deployment Repository**: Repository marked with `--deployment` flag containing deployment-ready packages that are actively managed.
- **Sync Schedule**: Cron expression defining periodic repository synchronization (e.g., `*/10 * * * *` for every 10 minutes).
- **Content Type**: Defines what the repository stores. `Package` is the standard type for KRM configuration packages. Other types like `Function` exist for storing KRM functions.
- **Branch**: Git branch Porch monitors for packages (defaults to `main`). Each repository tracks a single branch.
- **Directory**: Subdirectory within repository where packages are located. Use `/` for root or specify a path like `/blueprints`.
- **Namespace Scope**: Repositories exist within a Kubernetes namespace. Repository names must be unique per namespace, and packages inherit the repository's namespace. The same Git repository can be registered in multiple namespaces with different names, creating isolated package views per namespace.

---
