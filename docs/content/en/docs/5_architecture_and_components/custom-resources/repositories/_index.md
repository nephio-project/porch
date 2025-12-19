---
title: "Repositories"
type: docs
weight: 4
---

# Porch Repository Overview

## What is a Repository CR?

The Porch Repository is a Kubernetes [custom resource](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) that represents an external repository containing KPT packages. It serves as Porch's interface to Git repositories and OCI* registries that store package content.

## Purpose and Use Cases

### Primary Functions
- **Package Discovery**: Automatically discovers and catalogs packages from external repositories
- **Lifecycle Management**: Manages the complete lifecycle of configuration packages
- **Synchronization**: Keeps Porch's internal cache synchronized with external repository changes
- **Access Control**: Provides authentication and authorization for repository access

### Use Cases
- **Blueprint Repositories**: Store reusable configuration templates and blueprints
- **Deployment Repositories**: Store deployment-ready configurations for specific environments
- **Package Catalogs**: Centralized repositories of shareable configuration packages
- **Multi-Environment Management**: Separate repositories for dev, staging, and production configurations

## Repository Types

### Git Repositories
```yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: Repository
metadata:
  name: blueprints
  namespace: default
spec:
  type: git
  git:
    repo: https://github.com/example/blueprints.git
    branch: main
    directory: packages
```

## Key Specifications

### Repository Spec Fields

For detailed Repository CR specification fields, see the [API documentation](https://doc.crds.dev/github.com/nephio-project/porch/config.porch.kpt.dev/Repository/v1alpha1@v1.5.3#spec):

{{< iframe src="https://doc.crds.dev/github.com/nephio-project/porch/config.porch.kpt.dev/Repository/v1alpha1@v1.5.3#spec" sub="https://doc.crds.dev/github.com/nephio-project/porch/config.porch.kpt.dev/Repository/v1alpha1@v1.5.3#spec">}}

### Deployment vs Non-Deployment Repositories
- **Non-Deployment**: Contains blueprint packages for reuse and customization
- **Deployment**: Contains finalized, environment-specific configurations ready for deployment

## Package Structure Requirements

### Git Repository Structure
<pre>
repository-root/
├── package-a/
│   ├── Kptfile
│   └── resources.yaml
├── package-b/
│   ├── Kptfile
│   └── manifests/
└── nested/
    └── package-c/
        ├── Kptfile
        └── config.yaml
</pre>

### Package Identification
- Each package must contain a `Kptfile` at its root
- Packages can be nested within subdirectories
- The `directory` field in git spec defines the search root

## Authentication

### Basic Authentication

For basic authentication configuration and repository registration examples, see the [Basic Auth]({{% relref "/docs/7_cli_api/relevant_old_docs/porchctl-cli-guide.md#basic-auth" %}}) documentation.

### Workload Identity

For workload identity configuration and repository registration examples, see the [Workload Identity]({{% relref "/docs/7_cli_api/relevant_old_docs/porchctl-cli-guide.md#workload-identity" %}}) documentation.

## Repository Lifecycle

### Registration
1. Create Repository CR (manually or via `porchctl repo reg`)
2. Porch validates repository accessibility
3. Initial package discovery and caching
4. Repository marked as `Ready`

### Synchronization
1. Periodic sync based on `spec.sync.schedule` or default frequency
2. One-time sync using `spec.sync.runOnceAt` for immediate synchronization
3. Package discovery and cache updates
4. Status condition updates
5. Package change detection and notification

**Note**: One-time syncs should only be used when discrepancies are found between the external repository and Porch cache. Under normal conditions, rely on periodic syncs for regular synchronization.

### Package Operations
- **Discovery**: Automatic detection of new packages
- **Caching**: Local storage of package metadata and content
- **Revision Tracking**: Tracking of package revisions and changes
- **Access**: API access to package content through Porch

## Status and Conditions

### Repository Status
```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: Ready
    message: 'Repository Ready (next sync scheduled at: 2025-11-05T11:55:38Z)'
    lastTransitionTime: "2024-01-15T10:30:00Z"
```

### Condition Types
- **Ready**: Repository is accessible and synchronized
- **Error**: Authentication, network, or configuration issues
- **Reconciling**: Package reconciliation(sync) in progress

## Integration with Porch APIs

### PackageRevision Resources
- Repository CR enables creation of PackageRevision resources
- Each package in the repository becomes available as PackageRevision
- Package operations (clone, edit, propose, approve) work through PackageRevision API

### Function Evaluation
- Packages may contain KRM functions for validation and transformation
- Functions are executed during package operations (render, clone, etc.)
- Repository CR provides access to package content that may contain function configurations

## Best Practices

### Repository Organization
- Use clear, descriptive repository names
- Organize packages in logical directory structures
- Separate blueprint and deployment repositories
- Use consistent naming conventions

### Synchronization
- Set appropriate sync schedules based on change frequency
- Use one-time sync for immediate updates after changes
- Monitor repository conditions for sync issues

### Security
- Use least-privilege authentication credentials
- Regularly rotate authentication tokens
- Separate repositories by access requirements

### Performance
- Avoid overly large repositories
- Use directory filtering to limit package scope
- Monitor sync performance and adjust schedules accordingly

---

{{% alert title="Note" color="primary" %}}
OCI repository support is experimental and may not have full feature parity with Git repositories.
{{% /alert %}}