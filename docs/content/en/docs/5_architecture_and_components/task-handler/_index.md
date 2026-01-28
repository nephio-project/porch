---
title: "Task Handler"
type: docs
weight: 1
description: |
  Task execution engine that implements package transformation operations (init, clone, edit, upgrade, render) and builtin functions.
---

## What is the Task Handler?

The **Task Handler** (also known as the **CaD Library** in some legacy documentation) is the component that implements the actual task execution logic for package operations. While the Engine orchestrates WHEN and HOW to invoke tasks, the Task Handler implements WHAT each task does to transform package content.

The Task Handler is responsible for:

- **Task Implementations**: Executing init, clone, edit, upgrade, and render tasks
- **Package Transformations**: Modifying package resources based on task logic
- **Builtin Functions**: Applying built-in KRM functions (package-context generator)
- **Function Pipeline Execution**: Running the configured function pipeline during render
- **Resource Manipulation**: Reading, modifying, and writing package resources

## Role in the Architecture

The Task Handler sits between the Engine and the Function Runtime:

```
   Engine
       ↓
  Task Handler
       ↓
    ┌──┴────┬──────────┬──────────┐
    ↓       ↓          ↓          ↓
Package  Function    Task      Mutation
Transform Execute  Orchestrate Pattern
    ↓       ↓          ↓          ↓
    └───┬───┴──────────┴──────────┘
        ↓
  Function Runtime
        ↓
  Modified Draft
```

**Key architectural responsibilities:**

1. **Package Transformation Tasks**: Implements init, clone, edit, upgrade, and resource replacement operations that transform package content

2. **Function Execution**: 
   - Executes render pipeline defined in Kptfile
   - Applies builtin functions automatically (package-context generator)
   - Integrates with function runtime (builtin, gRPC, or multi-runtime)
   - Captures and returns function execution results

3. **Task Orchestration**:
   - Coordinates task execution through genericTaskHandler
   - Maps task types to mutation implementations
   - Chains mutations (task → builtin functions → render)
   - Integrates with Engine through ApplyTask, DoPRMutations, DoPRResourceMutations

4. **Draft Manipulation**: Modifies the mutable draft workspace provided by the Engine

5. **Error Handling**: Returns errors to Engine when task execution or function execution fails

6. **Resource Transformation**: Performs the actual package content transformations (copying resources, merging changes, generating files, healing comments)

**Integration with Engine:**

The Task Handler is invoked by the Engine through three main integration points:
- **ApplyTask**: During package revision creation (executes task + render)
- **DoPRMutations**: During package revision updates (patches Kptfile + render)
- **DoPRResourceMutations**: During package resource updates (replaces resources + render)

For details on how the Engine coordinates with the Task Handler, see [Engine - Task Coordination](../core-engine/functionality/task-coordination).

For details on Task Handler functionality, see [Task Handler Functionality](functionality).
