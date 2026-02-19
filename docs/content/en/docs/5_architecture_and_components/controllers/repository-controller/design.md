---
title: "Design Decisions"
type: docs
weight: 2
description: |
  Architectural decisions behind the Repository Controller design.
---

## Overview

This page explains the key architectural decisions that shaped the Repository Controller, focusing on the "why" rather than the "what." Each decision represents a deliberate trade-off made to balance competing concerns like performance, reliability, and operational simplicity.

These decisions address fundamental challenges in distributed systems: how to detect failures quickly without wasting resources, how to handle errors intelligently, and how to provide clear operational visibility. Understanding the rationale helps when troubleshooting issues or considering configuration changes.

## Dual Sync Strategy

**Decision**: Implement both lightweight health checks (5 minutes) and heavy full syncs (1 hour).

**Why**: Balances responsiveness with resource efficiency.

Health checks provide fast failure detection without the overhead of git operations. Full syncs ensure package discovery stays current. Running only health checks would miss new packages; running only full syncs would waste resources on unchanged repositories.

Most repositories don't change frequently, but connectivity issues need quick detection. This asymmetry drives the different intervals.

## Async Full Sync Execution

**Decision**: Full syncs run asynchronously in goroutines with semaphore limiting.

**Why**: Prevents controller blocking and enables concurrent processing.

Git operations can take significant time for large repositories. Synchronous execution would block the reconcile loop, preventing other repositories from being processed. The semaphore prevents resource exhaustion while maximizing throughput.

Health checks remain synchronous because they're fast (20 seconds max) and need immediate results for status updates.

## 20-Minute Stale Detection

**Decision**: Mark syncs as stale after 20 minutes, allowing new operations to start.

**Why**: Provides automatic recovery without aggressive timeouts.

This handles edge cases like goroutine panics, process restarts, or unexpectedly slow operations. The 20-minute threshold is conservative enough to avoid false positives on large repositories while still providing reasonable recovery time.

Alternative approaches (completion polling, shorter timeouts) would either waste resources or cause false failures.

## Error-Specific Retry Intervals

**Decision**: Different retry intervals based on error type (30 seconds to 10 minutes).

**Why**: Matches retry frequency to likelihood of resolution.

Network errors (30 seconds) are often transient and resolve quickly. Authentication errors (10 minutes) require manual intervention, so frequent retries waste resources and spam logs. This approach reduces unnecessary load while maintaining responsiveness for recoverable errors.

A single retry interval would either retry too aggressively for persistent errors or too slowly for transient ones.

## No Embedded Retry Timestamps

**Decision**: Status messages don't include "next retry at" timestamps.

**Why**: Simplifies status updates and prevents timing drift.

Early implementations embedded timestamps in messages, but this required updating status on every intermediate reconcile even when no sync occurred. This caused unnecessary API calls and made the actual retry time drift from the displayed time.

The `nextFullSyncTime` status field provides explicit scheduling visibility without the update overhead.

## Sync Priority Hierarchy

**Decision**: Strict priority order: in-progress > runOnceAt > spec-change > error-retry > periodic > health-check.

**Why**: Ensures critical operations aren't delayed by routine checks.

User-triggered one-time syncs (`runOnceAt`) take precedence over periodic syncs because they represent explicit user intent. Spec changes trigger immediate syncs because configuration updates should take effect quickly. Error retries use full syncs (not health checks) to ensure repositories get properly cached after recovery.

This hierarchy prevents scenarios like health checks delaying urgent spec changes or periodic syncs interfering with scheduled one-time operations.

## ObservedRunOnceAt Tracking

**Decision**: Track `observedRunOnceAt` in status to prevent duplicate syncs.

**Why**: Distinguishes between setting/clearing `runOnceAt` and other spec changes.

Without this field, setting `runOnceAt` would trigger two syncs: one from the spec change and one when the scheduled time arrives. The controller now recognizes when only `runOnceAt` changed and waits for the scheduled time without an immediate sync.

This also prevents duplicate syncs when clearing `runOnceAt` after completion.

## Separate Time Sources

**Decision**: Use `lastFullSyncTime` for scheduling, `condition.lastTransitionTime` for staleness.

**Why**: Different purposes require different semantics.

Cron schedules need to calculate from the last successful full sync, not from health checks or error state changes. Using condition timestamps for cron calculations would cause schedule drift (health checks would reset the timer).

The separation makes each field's purpose explicit and prevents subtle timing bugs.

## Health Checks Skip During RunOnceAt Wait

**Decision**: No health checks while waiting for scheduled `runOnceAt` time.

**Why**: Respects user intent for specific sync timing.

When a user schedules a one-time sync, they want that sync to happen at that time, not before. Health checks could trigger syncs earlier than intended. Since a full sync is imminent anyway, health checks provide no value during the wait period.

This also simplifies the sync decision logic by giving `runOnceAt` absolute priority.

## Git Cache Locking

**Decision**: Per-repository exclusive locks using `sync.RWMutex`.

**Why**: Git repository structure requires serialized access.

Git operations modify shared state (object database, references, index) that can't be safely accessed concurrently. While `RWMutex` theoretically allows concurrent reads, almost all git operations need exclusive access in practice.

The per-repository granularity allows different repositories to sync in parallel while preventing corruption within a single repository.

## Controller-Runtime Foundation

**Decision**: Built on Kubernetes controller-runtime framework.

**Why**: Leverages proven patterns and reduces custom code.

Controller-runtime provides battle-tested implementations of:
- Watch management and event handling
- Work queue with rate limiting
- Leader election for high availability
- Metrics and health checks
- Graceful shutdown

Building these from scratch would introduce bugs and maintenance burden. The framework's conventions also make the code familiar to Kubernetes developers.

## Related Documentation

- {{% relref "/docs/5_architecture_and_components/controllers/repository-controller/functionality/_index.md" %}} - What the controller does
- {{% relref "/docs/6_configuration_and_deployments/configurations/repository-sync.md" %}} - How to configure it
- {{% relref "/docs/6_configuration_and_deployments/configurations/cache.md" %}} - Cache configuration
