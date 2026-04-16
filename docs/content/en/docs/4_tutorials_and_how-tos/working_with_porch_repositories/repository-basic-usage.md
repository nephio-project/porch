---
title: "Repositories Basic Usage"
type: docs
weight: 4
description: "A basic usage of repositories guide in Porch"
---

## Basic Operations

These operations cover the fundamental commands for viewing and managing registered repositories.

### List Registered Repositories

View all repositories registered with Porch:

```bash
porchctl repo get --namespace default
```

This command queries Porch for all registered repositories in the specified namespace, displays repository type, content, sync schedule, and status and shows the repository address.

{{% alert title="Note" color="primary" %}}
`porchctl repo list` is an alias for `porchctl repo get` and can be used interchangeably:

```bash
porchctl repo list --namespace default
```

{{% /alert %}}

**Using kubectl:**

You can also use kubectl to list repositories:

```bash
kubectl get repositories -n default
```

{{% alert title="Note" color="primary" %}}
You can use the shortname `repo` instead of `repositories`:

```bash
kubectl get repo -n default
```

{{% /alert %}}

List repositories across all namespaces:

```bash
kubectl get repositories --all-namespaces
```

**Example output:**

```bash
NAME         TYPE   CONTENT   SYNC SCHEDULE   DEPLOYMENT   READY   ADDRESS                                           BRANCH   AGE
porch-test   git    Package                                True    https://github.com/example-org/test-packages.git  main     5d
blueprints   git    Package   */10 * * * *                 True    https://github.com/example/blueprints.git         main     10d
infra        git    Package   */10 * * * *    true         True    https://github.com/nephio-project/catalog         main     30d
```

**Understanding the output:**

- **NAME**: Repository name in Kubernetes
- **TYPE**: Repository type (`git` or `oci`)
- **CONTENT**: Content type (typically `Package`)
- **SYNC SCHEDULE**: Cron expression for periodic synchronization (if configured).
- **DEPLOYMENT**: Whether this is a deployment repository
- **READY**: Repository health status
- **ADDRESS**: Repository URL
- **BRANCH**: Git branch for package storage (git repositories only)
- **AGE**: Time since repository was registered

---

### Get Detailed Repository Information

View complete details about a specific repository:

```bash
porchctl repo get porch-test --namespace default -o yaml
```

This command retrieves the full Repository resource, shows configuration, authentication, and status information and displays in YAML format for easy reading.

**Example output:**

```yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: Repository
metadata:
  name: porch-test
  namespace: default
  creationTimestamp: "2025-11-21T16:27:27Z"
spec:
  content: Package
  type: git
  git:
    repo: https://github.com/example-org/test-packages.git
    branch: main
    directory: /
    secretRef:
      name: porch-test-auth
status:
  conditions:
  - type: Ready
    status: "True"
    reason: Ready
    message: 'Repository Ready'
    lastTransitionTime: "2025-11-26T09:45:03Z"
  packageCount: 42
  gitCommitHash: a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0
  lastFullSyncTime: "2026-03-19T10:00:00Z"
  nextFullSyncTime: "2026-03-19T10:10:00Z"
  observedGeneration: 1
  observedRunOnceAt: "2026-03-19T09:55:00Z"
```

**Key fields to inspect:**

- **spec.type**: Repository type (typically `git`)
- **spec.git**: Git-specific configuration (repo URL, branch, directory, credentials)
- **spec.content**: Content type stored in repository
- **status.conditions**: Repository health and sync status
  - **status**: `"True"` (healthy) or `"False"` (error)
  - **reason**: `Ready` or `Error`
  - **message**: Detailed error message when status is False
- **status.packageCount**: Number of package revisions found in the repository
- **status.gitCommitHash**: Current commit hash of the configured branch (git repositories only)
- **status.lastFullSyncTime**: Timestamp of the last successful full repository sync
- **status.nextFullSyncTime**: Timestamp when the next full sync is scheduled
- **status.observedGeneration**: Generation of the Repository spec that was last reconciled
- **status.observedRunOnceAt**: Last observed value of spec.sync.runOnceAt

---

### Update Repository Configuration

{{% alert title="Critical" color="danger" %}}
**Important: Key repository fields are IMMUTABLE after creation**

The following fields cannot be changed after a Repository is created:
- `spec.type`
- `spec.git.repo`
- `spec.git.branch`
- `spec.git.directory`
- `spec.oci.registry`

To change any of these fields, you must delete and recreate the Repository.
{{% /alert %}}

The workflow for changing immutable repository settings is to unregister and re-register the repository with the new configuration.

{{% alert title="Note" color="primary" %}}
There is no `porchctl repo update` command. The standard approach is unregister/reregister.
{{% /alert %}}

**Unregister and re-register:**

```bash
# Unregister the repository
porchctl repo unregister porch-test --namespace default

# Re-register with new configuration
porchctl repo register https://github.com/example/porch-test.git \
  --namespace default \
  --name=porch-test \
  --branch=develop \
  --directory=/new-path
```

**Editing mutable fields with kubectl:**

Only a few fields can be safely changed after creation. These include:

- `spec.sync.schedule`: Change the sync schedule
- `spec.sync.runOnceAt`: Trigger a one-time sync
- `spec.secretRef.name`: Change authentication credentials

To edit these fields:

```bash
kubectl edit repository porch-test -n default
```

{{% alert title="Warning" color="warning" %}}
Do not attempt to edit immutable fields (listed above) with kubectl. The Kubernetes validating webhook will reject the change.
{{% /alert %}}

---
