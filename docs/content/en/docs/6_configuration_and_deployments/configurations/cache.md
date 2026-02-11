---
title: "Cache"
type: docs
weight: 2
description: "Configure Porch caching mechanisms"
---

Porch supports two caching mechanisms to store package metadata and improve performance.

## Cache Types

### CR Cache (Default)

By default, Porch uses Custom Resource (CR) cache, which stores cache data as Kubernetes Custom Resources.

**Advantages:**
- No additional infrastructure required
- Automatic backup with cluster backups
- Simple setup and maintenance

**Use when:**
- Getting started with Porch
- Small to medium deployments
- Simplicity is preferred

### Database Cache

Database cache uses PostgreSQL to store cache data, providing better performance for large deployments.

**Advantages:**
- Better performance with large datasets
- Advanced querying capabilities
- Separate scaling from Kubernetes cluster

**Use when:**
- Large-scale deployments
- High-performance requirements
- Advanced querying needs

## Switching to Database Cache

### Prerequisites

- PostgreSQL database (v12+) **running and accessible**
- Database credentials and connection details
- Database must have the specified database created (e.g., `porch`)

{{% alert title="Important" color="warning" %}}
Ensure your PostgreSQL instance is running and accessible from the Porch server before configuring database cache. Porch will fail to start if it cannot connect to the database.
{{% /alert %}}

### Configuration Steps

1. **Create Database Secret:**

```bash
kubectl create secret generic porch-db-config \
  --namespace=porch-system \
  --from-literal=host=postgresql.example.com \
  --from-literal=port=5432 \
  --from-literal=database=porch \
  --from-literal=username=porch_user \
  --from-literal=password=your_password
```

2. **Configure Database Cache:**

**For Catalog Deployments (Pre-deployment):**

Modify the package configuration before deployment:

```bash
# Get the catalog package
kpt pkg get https://github.com/nephio-project/catalog/tree/main/nephio/core/porch
cd porch/
# Edit the porch-server deployment YAML in the package
```

**For Manual Deployments (Post-deployment):**

Update an existing deployment:

```bash
kubectl edit deployment -n porch-system porch-server
```

**Configuration (both methods):**

Add database configuration to the porch-server deployment:

```yaml
spec:
  template:
    spec:
      containers:
      - name: porch-server
        args:
        - --cache-type=DB
        env:
        - name: DB_DRIVER
          value: "pgx"
        - name: DB_HOST
          valueFrom:
            secretKeyRef:
              name: porch-db-config
              key: host
        - name: DB_PORT
          valueFrom:
            secretKeyRef:
              name: porch-db-config
              key: port
        - name: DB_NAME
          valueFrom:
            secretKeyRef:
              name: porch-db-config
              key: database
        - name: DB_USER
          valueFrom:
            secretKeyRef:
              name: porch-db-config
              key: username
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: porch-db-config
              key: password
```

3. **Restart Porch Server:**

```bash
kubectl rollout restart deployment/porch-server -n porch-system
```

## Local Development with Database Cache

For local development with database cache, see the [Local Development Environment]({{% relref "../deployments/local-dev-env-deployment" %}}) guide which covers both CR cache and database cache options.

## Monitoring Cache Performance

### Metrics

Porch exposes cache-related metrics:

```bash
# Check cache hit rates
kubectl port-forward -n porch-system svc/porch-server 8080:8080
curl http://localhost:8080/metrics | grep cache
```

### Logs

Monitor cache operations:

```bash
kubectl logs -n porch-system -l app=porch-server -f | grep cache
```

## Troubleshooting

### Common Issues

**Database connection failures:**
```bash
# Check secret configuration
kubectl get secret porch-db-config -n porch-system -o yaml

# Verify database connectivity
kubectl run -it --rm debug --image=postgres:12 --restart=Never -- \
  psql -h postgresql.example.com -U porch_user -d porch
```

**Performance issues:**
- Monitor database performance
- Check network latency between Porch and database
- Review database indexes and query performance
