# ODH Platform Utilities — Agent Guide

## What This Repository Is

`odh-platform-utilities` is a shared Go library for
[Open Data Hub](https://opendatahub.io/) module controller development. It
provides common types, interfaces, and utilities extracted from the
[ODH Operator](https://github.com/opendatahub-io/opendatahub-operator) so that
module controllers can be developed independently while conforming to the
platform contract.

The library serves two primary roles:

1. **Platform contract** — defines the types and interfaces
   (`PlatformObject`, `Status`, `Condition`, etc.) that the orchestrator and
   modules must agree on.
2. **Shared utilities** — provides reusable helpers for manifest rendering
   (Helm, Kustomize, Go templates), singleton enforcement, admission
   webhooks, and Kubernetes resource manipulation.

## Architecture Context

The ODH platform follows a hub-and-spoke architecture:

- The **ODH Operator** (hub/orchestrator) manages a DAG of module controllers,
  reading their CR status via the `PlatformObject` interface to aggregate
  health, detect versions, and gate upgrade progression.
- **Module controllers** (spokes) each own a cluster-scoped singleton CRD and
  reconcile their component's resources.

This library is the shared dependency that both sides import.

For architectural context see:
- [Integration Test Harness](docs/integration-testing.md)
- [ODH Operator Evolution](https://docs.google.com/document/d/1mOuXIKkqbh3rS35g4JdWTj5HvjQ_a-7u7HBwBIqlIpI)
- [Onboarding Guide](https://docs.google.com/document/d/1FeJk5mMPGMGMNqMAiGn0-cTKcNxblDYAkhU4DOmcpns)

## Repository Structure

This repository has one **product Go module**:
`github.com/opendatahub-io/odh-platform-utilities` at the repository root.
Packages under `api/`, `pkg/`, `framework/`, and `testing/` are all part of that
module and are released together with root `v*` tags.

`flakiness/` and the modules under `examples/` are standalone tooling modules,
not product modules. CI validates the root product module and `flakiness/`
separately.

A consumer developing against a local checkout needs one replacement:

```go
replace github.com/opendatahub-io/odh-platform-utilities => /path/to/odh-platform-utilities
```

### Product Module

```text
api/
  common/          Platform contract: PlatformObject interface, Status,
                   Condition, ComponentRelease, Release, Platform,
                   Phase/Condition constants, ManagementSpec, accessor
                   interfaces, and DeepCopy methods.
    validation/    PlatformObject contract validation
                   (Validate, ValidatePlatformObject).

pkg/
  cluster/         Singleton enforcement, metadata options, dynamic ownership
                   (GetSingleton[T], MetaOptions, OwnerRefFrom, ControlledBy,
                   OwnedBy, WithOwnerAnnotations, EnqueueByOwnerAnnotation).
  status/          Safe status subresource updates with automatic conflict
                   retry (Update[T]).
  deploy/          Resource deployment utilities: SSA/patch deploy with
                   pluggable merge strategies, caching, ordering, metrics.
  metadata/        Well-known label and annotation constants.
    annotations/   Contract and recommended annotation keys.
    labels/        Contract and recommended label keys, NormalizePartOfValue.
  webhook/         Admission webhook builder plus singleton validation helpers
                   (ValidateSingletonCreation, CountObjects, DenyCountGtZero).
  client/          Shared Kubernetes client helpers.
  manager/         Controller manager construction helpers.
  rules/           RBAC rule evaluation and authorized resource discovery.
  ingressassignment/
                   Namespace ingress-assignment resolution.
  testing/         Gomega matchers and jq-based test assertions.
  controller/      Controller utilities.
    conditions/    Low-level condition management.
    gc/            Garbage collection (Collector, RBAC authorization,
                   predicates, metrics).
    predicates/    Default and specialized event predicates.
    handlers/      Watch event handlers.
    types/         Reconciliation requests, manifest descriptors, and hashes.
    reconciler/    Generic reconciler and builder.
    actions/       Reconcile pipeline actions: render, deploy, status, GC,
                   caching, dynamic ownership, deletion, and sanity checks.
  render/          Manifest rendering engines (Helm, Kustomize, Go template).
    cacher/        Render caching layer.
    helm/          Helm chart renderer.
    kustomize/     Kustomize overlay renderer.
    template/      Go text/template renderer.
  resources/       Kubernetes resource helpers (Decode, Hash, Apply, Sort,
                   SetLabels, FormatObjectReference, HasAnnotation,
                   GetAnnotation, IsOwnedByType,
                   GetGroupVersionKindForObject, ListAvailableAPIResources,
                   Resource type).
  template/        Template function map (indent, nindent, toYaml).
  tls/             OpenShift TLS profile resolution (APIServer fetch,
                   crypto/tls.Config mapping, proxy flag strings,
                   SecurityProfileWatcher). Scoped openshift/api exception.

testing/
  integration/      Live-cluster PR gate (`integration.Run`).

framework/          Retained behaviorally distinct helpers; still part of the
                    root module (see below).
docs/               Documentation beyond GoDoc.
examples/           Standalone runnable tool modules.
flakiness/          Standalone flakiness tool module.
```

### Moved and Retained Framework Packages

Most former framework public packages now use root `pkg/` paths:

| Previous path | Current path |
|---------------|--------------|
| `framework/controller/actions` | `pkg/controller/actions` |
| `framework/controller/reconciler` | `pkg/controller/reconciler` |
| `framework/controller/handlers` | `pkg/controller/handlers` |
| `framework/controller/types` | `pkg/controller/types` |
| `framework/controller/predicates` | `pkg/controller/predicates` |
| `framework/client` | `pkg/client` |
| `framework/manager` | `pkg/manager` |
| `framework/tls` | `pkg/tls` |
| `framework/rules` | `pkg/rules` |
| `framework/cluster/gvk` | `pkg/cluster/gvk` |
| `framework/metadata` | `pkg/metadata` |
| `framework/webhook` | `pkg/webhook` |
| `framework/utils/ingressassignment` | `pkg/ingressassignment` |
| `framework/utils/test` | `pkg/testing` |
| `framework/testing` | `testing` |

`framework/api` was deleted. Contract types, including `Release` and `Platform`,
are in `api/common`.

The following behaviorally distinct packages intentionally remain under
`framework/` pending separate behavior deduplication:

- `framework/controller/conditions`
- `framework/cluster`
- `framework/resources`
- `framework/render/kustomize`
- `framework/utils/template`

These retained paths are compatibility/helper packages in the root module, not
a separate framework module.

#### Key Framework Concepts

**Action Pipeline**: The reconciler executes an ordered list of `actions.Fn`
functions. Each action receives a `ReconciliationRequest` and can render
manifests, deploy resources, run GC, update conditions, or perform arbitrary
logic. Actions communicate via the shared `ReconciliationRequest.Resources`
slice and `Extensions` map.

**Typical Action Chain**: render (Helm/Kustomize/template) -> deploy ->
dynamic ownership -> GC. The GC action MUST be last.

**Condition Manager**: Inspired by Knative, the `conditions.Manager` tracks a
"happy" condition and zero or more dependent conditions. It automatically
recomputes the happy condition based on the worst-severity dependent. Stale
conditions from previous reconciles are cleaned up automatically.

**Deploy Modes**: SSA (server-side apply, default) or Patch. Both support
per-GVK customizers for special handling (e.g. preserving user-set replicas on
Deployments, removing rules from aggregated ClusterRoles).

**Dynamic Ownership**: When enabled, the reconciler dynamically registers
watches for any GVK it deploys, tracks ownership via owner references, and
supports unmanaged resources (annotation-based opt-out).

## Key Types and Where to Find Them

| Type / Symbol | Package | Purpose |
|---------------|---------|---------|
| `PlatformObject` | `api/common` | Central interface the orchestrator reads |
| `Status` | `api/common` | Common status block (Phase, ObservedGeneration, Conditions) |
| `Condition` | `api/common` | Individual condition observation |
| `ConditionType` | `api/common` | Typed string for condition type identifiers |
| `ConditionTypeReady` | `api/common` | Mandatory condition type constant |
| `ComponentRelease` | `api/common` | Release metadata for a component |
| `Release` | `api/common` | Operator version and platform release metadata |
| `Platform` | `api/common` | Platform release identifier |
| `ManagementSpec` | `api/common` | Management state (Managed/Removed) |
| `Validate` | `api/common/validation` | Runtime PlatformObject contract validation |
| `ValidatePlatformObject` | `api/common/validation` | Test helper wrapping Validate |
| `GetSingleton[T]` | `pkg/cluster` | Retrieve the single CR instance |
| `WithOwnerAnnotations` | `pkg/cluster` | Stamp cross-namespace ownership labels/annotations |
| `EnqueueByOwnerAnnotation` | `pkg/cluster` | MapFunc to resolve owner annotations into reconcile requests |
| `ValidateSingletonCreation` | `pkg/webhook` | Admission webhook singleton guard |
| `Deployer` | `pkg/deploy` | Stateful deployer with cache, merge, ordering |
| `MergeFn` | `pkg/deploy` | Pluggable merge strategy per GVK |
| `Cache` | `pkg/deploy` | TTL-based deploy fingerprint cache |
| `MergeDeployments` | `pkg/deploy` | Preserve user-set replicas/resources |
| `Hash` | `pkg/resources` | SHA-256 content hash of unstructured resource |
| `Apply` | `pkg/resources` | Server-side apply wrapper |
| `SortByApplyOrder` | `pkg/resources` | Dependency-order resource sorting |
| `Manager` | `pkg/controller/conditions` | Condition manager with automatic aggregation |
| `SetStatusCondition` | `pkg/controller/conditions` | Upsert condition with transition time management |
| `FindStatusCondition` | `pkg/controller/conditions` | Get deep copy of a condition |
| `gc.Collector` | `pkg/controller/gc` | Garbage collection of stale resources |
| `gc.RunParams` | `pkg/controller/gc` | Per-reconcile inputs for GC |
| `gc.ListAuthorizedResources` | `pkg/controller/gc` | RBAC-filtered resource discovery |
| `resources.Resource` | `pkg/resources` | API resource type with GVR/GVK/scope |
| `status.Update` | `pkg/status` | Safe status write with conflict retry |
| `status.ErrRetriesExhausted` | `pkg/status` | Sentinel error for exhausted retries |
| `status.WithMaxRetries` | `pkg/status` | Option to configure retry count |
| `GenerationChangedPredicate` | `pkg/controller/predicates` | Update filter on generation change |
| `LabelSelectorPredicate` | `pkg/controller/predicates` | Event filter by label selector |
| `AnnotationChangedPredicate` | `pkg/controller/predicates` | Update filter on single annotation key |
| `DeletionPredicate` | `pkg/controller/predicates` | Pass only delete events |
| `FromAPIServer` | `pkg/tls` | Fetch cluster TLS profile as version/cipher flag strings |
| `ConfigFromProfile` | `pkg/tls` | Map a TLSProfileSpec to controller-runtime TLSOpts |
| `SecurityProfileWatcher` | `pkg/tls` | Restart the manager when the cluster TLS profile or adherence changes |

## Build, Test, and Lint Commands

```bash
# The product module uses Go 1.26.0. Root ./... includes api/, pkg/,
# framework/, and testing/.
make test          # Run tests with race detector and coverage
make lint          # Run golangci-lint (installs if missing)
make lint-fix      # Auto-fix lint issues
make fmt           # Format code (gofmt + goimports)
make vet           # Run go vet
make tidy          # Run go mod tidy
make generate      # Regenerate DeepCopy methods (requires controller-gen)
make all           # fmt + vet + lint + test
make verify-tidy   # Verify go.mod/go.sum are tidy (CI check)
make verify-fmt       # Verify code formatting (CI check)
make verify-generate  # Verify generated deepcopy files are up to date (CI check)
```

## Development Principles

### Reuse Before Writing

**This is a utilities library — search before you write.** Before creating any
new helper, constant, or utility function:

1. **Search the codebase** for existing implementations. Common locations:
   - `api/common/` — condition types, phases, platform contract types
   - `pkg/resources/` — root-module helpers: `Decode`, `Hash`, `SortByApplyOrder`, `Resource`
   - `framework/resources/` — retained helpers: `Apply`, `GvkToUnstructured`, `Source`, `EnsureGroupVersionKind`
   - `pkg/cluster/gvk/` — well-known GVK constants
   - `framework/controller/conditions/` — `IsStatusConditionTrue`, `FindStatusCondition`

2. **Check if it belongs in a shared location.** If you need a GVK constant,
   add it to `pkg/cluster/gvk/gvk.go`. If you need a condition type,
   check `api/common/types.go`. Don't define locals that duplicate shared code.

3. **Convert types to use existing utilities.** Example: to check conditions on
   an `unstructured.Unstructured`, convert to `api.Status` and use
   `conditions.IsStatusConditionTrue()` rather than writing a new function:
   ```go
   statusRaw, found, err := unstructured.NestedMap(u.Object, "status")
   if err != nil || !found {
       return // status not yet populated
   }
   var status api.Status
   if err := runtime.DefaultUnstructuredConverter.FromUnstructured(statusRaw, &status); err != nil {
       return
   }
   conditions.IsStatusConditionTrue(&status, string(common.ConditionTypeReady))
   ```

### Type-Driven Design

Make invalid states unrepresentable through types, not runtime validation:

- **Required params as positional args** — compiler enforces presence
- **Functional options for optional config** — clear API, sensible defaults
- **Strong types over primitives** — `Source` interface over `string` for paths/URLs
- **Constants over magic strings** — `common.ConditionTypeReady` over `"Ready"`

When types can't catch a problem (e.g., file existence), front-load validation
at logical boundaries and fail fast with clear error messages.

## Coding Conventions

### Error Handling

- Wrap errors with `fmt.Errorf("context: %w", err)` to preserve the chain.
- Define sentinel errors (`var ErrFoo = errors.New(...)`) at package level for
  errors callers should match with `errors.Is`.
- Never discard errors silently; return them or log at an appropriate level.

### Naming

- Exported types use descriptive PascalCase names.
- Accessor interfaces follow the `With<Noun>` pattern (e.g. `WithStatus`).
- Condition type constants use `ConditionType<Name>` (e.g. `ConditionTypeReady`).
- Phase constants use `Phase<Name>` (e.g. `PhaseReady`).
- Sentinel errors use `Err<Description>` (e.g. `ErrNoInstance`).

### Kubebuilder Markers

Types in `api/common` carry kubebuilder markers (`+kubebuilder:validation:Enum`,
`+listType`, `+listMapKey`, `+kubebuilder:object:generate=true`). These markers
must be preserved — they drive CRD schema generation and server-side apply
merge strategy for module teams.

### Testing

- Use `t.Parallel()` in all test functions and subtests.
- Prefer table-driven tests for variations of the same assertion.
- Place tests in the `_test` package suffix to exercise the public API.
- Use `github.com/stretchr/testify/assert` and `require` for assertions.

### Dependencies

Keep external dependencies minimal. Direct dependencies are limited to:
- `k8s.io/apimachinery` — Kubernetes API machinery types
- `sigs.k8s.io/controller-runtime` — controller-runtime client interfaces
- `k8s.io/api` — core Kubernetes API types (for admission)

`pkg/tls` is a scoped product-module exception: it may import
`github.com/openshift/api/config/v1`, `github.com/openshift/library-go/pkg/crypto`,
and `github.com/openshift/controller-runtime-common/pkg/tls` for OpenShift TLS
profile and runtime helpers. No other package may import those modules.
`pkg/cluster/openshift` stays unstructured.

Do not introduce dependencies on
`opendatahub-operator` internal packages.

## Versioning Strategy

This project follows [Semantic Versioning](https://semver.org/).

- **Pre-v1** (`v0.x.x`): Breaking changes may occur in minor bumps and will be
  documented in release notes.
- **Post-v1** (`v1.0.0+`): Breaking changes require a major version bump.

See [docs/VERSIONING.md](docs/VERSIONING.md) for the full policy.

## Breaking Change Policy

Changes to the following are considered breaking:
- Removing or renaming exported types, interfaces, constants, or functions
- Changing method signatures on exported interfaces
- Changing JSON struct tags on exported types
- Removing kubebuilder markers that affect CRD generation

When possible, deprecate with a migration window rather than removing outright.
