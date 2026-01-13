---
title: "Repository Sync Configuration"
type: docs
weight: 1
description: Configure repository synchronization for Porch Repositories
---

## Sync Configuration Fields

The `spec.sync` field in a Repository CR controls synchronization behavior with the external repository. Repositories without sync configuration use the system default for periodic synchronization (10 minutes, overridden by RepoSyncFrequency parameter in porch-server deployment).

```yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: Repository
metadata:
  name: example-repo
  namespace: default
spec:
  sync:
    schedule: "*/10 * * * *"           # Periodic sync using cron expression
    runOnceAt: "2024-01-15T10:30:00Z"  # One-time sync at specific time
```

### Schedule Field

The `schedule` field accepts standard [cron expressions](https://en.wikipedia.org/wiki/Cron) for periodic synchronization:

- **Format**: Standard 5-field cron expression (`minute hour day month weekday`)
- **Examples**:
  - `"*/10 * * * *"` - Every 10 minutes
  - `"0 */2 * * *"` - Every 2 hours
  - `"0 9 * * 1-5"` - 9 AM on weekdays
  - `"0 0 * * 0"` - Weekly on Sunday at midnight

### RunOnceAt Field

The `runOnceAt` field schedules a one-time sync at a specific timestamp:

- **Format**: RFC3339 timestamp (`metav1.Time`)
- **Examples**:
  - `"2025-01-15T14:30:00Z"` - Sync at 2:30 PM UTC on January 15, 2025
  - `"2025-12-25T00:00:00Z"` - Sync at midnight UTC on Christmas Day
  - `"2025-06-01T09:15:30Z"` - Sync at 9:15:30 AM UTC on June 1st
  - `"2025-12-10T15:45:00-05:00"` - Sync at 3:45 PM EST (UTC-5) on March 10th
- **Behavior**: 
  - Executes once at the specified time
  - Ignored if timestamp is in the past
  - Independent of periodic schedule
  - Can be updated to reschedule

**Note**: One-time syncs should only be used when discrepancies are found between the external repository and Porch cache. Under normal conditions, rely on periodic syncs for regular synchronization.

## Complete Examples

### Git Repository with Periodic Sync

```yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: Repository
metadata:
  name: blueprints
  namespace: default
spec:
  description: Blueprints with hourly sync
  type: git
  sync:
    schedule: "0 * * * *"  # Every hour
  git:
    repo: https://github.com/example/blueprints.git
    branch: main
    directory: packages
```

### Combined Periodic and One-time Sync

```yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: Repository
metadata:
  name: combined-sync
  namespace: default
spec:
  type: git
  sync:
    schedule: "0 */6 * * *"            # Every 6 hours
    runOnceAt: "2024-01-15T09:00:00Z"  # Sync once
  git:
    repo: https://github.com/example/repo.git
    branch: main
```

## Sync Behavior

### Default Behavior
- Without `spec.sync`: Uses system default sync frequency
- Empty `schedule`: Falls back to default frequency
- Invalid cron expression: Falls back to default frequency

**Default frequency**: 10 minutes (overridden by RepoSyncFrequency parameter in porch-server deployment)

### Sync Manager Operation
Each repository runs two independent goroutines:

1. **Periodic Sync** (`syncForever`):
   - Syncs once at startup
   - Follows cron schedule or default frequency
   - Updates repository conditions after each sync

2. **One-time Sync** (`handleRunOnceAt`):
   - Monitors `runOnceAt` field changes
   - Creates/cancels timers as needed
   - Executes independently of periodic sync

### Status Updates
Repository sync status is reflected in the Repository CR conditions:

```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: Ready
    message: 'Repository Ready (next sync scheduled at: 2025-11-05T11:55:38Z)'
    lastTransitionTime: "2024-01-15T10:30:00Z"
```

## Troubleshooting

### Common Issues

1. **Invalid Cron Expression**:
   - Check porch-server logs for parsing errors
   - Verify 5-field format
   - Repository falls back to default frequency

2. **Past RunOnceAt Time**:
   - One-time sync is skipped
   - Update to future timestamp
   - Check porch-server logs for details

3. **Sync Failures**:
   - Check repository conditions
   - Verify authentication credentials
   - Review repository accessibility
   - Check porch-server logs for detailed error information

### Monitoring
- Repository conditions show sync status
- Porch-server logs contain detailed sync information, next sync times, and any errors


## CLI Commands

For repository registration and sync commands, see the [porchctl CLI guide]({{% relref "/docs/7_cli_api/relevant_old_docs/porchctl-cli-guide.md" %}}):
- [Repository Registration]({{% relref "/docs/7_cli_api/relevant_old_docs/porchctl-cli-guide.md#repository-registration" %}}) - Register repositories with sync configuration
- [Repository Sync Command]({{% relref "/docs/7_cli_api/relevant_old_docs/porchctl-cli-guide.md#repository-sync-command" %}}) - Trigger immediate repository synchronization

---

{{% alert title="Note" color="primary" %}}
OCI repository support is experimental and may not have full feature parity with Git repositories.
{{% /alert %}}