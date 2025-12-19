---
title: "Authenticating to Remote Git Repositories"
type: docs
weight: 2
description: ""
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

## Porch Server to Git Interaction

The porch server handles interaction with associated git repositories through the use of porch repository CR (Custom Resource) which act as a link between the porch server and the git repositories the server is meant to interact with and store packages on.

More information on porch repositories can be found [here]({{% relref "/docs/2_concepts/relevant_old_docs/package-orchestration.md#repositories" %}}).

There are 2 main methods of authenticating to a git repository and an additional configuration.
These are

1. Basic Authentication
2. Bearer Token Authentication
3. HTTPS/TLS Configuration

### Basic Authentication

A porch repository object can be created through the use of the `porchctl repo reg porch-test-repository -n porch-test http://example-ip:example-port/repo.git --repo-basic-password=password --repo-basic-username=username` command which creates a secret and repository object.

The basic authentication secret must meet the following criteria:

- Exist in the same namespace as the Repository CR (Custom Resource) that requires it.
- Have a Data keys named *username* and *password* containing the relevant information.
- Be of type *basic-auth*.

The value used in the *password* field can be substituted for a base64 encoded Personal Access Token (PAT) from the GIT instance being used. An Example of this can be found [here]({{% relref "/docs/7_cli_api/relevant_old_docs/porchctl-cli-guide.md#repository-registration" %}})

Which would be the equivalent of doing a `kubectl apply -f` on a yaml file with the following content (assuming the porch-test namespace exists on the cluster):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: git-auth-secret
  namespace: porch-test
data:
  username: base-64-encoded-username
  password: base-64-encoded-password  # or base64-encoded-PAT
type: kubernetes.io/basic-auth

---
apiVersion: config.porch.kpt.dev/v1alpha1
kind: Repository

metadata:
  name: porch-test-repository
  namespace: porch-test

spec:
  description: porch test repository
  content: Package
  deployment: false
  type: git
  git:
    repo: http://example-ip:example-port/repo.git
    directory: /
    branch: main
    secretRef:
      name: git-auth-secret
```

When The Porch Server is interacting with a Git instance through this http-basic-auth configuration it does so over HTTP. An example HTTP Request using this configuration can be seen below.

```logs
PUT
https://example-ip/apis/config.porch.kpt.dev/v1alpha1/namespaces/porch-test/repositories/porch-test-repo/status
Request Headers:
     User-Agent: __debug_bin1520795790/v0.0.0 (linux/amd64) kubernetes/$Format
     Authorization: Basic bmVwaGlvOnNlY3JldA== 
     Accept: application/json, */*
     Content-Type: application/json
```

where *bmVwaGlvOnNlY3JldA==* is base64 encoded in the format *username:password* and after base64 decoding becomes *nephio:secret*. For simple personal access token login, the password section can be substituted with the PAT token.

### Bearer Token Authentication

The authentication to the git repository can be configured to be in bearer token format by altering the secret used in the porch repository object.

The bearer token authentication secret must meet the following criteria:

- Exist in the same namespace as the Repository CR (Custom Resource) that requires it
- Have a Data key named *bearerToken* containing the relevant git token information.
- Be of type *Opaque*.

For example:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: git-auth-secret
  namespace: porch-test
data:
  bearerToken: base-64-encoded-bearer-token
type: Opaque
```

When The Porch Server is interacting with a Git instance through this http-token-auth configuration, it does so overt HTTP. An example HTTP Request using this configuration can be seen below.

```logs
PUT https://example-ip/apis/config.porch.kpt.dev/v1alpha1/namespaces/porch-test/repositories/porch-test-repo/status
Request Headers:
     User-Agent: __debug_bin1520795790/v0.0.0 (linux/amd64) kubernetes/$Format
     Authorization: Bearer 4764aacf8cc6d72cab58e96ad6fd3e3746648655
     Accept: application/json, */*
     Content-Type: application/json
```

where *4764aacf8cc6d72cab58e96ad6fd3e3746648655* in the Authorization header is a PAT token, but can be whichever type of bearer token is accepted by the user's git instance.

{{% alert title="Note" color="primary" %}}
Please Note that the Porch server caches the authentication credentials from the secret, therefore if the secret's contents are updated they may in fact not be the credentials used in the authentication.

When the cached old secret credentials are no longer valid the porch server will query the secret again to use the new credentials.

If these new credentials are valid they become the new cached authentication credentials.
{{% /alert %}}

### HTTPS/TLS Configuration

To enable the porch server to communicate with a custom git deployment over HTTPS, we must:

1. Provide an additional arguments flag *use-git-cabundle=true* to the porch-server deployment.
2. Provide an additional Kubernetes secret containing the relevant certificate chain in the form of a cabundle.

The secret itself must meet the following criteria:

- Exist in the same namespace as the Repository CR that requires it.
- Be named specifically \<namespace\>-ca-bundle.
- Have a Data key named *ca.crt* containing the relevant ca certificate (chain).

For example, a Git Repository is hosted over HTTPS at the URL: `https://my-gitlab.com/joe.bloggs/blueprints.git`

Before creating the new Repository in the **GitLab** namespace, we must create a secret that fulfils the criteria above.

`kubectl create secret generic gitlab-ca-bundle --namespace=gitlab --from-file=ca.crt`

Which would produce the following:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: gitlab-ca-bundle
  namespace: gitlab
type: Opaque
data:
  ca.crt: FAKE1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNuakNDQWdHZ0F3SUJBZ0lRTEdmUytUK3YyRDZDczh1MVBlUUlKREFLQmdncWhrak9QUVFEQkRBZE1Sc3cKR1FZRFZRUURFeEpqWlhKMExXMWhibUZuWlhJdWJHOWpZV3d3SGhjTk1qUXdOVE14TVRFeU5qTXlXaGNOTWpRdwpPREk1TVRFeU5qTXlXakFWTVJNd0VRWURWUVFGRXdveE1qTTBOVFkzT0Rrd01JSUJJakFOQmdrcWhraUc5dzBCCkFRRUZBQU9DQVE4QU1JSUJDZ0tDQVFFQXhCUUtWMEVzQ1JOOGxuV3lQR1ZWNXJwam5QZkI2emszK0N4cEp2NVMKUWhpMG1KbDI0elV1WWZjRzNxdFUva1NuREdjK3NQRUY0RmlOcUlsSTByWHBQSXBPazhKbjEvZU1VT3RkZUUyNgpSWEZBWktjeDVvdUJyZVNja3hsN2RPVkJnOE1EM1h5RU1PQU5nM0hJZ1J4ZWx2U2p1dy8vMURhSlRnK0lBS0dUCkgrOVlRVFcrZDIwSk5wQlR3NkdnQlRsYmdqL2FMRWEwOXVYSVBjK0JUSkpXRThIeDhkVjFNbEtHRFlDU29qZFgKbG9TN1FIa0dsSVk3M0NPZVVGWEVnTlFVVmZaZHdreXNsT3F4WmdXUTNZTFZHcEFyRitjOVdyUGpQQU5NQWtORQpPdHRvaG8zTlRxQ3FST3JEa0RMYWdsU1BKSUd1K25TcU5veVVxSUlWWkV5R1dRSURBUUFCbzJBd1hqQU9CZ05WCkhROEJBZjhFQkFNQ0JhQXdEQVlEVlIwVEFRSC9CQUl3QURBZkJnTlZIU01FR0RBV2dCUitFZTVDTnVJSkcwZjkKV3J3VzdqYUZFeVdzb1RBZEJnTlZIUkVFRmpBVWdoSm5hWFJzWVdJdVpYaGhiWEJzWlM1amIyMHdDZ1lJS29aSQp6ajBFQXdRRGdZb0FNSUdHQWtGLzRyNUM4bnkwdGVIMVJlRzdDdXJHYk02SzMzdTFDZ29GTkthajIva2ovYzlhCnZwODY0eFJKM2ZVSXZGMEtzL1dNUHNad2w2bjMxUWtXT2VpM01aYWtBUUpCREw0Kyt4UUxkMS9uVWdqOW1zN2MKUUx3NXVEMGxqU0xrUS9mOTJGYy91WHc4QWVDck5XcVRqcDEycDJ6MkUzOXRyWWc1a2UvY2VTaWFPUm16eUJuTwpTUTg9Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0=
```
