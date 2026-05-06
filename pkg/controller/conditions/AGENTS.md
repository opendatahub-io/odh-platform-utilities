# Condition Management Utilities - AI Agent Guide

## Package Purpose

Knative-inspired condition management for Kubernetes module controllers with automatic aggregation and severity-based filtering.

## Location

`github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions`

## When to Use

**Use the Manager when:**
- Building a module controller with a top-level Ready condition and dependents
- You want automatic aggregation of unhappy conditions
- You need severity-based filtering (Error vs Info)

**Use low-level helpers when:**
- Propagating status from sub-component CRs upward
- You need manual control without automatic aggregation

## Common Patterns

### Pattern 1: Module Controller

Create a Manager with the happy condition (Ready) and its dependents (ProvisioningSucceeded). On deploy failure, mark the dependent false with the error. On success, mark it true and call Sort before updating status.

### Pattern 2: Status Propagation

Use FindStatusCondition to retrieve a child CR's condition, then MarkFrom to propagate it into the parent Manager under a new condition name.

## Key Concepts

### Severity Controls Aggregation

- **ConditionSeverityError** (default, empty string): Participates in happiness rollup
- **ConditionSeverityInfo**: Does NOT participate, use for informational conditions

A condition marked false with severity Error affects the Ready aggregation. A condition marked false with severity Info does not.

### Aggregation Rules

`RecomputeHappiness()` (called automatically):
1. Filters to ConditionSeverityError dependents only
2. Finds False or Unknown conditions
3. Prioritizes False over Unknown
4. Propagates first unhappy dependent to happy condition
5. If all healthy: sets happy to True

### PlatformObject Contract Compliance

Mandatory conditions for module CRs:
- **Ready**: Top-level aggregate (orchestrator checks this)
- **ProvisioningSucceeded**: MUST aggregate into Ready
- **Degraded**: COULD aggregate depending on severity

Create a Manager with Ready as the happy condition and ProvisioningSucceeded and Degraded as dependents.

## Functional Options

- `WithReason(string)`: Set condition reason
- `WithMessage(format, ...args)`: Set message with formatting
- `WithError(error)`: Set severity=Error, reason="Error", message=err.Error()
- `WithSeverity(ConditionSeverity)`: Set severity (Error or Info)
- `WithObservedGeneration(int64)`: Stamp reconciled generation

## Common Mistakes

1. **Forgetting to call Sort()** before status update
2. **Empty severity defaults to Error** - set Info explicitly if needed
3. **Don't manually call RecomputeHappiness()** - it's automatic
4. **Manager is not thread-safe** - ensure external synchronization
5. **MarkFrom does NOT copy ObservedGeneration** - only copies status, reason, message, and severity to avoid confusing generation tracking between condition types

## Low-Level Helpers

- **SetStatusCondition** — upsert a condition with transition time management
- **FindStatusCondition** — retrieve a deep copy of a condition (prevents mutation)
- **RemoveStatusCondition** — remove a condition by type
- **IsStatusConditionTrue** — convenience check for True status

## Transition Time Behavior

- `LastTransitionTime` updates ONLY when status changes (True/False/Unknown)
- Reason/message changes do NOT update LastTransitionTime
- Matches Kubernetes conventions

## Relationship to PlatformObject Contract

This package provides the **management tooling** for the contract types in `api/common`:
- `api/common` defines the types (Condition, ConditionsAccessor, constants)
- `pkg/controller/conditions` provides the Manager and helpers to operate on those types

See `docs/platform-object-contract.md` for full contract details.
