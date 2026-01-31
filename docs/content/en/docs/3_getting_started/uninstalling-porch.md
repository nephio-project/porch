---
title: "Uninstalling Porch"
type: docs
weight: 3
description: Guide for removing Porch from your Kubernetes cluster.
---

## Uninstalling Porch Server

Navigate to the directory where you installed Porch:

```bash
cd porch-{{% params "latestTag" %}}
```

Remove Porch components using kpt:

```bash
kpt live destroy
```

This will delete all Porch resources from your cluster, including:
- Porch server deployment
- Function runner deployment
- Porch controllers deployment
- Services, ConfigMaps, and Secrets
- Custom Resource Definitions (CRDs)

{{% alert color="warning" title="Warning" %}}
Destroying Porch will **not** delete your Git repositories or the packages stored in them. However, PackageRevision and Repository resources in your cluster will be removed.
{{% /alert %}}

## Verify Uninstallation

Check that Porch components are removed:

```bash
kubectl get all -n porch-system
```

You should see:

```bash
No resources found in porch-system namespace.
```

Verify Porch API resources are removed:

```bash
kubectl api-resources | grep porch
```

This should return no results.

## Uninstalling porchctl CLI

Remove the porchctl binary from your system:

**If installed to `/usr/local/bin/` (requires root):**

```bash
sudo rm /usr/local/bin/porchctl
```

**If installed to `~/.local/bin/`:**

```bash
rm ~/.local/bin/porchctl
```

**Remove autocompletion (if configured):**

```bash
rm ~/.local/share/bash-completion/completions/porchctl
```

## Verify CLI Removal

Check that porchctl is no longer available:

```bash
porchctl version
```

You should see:

```bash
bash: porchctl: command not found
```

## Clean Up Installation Files

Remove the downloaded Porch package directory:

```bash
cd ..
rm -rf porch-{{% params "latestTag" %}}
```

## Troubleshooting

### Resources not deleting

If `kpt live destroy` hangs or fails:

1. Check for finalizers blocking deletion:
   ```bash
   kubectl get packagerevisions -A -o yaml | grep finalizers
   ```

2. Force delete stuck resources:
   ```bash
   kubectl delete packagerevisions --all -A --force --grace-period=0
   kubectl delete repositories --all -A --force --grace-period=0
   ```

3. Manually delete the namespace:
   ```bash
   kubectl delete namespace porch-system --force --grace-period=0
   ```

### CRDs remain after uninstall

If Porch CRDs are still present:

```bash
kubectl get crds | grep porch
```

Manually delete them:

```bash
kubectl delete crd packagerevisions.porch.kpt.dev
kubectl delete crd packagerevisionresources.porch.kpt.dev
kubectl delete crd repositories.config.porch.kpt.dev
kubectl delete crd functions.config.porch.kpt.dev
kubectl delete crd packagevariants.config.porch.kpt.dev
kubectl delete crd packagevariantsets.config.porch.kpt.dev
```

## Complete Cluster Cleanup

If you're using a local test cluster (kind/minikube) and want to start fresh:

**kind:**
```bash
kind delete cluster
```

**minikube:**
```bash
minikube delete
```

This removes the entire cluster, including Porch and all other resources.
