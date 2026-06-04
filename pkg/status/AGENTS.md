# Status Package

## What This Package Does

`pkg/status` provides safe status subresource updates with automatic conflict
retry for Kubernetes controllers. When multiple actors (e.g., a module
controller and the platform orchestrator) write to the same resource's status
subresource, optimistic concurrency conflicts are common. This package handles
those conflicts transparently.

## Key Types

| Type | Purpose |
|------|---------|
| `Update[T]` | Generic function: applies mutation, writes status, retries on conflict |
| `Option` | Functional option for configuring Update behavior |
| `WithMaxRetries` | Sets the maximum number of conflict retries (default 5) |
| `ErrRetriesExhausted` | Sentinel error returned when all retry attempts fail |
| `ErrNilMutateFn` | Sentinel error returned when mutateFn is nil |

## How It Works

```text
1. Apply mutateFn(obj)        — caller's status mutation
2. client.Status().Update()   — write to Kubernetes
3. If conflict:
   a. client.Get()            — re-read latest version
   b. mutateFn(obj)           — re-apply mutation on fresh copy
   c. client.Status().Update() — retry write
4. Repeat until success or maxRetries exhausted
```

## Integration With Other Packages

- **pkg/controller/conditions**: The conditions Manager mutates status
  in-memory. Call `status.Update()` afterward to persist those changes.
- **pkg/controller/action**: Use `status.Update()` inside an action pipeline
  step to write status at the end of reconciliation.
- **pkg/deploy**: Deploy handles resource creation; status.Update handles
  the status subresource write. They are complementary.

## Conventions

- All exported functions have GoDoc comments.
- Tests use `_test` package suffix, `t.Parallel()`, Gomega assertions.
- No imports from `github.com/opendatahub-io/opendatahub-operator`.
- Generic constraint is `client.Object`, not `PlatformObject`, to keep
  the utility usable with any Kubernetes resource.
