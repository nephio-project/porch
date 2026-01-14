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

**What this does:**

- Queries Porch for all registered repositories in the specified namespace
- Displays repository type, content, sync schedule, and status
- Shows the repository address

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

List repositories across all namespaces:

```bash
kubectl get repositories --all-namespaces
```

**Example output:**

```bash
NAME         TYPE   CONTENT   SYNC SCHEDULE   DEPLOYMENT   READY   ADDRESS
porch-test   git    Package                                True    https://github.com/example-org/test-packages.git
blueprints   git    Package   */10 * * * *                 True    https://github.com/example/blueprints.git
infra        git    Package   */10 * * * *    true         True    https://github.com/nephio-project/catalog
```

**Understanding the output:**

- **NAME**: Repository name in Kubernetes
- **TYPE**: Repository type (`git` or `oci`)
- **CONTENT**: Content type (typically `Package`)
- **SYNC SCHEDULE**: Cron expression for periodic synchronization (if configured).
- **DEPLOYMENT**: Whether this is a deployment repository
- **READY**: Repository health status
- **ADDRESS**: Repository URL

---

### Get Detailed Repository Information

View complete details about a specific repository:

```bash
porchctl repo get porch-test --namespace default -o yaml
```

**What this does:**

- Retrieves the full Repository resource
- Shows configuration, authentication, and status information
- Displays in YAML format for easy reading

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
    message: 'Repository Ready (next sync scheduled at: 2025-11-26T09:48:03Z)'
    lastTransitionTime: "2025-11-26T09:45:03Z"
```

**Key fields to inspect:**

- **spec.type**: Repository type (typically `git`)
- **spec.git**: Git-specific configuration (repo URL, branch, directory, credentials)
- **spec.content**: Content type stored in repository
- **status.conditions**: Repository health and sync status
  - **status**: `"True"` (healthy) or `"False"` (error)
  - **reason**: `Ready` or `Error`
  - **message**: Detailed error message when status is False

---

### Update Repository Configuration

The typical workflow for changing repository settings is to unregister and re-register the repository with new configuration. This is the recommended approach.

{{% alert title="Note" color="primary" %}}
There is no `porchctl repo update` command. The standard approach is unregister/reregister.
{{% /alert %}}

**Recommended approach - Unregister and re-register:**

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

**Alternative - Direct kubectl editing (NOT RECOMMENDED):**

While you can edit the repository resource directly with kubectl, this is highly discouraged:

```bash
kubectl edit repository porch-test -n default
```

{{% alert title="Warning" color="warning" %}}
Direct editing with kubectl is not recommended because:

- Some values like `url`, `branch`, and `directory` are immutable or not designed to be changed this way
- Changes to authentication secrets (like `secretRef`) are cached by Porch and won't take effect immediately
- Secret changes only apply when authentication fails and Porch refreshes the cached credentials
- This can lead to unpredictable behavior
{{% /alert %}}

**If you must use kubectl editing:**

Only modify fields that are safe to change, such as:

- `secretRef.name`: Change authentication credentials (with caching caveats above)

Avoid changing:

- `url`: Repository URL
- `branch`: Git branch
- `directory`: Package directory path

---
