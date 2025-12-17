---
title: "Working with Package Revisions"
type: docs
weight: 3
description: A group of guides outlining how to interact with Package Revisions in Porch
---

## Prerequisites

- Porch deployed on a Kubernetes cluster [Setup Porch Guide]({{% relref "/docs/3_getting_started/installing-porch.md" %}}).
- **Porchctl** CLI tool installed [Setup Porchctl Guide]({{% relref "/docs/3_getting_started/installing-porchctl.md" %}}).
- A Git repository registered with Porch [Setup Repositories Guide]({{% relref "/docs/4_tutorials_and_how-tos/working_with_porch_repositories/repository-registration.md" %}}).
- **Kubectl** configured to access your cluster.

---

## Understanding Package Revisions

In Porch, you work with **PackageRevisions** - there is no separate "Package" resource. When we say "package" colloquially, we're referring to a PackageRevision. The `rpkg` command stands for "revision package".

PackageRevisions are Kubernetes resources that represent versioned collections of configuration files stored in Git repositories. Each PackageRevision contains:

- **KRM Resources**: Kubernetes Resource Model files (YAML configurations)
- **Kptfile**: Package metadata and pipeline configuration
- **Pipeline Functions**: KRM functions that transform package resources
- **Lifecycle State**: Current state in the package workflow

**PackageRevision Operations:**

- **Creation**: `init`, `clone`, `copy` - Create new PackageRevisions from scratch, existing packages, or new revisions
- **Inspection**: `get` - List and view PackageRevision information and metadata
- **Content Management**: `pull`, `push` - Move PackageRevision content between Git repositories and local filesystem
- **Lifecycle Management**: `propose`, `approve`, `reject` - Control PackageRevision workflow states
- **Upgrading**: `upgrade` - Create new revision upgrading downstream to more recent upstream package
- **Deletion**: `propose-delete`, `del` - Propose deletion of published PackageRevisions, then delete them

---

## PackageRevision Lifecycle

PackageRevisions follow a structured lifecycle with three main states:

- **Draft**: Work in progress, fully editable. Revision number is 0.
- **Proposed**: Ready for review, still editable. Revision number remains 0.
- **Published**: Approved and immutable. Revision number increments to 1+.

**Lifecycle Transitions:**

1. **Draft → Proposed**: `porchctl rpkg propose` - Signal readiness for review
2. **Proposed → Published**: `porchctl rpkg approve` - Approve and make immutable
3. **Proposed → Draft**: `porchctl rpkg reject` - Return for more work

**Additional States:**

- **DeletionProposed**: PackageRevision marked for deletion, pending approval

---

## PackageRevision Naming

Porch generates PackageRevision names automatically using a consistent format:

- **Format**: `{repositoryName}.{packageName}.{workspaceName}`
- **Example**: `porch-test.my-first-package.v1`

**Name Components:**

- **Repository Name**: Name of the registered Git repository
- **Package Name**: Logical name for the package (can have multiple revisions)
- **Workspace Name**: Unique identifier within the package (maps to Git branch/tag)

**Important Notes:**

- Workspace names must be unique within a package
- Multiple PackageRevisions can share the same package name with different workspaces
- Published PackageRevisions get tagged in Git using the workspace name

---

## Working with PackageRevision Content

PackageRevisions contain structured configuration files that can be modified through various operations:

**Local Operations:**

1. **Pull**: Download PackageRevision contents to local filesystem
2. **Modify**: Edit files locally using standard tools
3. **Push**: Upload changes back to Porch (triggers pipeline rendering)

**Pipeline Processing:**

- KRM functions defined in the Kptfile automatically transform resources
- Functions run when PackageRevisions are pushed to Porch
- Common functions: set-namespace, apply-replacements, search-replace

**Content Structure:**

```bash
package-revision/
├── Kptfile                   # Package metadata and pipeline
├── .KptRevisionMetadata      # Porch-managed metadata
├── package-context.yaml      # Package context information
├── README.md                 # Package documentation
└── *.yaml                    # KRM resources
```

---

## Repository Integration

PackageRevisions are stored in Git repositories registered with Porch:

**Git Branch Mapping:**

- **Draft**: Stored in `draft/{workspace}` branch
- **Proposed**: Stored in `proposed/{workspace}` branch  
- **Published**: Tagged as `{workspace}` and stored in main branch

**Repository Types:**

- **Blueprint Repositories**: Contain upstream package templates for cloning
- **Deployment Repositories**: Store deployment-ready packages (marked with `--deployment` flag)

**Synchronization:**

- Porch automatically syncs with Git repositories
- Manual sync: `porchctl repo sync <repository-name>`
- Periodic sync can be configured with cron expressions

---

## Troubleshooting

Common issues when working with PackageRevisions and their solutions:

**PackageRevision stuck in Draft?**

- Check readiness conditions: `porchctl rpkg get <PACKAGE-REVISION> -o yaml | grep -A 5 conditions`
- Verify all required fields are populated in the Kptfile
- Check for pipeline function errors in Porch server logs

**Push fails with conflict?**

- Pull the latest version first: `porchctl rpkg pull <PACKAGE-REVISION> ./dir`
- The PackageRevision may have been modified by someone else
- Resolve conflicts locally and push again

**Cannot modify Published PackageRevision?**

- Published PackageRevisions are immutable by design
- Create a new revision using `porchctl rpkg copy`
- Use the copying workflow to create editable versions

**PackageRevision not found?**

- Verify the exact PackageRevision name: `porchctl rpkg get --namespace default`
- Check you're using the correct namespace
- Ensure the repository is registered and synchronized

**Permission denied errors?**

- Check RBAC permissions: `kubectl auth can-i get packagerevisions -n default`
- Verify service account has proper roles for PackageRevision operations
- Ensure repository authentication is configured correctly

**Pipeline functions failing?**

- Check function image availability and version
- Verify function configuration in Kptfile
- Review function logs in Porch server output during push operations

---

## Key Concepts

Important terms and concepts for working with PackageRevisions:

- **PackageRevision**: The Kubernetes resource managed by Porch (there is no separate "Package" resource)
- **Workspace**: Unique identifier for a PackageRevision within a package (maps to Git branch/tag)
- **Lifecycle**: Current state of the PackageRevision (Draft, Proposed, Published, DeletionProposed)
- **Revision Number**: 0 for Draft/Proposed, 1+ for Published (increments with each publication)
- **Latest**: Flag indicating the most recent published PackageRevision of a package
- **Pipeline**: KRM functions defined in Kptfile that transform PackageRevision resources
- **Upstream/Downstream**: Relationship between source PackageRevisions (upstream) and their clones (downstream)
- **Repository**: Git repository where PackageRevisions are stored and managed
- **Namespace Scope**: PackageRevisions exist within a Kubernetes namespace and inherit repository namespace
- **Rendering**: Process of executing pipeline functions to transform PackageRevision resources
- **Kptfile**: YAML file containing PackageRevision metadata, pipeline configuration, and dependency information

---
