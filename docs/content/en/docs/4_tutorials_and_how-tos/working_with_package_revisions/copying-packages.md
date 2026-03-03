---
title: "Copying Package Revisions"
type: docs
weight: 4
description: "A step by step guide to copying package revisions in Porch"
---

## Tutorial Overview

You will learn how to:

1. Find a PackageRevision to copy
2. Copy a PackageRevision to create a new revision
3. Modify the copied PackageRevision
4. Propose and approve the new revision

{{% alert title="Note" color="primary" %}}
The tutorial assumes a porch repository is initialized with the "porch-test" name.
We recommended to use this for simpler copy pasting of commands otherwise replace any "porch-test" value with your repository's name in the below commands.
{{% /alert %}}

---

## Key Concepts

- **Copy**: Creates a new independent PackageRevision within the same repository
- **Source PackageRevision**: The original PackageRevision being copied
- **Target PackageRevision**: The new PackageRevision created by the copy operation
- **Workspace**: Must be unique within the package for the target
- **Same-repository operation**: Copy only works within a single repository
- **Immutability**: Published PackageRevisions cannot be modified, only copied
- **Clone vs Copy**: Use clone for cross-repository operations, copy for same-repository versions

---

## Understanding Copy Operations

Copying creates a new PackageRevision based on an existing one **within the same repository**. The copied PackageRevision is completely **independent with no upstream link** to the source.

---

## When to Use Copy

**Use `porchctl rpkg copy` when:**

- You need to create a new version of a published PackageRevision (published revisions are immutable)
- You want to create variations of a package within the same repository
- You need an independent copy with no upstream relationship
- You're iterating on a package and need a new workspace
- Source and target are in the **same repository**

**Do NOT use copy when:**

- You need to move a package to a **different repository** - use `porchctl rpkg clone` instead
- You want to maintain an upstream relationship for updates - use `porchctl rpkg clone` instead
- You're importing blueprints from a central repository - use `porchctl rpkg clone` instead

{{% alert title="Note" color="primary" %}}
For cross-repository operations or maintaining upstream relationships, see the [Cloning Package Revisions Guide]({{% relref "/docs/4_tutorials_and_how-tos/working_with_package_revisions/cloning-packages.md" %}}).
{{% /alert %}}

---

## Step 1: Find a PackageRevision to Copy

First, list available PackageRevisions to find one to copy:

```bash
porchctl rpkg get --namespace default
```

**Example output:**

```bash
NAME                             PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.my-app.v1             my-app             v1              1          true     Published   porch-test
blueprints.nginx.main            nginx              main            5          true     Published   blueprints
```

**What to look for:**

- Published PackageRevisions are good candidates for copying
- Note the full NAME (e.g., `porch-test.my-app.v1`)
- Check the LATEST column to find the most recent version

---

## Step 2: Copy the PackageRevision

Copy an existing PackageRevision to create a new one:

```bash
porchctl rpkg copy \
  porch-test.my-app.v1 \
  my-app \
  --namespace default \
  --workspace v2
```

**What this does:**

- Creates a new PackageRevision based on `porch-test.my-app.v1`
- Names the new PackageRevision `my-app` (package name)
- Uses `v2` as the workspace name (must be unique within the package)
- Starts in `Draft` lifecycle state
- Copies all resources from the source PackageRevision

**Verify the copy was created:**

```bash
porchctl rpkg get --namespace default --name my-app
```

**Example output:**

```bash
NAME                             PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.my-app.v1             my-app             v1              1          true     Published   porch-test
porch-test.my-app.v2             my-app             v2              0          false    Draft       porch-test
```

---

## Step 3: Modify the Copied PackageRevision

After copying, you can modify the new PackageRevision. Pull it locally:

```bash
porchctl rpkg pull porch-test.my-app.v2 ./my-app-v2 --namespace default
```

**Make your changes:**

```bash
vim ./my-app-v2/Kptfile
```

For example, you can update the description:

```yaml
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: my-app
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  description: My app version 2 with improvements
pipeline:
  mutators:
    - image: gcr.io/kpt-fn/set-namespace:v0.4.1
      configMap:
        namespace: production
```

**Push the changes back:**

```bash
porchctl rpkg push porch-test.my-app.v2 ./my-app-v2 --namespace default
```

---

## Step 4: Propose and Approve

Once you're satisfied with the changes, propose the PackageRevision:

```bash
porchctl rpkg propose porch-test.my-app.v2 --namespace default
```

**Verify the state change:**

```bash
porchctl rpkg get porch-test.my-app.v2 --namespace default
```

**Example output:**

```bash
NAME                             PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.my-app.v2             my-app             v2              0          false    Proposed    porch-test
```

**Approve to publish:**

```bash
porchctl rpkg approve porch-test.my-app.v2 --namespace default
```

**Verify the publication:**

```bash
porchctl rpkg get --namespace default --name my-app
```

**Example output:**

```bash
NAME                             PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.my-app.v1             my-app             v1              1          false    Published   porch-test
porch-test.my-app.v2             my-app             v2              2          true     Published   porch-test
```

Notice the following changes:

- `v2` now has revision number `2`
- `v2` is marked as `LATEST`
- `v1` is no longer the latest

---

{{% alert title="Note" color="primary" %}}
For complete details on the `porchctl rpkg copy` command options and flags, see the [Porch CLI Guide]({{% relref "/docs/7_cli_api/porchctl.md" %}}).
{{% /alert %}}

---

## Common Use Cases

Here are practical scenarios where copying PackageRevisions is useful.

### Creating a New Version

When you need to update a published PackageRevision in the same Repository:

```bash
# Copy the latest published version
porchctl rpkg copy porch-test.my-app.v2 my-app --namespace default --workspace v3

# Make changes
porchctl rpkg pull porch-test.my-app.v3 ./my-app-v3 --namespace default
# ... edit files ...
porchctl rpkg push porch-test.my-app.v3 ./my-app-v3 --namespace default

# Publish
porchctl rpkg propose porch-test.my-app.v3 --namespace default
porchctl rpkg approve porch-test.my-app.v3 --namespace default
```

### Creating Environment-Specific Workspaces

Create different workspace variations of the same base PackageRevision:

```bash
# Copy for development environment
porchctl rpkg copy porch-test.my-app.v1 my-app --namespace default --workspace dev

# Copy for staging environment
porchctl rpkg copy porch-test.my-app.v1 my-app --namespace default --workspace staging

# Copy for production environment
porchctl rpkg copy porch-test.my-app.v1 my-app --namespace default --workspace prod
```

---

## Troubleshooting

Common issues when copying PackageRevisions and how to resolve them.

**Copy fails with "workspace already exists":**

- The workspace name must be unique within the package
- Choose a different workspace name: `--workspace v3` or `--workspace dev-2`
- List existing workspaces with the `porchctl rpkg get --namespace default --name <package>` command

**Copy fails with "source not found":**

- Verify that the source PackageRevision exists with the `porchctl rpkg get --namespace default` command
- Check the exact name including repository, package, and workspace
- Ensure you have permission to read the source PackageRevision
- Ensure the source is in the same repository (copy only works within the same repository)

**Copied PackageRevision has unexpected content:**

- The copy includes all resources from the source at the time of copying
- Pull and inspect with the `porchctl rpkg pull <name> ./dir --namespace default` command
- Make corrections and push back

---
