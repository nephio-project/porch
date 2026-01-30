---
title: "PackageVariantSet Controller"
type: docs
weight: 4
description: |
  Controller for managing PackageVariantSet resources.
---

## Overview

The **PackageVariantSet Controller** manages bulk creation of PackageVariant CRs based on target selectors. It implements a declarative fan-out pattern where a single upstream package is automatically instantiated across multiple downstream targets using template-based generation with CEL expressions.

**Key characteristics:**

- **Fan-out pattern**: One upstream package → multiple PackageVariant CRs → multiple downstream packages
- **Target selection**: Three mechanisms for identifying targets (repository list, repository selector, object selector)
- **Template-based generation**: Single template generates multiple PackageVariants with per-target customization
- **CEL expression evaluation**: Dynamic configuration using Common Expression Language
- **Set-based reconciliation**: Desired state computed and reconciled against existing PackageVariants
- **Owner reference management**: PackageVariantSet owns all generated PackageVariants

## Reconciliation Overview

The PackageVariantSet controller implements a declarative fan-out pattern where a single upstream package is automatically instantiated across multiple downstream targets using template-based generation with CEL expressions.

**Key reconciliation characteristics:**
- **Set-based**: Computes desired PackageVariants and reconciles with existing
- **Template-driven**: Single template generates multiple PackageVariants
- **CEL-powered**: Dynamic configuration using Common Expression Language
- **Idempotent**: Safe to call multiple times

**For detailed reconciliation flows, validation logic, error handling, and set-based reconciliation process, see [PackageVariantSet Reconciliation](functionality/packagevariantset-reconciliation).**

## Target Selection Overview

The controller supports three mechanisms for identifying downstream targets:

**1. Repository List**: Explicit list of repositories and package names
**2. Repository Selector**: Label-based selection of Repository CRs  
**3. Object Selector**: Label-based selection of arbitrary Kubernetes objects

**For detailed target selection mechanisms, unrolling process, and pvContext structure, see [PackageVariantSet Reconciliation - Target Unrolling](functionality/packagevariantset-reconciliation#target-unrolling).**

## Template Evaluation Overview

The controller uses CEL (Common Expression Language) expressions for dynamic PackageVariant configuration.

**CEL environment variables:**
- **repoDefault**, **packageDefault**: Default names from target unrolling
- **upstream**: Upstream PackageRevision object
- **repository**: Downstream Repository object  
- **target**: Matched target object (ObjectSelector only)

**Security model:**
- Only name, namespace, labels, annotations accessible
- Prevents leaking sensitive cluster data
- Namespace isolation enforced

**For detailed CEL expression environment, template rendering process, evaluation order, and expression examples, see [PackageVariantSet Reconciliation - Template Evaluation](functionality/packagevariantset-reconciliation#template-evaluation).**

## PackageVariant Generation Overview

The controller uses set-based reconciliation to manage PackageVariants:

**Set reconciliation:**
- Computes desired set of PackageVariants from targets
- Compares with existing PackageVariants
- Creates, updates, or deletes to match desired state

**Identifier and naming:**
- Identifier: `{pvsName}-{downstreamRepo}-{downstreamPackage}`
- Names truncated with hash if > 63 characters
- Stable across reconciliations

**Owner references:**
- PackageVariantSet owns all generated PackageVariants
- Enables garbage collection
- Cascading deletion

**For detailed set-based reconciliation logic, identifier generation, PackageVariant creation structure, and update vs create logic, see [PackageVariantSet Reconciliation - PackageVariant Generation](functionality/packagevariantset-reconciliation#packagevariant-generation).**

## Status Conditions

**Condition types:**

1. **Stalled** - Whether PackageVariantSet is making progress:
   - Status=True: Validation error, upstream not found, or target unroll error
   - Status=False: All validation passed, upstream found, targets unrolled
   - Reasons: "ValidationError", "UpstreamNotFound", "NoMatchingTargets", "UnexpectedError", "Valid"

2. **Ready** - Whether reconciliation succeeded:
   - Status=True: Successfully ensured all PackageVariants
   - Status=False: Error during reconciliation
   - Reasons: "Reconciled", "UnexpectedError"

## Watch Configuration

The controller watches multiple resource types to trigger reconciliation:

**Primary watch:**
- **PackageVariantSet**: Main resource being reconciled

**Secondary watches:**
- **PackageVariant**: Changes trigger reconciliation of owning PackageVariantSet
- **PackageRevision**: Changes trigger reconciliation of all PackageVariantSets in namespace

**Watch rationale:**
- **PackageVariant watch**: Detect external changes to generated PackageVariants
- **PackageRevision watch**: Detect upstream package changes (new versions)
- **Namespace-scoped**: All watches limited to same namespace

**Reconciliation triggers:**
- PackageVariantSet created/updated/deleted
- PackageVariant created/updated/deleted (any in namespace)
- PackageRevision created/updated/deleted (any in namespace)

**Implications:**
- Broad watches ensure consistency
- May trigger unnecessary reconciliations
- Trade-off: simplicity over efficiency
- Acceptable for typical scale (hundreds of resources)

## Design Patterns

The PackageVariantSet controller implements three key design patterns:

**Fan-Out Pattern**: One upstream package → multiple targets → multiple PackageVariants → multiple downstream packages

**Template Pattern**: Static values provide defaults, dynamic CEL expressions override per-target

**Set-Based Reconciliation**: Compute desired state, compare with existing, apply minimal changes

**For detailed pattern descriptions and benefits, see [PackageVariantSet Reconciliation - Design Patterns](functionality/packagevariantset-reconciliation#design-patterns).**

## Error Handling

**Validation errors:**
- Set Stalled=True condition
- Include all validation errors in message
- Do not requeue (requires spec change)
- Clear error messages for debugging

**Upstream not found:**
- Set Stalled=True condition
- Do not requeue (watch will trigger)
- Wait for upstream to appear
- Automatic recovery when available

**Target unroll errors:**
- Set Stalled=True condition
- Include error details in message
- May indicate missing CRD
- Requires investigation

**CEL evaluation errors:**
- Returned during template rendering
- Include expression and error details
- Set Stalled=True condition
- Requires expression fix

**PackageVariant operation errors:**
- Returned during ensure operation
- Set Stalled=True condition
- May be transient (retry on next reconciliation)
- Logged for debugging
