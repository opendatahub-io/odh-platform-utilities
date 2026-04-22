# Migration from ODH Operator Types

This document maps the types in the ODH Operator's internal packages to their
equivalents in `odh-platform-utilities`. Use this guide when migrating an
existing module controller from the operator's monorepo to a standalone
controller that imports this shared library.

## Import Path Changes

| Old Import (operator) | New Import (shared library) |
|----------------------|----------------------------|
| `github.com/opendatahub-io/opendatahub-operator/api/common` | `github.com/opendatahub-io/odh-platform-utilities/api/common` |
| `github.com/opendatahub-io/opendatahub-operator/internal/controller/status` | `github.com/opendatahub-io/odh-platform-utilities/api/common` |
| `github.com/opendatahub-io/opendatahub-operator/pkg/cluster` | `github.com/opendatahub-io/odh-platform-utilities/pkg/cluster` |
| `github.com/opendatahub-io/opendatahub-operator/pkg/webhook` | `github.com/opendatahub-io/odh-platform-utilities/pkg/webhook` |

## Type Mapping

### Status Types (`api/common`)

| Operator Type | Shared Library Type | Notes |
|--------------|---------------------|-------|
| `common.Status` | `common.Status` | Same structure. Embed with `` `json:",inline"` `` |
| `common.Condition` | `common.Condition` | Same fields. `Status` field uses `metav1.ConditionStatus` |
| `common.ComponentRelease` | `common.ComponentRelease` | Unchanged |
| `common.ComponentReleaseStatus` | `common.ComponentReleaseStatus` | Unchanged |
| `common.ManagementSpec` | `common.ManagementSpec` | **Changed:** No longer depends on `github.com/openshift/api`. Uses `common.ManagementState` (`string` type) instead of `operatorv1.ManagementState` |

### ManagementState Migration

The operator's `ManagementSpec` imported `ManagementState` from
`github.com/openshift/api/operator/v1`. The shared library defines its own
`ManagementState` type as a plain Go `string`:

**Before (operator):**
```go
import operatorv1 "github.com/openshift/api/operator/v1"

type ManagementSpec struct {
    ManagementState operatorv1.ManagementState `json:"managementState,omitempty"`
}
```

**After (shared library):**
```go
import "github.com/opendatahub-io/odh-platform-utilities/api/common"

// common.ManagementSpec already defines:
// type ManagementState string
// const Managed ManagementState = "Managed"
// const Removed ManagementState = "Removed"

type MyComponentSpec struct {
    common.ManagementSpec `json:",inline"`
}
```

The JSON serialization is identical (`"Managed"`, `"Removed"`), so this is a
wire-compatible change. CRDs do not need regeneration for the management state
field.

### Accessor Interfaces

| Operator Interface | Shared Library Interface | Notes |
|-------------------|-------------------------|-------|
| `common.WithStatus` | `common.WithStatus` | Same: `GetStatus() *Status` |
| `common.ConditionsAccessor` | `common.ConditionsAccessor` | Same: `GetConditions()`, `SetConditions()` |
| `common.WithReleases` | `common.WithReleases` | **Changed:** `GetReleaseStatus` now returns `*ComponentReleaseStatus` (was `*[]ComponentRelease`). `SetReleaseStatus` now accepts `ComponentReleaseStatus` by value (not pointer) to prevent nil-dereference panics |
| `common.PlatformObject` | `common.PlatformObject` | **Changed:** Now composes `WithReleases` (previously omitted) |

### PlatformObject Change

The operator's `PlatformObject` did not compose `WithReleases`:

```go
// Old (operator)
type PlatformObject interface {
    client.Object
    WithStatus
    ConditionsAccessor
}
```

The shared library adds `WithReleases`:

```go
// New (shared library)
type PlatformObject interface {
    client.Object
    WithStatus
    ConditionsAccessor
    WithReleases  // ← NEW: required for upgrade mode
}
```

**Migration action:** Implement `GetReleaseStatus()` and `SetReleaseStatus()`
on your module CR. If your module has no sub-components, return an empty
`ComponentReleaseStatus`.

### Condition Constants

| Operator Constant (internal/) | Shared Library Constant | Value |
|------------------------------|------------------------|-------|
| `status.ConditionTypeReady` | `common.ConditionTypeReady` | `"Ready"` |
| `status.ConditionTypeProvisioningSucceeded` | `common.ConditionTypeProvisioningSucceeded` | `"ProvisioningSucceeded"` |
| `status.ConditionTypeDegraded` | `common.ConditionTypeDegraded` | `"Degraded"` |

These were previously in `internal/controller/status/status.go` and not
importable by external consumers. They are now in `api/common`.

### Phase Constants

| Operator Constant (internal/) | Shared Library Constant | Value |
|------------------------------|------------------------|-------|
| `status.PhaseReady` | `common.PhaseReady` | `"Ready"` |
| `status.PhaseNotReady` | `common.PhaseNotReady` | `"Not Ready"` |

Also moved from `internal/` to `api/common`.

## Singleton Utilities

### GetSingleton

| Operator Location | Shared Library Location |
|-------------------|------------------------|
| `pkg/cluster.GetSingleton[T]` | `pkg/cluster.GetSingleton[T]` |

The function signature is identical:

```go
func GetSingleton[T client.Object](ctx context.Context, c client.Client, target T) error
```

**Changed:** When no instance is found, the operator returned
`k8serr.NewNotFound(...)` (compatible with `k8serr.IsNotFound(err)`). The
shared library returns a wrapped `ErrNoInstance` sentinel instead. Callers
should migrate from `k8serr.IsNotFound(err)` to
`errors.Is(err, cluster.ErrNoInstance)`.

### Webhook Utilities

| Operator Location | Shared Library Location |
|-------------------|------------------------|
| `pkg/webhook.ValidateSingletonCreation` | `pkg/webhook.ValidateSingletonCreation` |
| `pkg/webhook.CountObjects` | `pkg/webhook.CountObjects` |
| `pkg/webhook.DenyCountGtZero` | `pkg/webhook.DenyCountGtZero` |

`ValidateSingletonCreation` and `CountObjects` signatures are unchanged.

**Changed:** `DenyCountGtZero` was refactored. The old signature was
`DenyCountGtZero(ctx, cli, gvk, denyMessage)` and performed both counting
and deny logic. The new signature is `DenyCountGtZero(count, gvk)` — it
only handles the deny decision. Counting is now done separately via
`CountObjects`.

## Migration Checklist

1. [ ] Add `github.com/opendatahub-io/odh-platform-utilities` to your `go.mod`
2. [ ] Replace operator import paths with shared library import paths
3. [ ] Replace `operatorv1.ManagementState` references with `common.ManagementState`
4. [ ] Implement `WithReleases` on your module CR (if not already done)
5. [ ] Replace `status.ConditionTypeReady` etc. with `common.ConditionTypeReady`
6. [ ] Replace `status.PhaseReady` etc. with `common.PhaseReady`
7. [ ] Run `go mod tidy` to clean up removed operator dependencies
8. [ ] Verify no imports from `github.com/opendatahub-io/opendatahub-operator/internal/`
9. [ ] Verify no imports from `github.com/openshift/api`
10. [ ] Run tests to confirm behavior is unchanged
