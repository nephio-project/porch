---
title: "Cloning Package Revisions"
type: docs
weight: 5
description: "A step by step guide to cloning package revisions in Porch"
---

## Tutorial Overview

You will learn how to:

1. Find a PackageRevision to clone
2. Clone a PackageRevision to a different repository
3. Modify the cloned PackageRevision
4. Propose and approve the new revision

{{% alert title="Note" color="primary" %}}
Please note the tutorial assumes repositories are initialized with the "blueprints" and "deployments" names.
We recommended to use these for simpler copy pasting of commands otherwise replace these values with your repository names in the below commands.
{{% /alert %}}

---

## Understanding Clone Operations

Cloning creates a new PackageRevision based on an existing one and works **across different repositories**. The cloned PackageRevision maintains an **upstream reference** to its source, allowing it to receive updates.

---

## When to Use Clone

**Use `porchctl rpkg clone` when:**

- You need to import a package from a **different repository** (cross-repository operation)
- You want to maintain an **upstream relationship** for future updates
- You're importing blueprints from a central repository to deployment repositories
- You need to pull packages from external Git or OCI repositories
- You want to keep deployment packages synchronized with their upstream sources
- You're following the blueprint → deployment pattern

**Do NOT use clone when:**

- Source and target are in the **same repository** - use `porchctl rpkg copy` instead
- You want a completely independent copy with no upstream link - use `porchctl rpkg copy` instead
- You're just creating a new version within the same repository - use `porchctl rpkg copy` instead

{{% alert title="Note" color="primary" %}}
For same-repository operations without upstream relationships, see [Copying Package Revisions Guide]({{% relref "/docs/4_tutorials_and_how-tos/working_with_package_revisions/copying-packages.md" %}}).
{{% /alert %}}

---

## Step 1: Find a PackageRevision to Clone

First, list available PackageRevisions to find one to clone:

```bash
porchctl rpkg get --namespace default
```

**Example output:**

```bash
NAME                             PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
blueprints.nginx.main            nginx              main            5          true     Published   blueprints
blueprints.wordpress.v1          wordpress          v1              3          true     Published   blueprints
deployments.my-app.v1            my-app             v1              1          true     Published   deployments
```

**What to look for:**

- Published PackageRevisions from blueprint repositories are good candidates for cloning
- Note the full NAME (e.g., `blueprints.nginx.main`)
- Check the REPOSITORY column to identify the source repository

---

## Step 2: Clone the PackageRevision

Clone an existing PackageRevision to a different repository:

```bash
porchctl rpkg clone \
  blueprints.nginx.main \
  my-nginx \
  --namespace default \
  --repository deployments \
  --workspace v1
```

**What this does:**

- Creates a new PackageRevision based on `blueprints.nginx.main`
- Names the new PackageRevision `my-nginx` (package name)
- Places it in the `deployments` repository (different from source)
- Uses `v1` as the workspace name
- Starts in `Draft` lifecycle state
- Maintains an upstream reference to `blueprints.nginx.main`

**Verify the clone was created:**

```bash
porchctl rpkg get --namespace default --name my-nginx
```

**Example output:**

```bash
NAME                             PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
deployments.my-nginx.v1          my-nginx           v1              0          false    Draft       deployments
```

---

## Step 3: Modify the Cloned PackageRevision

After cloning, you can modify the new PackageRevision. Pull it locally:

```bash
porchctl rpkg pull deployments.my-nginx.v1 ./my-nginx --namespace default
```

**Make your changes:**

```bash
vim ./my-nginx/Kptfile
```

For example, customize the namespace:

```yaml
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: my-nginx
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  description: Nginx deployment for production
upstream:
  type: git
  git:
    repo: blueprints
    directory: nginx
    ref: main
pipeline:
  mutators:
    - image: gcr.io/kpt-fn/set-namespace:v0.4.1
      configMap:
        namespace: production
```

**Push the changes back:**

```bash
porchctl rpkg push deployments.my-nginx.v1 ./my-nginx --namespace default
```

---

## Step 4: Propose and Approve

Once you're satisfied with the changes, propose the PackageRevision:

```bash
porchctl rpkg propose deployments.my-nginx.v1 --namespace default
```

**Verify the state change:**

```bash
porchctl rpkg get deployments.my-nginx.v1 --namespace default
```

**Example output:**

```bash
NAME                             PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
deployments.my-nginx.v1          my-nginx           v1              0          false    Proposed    deployments
```

**Approve to publish:**

```bash
porchctl rpkg approve deployments.my-nginx.v1 --namespace default
```

**Verify publication:**

```bash
porchctl rpkg get deployments.my-nginx.v1 --namespace default
```

**Example output:**

```bash
NAME                             PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
deployments.my-nginx.v1          my-nginx           v1              1          true     Published   deployments
```

---

{{% alert title="Note" color="primary" %}}
For complete details on the `porchctl rpkg clone` command options and flags, see the [Porch CLI Guide]({{% relref "/docs/7_cli_api/relevant_old_docs/porchctl-cli-guide.md" %}}).
{{% /alert %}}

---

## Common Use Cases

Here are practical scenarios where cloning PackageRevisions is useful.

### Importing from Blueprint Repository

Clone a blueprint package to your deployment repository:

```bash
# Clone from blueprints to deployments
porchctl rpkg clone \
  blueprints.base-app.main \
  my-app \
  --namespace default \
  --repository deployments \
  --workspace v1

# Customize and publish
porchctl rpkg pull deployments.my-app.v1 ./my-app --namespace default
# ... customize ...
porchctl rpkg push deployments.my-app.v1 ./my-app --namespace default
porchctl rpkg propose deployments.my-app.v1 --namespace default
porchctl rpkg approve deployments.my-app.v1 --namespace default
```

### Cloning from External Git Repository

Clone directly from a Git repository URL:

```bash
# Clone from external Git repo
porchctl rpkg clone \
  https://github.com/example/blueprints.git \
  external-app \
  --namespace default \
  --repository deployments \
  --workspace v1 \
  --ref main \
  --directory packages/app

# Publish
porchctl rpkg propose deployments.external-app.v1 --namespace default
porchctl rpkg approve deployments.external-app.v1 --namespace default
```

---

## Troubleshooting

Common issues when cloning PackageRevisions and how to resolve them.

**Clone fails with "repository not found"?**

- Verify the target repository exists: `porchctl repo get --namespace default`
- Check the repository name is correct
- Ensure you have permission to write to the target repository

**Clone fails with "source not found"?**

- Verify the source PackageRevision exists: `porchctl rpkg get --namespace default`
- Check the exact name including repository, package, and workspace
- Ensure you have permission to read the source PackageRevision

**Clone fails with "workspace already exists"?**

- The workspace name must be unique within the package in the target repository
- Choose a different workspace name: `--workspace v2` or `--workspace prod`
- List existing workspaces: `porchctl rpkg get --namespace default --name <package>`

**Cloned PackageRevision has unexpected content?**

- The clone includes all resources from the source at the time of cloning
- Pull and inspect: `porchctl rpkg pull <name> ./dir --namespace default`
- Make corrections and push back

**Need to clone within the same repository?**

- Use `porchctl rpkg copy` instead of `clone` for same-repository operations
- The `copy` command is simpler and doesn't maintain upstream references
- See [Copying Package Revisions Guide]({{% relref "/docs/4_tutorials_and_how-tos/working_with_package_revisions/copying-packages.md" %}})

---

## Key Concepts

- **Clone**: Creates a new PackageRevision that can be in a different repository
- **Upstream Reference**: Cloned packages maintain a link to their source for updates
- **Cross-repository**: Clone works across different repositories, unlike copy
- **Source Types**: Can clone from Porch packages, Git URLs, or OCI repositories
- **Workspace**: Must be unique within the package in the target repository
- **Blueprint Pattern**: Common pattern is blueprints repository → deployment repositories

---
