---
title: "Task Handler Functionality"
type: docs
weight: 3
description: |
  Overview of task handler functionality and detailed documentation pages.
---

The Task Handler provides three core functional areas that work together to transform package content:

## Functional Areas

### Package Transformation Tasks

Implements all operations that transform package content through:
- **Init Task**: Creates new packages from scratch with Kptfile and README
- **Clone Task**: Copies packages from upstream sources (registered repos, Git, OCI)
- **Edit Task**: Creates new revisions from existing published packages
- **Upgrade Task**: Performs three-way merge to upgrade packages to new upstream versions
- **Resource Mutations**: Replaces package resources while preserving comments

For detailed architecture and process flows, see [Package Transformation Tasks](package-transformations).

### Function Execution

Executes KRM functions to validate and transform package resources through:
- **Render Pipeline**: Executes function pipeline defined in Kptfile
- **Builtin Functions**: Applies built-in functions (package-context generator)
- **Function Runtime Integration**: Delegates to builtin, gRPC, or multi-runtime
- **Result Handling**: Captures function execution results and errors

For detailed architecture and process flows, see [Function Execution](function-execution).

### Task Orchestration

Coordinates task execution and integrates with Engine through:
- **ApplyTask**: Executes task + render during package revision creation
- **DoPRMutations**: Patches Kptfile + render during package revision updates
- **DoPRResourceMutations**: Replaces resources + render during resource updates
- **Mutation Pattern**: Common interface for all package transformations
- **Task Mapping**: Maps task types to mutation implementations

For detailed architecture and process flows, see [Task Orchestration](task-orchestration).

## How They Work Together

```
┌─────────────────────────────────────────────────────────┐
│                    Task Handler                         │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │   Package        │      │    Function      │         │
│  │ Transformations  │ ───> │   Execution      │         │
│  │                  │      │                  │         │
│  │  • Init          │      │  • Render        │         │
│  │  • Clone         │      │  • Builtin Funcs │         │
│  │  • Edit          │      │  • Runtime       │         │
│  │  • Upgrade       │      │                  │         │
│  │  • Replace Res   │      │                  │         │
│  └──────────────────┘      └──────────────────┘         │
│           │                         │                   │
│           └────────┬────────────────┘                   │
│                    ↓                                    │
│          ┌──────────────────┐                           │
│          │      Task        │                           │
│          │  Orchestration   │                           │
│          │                  │                           │
│          │  • ApplyTask     │                           │
│          │  • DoPRMutations │                           │
│          │  • DoPRResource  │                           │
│          │    Mutations     │                           │
│          │  • Mutation      │                           │
│          │    Pattern       │                           │
│          └──────────────────┘                           │
└─────────────────────────────────────────────────────────┘
```

**Integration flow:**
1. **Task Orchestration** receives request from Engine
2. **Package Transformations** execute task-specific logic (init, clone, edit, upgrade, replace)
3. **Function Execution** applies builtin functions and runs render pipeline
4. **Task Orchestration** returns modified draft to Engine

Each functional area is documented in detail on its own page with architecture diagrams, process flows, and implementation specifics.
