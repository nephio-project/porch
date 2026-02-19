---
title: "Sync Behavior"
type: docs
weight: 1
description: |
  Health checks, full sync operations, and sync decision logic.
---

## Overview

The Repository controller uses two types of sync operations to keep repositories up-to-date:

- **Health checks**: Lightweight connectivity validation that runs frequently to detect failures quickly
- **Full syncs**: Complete repository synchronization that runs less often to fetch content and discover packages

The controller intelligently decides which operation to perform based on repository state and configured scheduling.

## Sync Operations

### Health Check

Health checks perform quick connectivity validation without fetching repository contents. They use a 20-second timeout and execute synchronously in the reconcile loop, validating only that the repository is reachable without the overhead of git operations.

Health checks only run when the repository is healthy. If the repository has an error condition, health checks are skipped and only error retry logic determines when to attempt recovery.

### Full Sync

Full syncs perform complete repository synchronization, fetching the latest content and discovering all packages. They execute asynchronously with semaphore limiting to control concurrency.

After the first successful sync, the repository instance is cached, making subsequent syncs much faster. The timing varies based on repository size and network conditions. Cached operations are typically more efficient.

## Sync Decision Priority

The controller evaluates several conditions to determine which operation to perform:

1. **Sync in progress** → Skip (with 20-minute stale detection)
2. **RunOnceAt due** → Full sync (highest priority)
3. **Spec changed** → Full sync (unless RunOnceAt pending)
4. **Error retry due** → Full sync (ensures repository gets cached)
5. **Full sync due** → Full sync (periodic, based on frequency or schedule)
6. **Health check due** → Health check (only when healthy)

This priority order ensures that one-time syncs execute immediately, spec changes trigger immediate updates, and error recovery gets priority over routine operations.

## Stale Sync Detection

The controller monitors for syncs that get stuck in the "Reconciling" state for more than 20 minutes. When detected, these stale syncs are abandoned and new operations can start. This provides automatic recovery from goroutine panics or process restarts without manual intervention.

The `meta.SetStatusCondition()` function preserves timestamps during this process, ensuring accurate tracking of when operations actually complete.

## Sync Scheduling

### Frequency-Based Scheduling

By default, health checks run every 5 minutes and full syncs run every hour. These intervals can be configured using command-line flags (see [Repository Controller Configuration]({{% relref "/docs/6_configuration_and_deployments/configurations/components/porch-controllers-config.md#repository-controller-configuration" %}})):

```bash
--repositories.health-check-frequency=5m
--repositories.full-sync-frequency=1h
```

### Cron-Based Scheduling

Repositories can override the default full sync frequency with a cron schedule (see [Repository Cron Sync Configuration]({{% relref "/docs/6_configuration_and_deployments/configurations/repository-sync.md#schedule-field" %}})):

```yaml
spec:
  sync:
    schedule: "0 */6 * * *"  # Every 6 hours
```

The schedule uses standard cron expression format and calculates the next sync time from the `lastFullSyncTime` status field. This ensures accurate scheduling even when health checks or errors update other timestamps.

### One-Time Sync

For immediate synchronization, the `runOnceAt` field schedules a one-time sync at a specific time (see [Repository One-time Sync Configuration]({{% relref "/docs/6_configuration_and_deployments/configurations/repository-sync.md#runonceat-field" %}})):

```yaml
spec:
  sync:
    runOnceAt: "2025-01-15T10:30:00Z"
```

This operation has the highest priority and executes before any other scheduled operations. After execution, the field is automatically cleared. The `observedRunOnceAt` status field tracks this to prevent duplicate syncs when the field is set, updated, or cleared.

The CLI provides a convenient way to trigger one-time syncs (see [porchctl repo sync]({{% relref "/docs/7_cli_api/porchctl.md#repo-sync" %}})):

```bash
porchctl repo sync <repo-name> --run-once 5m
```

## Sync Behavior During Errors

When a repository enters an error state, the controller changes its behavior to focus on recovery. Health checks are blocked since they can't help fix the underlying problem. Instead, error retry logic determines when to attempt recovery using a full sync operation.

An error retry takes precedence over periodic sync scheduling. When error conditions exist, periodic syncs are skipped to prevent conflicts between recovery attempts and routine operations. This ensures the repository gets properly cached during recovery rather than just checking connectivity.

For detailed error-specific retry intervals and recovery strategies, see [Error Handling]({{% relref "/docs/5_architecture_and_components/controllers/repository-controller/functionality/error-handling.md#overview" %}}).

