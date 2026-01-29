---
title: "Upstream & Downstream"
type: docs
weight: 6
description: |
  Understanding upstream and downstream packages: the source/derivation relationship in Porch.
---

## What are Upstream and Downstream Packages?

**Upstream** and **downstream** describe source-to-derivation relationship between packages and package revisions in Porch.
This relationship is fundamental to package reuse, customization, upgrade control, and lifecycle management.

- **Upstream package**: The source or template package that provides the base content
- **Downstream package**: A derived package created by cloning or copying from an upstream package

This relationship enables:
- Package reuse across teams and environments
- Centralized maintenance of template packages
- Tracking of package lineage and updates
- Automated propagation of upstream changes to downstream packages

## How the Relationship Works

When you create a downstream package revision from an upstream package revision, Porch establishes and tracks this relationship.
The downstream package revision maintains a record of its upstream source, including the exact version (Git commit) it was
derived from.

This tracking enables:
- **Lineage visibility**: Know where a package revision came from
- **Update awareness**: Detect when upstream has new versions
- **Merge intelligence**: Preserve downstream customizations when upgrading to new upstream versions
- **Audit trail**: Understand the evolution of a package over time

## Common Patterns

### Blueprint → Deployment

The most common pattern in Porch:

**Blueprint repositories** (upstream):
- Contain reusable template packages
- Maintained by platform teams
- Represent best practices and organizational standards
- Examples: application templates, infrastructure patterns, policy bundles

**Deployment repositories** (downstream):
- Contain environment-specific packages
- Created by cloning from blueprints
- Customized for specific clusters, regions, or teams
- Examples: prod-app, staging-database, dev-cluster-config

### Multi-Level Derivation

A package revision can be both upstream and downstream simultaneously:

- **Base template** (blueprint repo) → upstream for regional template
- **Regional template** (regional repo) → upstream for environment-specific packages
- **Environment package** (deployment repo) → final deployed configuration

Each level adds its own customizations while maintaining the chain of upstream relationships.

## Upstream Changes and Downstream Updates

When an upstream package publishes a new version:

**Manual workflow**: Users can choose when to upgrade their downstream packages to incorporate upstream changes. Porch
merges upstream updates with downstream customizations.

**Automated workflow**: PackageVariant controllers can automatically detect upstream changes and create new downstream
package revisions, applying customizations consistently.

**Customization preservation**: Downstream-specific modifications (labels, resources, function configurations) are preserved
during upstream updates through intelligent merge strategies.

## Upstream Sources

Upstream packages can originate from:

- **Registered repositories**: Packages in repositories that Porch manages
- **External Git repositories**: Direct references to Git repos not registered in Porch
- **External OCI registries**: Direct references to OCI artifacts

This flexibility allows organizations to consume packages from various sources while maintaining consistent downstream
management through Porch.

## Key Points

- Upstream == source package; Downstream == derived package
- Porch tracks the upstream → downstream relationship automatically
- Common pattern: blueprints (upstream) → deployments (downstream)
- Downstream packages can be updated when upstream changes
- Customizations in downstream packages are preserved during updates
- A package can be both upstream (to others) and downstream (from another)
- PackageVariant automates the upstream → downstream lifecycle
