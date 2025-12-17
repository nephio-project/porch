---
title: "Creating Package Revisions"
type: docs
weight: 3
description: "A step by step guide to creating a package revision in Porch" 
---

## Tutorial Overview

You will learn how to:

1. Initialize a new package revision
2. Pull the package revision locally
3. Modify the package revision contents
4. Push changes back to Porch
5. Propose the package revision for review
6. Approve or reject the package revision

---

{{% alert title="Note" color="primary" %}}
Please note the tutorial assumes a porch repository is initialized with the "porch-test" name.
We recommended to use this for simpler copy pasting of commands otherwise replace any "porch-test" value with your repository's name in the below commands.
{{% /alert %}}

## Step 1: Initialize Your First Package Revision

Create a new package revision in Porch using the `init` command:

```bash
porchctl rpkg init my-first-package \
  --namespace=default \
  --repository=porch-test \
  --workspace=v1 \
  --description="My first Porch package"
```

**What this does:**

- Creates a new PackageRevision named `my-first-package`
- Places it in the `porch-test` repository
- Uses `v1` as the workspace name (must be unique within this package)
- Starts in `Draft` lifecycle state

![Diagram](/static/images/porch/guides/init-workflow.drawio.svg)

**Verify the package revision was created:**

```bash
porchctl rpkg get --namespace default
```

You should see your package revision listed with lifecycle `Draft`:

```bash
NAME                             PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.my-first-package.v1   my-first-package   v1              0          false    Draft       porch-test
```

---

## Step 2: Pull the Package Revision Locally

Download the package revision contents to your local filesystem:

```bash
porchctl rpkg pull porch-test.my-first-package.v1 ./my-first-package --namespace default
```

**What this does:**

- Fetches all resources from the PackageRevision
- Saves them to the `./my-first-package` directory
- Includes the Kptfile and any other resources

![Diagram](/static/images/porch/guides/pull-workflow.drawio.svg)

**Explore the package revision contents:**

```bash
ls -al ./my-first-package
```

You'll see:

- The `Kptfile` - PackageRevision metadata and pipeline configuration
- Other YAML files (if any were created)

```bash
total 24
drwxr-x--- 2 user user 4096 Nov 24 13:27 .
drwxr-xr-x 4 user user 4096 Nov 24 13:27 ..
-rw-r--r-- 1 user user  259 Nov 24 13:27 .KptRevisionMetadata
-rw-r--r-- 1 user user  177 Nov 24 13:27 Kptfile
-rw-r--r-- 1 user user  488 Nov 24 13:27 README.md
-rw-r--r-- 1 user user  148 Nov 24 13:27 package-context.yaml
```

**Alternatively:**

If you have the tree command installed on your system you can use it to view the hierarchy of the package

```bash
tree ./my-first-package/
```

Should return the following output:

```bash
my-first-package/
├── Kptfile
├── README.md
└── package-context.yaml

1 directory, 3 files
```

---

## Step 3: Modify the Package Revision

Let's add a simple KRM function to the pipeline.

Open the `Kptfile` in your editor of choice:

```bash
vim ./my-first-package/Kptfile
```

Add a mutator function to the pipeline section so that your `Kptfile` looks like so:

```yaml
apiVersion: kpt.dev/v1
kind: Kptfile
metadata:
  name: my-first-package
  annotations:
    config.kubernetes.io/local-config: "true"
info:
  description: My first Porch package
pipeline:
  mutators:
    - image: gcr.io/kpt-fn/set-namespace:v0.4.1
      configMap:
        namespace: production
```

**What this does:**

- Adds a `set-namespace` function to the pipeline
- This function will set the namespace to `production` for all resources
- These Functions are not rendered until the package is "pushed" to porch

**Add new resource:**

Create a new configmap:

```bash
vim ./my-first-package/test-config.yaml
```

Now add the following content to this new configmap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: "value"
```

**Save and close the file.**

{{% alert title="Note" color="primary" %}}
Changes are LOCAL ONLY (Porch doesn't know about them yet) at this stage
{{% /alert %}}

---

## Step 4: Push Changes Back to Porch

Upload your modified package revision back to Porch:

```bash
porchctl rpkg push porch-test.my-first-package.v1 ./my-first-package --namespace default
```

**What this does:**

- Updates the PackageRevision in Porch
- Triggers rendering (executes pipeline functions)
- PackageRevision remains in `Draft` state

![Diagram](/static/images/porch/guides/push-workflow.drawio.svg)

**Successful output:**

This describes how the KRM function was run by porch and has updated the namespace in our new configmap.

```bash
[RUNNING] "gcr.io/kpt-fn/set-namespace:v0.4.1"
[PASS] "gcr.io/kpt-fn/set-namespace:v0.4.1"
  Results:
    [info]: namespace "" updated to "production", 1 value(s) changed
porch-test.my-first-package.v1 pushed
```

---

## Step 5: Propose the Package Revision

Move the package revision to `Proposed` state for review:

```bash
porchctl rpkg propose porch-test.my-first-package.v1 --namespace default
```

**What this does:**

- Changes lifecycle from `Draft` to `Proposed`
- Signals the package revision is ready for review
- PackageRevision can still be modified if needed

![Diagram](/static/images/porch/guides/propose-workflow.drawio.svg)

{{% alert title="Note" color="primary" %}}
A lifecycle state change from `Draft` to `Proposed` means that in Git the package revision has moved from the `draft` branch to the `proposed` branch
{{% /alert %}}

**Verify the state change:**

```bash
porchctl rpkg get porch-test.my-first-package.v1 --namespace default
```

The lifecycle should now show `Proposed`.

```bash
NAME                             PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.my-first-package.v1   my-first-package   v1              0          false    Proposed    porch-test
```

---

## Step 6a: Approve the Package Revision

If the package revision looks good, approve it to publish:

```bash
porchctl rpkg approve porch-test.my-first-package.v1 --namespace default
```

**What this does:**

- Changes PackageRevision lifecycle from `Proposed` (revision 0) to `Published` (revision 1)
- PackageRevision becomes **immutable** (content cannot be changed)
- Records who approved and when
- PackageRevision is now available for cloning/deployment

![Diagram](/static/images/porch/guides/approve-workflow.drawio.svg)

**Verify publication:**

```bash
porchctl rpkg get porch-test.my-first-package.v1 --namespace default -o yaml | grep -E "lifecycle|publishedBy|publishTimestamp"
```

**Verify the state change:**

```bash
porchctl rpkg get porch-test.my-first-package.v1 --namespace default
```

The lifecycle should now show `Published`.

```bash
NAME                               PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.my-first-package.main   my-first-package   main            -1         true     Published   porch-test
porch-test.my-first-package.v1     my-first-package   v1              1          false    Published   porch-test
```

{{% alert title="Note" color="primary" %}}
Porch automatically creates a main branch-tracking PackageRevision (with workspace `main` and revision `-1`) to track the latest published version of this package.
{{% /alert %}}

---

## Step 6b: Reject the Package Revision (Alternative)

If the package revision needs more work, reject it to return to `Draft`:

```bash
porchctl rpkg reject porch-test.my-first-package.v1 --namespace default
```

**What this does:**

- Changes lifecycle from `Proposed` back to `Draft`
- Allows further modifications
- You can then make changes and re-propose

![Diagram](/static/images/porch/guides/reject-workflow.drawio.svg)

If the package revision is rejected, the process begins again from Step 2 until the desired state is achieved.

![Diagram](/static/images/porch/guides/lifecycle-workflow.drawio.svg)

---
