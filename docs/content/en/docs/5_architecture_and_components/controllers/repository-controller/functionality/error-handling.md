---
title: "Error Handling"
type: docs
weight: 2
description: |
  Error-specific retry intervals, recovery strategies, and failure handling.
---

## Overview

When repository operations fail, the controller uses intelligent retry logic based on error type. Different errors require different retry strategies - transient network issues retry quickly, while authentication problems that need manual intervention retry less frequently to avoid wasting resources.

For how errors affect sync scheduling behavior, see [Sync Behavior During Errors]({{% relref "/docs/5_architecture_and_components/controllers/repository-controller/functionality/sync-behavior.md#sync-behavior-during-errors" %}}).

## Error-Specific Retry Intervals

The controller analyzes error messages to determine appropriate retry timing:

| Error Type | Retry Interval | Error Patterns | Rationale |
|------------|----------------|----------------|----------|
| **Network Issues** | 30 seconds | "no such host", "connection refused" | Often transient, retry quickly |
| **Authentication** | 10 minutes | "authentication required", "permission denied" | Needs manual fix, avoid spam |
| **Git Repo/Branch** | 2 minutes | "not found", "invalid", "branch" | May be transient (repo creation) |
| **Timeouts** | 1 minute | "timeout", "deadline exceeded" | Network congestion, moderate retry |
| **TLS/SSL Issues** | 5 minutes | "certificate", "tls", "ssl" | Configuration issues, less frequent |
| **Rate Limiting** | 5 minutes | "rate limit", "too many requests" | Need to back off from API |
| **Default** | 30 seconds | All other errors | Configurable fallback |

## Recovery Behavior

When a repository enters an error state, the controller adjusts its behavior to focus on recovery:

- **Health checks are blocked** - They can't help fix the underlying problem
- **Periodic syncs are skipped** - Prevents conflicts with recovery attempts  
- **Error retry takes precedence** - Uses full sync to ensure repository gets properly cached
- **Retry timing is error-specific** - Based on the table above

The controller calculates the next retry time using the error's last transition time plus the error-specific interval. This prevents wasteful frequent retries for persistent configuration errors while allowing quick recovery from transient issues.

## Status Representation

Repository status reflects the current state and provides visibility into sync timing:

**Healthy Repository:**
```yaml
status:
  nextFullSyncTime: "2026-01-12T23:44:21Z"
  conditions:
  - type: RepositoryReady
    status: "True"
    message: "Repository Ready"
```

**Error State:**
```yaml
status:
  conditions:
  - type: RepositoryReady
    status: "False"
    reason: Error
    message: "Failed to sync repository: authentication required"
    lastTransitionTime: "2026-01-12T23:34:21Z"
```

The controller uses the `lastTransitionTime` plus the error-specific interval to determine when to retry. Error messages focus on describing the failure rather than embedding retry timestamps.

## Recovery Strategies by Error Type

The controller applies different strategies based on whether errors are likely transient or persistent:

**Transient Errors** (quick retry):
- Network issues (30s) - Connection problems often resolve quickly
- Timeouts (1m) - May be temporary network congestion
- Git repo issues (2m) - Repository might be in process of creation

**Persistent Errors** (slower retry):
- Authentication (10m) - Requires manual credential updates
- TLS/SSL (5m) - Configuration issues need manual fixes
- Rate limiting (5m) - Need to back off from API calls

This approach balances quick recovery from transient issues with resource efficiency for problems that require manual intervention.
