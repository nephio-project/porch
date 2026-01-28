---
title: "Catalog Deployment"
type: docs
weight: 2
description: "Deploy Porch using the Nephio catalog for production environments"
---

This guide covers deploying Porch in production environments using the [Nephio catalog](https://github.com/nephio-project/catalog/tree/main/nephio/core/porch).

## Configuration Planning

Before deploying Porch, determine which features you need:

### Pre-deployment Configuration Required

These features must be configured **before** deployment:

#### Porch Server
- [Cache Configuration]({{% relref "../configurations/cache" %}}) - Switch to database cache (requires deployment args)
- [Cert-Manager Webhooks]({{% relref "../configurations/components/porch-server-config/cert-manager-webhooks" %}}) - Enable cert-manager webhook integration (requires deployment env vars)
- [Jaeger Tracing]({{% relref "../configurations/components/porch-server-config/jaeger-tracing" %}}) - Enable distributed tracing (requires deployment env vars)
- [Git Custom TLS]({{% relref "../configurations/components/porch-server-config/git-authentication#3-httpstls-configuration" %}}) - Enable custom TLS certificates for Git repositories (requires `--use-git-cabundle=true` arg)

#### Function Runner
- [Private Registries]({{% relref "../configurations/components/function-runner-config/private-registries-config" %}}) - Configure private container registries (requires deployment args and volume mounts)

### Post-deployment Configuration

These features can be configured **after** deployment:

- [Git Authentication]({{% relref "../configurations/components/porch-server-config/git-authentication" %}}) - Configure basic authentication and bearer token for Git repositories

{{% alert title="Note" color="info" %}}
[Repository Sync]({{% relref "../configurations/repository-sync" %}}) configuration is currently located in the system configuration section but should be moved to a more logical location as it's about configuring individual Repository resources, not system-wide settings.
{{% /alert %}}

## Prerequisites

- Kubernetes cluster (v1.25+)
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) configured for your cluster
- [kpt](https://kpt.dev/installation/) CLI tool
- Cluster admin permissions

## Installation Steps

### 1. Get the Porch Package

```bash
kpt pkg get https://github.com/nephio-project/catalog/tree/main/nephio/core/porch
```

### 2. Configure the Package (Optional)

Review and modify the configuration in the `porch/` directory if needed:

```bash
cd porch/
# Review configuration files
ls -la
```

### 3. Render and Apply

```bash
# Render the package with any customizations
kpt fn render porch

# Initialize the package for lifecycle management
kpt live init porch

# Apply to your cluster
kpt live apply porch
```

## Verification

### Check Pod Status

Verify all Porch components are running:

```bash
kubectl get pods -n porch-system
```

Expected output:
```
NAME                                 READY   STATUS    RESTARTS   AGE
function-runner-xxx-xxx              1/1     Running   0          2m
function-runner-xxx-xxx              1/1     Running   0          2m
porch-controllers-xxx-xxx            1/1     Running   0          2m
porch-server-xxx-xxx                 1/1     Running   0          2m
```

### Verify API Resources

Confirm Porch CRDs are registered:

```bash
kubectl api-resources | grep porch
```



## Troubleshooting

### Common Issues

**Pods not starting:**
```bash
kubectl describe pods -n porch-system
kubectl logs -n porch-system -l app=porch-server
```

**CRDs not registered:**
```bash
kubectl get crd | grep porch
```

### Getting Help

For additional support:
- Check the [Porch GitHub issues](https://github.com/nephio-project/porch/issues)
- Join the [Nephio community](https://nephio.org/community/)
