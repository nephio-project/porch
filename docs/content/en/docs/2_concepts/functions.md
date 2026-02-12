---
title: "KRM Functions"
type: docs
weight: 7
description: |
  Understanding KRM functions in Porch: how functions transform and validate package resources.
---

## What are Functions in Porch?

**Functions** in Porch are [KRM (Kubernetes Resource Model) functions](https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md) -
containerized programs that transform or validate Kubernetes resource manifests within a package's files. Functions are
declared in a package's Kptfile and executed by Porch when rendering the package.

Functions enable:
- Automated resource generation and modification
- Policy enforcement and validation
- Configuration customization without manual editing
- Repeatable, auditable transformations

For details on how to declare and configure functions in the Kptfile pipeline, see the [kpt functions documentation](https://kpt.dev/book/04-using-functions/).

## Function Execution in Porch

Porch executes functions through a **function runner** component that calls kpt to orchestrate containerized function execution:

- The functions are run in isolated containers (Kubernetes pods managed by the `function-runner` microservice)
- Porch passes the package's resources to kpt, which passes the resources on as a [ResourceList](https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md#resourcelist)
  to each function in the pipeline in turn
- kpt executes the functions sequentially in the order declared in the Kptfile pipeline
- kpt passes the function results back to Porch and Porch stores them in the PackageRevisionResources's `status.renderStatus`
  field
- Execution is triggered automatically following creation/clone of a package revision, update of a package revision, and
  when a package revision is proposed

## When Functions Execute

**Automatic rendering occurs when:**
- A Draft package revision is created (init, clone, edit tasks)
- Package resources are modified by an update through the PackageRevisionResources API resource
- A package revision is proposed

**Manual rendering via render task:**
- Users can add an explicit `render` task to force re-execution of the pipeline
    - N.B.: the `render` task is not persisted in the package revision's task list

**Lifecycle constraints:**
- Functions only execute on **Draft** package revisions
- Proposed package revisions must be rejected back to **Draft** status in order to be eligible for rendering
- Published package revisions are immutable - their rendered state is frozen
- Function results are preserved in the `status.renderStatus` of the PackageRevisionResources API resource across lifecycle
  transitions

## Function Results in Porch

Function execution results are stored in the status of the PackageRevisionResources API resource:

```yaml
apiVersion: porch.kpt.dev/v1alpha1
kind: PackageRevisionResources
...
status:
  renderStatus:
    error: ""
    result:
      exitCode: 0
      items:
        - image: ghcr.io/kptdev/krm-functions-catalog/set-namespace:v0.4.1
          exitCode: 0
        - image: ghcr.io/kptdev/krm-functions-catalog/kubeconform:v0.1.2
          exitCode: 1
          results:
            - message: "Invalid resource configuration"
              severity: error
```

The `renderStatus` field contains:
- Overall exit code (0 for success, non-zero for failure)
- Per-function results including exit codes and validation messages
- Error details if function execution failed

Validation failures prevent Draft package revisions from being created and PackageRevisionResources from being updated.

## Key Points

- Functions are standard KRM functions declared in the Kptfile pipeline (see [kpt functions docs](https://kpt.dev/book/04-using-functions/))
- Porch invokes kpt to execute functions via a function-runner component using containerized execution
- Functions automatically execute during package rendering on Draft package revisions
- Function results are stored in `status.renderStatus` of the PackageRevisionResources view of a package revision
- Published packages are immutable - functions don't re-execute after publication
- Validation failures block Draft package creation and package revision resource updates
