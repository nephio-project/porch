---
title: "Git Authentication"
type: docs
weight: 1
description: "Configure Git repository authentication for Porch Server"
---

## Authentication Methods

The Porch Server handles interaction with Git repositories through Repository Custom Resources (CRs) that act as a link between the Porch Server and the Git repositories.

Porch Server supports three authentication methods for Git repositories:

1. [Basic Authentication](#1-basic-authentication) - Username and password or Personal Access Token (post-deployment)
2. [Bearer Token Authentication](#2-bearer-token-authentication) - Token-based authentication (post-deployment)
3. [HTTPS/TLS Configuration](#3-httpstls-configuration) - Custom TLS certificates for self-hosted Git (**requires pre-deployment configuration**)

### 1. Basic Authentication

Uses username and password or Personal Access Token (PAT). The secret must:
- Exist in the same namespace as the Repository CR
- Have data keys named `username` and `password`
- Be of type `kubernetes.io/basic-auth`

The `password` field can contain a base64-encoded Personal Access Token instead of a password.

#### Create Basic Auth Secret

```bash
kubectl create secret generic git-auth-secret \
  --namespace=default \
  --from-literal=username=your-username \
  --from-literal=password=your-password \
  --type=kubernetes.io/basic-auth
```

#### Repository Configuration

```yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: Repository
metadata:
  name: example-repo
  namespace: default
spec:
  type: git
  git:
    repo: https://github.com/example/repo.git
    branch: main
    secretRef:
      name: git-auth-secret
```

### 2. Bearer Token Authentication

Uses token-based authentication (e.g., GitHub PAT, GitLab token). The secret must:
- Exist in the same namespace as the Repository CR
- Have a data key named `bearerToken`
- Be of type `Opaque`

#### Create Bearer Token Secret

```bash
kubectl create secret generic git-token-secret \
  --namespace=default \
  --from-literal=bearerToken=your-token \
  --type=Opaque
```

#### Repository Configuration

```yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: Repository
metadata:
  name: example-repo
  namespace: default
spec:
  type: git
  git:
    repo: https://github.com/example/repo.git
    branch: main
    secretRef:
      name: git-token-secret
```

### 3. HTTPS/TLS Configuration

For Git repositories with custom TLS certificates. The CA bundle secret must:
- Exist in the same namespace as the Repository CR
- Be named exactly `<namespace>-ca-bundle`
- Have a data key named `ca.crt` containing the certificate chain

#### Enable TLS Support

{{% alert title="Pre-deployment Required" color="warning" %}}
The `--use-git-cabundle=true` argument must be added to the Porch Server deployment **before** deployment. This cannot be configured post-deployment.
{{% /alert %}}

Add the `--use-git-cabundle=true` argument to the Porch Server deployment.

#### Create CA Bundle Secret

The secret must be named `<namespace>-ca-bundle`:

```bash
kubectl create secret generic default-ca-bundle \
  --namespace=default \
  --from-file=ca.crt=/path/to/ca-certificate.crt
```

#### Repository Configuration

```yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: Repository
metadata:
  name: secure-repo
  namespace: default
spec:
  type: git
  git:
    repo: https://secure-git.example.com/repo.git
    branch: main
    secretRef:
      name: git-auth-secret
```

## Authentication Behavior

### Credential Caching

{{% alert title="Note" color="primary" %}}
Porch Server caches authentication credentials. If you update a secret, the server will continue using cached credentials until they become invalid, then fetch the updated credentials.
{{% /alert %}}

### HTTP Request Examples

**Basic Authentication:**
```
Authorization: Basic bmVwaGlvOnNlY3JldA==
```

**Bearer Token:**
```
Authorization: Bearer your-token-here
```

## Common Use Cases

- **GitHub**: Use Personal Access Token with bearer token authentication
- **GitLab**: Use Project Access Token or Personal Access Token  
- **Enterprise Git**: Use basic authentication with username/password
- **Self-hosted Git**: Use TLS configuration for custom certificates

## Using porchctl CLI

You can create repositories with basic authentication using the `porchctl` command:

```bash
# Basic authentication
porchctl repo reg my-repo -n default https://github.com/example/repo.git \
  --repo-basic-username=username \
  --repo-basic-password=password

# This creates both the secret and Repository CR automatically
```

{{% alert title="Note" color="primary" %}}
The `porchctl` CLI only supports basic authentication. For bearer token or TLS authentication, you must create the secrets and Repository CRs manually using `kubectl`.
{{% /alert %}}