---
title: "Package Mutation Pipeline Order"
type: docs
weight: 1
description:
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

## Why

This document explains the two different traversal strategies for package hydration in Porch's rendering pipeline: **Depth-First Search (DFS)** and **Breadth-First Search (BFS)**. These strategies determine the order in which kpt packages and their subpackages are processed during mutation and validation.

## Background

Porch uses a hydration process to transform kpt packages by running functions (mutators and validators) defined in Kptfiles. The order in which packages are processed can significantly impact the final output, especially when parent and child packages have interdependent transformations.

## Traversal Strategies

### Terminology

For a package structure like:
```
ROOT/
├── A/
├── B/
└───└─ C/
```
Let's define the key terms used throughout this documentation.

- Root: The top-level package that initiates the hydration process (e.g., ROOT)
- Child: A direct subpackage of another package (e.g., A, B, C are children of ROOT)
- Sibling: Packages that share the same parent (e.g., A and B are siblings)
- Descendant: Any package in the subtree below a given package, including children, grandchildren, etc.

### Default: Depth-First Search (DFS)

**Function**: `hydrate()`

The default hydration strategy processes packages using depth-first traversal in post-order. This means:
- All subpackages are processed **before** their parent packages
- Recursion naturally handles the traversal order
- Resources flow **bottom-up** through the package hierarchy

#### Processing Order
For the package structure shown before:

The execution order is: **C → A → B → ROOT** (alphabetical order within each level, then parent)

#### Implementation Details
- Uses recursive function calls to traverse the package tree
- Each package's pipeline receives:
  - All resources from its processed subpackages
  - Its own local resources
- Subpackage resources are appended to the parent's input before running the parent's pipeline

### Optional: Breadth-First Search (BFS)

**Function**: `hydrateBfsOrder()`

The BFS strategy processes packages in a top-down approach using explicit queues:
- Parent packages are processed **before** their subpackages
- Uses two-phase execution: discovery and pipeline execution
- Resources flow **top-down** through the package hierarchy

#### Processing Order
For the package structure shown before:

The execution order is: **ROOT → A → B → C** (parent first, then children in alphabetical order)

#### Implementation Details
- **Phase 1**: Breadth-first discovery of all packages and loading of local resources
- **Phase 2**: Sequential pipeline execution with scoped visibility
- Each package's pipeline receives:
  - Its own local resources
  - All resources from its descendants (children, grandchildren, etc.)

## Enabling BFS Mode

To use the BFS traversal strategy, add the following annotation to your root package's Kptfile:

```yaml
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: root-package
  annotations:
    kpt.dev/bfs-rendering: "true"
```

**Important**:
- The annotation must be set to exactly `"true"` (case-sensitive)
- Any other value or missing annotation defaults to DFS mode
- The annotation is only checked on the root package's Kptfile

## Key Differences and Use Cases

| Aspect | DFS (Default) | BFS (Optional) |
|--------|---------------|----------------|
| **Traversal Pattern** | Depth-first, post-order | Breadth-first, level-order |
| **Processing Direction** | Bottom-up (children → parent) | Top-down (parent → children) |
| **Resource Flow** | Subpackages feed into parent | Parent influences subpackages |
| **Queue Implementation** | Implicit (recursion) | Explicit (two queues) |
| **Resource Visibility** | Parent sees all subpackage outputs | Package sees self + all descendants |
| **Cycle Detection** | During traversal | During discovery phase |

### When to Use DFS (Default)
- **Aggregation scenarios**: When parent packages need to collect and process outputs from subpackages
- **Bottom-up customization**: When specializations at lower levels should inform higher-level decisions
- **Traditional kpt workflows**: Most existing kpt packages expect this behavior

### When to Use BFS
- **Template expansion**: When a root package serves as a template that configures subpackages
- **Top-down configuration**: When parent-level settings should cascade to children
- **Consistent base customization**: When you want to apply base transformations before specialized ones

## Practical Examples

### DFS Scenario: Configuration Aggregation
```
ROOT/              # Collects all service configs
├── service-a/     # Defines service-a configuration
├── service-b/     # Defines service-b configuration
└── monitoring/    # Defines monitoring for both services
```

With DFS, the ROOT package can aggregate configurations from all services and create a unified monitoring dashboard.

### BFS Scenario: Template-Based Deployment
```
ROOT/              # Contains base templates and global config
├── staging/       # Staging-specific overrides
├── production/    # Production-specific overrides
└── development/   # Development-specific overrides
```

With BFS, the ROOT package can set up base templates and global configurations that are then specialized by each environment-specific subpackage.

## Implementation Architecture

### Core Components

1. **hydrationContext**: Maintains global state during hydration including:
   - Package registry with hydration states (Dry, Hydrating, Wet)
   - Input/output file tracking for pruning
   - Function execution counters and results

2. **pkgNode**: Represents individual packages in the hydration graph:
   - Package metadata and file system access
   - Hydration state tracking
   - Accumulated resources after processing

3. **Pipeline Execution**: Both strategies share the same pipeline execution logic:
   - Mutator functions transform resources
   - Validator functions verify resources without modification
   - Function selection and exclusion based on selectors

### Resource Scoping

**DFS Resource Scope**:
- Input = subpackage outputs + own local resources
- Processes transitively accumulated resources

**BFS Resource Scope**:
- Input = own local resources + all descendant local resources
- Each package sees its complete subtree

## Error Handling and Validation

Both strategies include:
- **Cycle Detection**: Prevents infinite loops in package dependencies
- **State Validation**: Ensures packages are processed in correct order
- **Resource Validation**: Verifies KRM resource format compliance
- **Pipeline Validation**: Checks function configurations before execution

## Related Resources

- [Tree Traversal Algorithms](https://en.wikipedia.org/wiki/Tree_traversal)

## See Also

- **Source Code**: https://github.com/nephio-project/porch
- **File**: `internal/kpt/util/render/executor.go`
- **Key Functions**: `hydrate()` and `hydrateBfsOrder()`
- **Configuration**: `kpt.dev/bfs-rendering` annotation in `pkg/kpt/api/kptfile/v1/types.go`