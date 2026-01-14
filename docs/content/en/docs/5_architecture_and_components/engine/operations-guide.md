---
title: "Operations Guide"
type: docs
weight: 10
description: Performance, troubleshooting, and testing
---

This section covers operational aspects of the Engine including performance considerations, common issues, and testing strategies.

## Performance Considerations

### Operation Latency

**Typical latencies**:
- **List**: 10-100ms (served from cache)
- **Create**: 500ms-2s (includes Git operations)
- **Update**: 500ms-2s (includes Git operations)
- **Delete**: 200ms-1s (Git delete)

**Factors affecting performance**:
- Git repository size and location
- Network latency to Git/OCI
- Function execution time
- Number of files in package

### Optimization Strategies

**1. Use Built-in Functions**

Built-in functions execute in-process, avoiding network overhead:
- apply-replacements: ~10ms vs ~500ms (gRPC)
- set-namespace: ~5ms vs ~500ms (gRPC)
- starlark: ~20ms vs ~500ms (gRPC)

**Configuration**:
```go
engine.WithBuiltinFunctionRuntime(imagePrefix)
```

**2. Enable Multi-Runtime**

Multi-runtime tries built-in first, falls back to gRPC:
```go
engine.WithBuiltinFunctionRuntime(imagePrefix),
engine.WithGRPCFunctionRuntime(grpcOptions),
```

**3. Tune gRPC Message Size**

For large packages, increase gRPC message size:
```go
GRPCRuntimeOptions{
    FunctionRunnerAddress: "function-runner:9445",
    MaxGrpcMessageSize:    10 * 1024 * 1024, // 10MB
}
```

**4. Configure Retry Attempts**

Balance between reliability and latency:
```go
engine.WithRepoOperationRetryAttempts(3) // Default: 5
```

### Scalability

**Horizontal scaling**:
- Engine is stateless
- Multiple Porch Server replicas can run concurrently
- Cache layer provides consistency

**Bottlenecks**:
- Git repository operations (sequential per package)
- Function Runner capacity (pod spawning)
- Network bandwidth to Git/OCI

## Common Issues

### Issue 1: Optimistic Lock Conflicts

**Symptom**:
```
Error: the object has been modified; please apply your changes to the latest version and try again
```

**Cause**: Concurrent modifications to same package

**Resolution**:
1. Fetch latest version: `kubectl get packagerevision <name> -o yaml`
2. Apply changes to latest version
3. Retry update

**Prevention**: Implement retry logic in clients

### Issue 2: Workspace Name Conflicts

**Symptom**:
```
Error: package revision workspaceNames must be unique; package revision with name my-package in repo blueprints with workspaceName v1 already exists
```

**Cause**: Attempting to create package with existing workspace name

**Resolution**: Choose different workspace name

**Prevention**: Use unique workspace names (e.g., timestamp, UUID)

### Issue 3: Function Execution Timeout

**Symptom**:
```
Error: context deadline exceeded
```

**Cause**: Function takes too long to execute

**Resolution**:
1. Check function implementation for performance issues
2. Increase timeout in Function Runner configuration
3. Optimize package size (fewer resources)

**Prevention**: Test functions with realistic package sizes

### Issue 4: Git Push Failures

**Symptom**:
```
Error: failed to close package revision draft: failed to push to remote: <git error>
```

**Causes**:
- Invalid credentials
- Network issues
- Repository permissions
- Branch protection rules

**Resolution**:
1. Verify Git credentials in Secret
2. Check network connectivity
3. Verify repository permissions
4. Check branch protection settings

**Debugging**:
```bash
# Check repository status
kubectl get repository <name> -o yaml

# Check Porch Server logs
kubectl logs -n porch-system deployment/porch-server
```

### Issue 5: Rollback Failures

**Symptom**: Package creation fails, but draft remains in repository

**Cause**: Rollback failed (rare)

**Resolution**:
1. Manually delete draft branch in Git
2. Or wait for garbage collection

**Prevention**: Ensure Porch Server has proper Git permissions

## Troubleshooting

### Enable Debug Logging

**Porch Server**:
```yaml
spec:
  containers:
  - name: porch-server
    args:
    - --v=4  # Increase verbosity
```

**Look for**:
- Engine operation traces
- Git operations
- Function execution
- Watch notifications

### Trace Operations

Engine uses OpenTelemetry tracing:

**Enable Jaeger**:
```yaml
# Deploy Jaeger
kubectl apply -f deployments/tracing/deployment.yaml

# Configure Porch Server
env:
- name: OTEL_EXPORTER_JAEGER_ENDPOINT
  value: "http://jaeger:14268/api/traces"
```

**View traces**:
- Open Jaeger UI
- Search for "engine" service
- View operation spans

### Check Watch Events

**Test watch**:
```bash
kubectl get packagerevisions -w
```

**Expected behavior**:
- ADDED events when packages created
- MODIFIED events when packages updated
- DELETED events when packages deleted

**If not working**:
- Check Watcher Manager logs
- Verify watch registration
- Check client connection

### Inspect Package State

**Get PackageRevision**:
```bash
kubectl get packagerevision <name> -o yaml
```

**Check**:
- `spec.lifecycle`: Current lifecycle state
- `spec.tasks`: Tasks to be applied
- `status.conditions`: Error conditions
- `metadata.resourceVersion`: For optimistic locking

**Get PackageRevisionResources**:
```bash
kubectl get packagerevisionresources <name> -o yaml
```

**Check**:
- `spec.resources`: File contents
- `status.renderStatus`: Function execution results

## Testing

### Unit Testing

**Test Engine operations**:
```go
func TestCreatePackageRevision(t *testing.T) {
    // Create fake cache
    cache := fake.NewCache()
    
    // Create Engine
    engine, err := engine.NewCaDEngine(
        engine.WithCache(cache),
    )
    
    // Test operation
    result, err := engine.CreatePackageRevision(ctx, repo, pr, nil)
    
    // Assert results
    assert.NoError(t, err)
    assert.NotNil(t, result)
}
```

**Test validation**:
```go
func TestWorkspaceNameUniqueness(t *testing.T) {
    // Create package with workspace "v1"
    // Attempt to create another with workspace "v1"
    // Assert error
}
```

### Integration Testing

**Test with real Git repository**:
```go
func TestE2EPackageCreation(t *testing.T) {
    // Setup test Git repository
    // Create Engine with real cache
    // Create package
    // Verify in Git
    // Cleanup
}
```

**Test function execution**:
```go
func TestFunctionExecution(t *testing.T) {
    // Create package with function
    // Execute render
    // Verify function applied
}
```

### Performance Testing

**Measure operation latency**:
```go
func BenchmarkCreatePackageRevision(b *testing.B) {
    for i := 0; i < b.N; i++ {
        engine.CreatePackageRevision(ctx, repo, pr, nil)
    }
}
```

**Load testing**:
```bash
# Create many packages concurrently
for i in {1..100}; do
  kubectl apply -f package-$i.yaml &
done
wait
```

## Monitoring

### Metrics to Track

**Operation metrics**:
- Create latency (p50, p95, p99)
- Update latency (p50, p95, p99)
- Delete latency (p50, p95, p99)
- List latency (p50, p95, p99)

**Error metrics**:
- Optimistic lock conflicts per minute
- Validation failures per minute
- Git operation failures per minute
- Function execution failures per minute

**Watch metrics**:
- Active watchers
- Events sent per minute
- Watcher cleanup rate

### Health Checks

**Engine health**:
- Can open repositories
- Can list packages
- Can create packages

**Dependency health**:
- Cache accessible
- Function Runner reachable
- Git repositories accessible

## Best Practices

### For Operators

1. **Monitor Git repository health**: Ensure Git is accessible and performant
2. **Size Function Runner appropriately**: Enough capacity for concurrent functions
3. **Enable tracing**: For debugging production issues
4. **Set appropriate timeouts**: Balance reliability and latency
5. **Monitor optimistic lock conflicts**: High rate indicates contention

### For Developers

1. **Implement retry logic**: Handle optimistic lock conflicts gracefully
2. **Use unique workspace names**: Avoid conflicts
3. **Test with realistic data**: Ensure functions work with large packages
4. **Handle errors properly**: Check error types and respond appropriately
5. **Use watches for real-time updates**: More efficient than polling

### For Package Authors

1. **Keep packages small**: Faster operations, less memory
2. **Optimize functions**: Minimize execution time
3. **Use built-in functions**: When possible, for better performance
4. **Test locally first**: Use kpt CLI before deploying to Porch

## Summary

**Performance**:
- Use built-in functions for speed
- Enable multi-runtime for flexibility
- Tune gRPC message size for large packages
- Configure retry attempts appropriately

**Common issues**:
- Optimistic lock conflicts: Retry with latest version
- Workspace name conflicts: Use unique names
- Function timeouts: Optimize functions
- Git push failures: Check credentials and permissions
- Rollback failures: Manual cleanup may be needed

**Troubleshooting**:
- Enable debug logging
- Use distributed tracing
- Check watch events
- Inspect package state

**Testing**:
- Unit test validation logic
- Integration test with real repositories
- Performance test under load
- Monitor metrics in production

**Best practices**:
- Monitor dependencies
- Implement retry logic
- Use unique workspace names
- Keep packages small
- Test thoroughly

The Engine is designed for reliability and performance, but understanding these operational aspects helps ensure smooth production deployments.
