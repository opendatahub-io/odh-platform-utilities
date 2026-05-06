# Garbage Collection Utilities - AI Agent Guide

## Package Purpose

Garbage collection (GC) for Kubernetes module controllers. Compares what is deployed on the cluster against what the controller just rendered and deletes the difference. Handles API discovery, RBAC authorization, label-based selection, predicate filtering, and safe deletion.

## Location

`github.com/opendatahub-io/odh-platform-utilities/pkg/controller/gc`

## When to Use

**Use the Collector when:**
- Building a module controller that deploys resources and needs cleanup on config change / upgrade / removal
- You want automatic staleness detection via the deploy/GC annotation protocol
- You need RBAC-safe deletion (won't attempt to delete types without permission)

**Don't use when:**
- You have simple, known-type cleanup (just delete by name)
- Your controller doesn't stamp deploy/GC lifecycle annotations

## Key Concepts

### GC Must Be Last

GC should run after all deploy actions complete. Deploy stamps annotations, GC reads them. Running GC before deploy would delete resources that are about to be re-created.

### Skip When Not Generated

Callers should skip running the collector when nothing was generated in the current reconcile cycle. GC uses expensive API discovery and RBAC checks — don't pay that cost on no-op reconciles.

### Deploy/GC Annotation Protocol

The default object predicate checks these annotations (stamped by deploy):

| Annotation | Purpose |
|---|---|
| `platform.opendatahub.io/version` | Detects resources from previous releases |
| `platform.opendatahub.io/type` | Detects platform type changes |
| `platform.opendatahub.io/instance.generation` | Detects stale generation |
| `platform.opendatahub.io/instance.uid` | Detects deleted-and-recreated CRs |

If any annotation is missing, the resource is treated as stale (pre-annotation resource).

### Default Unremovables

CRDs and Leases are never deleted by default:
- **CRDs**: Managed at a higher level, not by individual controllers
- **Leases**: Used for leader election

Extend via the WithUnremovables option.

### RBAC Authorization

GC performs a SelfSubjectRulesReview to determine which resource types the controller can delete. This prevents noisy 403 errors when the controller doesn't have permission for certain types.

### Ownership Filtering

By default (onlyOwned = true), GC only deletes resources that have an owner reference matching the controller CR's GVK. Use WithOnlyCollectOwned(false) to delete any matching resource regardless of ownership.

## Options API

| Option | Description | Default |
|---|---|---|
| WithLabel(k, v) | Add label to selector | platform.opendatahub.io/part-of: \<kind\> |
| WithLabels(map) | Add multiple labels to selector | — |
| WithUnremovables(gvks...) | GVKs that should never be deleted | CRD, Lease |
| WithObjectPredicate(fn) | Custom per-object deletion logic | DefaultObjectPredicate |
| WithTypePredicate(fn) | Custom per-type filtering | DefaultTypePredicate (allow all) |
| WithOnlyCollectOwned(bool) | Only delete owned resources | true |
| InNamespace(ns) | Static namespace for RBAC checks | — |
| InNamespaceFn(fn) | Dynamic namespace resolver | — |
| WithDeletePropagationPolicy(p) | Deletion propagation policy | Foreground |

## Exported Authorization Helpers

These functions are independently useful beyond GC:

- **ListAuthorizedResources** — combines discovery filtering with RBAC permission checks to return resources the controller can interact with
- **RetrieveSelfSubjectRules** — retrieves RBAC resource rules for the current subject in a namespace
- **ComputeAuthorizedResources** — computes authorized resources from pre-fetched API resource lists and RBAC rules
- **HasPermissions** — checks if a subject has all required verbs for a specific API resource
- **IsResourceMatchingRule** — determines if an API resource matches an authorization rule

## Metrics

| Metric | Type | Labels | Description |
|---|---|---|---|
| action_gc_deleted_total | Counter | controller | Number of deleted resources |
| action_gc_cycles_total | Counter | controller | Number of GC cycles |

Metrics are automatically registered with the controller-runtime metrics registry.

## Common Mistakes

1. **Running GC before deploy** — deploy stamps annotations, GC reads them
2. **Running GC on no-op reconciles** — skip when nothing was generated
3. **Forgetting InNamespace** — RBAC checks need a namespace context
4. **Custom predicates that return true for everything** — will delete all matching resources
5. **Not setting Version/PlatformType in RunParams** — default predicate can't detect staleness

## Relationship to Other Packages

- **pkg/metadata/annotations**: Defines the annotation keys used by the default predicate
- **pkg/metadata/labels**: Defines PlatformPartOf used as default label selector
- **pkg/resources**: Provides Resource type, discovery, annotation helpers, ownership checks
