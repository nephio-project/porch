---
title: "Cache Functionality"
type: docs
weight: 5
description: |
  Overview of cache functionality and detailed documentation pages.
---

The Package Cache provides three core functional areas that work together to maintain consistency between external Git repositories and the internal cache:

## Functional Areas

### Repository Synchronization

Manages the synchronization of package repositories between external Git sources and the internal cache through:
- **SyncManager**: Orchestrates periodic and one-time synchronization operations
- **Change Detection**: Identifies additions, modifications, and deletions
- **Latest Revision Tracking**: Automatically identifies and labels the latest package revisions
- **Condition Management**: Updates repository status for visibility

For detailed architecture and process flows, see [Repository Synchronization](repository-synchronization).

### Caching Behavior

Optimizes performance by storing repository data and avoiding redundant operations through:
- **Cache Population**: Lazy loading and version-based refresh strategies
- **Cache Structure**: In-memory maps with concurrency control
- **Cache Consistency**: Version tracking, optimistic locking, and metadata synchronization
- **Performance Optimization**: Lock-free reads and efficient data structures

For detailed architecture and process flows, see [Caching Behavior](caching-behavior).

### Cache Invalidation

Maintains consistency with external repositories by invalidating stale data through:
- **Automatic Invalidation**: Background sync and version-based detection
- **Manual Invalidation**: User-driven one-time sync using porchctl or Repository CR update
- **Invalidation Strategies**: Selective deletion for package revisions
- **Metadata Cleanup**: Orphaned resource removal and missing metadata creation


## How They Work Together

```
┌─────────────────────────────────────────────────────────┐
│                    Package Cache                        │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────┐         │
│  │   Repository     │      │     Caching      │         │
│  │ Synchronization  │ ───> │    Behavior      │         │
│  │                  │      │                  │         │
│  │  • SyncManager   │      │  • Population    │         │
│  │  • Change Detect │      │  • Structure     │         │
│  │  • Latest Track  │      │  • Consistency   │         │
│  └──────────────────┘      └──────────────────┘         │
│           │                         │                   │
│           └────────┬────────────────┘                   │
│                    ↓                                    │
│          ┌──────────────────┐                           │
│          │      Cache       │                           │
│          │   Invalidation   │                           │
│          │                  │                           │
│          │  • Auto Invalid  │                           │
│          │  • Manual Invld  │                           │
│          │  • Strategies    │                           │
│          └──────────────────┘                           │
└─────────────────────────────────────────────────────────┘
```

**Integration flow:**
1. **Repository Synchronization** detects changes in Git repositories
2. **Caching Behavior** stores and serves the synchronized data efficiently
3. **Cache Invalidation** removes stale data when packages are deleted or modified

Each functional area is documented in detail on its own page with architecture diagrams, process flows, and implementation specifics.
