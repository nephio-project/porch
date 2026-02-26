# Porch API v1alpha2

This directory contains the v1alpha2 API types for Porch.

## Status

**EXPERIMENTAL** - v1alpha2 is under active development and not yet deployed.

## Key Changes from v1alpha1

### P0 Changes (Implemented)

1. **CRD Markers Added**
   - PackageRevision includes kubebuilder markers for CRD generation
   - Will be stored in etcd (git remains source of truth)
   - PackageRevisionResources remains aggregated API (size constraint)

2. **PorchPackage Removed**
   - Not included in v1alpha2 (unused by users/controllers)
   - Use PackageRevision queries with label selectors instead

3. **API Independence Documented**
   - Updated comments for Locator/GitLock/ResultList types
   - Clarified these are intentionally duplicated from kpt library

### P1-P4 Changes (Deferred)

The following improvements are documented but not yet implemented:
- Refactor Tasks → Creation Source Fields (explicit Init/Clone/Copy/Upgrade)
- Remove OCI support (incomplete, no users)
- Simplify PackageRevisionResources spec (remove redundant fields)
- Use metav1.Condition instead of custom Condition type
- Fix PublishedAt JSON tag mismatch
- Remove unused types (Selector, Parent)
- API polish and naming improvements

See [V1ALPHA2_MIGRATION_PLAN.md](../../.local/docs/pkgrev_ctr/V1ALPHA2_MIGRATION_PLAN.md) for details.

## Files

- `doc.go` - Package documentation and code generation markers
- `register.go` - Scheme registration for v1alpha2 types
- `packagerevision_types.go` - PackageRevision resource and related types
- `packagerevision_status_types.go` - PackageRevision status types (Condition, Locator, GitLock)
- `packagerevisionresources_types.go` - PackageRevisionResources resource
- `task_types.go` - Task types (Task, TaskType, TaskSpecs)
- `upstream_types.go` - Upstream package reference types (Git, OCI)
- `result_types.go` - Function result types (ResultList, Result, ResultItem)

## Next Steps

1. Generate deepcopy and other generated files:
   ```bash
   make generate
   ```

2. Generate CRD YAML:
   ```bash
   make manifests
   ```

3. Implement controller to sync between git and etcd CRDs

4. Create migration tooling to populate v1alpha2 CRDs from git

## Compatibility

v1alpha2 can coexist with v1alpha1:
- v1alpha1: Aggregated API (no etcd storage)
- v1alpha2: CRD (etcd storage)
- No conflict - different versions, different storage backends

## Documentation

- [V1ALPHA2_MIGRATION_PLAN.md](../../.local/docs/pkgrev_ctr/V1ALPHA2_MIGRATION_PLAN.md) - Comprehensive planning
- [V1ALPHA2_IMPLEMENTATION_PHASES.md](../../.local/docs/pkgrev_ctr/V1ALPHA2_IMPLEMENTATION_PHASES.md) - Implementation priorities
- [V1ALPHA2_SUMMARY.md](../../.local/docs/pkgrev_ctr/V1ALPHA2_SUMMARY.md) - Executive summary
