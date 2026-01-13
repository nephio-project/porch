---
title: "Using authenticated private registries with the Porch function runner"
type: docs
weight: 3
description: ""
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

The Porch function runner pulls kpt function images from registries and uses them for rendering kpt packages in Porch. The function runner is set up by default to fetch kpt function images from public container registries such as [GCR](https://gcr.io/kpt-fn/) and the configuration options described here are not required for such public registries.

## 1. Configuring function runner to operate with private container registries

This section describes how set up authentication for a private container registry containing kpt functions online e.g. (GitHub's GHCR) or locally e.g. (Harbor or JFrog) that require authentication (username/password or token).

To enable pulling of kpt function images from authenticated private registries by the Porch function runner the system requires:

1. Creating a Kubernetes secret using a JSON file according to the Docker configuration schema, containing valid credentials for each authenticated registry.
2. Mounting this new secret as a volume on the function runner.
3. Configuring private registry functionality in the function runner's arguments:
   1. Enabling the functionality using the argument *--enable-private-registries*.
   2. Providing the path and name of the mounted secret using the arguments *--registry-auth-secret-path* and *--registry-auth-secret-name* respectively.

### 1.1 Kubernetes secret setup for private registry using docker configuration

An example template of what a docker *config.json* file looks like is as follows below. The base64 encoded value *bXlfdXNlcm5hbWU6bXlfcGFzc3dvcmQ=* of the *auth* key decodes to *my_username:my_password*, which is the format used by the configuration when authenticating.

```json
{
    "auths": {
        "https://index.docker.io/v1/": {
            "auth": "bXlfdXNlcm5hbWU6bXlfcGFzc3dvcmQ="
        },
        "ghcr.io": {
            "auth": "bXlfdXNlcm5hbWU6bXlfcGFzc3dvcmQ="
        }
    }
}
```

A quick way to generate this secret for your use using your docker *config.json* would be to run the following command:

```bash
kubectl create secret generic <SECRET_NAME> --from-file=.dockerconfigjson=/path/to/your/config.json --type=kubernetes.io/dockerconfigjson --dry-run=client -o yaml -n porch-system
```

{{% alert title="Note" color="primary" %}}
The secret must be in the same namespace as the function runner deployment. By default, this is the *porch-system* namespace.
{{% /alert %}}

This should generate a secret template, similar to the one below, which you can add to the *2-function-runner.yaml* file in the Porch catalog package found [here](https://github.com/nephio-project/catalog/tree/main/nephio/core/porch)

```yaml
apiVersion: v1
data:
  .dockerconfigjson: <base64-encoded-data>
kind: Secret
metadata:
  creationTimestamp: null
  name: <SECRET_NAME>
  namespace: porch-system
type: kubernetes.io/dockerconfigjson
```

### 1.2 Mounting docker configuration secret to the function runner

Next you must mount the secret as a volume on the function runner deployment. Add the following sections to the Deployment object in the *2-function-runner.yaml* file:

```yaml
    volumeMounts:
      - mountPath: /var/tmp/auth-secret
        name: docker-config
        readOnly: true
volumes:
  - name: docker-config
    secret:
      secretName: <SECRET_NAME>
```

You may specify your desired paths for each `mountPath:` so long as the function runner can access them.

{{% alert title="Note" color="primary" %}}
The chosen `mountPath:` should use its own, dedicated sub-directory, so that it does not overwrite access permissions of the existing directory. For example, if you wish to mount on `/var/tmp` you should use `mountPath: /var/tmp/<SUB_DIRECTORY>` etc.
{{% /alert %}}

### 1.3 Configuring function runner environment variables for private registries

Lastly you must enable private registry functionality along with providing the path and name of the secret. Add the `--enable-private-registries`, `--registry-auth-secret-path` and `--registry-auth-secret-name` arguments to the function-runner Deployment object in the *2-function-runner.yaml* file:

```yaml
command:
  - --enable-private-registries=true
  - --registry-auth-secret-path=/var/tmp/auth-secret/.dockerconfigjson
  - --registry-auth-secret-name=<SECRET_NAME>
```

The `--enable-private-registries`, `--registry-auth-secret-path` and `--registry-auth-secret-name` arguments have default values of *false*, */var/tmp/auth-secret/.dockerconfigjson* and *auth-secret* respectively; however, these should be overridden to enable the functionality and match user specifications.

With this last step, if your Porch package uses kpt function images stored in an private registry (for example `- image: ghcr.io/private-registry/set-namespace:customv2`), the function runner will now use the secret info to replicate your secret on the `porch-fn-system` namespace and specify it as an `imagePullSecret` for the function pods, as documented [here](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/).

## 2. Configuring function runner to use custom TLS for private container registries

If your private container registry uses a custom certificate for TLS authentication then extra configuration is required for the function runner to integrate with. See below

1. Creating a Kubernetes secret using TLS information valid for all private registries you wish to use.
2. Mounting the secret containing the registries' TLS information to the function runner similarly to step 2.
3. Enabling TLS functionality and providing the path of the mounted secret to the function runner using the arguments *--enable-private-registries-tls* and *--tls-secret-path* respectively.

### 2.1 Kubernetes secret layout for TLS certificate

A typical secret containing TLS information will take on the a similar format to the following:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: <TLS_SECRET_NAME>
  namespace: porch-system
data:
  <CA_FILE_NAME>: <PEM_CERT_DATA>
type: kubernetes.io/tls
```

{{% alert title="Note" color="primary" %}}
The content in *<PEM_CERT_DATA>* must be in PEM (Privacy Enhanced Mail) format, and the *<CA_FILE_NAME>* must be *ca.crt* or *ca.pem*. No other values are accepted.
{{% /alert %}}

### 2.2 Mounting TLS certificate secret to the function runner

The TLS secret must then be mounted onto the function runner similarly to how the docker configuration secret was done previously in section 1.2

```yaml
    volumeMounts:
      - mountPath: /var/tmp/tls-secret/
        name: tls-registry-config
        readOnly: true
volumes:
  - name: tls-registry-config
    secret:
      secretName: <TLS_SECRET_NAME>
```

### 2.3 Configuring function runner environment variables for TLS on private registries

The *--enable-private-registries-tls* and *--tls-secret-path* variables are only required if a private registry has TLS enabled. They indicate to the function runner that it should attempt authentication to the registry using TLS, and should use the TLS certificate information found on the path provided in *--tls-secret-path*.

```yaml
command:
  - --enable-private-registries-tls=true
  - --tls-secret-path=/var/tmp/tls-secret/
```

The *--enable-private-registries-tls* and *--tls-secret-path* arguments have default values of *false* and */var/tmp/tls-secret/* respectively; however, these should be configured by the user and are only necessary when using a private registry secured with TLS.

### Function runner logic flow when TLS registries are enabled

It is important to note that enabling TLS registry functionality makes the function runner attempt connection to the registry provided in the porch file using the mounted TLS certificate. If this certificate is invalid for the provided registry, it will try again using the Intermediate Certificates stored on the machine for use in TLS with "well-known websites" (e.g. GitHub). If this also fails, it will attempt to connect without TLS: if this last resort fails, it will return an error to the user.

{{% alert title="Note" color="primary" %}}
It is vital that the user has pre-configured the Kubernetes node which the function runner is operating on with the same TLS certificate information as is used in the *<TLS_SECRET_NAME>* secret. If this is not configured correctly, then even if the certificate is correctly configured in the function runner, the kpt function will not run - the function runner will be able to pull the image, but the KRM function pod created to run the function will fail with the error *x509 certificate signed by unknown authority*.
This pre-configuration setup is heavily cluster/implementation-dependent - consult your cluster's specific documentation about adding self-signed certificates or private/internal CA certs to your cluster.
{{% /alert %}}
