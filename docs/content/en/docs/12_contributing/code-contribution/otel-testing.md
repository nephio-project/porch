---
title: "OTEL E2E Testing"
type: docs
weight: 5
description: Run OTEL E2E tests to validate trace and metrics export from all Porch components
---

This guide explains how to run the automated OTEL E2E test suite locally and in CI to validate that Porch components correctly export traces and metrics to observability backends.

## Overview

The OTEL E2E testing suite validates:
- ✅ All Porch components (porch-server, porch-controllers, function-runner) export traces to Jaeger
- ✅ All components export metrics on port 9464
- ✅ Real Porch operations generate properly instrumented traces
- ✅ No critical OTEL initialization or export errors

## Prerequisites

- Running Kind cluster with Porch deployed (see [Local Development Environment]({{% relref "/docs/6_configuration_and_deployments/deployments/local-dev-env-deployment" %}}))
- kubectl configured to access the cluster
- `jq` command-line JSON processor (for validation output)

## Local Testing

### Run OTEL E2E Tests

The test suite includes deployment, test execution, and validation in a single command:

```bash
# v1alpha1 with DB cache
make test-e2e-otel-db-cache

# v1alpha2 with DB cache and CRD
make test-e2e-otel-v1alpha2
```

Each command:
1. Deploys Porch with DB cache
2. Deploys monitoring stack (Prometheus, Grafana, Jaeger)
3. Runs a lightweight E2E test to generate activity
4. **Leaves deployment running** for debugging (see [Access Observability UIs](#access-observability-uis))

### Validate OTEL After Test

After the test completes, validate OTEL infrastructure:

```bash
# v1alpha1 validation
./scripts/testing/validate-otel.sh v1alpha1

# v1alpha2 validation
./scripts/testing/validate-otel.sh v1alpha2
```

The validation script checks:
1. ✅ Jaeger is running
2. ✅ Porch-server metrics endpoint responds
3. ✅ OTEL initialized without errors
4. ✅ Traces are captured in Jaeger
5. ✅ Lists all Porch components exporting traces

Example output:
```
=== OTEL Validation Checks for v1alpha2 ===
=== 1. Checking Jaeger availability ===
✓ Jaeger pod found: jaeger-66f5c97cb8-skllc
=== 2. Checking porch-server metrics endpoint ===
✓ Metrics endpoint responding
# HELP aggregator_discovery_aggregation_count_total [ALPHA] Counter
...
=== 6. Services reporting traces to Jaeger ===
✓ Porch components exporting traces:
  - porch-server
  - porch-controllers
  - porch-function-runner
```

## Access Observability UIs

While the test stack is running, access services via port-forward:

### Jaeger (Distributed Tracing)

```bash
# Terminal 1
kubectl port-forward -n porch-monitoring deployment/jaeger 16686:16686

# Terminal 2 - visit http://localhost:16686
# Query service: "porch-server", "porch-controllers", or "porch-function-runner"
# View operation spans like:
# - [START]::packageRevisions::Watch
# - cadEngine::ListPackageRevisions
# - dbpackagesql::pkgScanRowsFromDB
```

### Prometheus (Metrics)

```bash
# Terminal 1
kubectl port-forward -n porch-monitoring deployment/prometheus 9090:9090

# Terminal 2 - visit http://localhost:9090
# Query: http_server_requests_total, grpc_server_requests_total, etc.
```

### Grafana (Dashboards)

```bash
# Terminal 1
kubectl port-forward -n porch-monitoring deployment/grafana 3000:3000

# Terminal 2 - visit http://localhost:3000
# Default user: porch
# Password: (check logs or extract from grafana-admin-creds secret)
```

## CI Integration

### GitHub Actions Workflow

The OTEL E2E tests run automatically in GitHub Actions:

**File**: `.github/workflows/porch-e2e-otel-weekly.yaml`

**Triggers**:
- **Scheduled**: Every Sunday at 2 AM UTC
- **Manual**: Trigger via GitHub Actions UI (`workflow_dispatch`)

**What it does**:
1. Builds Porch images (parallel)
2. Runs **v1alpha1 E2E tests** (parallel job)
3. Runs **v1alpha2 E2E tests** (parallel job)
4. Each job:
   - Deploys Porch + monitoring stack
   - Runs lightweight E2E test (~20-30 seconds)
   - Validates OTEL infrastructure
   - Reports which components export traces

**Matrix Strategy**: Both API versions run in parallel for fast feedback (~45 min total).

## Test Selection

### v1alpha1: TestRegisterRepository
- **Operation**: Registers a git repository
- **Why**: Lightweight, no function rendering
- **Duration**: ~20-30 seconds
- **Traces**: API calls and repository operations

### v1alpha2: Metrics Tests
- **Operation**: Validates metrics collection via Ginkgo
- **Why**: Lightweight, infrastructure-focused
- **Duration**: ~20-30 seconds
- **Traces**: Package operations and metrics collection

Both tests generate real Porch operations that are instrumented and traced.

## Make Targets

```bash
# Deploy + test + leave running (v1alpha1 DB cache)
make test-e2e-otel-db-cache

# Deploy + test + leave running (v1alpha2 DB cache + CRD)
make test-e2e-otel-v1alpha2
```

## Troubleshooting

### No traces in Jaeger

1. **Check Porch components are running**:
   ```bash
   kubectl get pods -n porch-system
   ```

2. **Check OTEL env vars are set**:
   ```bash
   kubectl get deployment porch-server -n porch-system \
     -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="OTEL_TRACES_EXPORTER")].value}'
   ```

3. **Check Jaeger OTLP endpoint reachable**:
   ```bash
   kubectl logs -n porch-system deployment/porch-server | grep "jaeger-otlp\|traces export"
   ```

### Metrics endpoint not responding

1. **Check port-forward is active**:
   ```bash
   curl http://localhost:9464/metrics | head
   ```

2. **Check porch-server pod is healthy**:
   ```bash
   kubectl get pod -n porch-system -l app=porch-server
   kubectl logs -n porch-system -l app=porch-server | grep "ERROR\|error"
   ```

### Export errors in logs

- "context deadline exceeded" - transient connection issue during startup, not critical
- "export failed" - actual export failure, check logs and Jaeger connectivity

## Cleanup

```bash
# Stop port forwarding (optional, happens automatically)
find /tmp/tmp*_porch-monitoring-pf.pid.d/ -name '*.pid' -exec pkill -F '{}' \;

# Destroy Porch deployment
make destroy

# Remove monitoring stack
./scripts/monitoring/deploy-monitoring.sh remove
```

## Next Steps

- Configure OTEL exporters: [OpenTelemetry Configuration]({{% relref "/docs/6_configuration_and_deployments/configurations/opentelemetry" %}})
- Deploy monitoring stack: [Local Performance Monitoring Deployment]({{% relref "/docs/6_configuration_and_deployments/deployments/local-performance-monitoring-deployment" %}})
- Run performance tests: [Performance Tests]({{% relref "/docs/12_contributing/code-contribution/performance-tests" %}})

