# Repository Controller

The Repository controller manages the lifecycle of Repository CRDs, handling connectivity checks and cache warming operations.

## Configuration

### Cache Warming Settings

The controller uses a worker pool pattern for asynchronous cache warming to handle large-scale deployments:

| Flag | Default | Description |
|------|---------|-------------|
| `--cache-warming-queue-size` | 1000 | Size of the bounded queue for cache warming requests |
| `--cache-warming-worker-count` | 10 | Number of goroutines processing cache warming |

### Sync Settings

| Flag | Default | Description |
|------|---------|-------------|
| `--default-sync-interval` | 5m | Default sync interval for repositories |
| `--min-sync-interval` | 1m | Minimum allowed sync interval |
| `--max-sync-interval` | 24h | Maximum allowed sync interval |
| `--connectivity-retry-interval` | 1m | Retry interval for connectivity failures |

## Scaling Recommendations

### Embedded Controller (CRCache)
For embedded deployment with API server:
- **Queue size**: 100 - conservative for shared resources
- **Workers**: 3 - minimal resource usage
- **Scale**: Up to ~1K repositories

### Separate Controller (DBCache)
For dedicated controller deployment:
- **1K repositories**: Default settings (queue: 1000, workers: 10)
- **10K repositories**: `--repositories.cache-warming-queue-size=5000 --repositories.cache-warming-worker-count=20`
- **100K repositories**: `--repositories.cache-warming-queue-size=10000 --repositories.cache-warming-worker-count=50`

## Tuning for Specific Scenarios

### Handling Bursts

When many repositories are created/updated simultaneously (e.g., during initial deployment or mass updates):

**Increase queue size** to buffer requests without dropping:
```bash
--cache-warming-queue-size=5000  # Prevents "queue full" log messages
```

**Symptoms of insufficient queue size:**
- Log messages: `Cache warming queue full, skipping`
- Repositories stuck in "connecting" state longer than expected

### Faster Cache Warming

To reduce time for repositories to become ready:

**Increase worker count** for more concurrent cache operations:
```bash
--cache-warming-worker-count=50  # More parallel Git clones/fetches
```

**Trade-offs:**
- More workers = faster cache warming but higher CPU/memory/network usage
- Consider Git server rate limits and network bandwidth
- Monitor pod resource usage and adjust accordingly

### Optimal Ratio

**Queue size should be ≥ worker count × average cache warming time (seconds) × reconciliation rate (repos/sec)**

Example: 20 workers, 30s avg cache warming, 10 repos/sec burst = 20 × 30 × 10 = 6000 queue size

## Architecture

The controller separates fast operations (connectivity checks, status updates) from slow operations (cache warming) using a bounded queue and worker pool pattern. This prevents blocking the reconciliation loop while handling cache warming failures gracefully.