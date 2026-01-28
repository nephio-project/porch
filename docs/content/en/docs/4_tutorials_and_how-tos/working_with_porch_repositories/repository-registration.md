---
title: "Registering Repositories"
type: docs
weight: 3
description: "Registering Repositories guide in Porch"
---

Registering a repository connects Porch to your Git storage backend, allowing it to discover and manage packages. You can register repositories with various authentication methods and configuration options.

### Register a Git Repository

Register a Git repository with Porch:

```bash
porchctl repo register https://github.com/example/porch-test.git \
  --namespace default \
  --name=porch-test \
  --description="Blueprint packages" \
  --branch=main
```

**What this does:**

- Registers the Git repository with Porch
- Creates a Repository resource in Kubernetes
- Begins synchronizing packages from the repository

**Example output:**

```bash
porch-test created
```

**Verify registration:**

Check that the repository was registered successfully:

```bash
porchctl repo get porch-test --namespace default
```

Look for `READY: True` in the output to confirm the repository is accessible and synchronized.

```bash
NAME         TYPE   CONTENT   SYNC SCHEDULE   DEPLOYMENT   READY   ADDRESS
porch-test   git    Package                                True    https://github.com/example-repo/porch-test.git
```

**If READY shows False:**

Inspect the detailed status to see the error message:

```bash
porchctl repo get porch-test --namespace default -o yaml
```

Check the `status.conditions` section for the error details:

```yaml
status:
  conditions:
  - type: Ready
    status: "False"
    reason: Error
    message: 'failed to list remote refs: repository not found: Repository not found.'
    lastTransitionTime: "2025-11-27T14:32:20Z"
```

**Common error messages:**

- `failed to list remote refs: repository not found` - Repository URL is incorrect or repository doesn't exist
- `failed to resolve credentials: cannot resolve credentials in a secret <namespace>/<secret-name>: secrets "<secret-name>" not found` - Authentication secret doesn't exist or name is misspelled
- `failed to resolve credentials: resolved credentials are invalid` - Credentials in the secret are invalid or malformed
- `branch "<branch-name>" not found in repository` - Specified branch doesn't exist in the repository
- `repository URL is empty` - Repository URL not specified in configuration
- `target branch is empty` - Branch name not specified in configuration

---

### Register with Authentication

For private repositories, provide authentication credentials:

**Using Basic Auth:**

```bash
porchctl repo register https://github.com/example/private-repo.git \
  --namespace default \
  --name=private-repo \
  --repo-basic-username=myusername \
  --repo-basic-password=mytoken
```

**Using Workload Identity (GCP):**

```bash
porchctl repo register https://github.com/example/private-repo.git \
  --namespace default \
  --name=private-repo \
  --repo-workload-identity
```

{{% alert title="Note" color="primary" %}}
For production environments, use secret management solutions (external secret stores, sealed-secrets) rather than embedding credentials in commands.

See [Git Authentication]({{% relref "/docs/6_configuration_and_deployments/configurations/components/porch-server-config/git-authentication" %}}) for detailed authentication configuration.
{{% /alert %}}

---

### Register with Advanced Options

Configure additional repository settings:

```bash
porchctl repo register https://github.com/nephio-project/catalog \
  --namespace default \
  --name=infra \
  --directory=infra \
  --deployment=true \
  --sync-schedule="*/10 * * * *" \
  --description="Infrastructure packages"
```

**Common flags:**

- `--name`: Repository name in Kubernetes (defaults to last segment of URL)
- `--description`: Brief description of the repository
- `--branch`: Git branch to use (defaults to `main`)
- `--directory`: Subdirectory within repository containing packages. Use `/` for repository root, or specify a path like `/blueprints` or `infra/packages`. Leading slash is optional.
- `--deployment`: Mark as deployment repository (packages are deployment-ready)
- `--sync-schedule`: Cron expression for periodic sync (e.g., `*/10 * * * *` for every 10 minutes). Format: `minute hour day month weekday`.
- `--repo-basic-username`: Username for basic authentication
- `--repo-basic-password`: Password/token for basic authentication
- `--repo-workload-identity`: Use workload identity for authentication

{{% alert title="Note" color="primary" %}}
For complete command syntax and all available flags, see the [Porchctl CLI Guide]({{% relref "/docs/7_cli_api/relevant_old_docs/porchctl-cli-guide.md" %}}).
{{% /alert %}}

---
