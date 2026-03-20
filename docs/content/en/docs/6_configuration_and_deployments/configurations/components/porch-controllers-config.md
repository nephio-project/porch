---
title: "Porch Controllers"
type: docs
weight: 2
description: "Configure the Porch controllers component"
---

The Porch controllers manage Repository synchronization, PackageVariants, and PackageVariantSets.

## Enabling Controllers

### Command Line Arguments

The controllers support these command line arguments:

```bash
args:
- --reconcilers=repositories,packagevariants,packagevariantsets  # Comma-separated list
# OR use --reconcilers=* to enable all controllers
```

### Repository Controller Configuration

The Repository Controller supports these additional flags:

```bash
args:
- --reconcilers=repositories
- --repositories.max-concurrent-reconciles=100
- --repositories.max-concurrent-syncs=50
- --repositories.health-check-frequency=5m
- --repositories.full-sync-frequency=1h
- --repositories.cache-type=CR  # or DB
```

**Configuration Parameters:**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `max-concurrent-reconciles` | 100 | Parallel reconcile loops |
| `max-concurrent-syncs` | 50 | Parallel sync operations |
| `health-check-frequency` | 5m | Lightweight connectivity checks |
| `full-sync-frequency` | 1h | Complete repository sync |
| `cache-type` | CR | Cache implementation (CR or DB) - see [Cache Configuration]({{% relref "/docs/6_configuration_and_deployments/configurations/cache.md" %}}) |

**Cache Type:**

The `cache-type` parameter determines how package data is stored:
- **CR**: Custom Resources for metadata, in-memory caching (simpler, no database required)
- **DB**: PostgreSQL database for metadata and content (production-grade, scalable)

{{% alert title="Note" color="info" %}}
When using `--repositories.cache-type=DB`, you must also configure database connection settings via environment variables. See [Cache Configuration]({{% relref "/docs/6_configuration_and_deployments/configurations/cache.md" %}}) for complete setup instructions.
{{% /alert %}}

**Tuning Guidance:**

Adjust these parameters based on your deployment characteristics:

- **Concurrency settings** (`max-concurrent-reconciles`, `max-concurrent-syncs`):
  - Higher values increase throughput but consume more resources
  - Start with defaults and adjust based on observed CPU/memory usage
  - Monitor controller logs for reconciliation delays

- **Frequency settings** (`health-check-frequency`, `full-sync-frequency`):
  - More frequent checks detect issues faster but increase load
  - Less frequent checks reduce overhead but delay change detection
  - Balance based on your tolerance for sync lag vs resource usage

For detailed sync behavior and scheduling, see [Repository Sync Configuration]({{% relref "/docs/6_configuration_and_deployments/configurations/repository-sync.md" %}}).