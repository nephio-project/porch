---
title: "Repository Controller"
type: docs
weight: 1
description: |
  Kubernetes controller for repository synchronization and lifecycle management.
---

## Overview

The Repository Controller manages the synchronization of [Porch repositories]({{% relref "/docs/2_concepts/repositories.md" %}}) with external Git sources. It continuously monitors repositories and keeps their package content up-to-date through periodic synchronization.

The controller handles several key responsibilities:

- Synchronizes repositories with external Git sources on configurable schedules
- Performs lightweight health checks to detect connectivity issues quickly
- Executes full sync operations to fetch content and discover packages
- Maintains repository status with sync timestamps, package counts, and git commit hashes
- Implements smart retry logic with error-type-specific intervals
- Controls concurrency to prevent resource exhaustion

## How It Works

The controller operates as a standard Kubernetes controller, watching Repository custom resources and reconciling their desired state with actual state:

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│ Repository      │    │ Controller       │    │ Cache Layer     │
│ CRD             │───▶│ Reconcile Loop   │───▶│ (CR/DB Cache)   │
│                 │    │                  │    │                 │
│ • Git config    │    │ • Sync decision  │    │ • Package data  │
│ • Sync schedule │    │ • Async workers  │    │ • Git cache     │
│ • Credentials   │    │ • Status update  │    │ • Database      │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

The controller uses a dual sync strategy to balance responsiveness with efficiency. Health checks run frequently to detect problems quickly, while full syncs run less often to fetch content and discover packages. This approach minimizes unnecessary git operations while maintaining up-to-date repository state.

When repositories encounter errors, the controller automatically retries with intervals tailored to the error type. The controller also detects stale syncs and automatically recovers.

## Key Features

**Intelligent Sync Scheduling**: The controller prioritizes operations based on urgency, ensuring one-time syncs and spec changes execute immediately while routine operations happen on schedule.

**Flexible Configuration**: Repositories can use frequency-based scheduling, cron expressions, or one-time syncs to control when synchronization happens.

**Production-Grade Reliability**: Automatic retry with smart backoff, stale sync detection, and concurrent operation limiting ensure reliable operation at scale.

**Rich Status Information**: The controller maintains detailed status to support monitoring and troubleshooting.

## Configuration

For cache configuration, see [Cache Configuration]({{% relref "/docs/6_configuration_and_deployments/configurations/cache.md" %}}).

For controller-specific settings, see [Repository Controller Configuration]({{% relref "/docs/6_configuration_and_deployments/configurations/components/porch-controllers-config.md#repository-controller-configuration" %}}).
