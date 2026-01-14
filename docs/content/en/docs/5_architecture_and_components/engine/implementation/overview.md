---
title: "Code Structure"
type: docs
weight: 5
description: Engine code organization and key files
---

This section provides a code-level view of the Engine implementation, showing where to find specific functionality and how the code is organized.

## Directory Structure

```
pkg/engine/
├── engine.go                  # Main Engine implementation
├── options.go                 # Configuration options
├── builtinruntime.go          # Built-in function runtime
├── grpcruntime.go             # gRPC function runtime
├── watchermanager.go          # Watch notification system
├── engine_test.go             # Engine tests
├── builtinruntime_test.go     # Built-in runtime tests
├── grpcruntime_test.go        # gRPC runtime tests
├── watchermanager_test.go     # Watcher manager tests
├── pushpr.go                  # Push PR helper (deprecated)
├── pushpr_test.go             # Push PR tests
├── safejoin.go                # Safe path joining utility
└── safejoin_test.go           # Safe join tests
```

## Key Files

### engine.go

**Purpose**: Main Engine implementation

**Key types**:
```go
// CaDEngine is the main interface
type CaDEngine interface {
    ObjectCache() WatcherManager
    
    // PackageRevision operations
    CreatePackageRevision(...)
    UpdatePackageRevision(...)
    DeletePackageRevision(...)
    ListPackageRevisions(...)
    
    // PackageRevisionResources operations
    UpdatePackageResources(...)
    
    // Package operations (deprecated)
    CreatePackage(...)
    UpdatePackage(...)
    DeletePackage(...)
    ListPackages(...)
}

// cadEngine is the implementation
type cadEngine struct {
    cache            cachetypes.Cache
    userInfoProvider repository.UserInfoProvider
    watcherManager   *watcherManager
    taskHandler      task.TaskHandler
}
```

**Key functions**:

**CreatePackageRevision** (lines ~90-200):
- Validates request
- Creates draft
- Applies tasks
- Updates lifecycle
- Closes draft
- Returns result

**UpdatePackageRevision** (lines ~250-400):
- Validates resourceVersion (optimistic locking)
- Handles Published packages (metadata-only updates)
- Handles Draft/Proposed packages (full updates)
- Applies mutations
- Closes draft
- Notifies watchers

**DeletePackageRevision** (lines ~450-470):
- Opens repository
- Delegates to repository.DeletePackageRevision()

**ListPackageRevisions** (lines ~100-120):
- Opens repository from cache
- Delegates to repository.ListPackageRevisions()

**UpdatePackageResources** (lines ~500-600):
- Validates resourceVersion
- Checks lifecycle is Draft
- Creates draft
- Applies resource mutations
- Closes draft
- Returns render status

**Helper functions**:

**ensureUniqueWorkspaceName** (lines ~230-240):
- Checks workspace name uniqueness
- Returns error if duplicate found

**validateCloneTask** (lines ~240-250):
- Validates clone task
- Ensures package doesn't already exist

**validateUpgradeTask** (lines ~210-230):
- Validates upgrade task
- Ensures source revisions are Published

**updatePkgRevMeta** (lines ~420-435):
- Updates PackageRevision metadata
- Handles labels, annotations, finalizers, owner references

### options.go

**Purpose**: Configuration options for Engine initialization

**Key types**:
```go
type EngineOption interface {
    apply(engine *cadEngine) error
}

type EngineOptionFunc func(engine *cadEngine) error
```

**Available options**:

**WithCache**:
```go
func WithCache(cache cachetypes.Cache) EngineOption
```
- Sets the cache implementation (CR or DB)

**WithBuiltinFunctionRuntime**:
```go
func WithBuiltinFunctionRuntime(imagePrefix string) EngineOption
```
- Adds built-in function runtime
- Supports apply-replacements, set-namespace, starlark

**WithGRPCFunctionRuntime**:
```go
func WithGRPCFunctionRuntime(options GRPCRuntimeOptions) EngineOption
```
- Adds gRPC function runtime
- Connects to Function Runner service

**WithFunctionRuntime**:
```go
func WithFunctionRuntime(runtime fn.FunctionRuntime) EngineOption
```
- Sets custom function runtime

**WithCredentialResolver**:
```go
func WithCredentialResolver(resolver repository.CredentialResolver) EngineOption
```
- Sets credential resolver for Git/OCI authentication

**WithReferenceResolver**:
```go
func WithReferenceResolver(resolver repository.ReferenceResolver) EngineOption
```
- Sets reference resolver for package references

**WithUserInfoProvider**:
```go
func WithUserInfoProvider(provider repository.UserInfoProvider) EngineOption
```
- Sets user info provider for audit trail

**WithWatcherManager**:
```go
func WithWatcherManager(watcherManager *watcherManager) EngineOption
```
- Sets watcher manager for real-time notifications

**WithRepoOperationRetryAttempts**:
```go
func WithRepoOperationRetryAttempts(retryAttempts int) EngineOption
```
- Sets retry attempts for repository operations

**Usage pattern**:
```go
engine, err := engine.NewCaDEngine(
    engine.WithCache(cache),
    engine.WithBuiltinFunctionRuntime("gcr.io/kpt-fn"),
    engine.WithGRPCFunctionRuntime(grpcOptions),
    engine.WithCredentialResolver(resolver),
    engine.WithWatcherManager(watcherManager),
)
```

### builtinruntime.go

**Purpose**: Built-in function runtime implementation

**Key types**:
```go
type builtinRuntime struct {
    fnMapping map[string]fnsdk.ResourceListProcessor
}

type builtinRunner struct {
    ctx       context.Context
    processor fnsdk.ResourceListProcessor
}
```

**Supported functions**:
- **apply-replacements**: v0.1.1
- **set-namespace**: v0.4.1
- **starlark**: v0.4.3

**How it works**:
1. Maps function images to Go implementations
2. When function requested, returns runner with Go implementation
3. Runner executes function directly in process
4. No network overhead, very fast

**Key functions**:

**newBuiltinRuntime**:
```go
func newBuiltinRuntime(imagePrefix string) *builtinRuntime
```
- Creates runtime with function mappings
- Supports multiple image prefixes (gcr.io, custom)

**GetRunner**:
```go
func (br *builtinRuntime) GetRunner(ctx context.Context, funct *v1.Function) (fn.FunctionRunner, error)
```
- Returns runner for function
- Returns NotFoundError if function not built-in

**Run**:
```go
func (br *builtinRunner) Run(r io.Reader, w io.Writer) error
```
- Executes function
- Reads ResourceList from reader
- Writes transformed ResourceList to writer
- Catches panics and converts to errors

### grpcruntime.go

**Purpose**: gRPC function runtime implementation

**Key types**:
```go
type GRPCRuntimeOptions struct {
    FunctionRunnerAddress string
    MaxGrpcMessageSize    int
    DefaultImagePrefix    string
}

type grpcRuntime struct {
    cc     *grpc.ClientConn
    client evaluator.FunctionEvaluatorClient
}

type grpcRunner struct {
    ctx    context.Context
    client evaluator.FunctionEvaluatorClient
    image  string
}
```

**How it works**:
1. Connects to Function Runner service via gRPC
2. When function requested, returns runner with gRPC client
3. Runner sends function execution request to Function Runner
4. Function Runner spawns pod, executes function, returns result

**Key functions**:

**newGRPCFunctionRuntime**:
```go
func newGRPCFunctionRuntime(options GRPCRuntimeOptions) (*grpcRuntime, error)
```
- Creates gRPC client connection
- Configures message size limits
- Returns runtime

**GetRunner**:
```go
func (gr *grpcRuntime) GetRunner(ctx context.Context, fn *v1.Function) (fn.FunctionRunner, error)
```
- Returns runner for any function
- Always succeeds (Function Runner handles unknown functions)

**Run**:
```go
func (gr *grpcRunner) Run(r io.Reader, w io.Writer) error
```
- Reads ResourceList from reader
- Sends EvaluateFunction gRPC request
- Writes result to writer

**Close**:
```go
func (gr *grpcRuntime) Close() error
```
- Closes gRPC connection
- Called during shutdown

### watchermanager.go

**Purpose**: Watch notification system

**Key types**:
```go
type WatcherManager interface {
    WatchPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter, callback ObjectWatcher) error
}

type ObjectWatcher interface {
    OnPackageRevisionChange(eventType watch.EventType, obj repository.PackageRevision) bool
}

type watcherManager struct {
    mutex    sync.Mutex
    watchers []*watcher
}

type watcher struct {
    isDoneFunction func() error
    callback       ObjectWatcher
    filter         repository.ListPackageRevisionFilter
}
```

**How it works**:
1. Clients register watchers with filter
2. Engine notifies watchers when packages change
3. Watchers receive events matching their filter
4. Watchers automatically cleaned up when context cancelled

**Key functions**:

**WatchPackageRevisions**:
```go
func (r *watcherManager) WatchPackageRevisions(ctx context.Context, filter repository.ListPackageRevisionFilter, callback ObjectWatcher) error
```
- Registers new watcher
- Reuses empty slots for efficiency
- Returns immediately (non-blocking)

**NotifyPackageRevisionChange**:
```go
func (r *watcherManager) NotifyPackageRevisionChange(eventType watch.EventType, obj repository.PackageRevision) int
```
- Sends event to all matching watchers
- Cleans up finished watchers
- Returns number of watchers notified

**Watcher cleanup**:
- Automatic when context cancelled
- Automatic when callback returns false
- Slots reused for new watchers

## Dependencies

### Internal Dependencies

**pkg/cache**:
- `cachetypes.Cache`: Cache interface
- Used for repository access

**pkg/task**:
- `task.TaskHandler`: Task execution
- Used for applying package operations

**pkg/repository**:
- `repository.Repository`: Repository interface
- `repository.PackageRevision`: Package revision interface
- `repository.CredentialResolver`: Credential resolution
- `repository.ReferenceResolver`: Reference resolution
- `repository.UserInfoProvider`: User info extraction

**pkg/kpt/fn**:
- `fn.FunctionRuntime`: Function runtime interface
- `fn.FunctionRunner`: Function runner interface
- `fn.MultiRuntime`: Multiple runtime coordinator

### External Dependencies

**k8s.io/apimachinery**:
- `watch.EventType`: Watch event types
- `apierrors`: Kubernetes error types
- `metav1`: Kubernetes metadata types

**go.opentelemetry.io/otel**:
- `trace`: Distributed tracing
- Used for observability

**google.golang.org/grpc**:
- gRPC client for Function Runner

## Testing

### Unit Tests

**engine_test.go**:
- Tests Engine operations
- Uses fake implementations
- Tests validation logic

**builtinruntime_test.go**:
- Tests built-in function execution
- Tests function mapping
- Tests error handling

**grpcruntime_test.go**:
- Tests gRPC runtime
- Uses mock gRPC server
- Tests connection handling

**watchermanager_test.go**:
- Tests watch registration
- Tests event notification
- Tests watcher cleanup

### Integration Tests

**test/e2e/e2e_test.go**:
- End-to-end tests with real Engine
- Tests complete workflows
- Tests error scenarios

## Initialization Flow

```go
// 1. Create Engine with options
engine, err := engine.NewCaDEngine(
    engine.WithCache(cache),
    engine.WithBuiltinFunctionRuntime(imagePrefix),
    engine.WithGRPCFunctionRuntime(grpcOptions),
    engine.WithCredentialResolver(resolver),
    engine.WithReferenceResolver(refResolver),
    engine.WithUserInfoProvider(userInfoProvider),
    engine.WithWatcherManager(watcherManager),
)

// 2. Engine initializes internal state
// - taskHandler created
// - Dependencies injected
// - Runtimes configured

// 3. Engine ready for operations
result, err := engine.CreatePackageRevision(ctx, repo, pr, parent)
```

## Error Handling

**Error types returned**:
- `fmt.Errorf`: Simple errors
- `pkgerrors.Wrap`: Wrapped errors with context
- `apierrors.NewConflict`: Optimistic lock conflicts
- `apierrors.NewInternalError`: Internal errors

**Error patterns**:
```go
// Validation error
if invalid {
    return nil, fmt.Errorf("validation failed: %s", reason)
}

// Wrapped error
if err != nil {
    return nil, pkgerrors.Wrapf(err, "failed to open repository")
}

// Conflict error
if resourceVersion != current {
    return nil, apierrors.NewConflict(resource, name, fmt.Errorf(OptimisticLockErrorMsg))
}
```

## Observability

**Tracing**:
```go
var tracer = otel.Tracer("engine")

func (cad *cadEngine) CreatePackageRevision(ctx context.Context, ...) {
    ctx, span := tracer.Start(ctx, "cadEngine::CreatePackageRevision", trace.WithAttributes())
    defer span.End()
    
    // Operation code
}
```

**Logging**:
```go
klog.Infof("engine: sent %d for updated PackageRevision %s/%s", sent, namespace, name)
klog.Warningf("Failed to rollback package revision creation: %v", err)
```

## Summary

The Engine code is organized into:

**Core files**:
- `engine.go`: Main implementation (CRUD operations, validation, orchestration)
- `options.go`: Configuration via functional options
- `watchermanager.go`: Real-time notifications

**Runtime files**:
- `builtinruntime.go`: Fast, in-process function execution
- `grpcruntime.go`: Scalable, isolated function execution

**Key patterns**:
- Interface-based design for testability
- Dependency injection via options
- Comprehensive error handling
- Distributed tracing for observability
- Optimistic concurrency control

**Code locations**:
- Package operations: `engine.go` lines 90-600
- Configuration: `options.go`
- Function runtimes: `builtinruntime.go`, `grpcruntime.go`
- Watch system: `watchermanager.go`

Understanding this code structure helps developers navigate the Engine implementation, add new features, and debug issues.
