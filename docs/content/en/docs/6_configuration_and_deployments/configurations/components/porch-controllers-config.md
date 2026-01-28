---
title: "Porch Controllers"
type: docs
weight: 2
description: "Configure the Porch controllers component"
---

The Porch controllers manage the lifecycle of PackageVariants and PackageVariantSets.

## Available Controllers

Porch includes the following controllers:

- **PackageVariant Controller** - Manages PackageVariant resources
- **PackageVariantSet Controller** - Manages PackageVariantSet resources

## Configuration Options

### Command Line Arguments

The controllers support these command line arguments:

```bash
args:
- --reconcilers=packagevariants,packagevariantsets  # Comma-separated list of controllers to enable
# OR use --reconcilers=* to enable all controllers
```

### Default Configuration

The controllers use these hardcoded default settings (not configurable):

- **Metrics bind address**: `:8080`
- **Health probe bind address**: `:8081`
- **Webhook port**: `9443`
- **Leader election**: Disabled
- **Leader election ID**: `porch-operators.config.porch.kpt.dev`

## Resource Limits

```yaml
resources:
  requests:
    memory: "128Mi"
    cpu: "100m"
  limits:
    memory: "512Mi"
    cpu: "500m"
```

## Health Checks

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8081
  initialDelaySeconds: 30
  periodSeconds: 30
  failureThreshold: 3
  timeoutSeconds: 5

readinessProbe:
  httpGet:
    path: /readyz
    port: 8081
  initialDelaySeconds: 5
  periodSeconds: 5
  failureThreshold: 3
  timeoutSeconds: 3
```

## RBAC Requirements

The controllers require these permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: porch-controllers
rules:
# Core Porch resources
- apiGroups: ["config.porch.kpt.dev"]
  resources: ["*"]
  verbs: ["*"]
- apiGroups: ["porch.kpt.dev"]
  resources: ["*"]
  verbs: ["*"]
# Leader election
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
# Events
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

## Troubleshooting

### Check Controller Status

```bash
# Check if controllers are running
kubectl get pods -n porch-system -l k8s-app=porch-controllers

# Check controller logs
kubectl logs -n porch-system deployment/porch-controllers
```

### Verify Enabled Controllers

Check the deployment arguments:

```bash
kubectl get deployment -n porch-system porch-controllers -o yaml | grep -A5 args
```

### Common Issues

- **Controllers not starting**: Check RBAC permissions and ensure required CRDs are installed
- **PackageVariants not reconciling**: Verify `--reconcilers` argument includes `packagevariants`
- **Leader election conflicts**: Multiple controller instances may cause conflicts if leader election is enabled