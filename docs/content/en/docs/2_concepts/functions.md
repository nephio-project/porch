---
title: "KRM Functions"
type: docs
weight: 7
description: |
  Understanding KRM functions in Porch: how functions transform and validate package resources.
---

## What are Functions in Porch?

**Functions** in Porch are [KRM (Kubernetes Resource Model) functions](https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md) - containerized programs that transform or validate Kubernetes resource manifests within a package. Functions are declared in a package's Kptfile and executed by Porch when rendering the package.

Functions enable:
- Automated resource generation and modification
- Policy enforcement and validation
- Configuration customization without manual editing
- Repeatable, auditable transformations

For details on how to declare and configure functions in the Kptfile pipeline, see the [kpt functions documentation](https://kpt.dev/book/04-using-functions/).

## Function Execution in Porch

Porch executes functions through a **function runner** component that orchestrates containerized function execution:

- Functions run in isolated containers (Kubernetes pods managed by the function-runner deployment)
- Each function receives the package's resources as a [ResourceList](https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md#resourcelist)
- Functions execute sequentially in the order declared in the Kptfile pipeline
- Function results are captured and stored in the PackageRevision's `status.renderStatus` field
- Execution happens automatically during package rendering operations

## When Functions Execute

Porch automatically executes functions during package rendering:

**Automatic rendering occurs when:**
- A Draft PackageRevision is created (init, clone, edit tasks)
- Package resources are modified via PackageRevisionResources updates
- Tasks are added to a Draft PackageRevision

**Manual rendering via render task:**
- Users can add an explicit `render` task to force re-execution of the pipeline

**Lifecycle constraints:**
- Functions only execute on **Draft** PackageRevisions
- Published packages are immutable - their rendered state is frozen
- Function results are preserved in `status.renderStatus` across lifecycle transitions

## Function Results in Porch

Function execution results are stored in the PackageRevision's status:

```yaml
apiVersion: porch.kpt.dev/v1alpha1
kind: PackageRevision
status:
  renderStatus:
    result:
      exitCode: 0
      items:
        - image: gcr.io/kpt-fn/set-namespace:v0.4.1
          exitCode: 0
        - image: gcr.io/kpt-fn/kubeval:v0.3.0
          exitCode: 1
          results:
            - message: "Invalid resource configuration"
              severity: error
```

The `renderStatus` field contains:
- Overall exit code (0 for success, non-zero for failure)
- Per-function results including exit codes and validation messages
- Error details if function execution failed

Validation failures don't prevent Draft packages from being created, but they appear in the status for review before proposing or publishing.

## Key Points

- Functions are standard KRM functions declared in the Kptfile pipeline (see [kpt functions docs](https://kpt.dev/book/04-using-functions/))
- Porch executes functions via a function-runner component using containerized execution
- Functions automatically execute during package rendering on Draft PackageRevisions
- Function results are stored in `status.renderStatus` of the PackageRevision
- Published packages are immutable - functions don't re-execute after publication
- Validation failures are recorded but don't block Draft package creation
