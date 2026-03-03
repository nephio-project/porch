---
title: "Functionality"
type: docs
weight: 4
description: |
  Overview of controller functionality and detailed documentation pages.
---

The Controllers provide core functional areas that work together to automate package variant creation and management:

## Functional Areas

### PackageVariant Reconciliation

Implements continuous synchronization between upstream and downstream packages through:
- **State Machine**: Determines actions based on downstream state (no downstream, upstream changed, mutations changed, up-to-date)
- **Upstream Tracking**: Uses UpstreamLock comparison to detect upstream version changes
- **Draft Management**: Creates clone, upgrade, or edit drafts based on detected changes
- **Adoption and Deletion**: Manages ownership and cleanup of downstream packages through policies

For detailed architecture and process flows, see [PackageVariant Reconciliation]({{% relref "/docs/5_architecture_and_components/controllers/functionality/packagevariant-reconciliation.md" %}}).

### PackageVariantSet Reconciliation

Implements declarative fan-out pattern for bulk package instantiation through:
- **Target Unrolling**: Converts target selectors (repository list, repository selector, object selector) into concrete downstream contexts
- **Template Evaluation**: Uses CEL expressions to generate customized PackageVariant specs per target
- **Set-Based Reconciliation**: Ensures desired PackageVariants match actual through create/update/delete operations
- **Dynamic Discovery**: Automatically adjusts as target repositories or objects change

For detailed architecture and process flows, see [PackageVariantSet Reconciliation]({{% relref "/docs/5_architecture_and_components/controllers/functionality/packagevariantset-reconciliation.md" %}}).

### Mutation Application

Applies systematic transformations to downstream packages through:
- **Package Context Injection**: Adds or modifies data in package-context.yaml ConfigMap
- **KRM Function Injection**: Prepends KRM functions to Kptfile pipeline with unique naming
- **Config Injection**: Injects data from in-cluster Kubernetes resources into package resources
- **Change Detection**: Compares original and modified resources to avoid unnecessary updates

For detailed architecture and process flows, see [Mutation Application]({{% relref "/docs/5_architecture_and_components/controllers/functionality/mutation-application.md" %}}).

## How They Work Together

```
┌─────────────────────────────────────────────────────────┐
│                    Controllers                          │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │ PackageVariantSet│      │  PackageVariant  │         │
│  │                  │ ───> │                  │         │
│  │  • Target Unroll │      │  • State Machine │         │
│  │  • Template Eval │      │  • Upstream Track│         │
│  │  • Set Reconcile │      │  • Draft Mgmt    │         │
│  └──────────────────┘      └──────────────────┘         │
│                                     │                   │
│                                     ↓                   │
│                           ┌──────────────────┐          │
│                           │    Mutation      │          │
│                           │   Application    │          │
│                           │                  │          │
│                           │  • Pkg Context   │          │
│                           │  • KRM Functions │          │
│                           │  • Config Inject │          │
│                           └──────────────────┘          │
└─────────────────────────────────────────────────────────┘
```

**Integration flow:**
1. **PackageVariantSet** unrolls targets and creates/manages PackageVariant CRs
2. **PackageVariant** reconciles each CR, tracking upstream and creating drafts as needed
3. **Mutation Application** transforms draft packages by injecting context, functions, and configuration
4. **Result**: Customized downstream packages synchronized with upstream and tailored per target

Each functional area is documented in detail on its own page with architecture diagrams, process flows, and implementation specifics.
