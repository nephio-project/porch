---
title: "PackageVariant Controllers"
type: docs
weight: 1
description: |
  PackageVariant and PackageVariantSet controllers for package automation.
---

## Overview

PackageVariant and PackageVariantSet controllers automate creation and management of package revisions through two levels of templating:

- **PackageVariant**: Creates multiple downstream package revisions from a single upstream package
- **PackageVariantSet**: Creates multiple PackageVariants from a single template

Both controllers run in the same pod as Repository Controller and are configured via the `--reconcilers` flag.

## What Do These Controllers Do?

The PackageVariant controllers provide declarative, template-based automation for creating multiple downstream variants of packages:

- **Declarative Package Management**: Define desired package variants rather than manually creating each PackageRevision
- **Multi-Target Distribution**: Create variants across multiple repositories or for multiple targets from a single declaration  
- **Change Detection and Reconciliation**: Automatically detect upstream package changes and create new downstream revisions (upgrades or edits)
- **Template-Based Generation**: Use CEL expressions to dynamically generate package metadata, labels, annotations, and configuration based on target context
- **Ownership Management**: Track which PackageRevisions are owned by which PackageVariant with configurable deletion policies (delete or orphan)
- **Resource Injection**: Dynamically inject configuration from in-cluster resources into generated packages

## Controllers

- [**PackageVariant Controller**](packagevariant.md) - Automates downstream package creation with customizations
- [**PackageVariantSet Controller**](packagevariantset.md) - Automates PackageVariant creation from templates

## Architecture

- [**Design Patterns**](design.md) - Controller design patterns and architecture decisions
- [**Interactions with Porch APIs**](interactions.md) - How controllers interact with Porch API

## Functionality

- [**PackageVariant Reconciliation**](functionality/packagevariant-reconciliation.md) - PackageVariant reconciliation logic
- [**PackageVariantSet Reconciliation**](functionality/packagevariantset-reconciliation.md) - PackageVariantSet reconciliation logic
- [**Mutation Application**](functionality/mutation-application.md) - Package mutation operations
