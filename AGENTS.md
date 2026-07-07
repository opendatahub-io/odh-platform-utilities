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
- [Onboarding Guide for ODH Operator Modules](https://docs.google.com/document/d/1FeJk5mMPGMGMNqMAiGn0-cTKcNxblDYAkhU4DOmcpns)
- [ODH Operator Evolution](https://docs.google.com/document/d/1mOuXIKkqbh3rS35g4JdWTj5HvjQ_a-7u7HBwBIqlIpI)

## Repository Structure

This repository contains **two Go modules**:

| Module | Path | Purpose |
|--------|------|---------|
| `github.com/opendatahub-io/odh-platform-utilities` | `/` (root) | Low-level, dependency-light utilities |
| `github.com/opendatahub-io/odh-platform-utilities/framework` | `framework/` | Opinionated controller framework |

The `framework/` module depends on the root module (via `replace` directive for
local development).

### Root Module

```text
api/
  common/          Platform contract: PlatformObject interface, Status,
                   Condition, ComponentRelease types, Phase/Condition
                   constants, ManagementSpec, accessor interfaces,
                   DeepCopy methods.
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
  webhook/         Admission webhook helpers for singleton validation
                   (ValidateSingletonCreation, CountObjects, DenyCountGtZero).
  controller/      Controller utilities.
    conditions/    Knative-inspired condition management with automatic
                   aggregation, severity-based filtering, Manager pattern,
                   and low-level condition CRUD helpers.
    gc/            Garbage collection (Collector, RBAC authorization,
                   predicates, metrics).                   
    predicates/    Optional event-filtering predicates for controller-runtime
                   (GenerationChanged, LabelSelector, AnnotationChanged,
                   Deletion).
    actions/       Action function type (Fn) and ReconciliationRequest definition.        
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

docs/              Documentation beyond GoDoc.
examples/          Runnable usage examples.
```

### Framework Module (`framework/`)

The framework module provides an opinionated, batteries-included controller
framework built on controller-runtime. It is designed for teams building ODH
module controllers and provides a complete reconcile/deploy/GC lifecycle out of
the box.

```text
framework/
  api/                          Re-exports root-module platform types;
                                adds Release, Platform types.
  cluster/                      CRD existence checks, API availability,
                                singleton listing (HasCRD, IsAPIAvailable,
                                ListGVK).
    gvk/                        Well-known GroupVersionKind constants
                                (Deployment, ClusterRole, monitoring CRDs).
  controller/
    reconciler/                 Generic Reconciler with:
                                - Finalizer management (add/remove)
                                - Action pipeline execution
                                - Condition aggregation & phase computation
                                - Status SSA write
                                - Pre-apply hooks
                                - Dynamic ownership support
                                - Default requeue interval for polling
                                  non-Kubernetes state
                                Options: WithRelease, WithFinalizerName,
                                WithConditionsManagerFactory, WithPhaseNames,
                                WithPreApplyFn, WithDynamicOwnership,
                                WithDefaultRequeueAfter, etc.
                                ReconcilerBuilder also exposes
                                WatchesRawSource to register a raw
                                controller-runtime source.Source (e.g. a
                                channel-backed source.Channel) for triggering
                                reconciles from non-Kubernetes events.
    actions/                    Action function type (Fn) and Getter[T].
      deploy/                   Resource deployment via SSA or patch:
                                - Deploy cache (fingerprint-based skip)
                                - Per-GVK customizers (Deployments, ClusterRoles,
                                  observability CRs)
                                - Merge strategies (preserve user replicas/resources)
                                - Configurable labels, annotations, field owner
                                - Apply-order sorting
                                Options: WithMode, WithCache, WithApplyOrder,
                                WithApplyCustomizer, WithPatchCustomizer, etc.
      gc/                       Label-selector garbage collection:
                                - RBAC-aware resource discovery
                                - Type and object predicates
                                - Configurable propagation policy
                                - Unremovable GVK safelist (CRDs, Leases)
                                - Prometheus metrics
                                Options: WithPartOfLabel, WithUnremovables,
                                WithObjectPredicate, WithTypePredicate, etc.
      render/
        helm/                   Helm chart render action.
        kustomize/              Kustomize render action.
        template/               Go template render action.
      deleteresource/           Explicit resource deletion action.
      dynamicownership/         Dynamic watch registration for deployed
                                resources with ownership tracking and
                                CRD watches.
      sanitycheck/              Pre-reconciliation cluster state validation
                                (ensure unwanted CRDs are absent).
      status/
        deployments/            Deployment availability status checks.
      resourcecacher/           Reconciliation-level resource caching.
      cacher/                   Action-level caching (skip re-execution
                                when inputs unchanged).
      errors/                   StopError sentinel for halting the action
                                pipeline without returning a real error.
    conditions/                 Knative-inspired condition Manager:
                                - Automatic happiness aggregation from
                                  dependent conditions
                                - Severity-based filtering (Error only)
                                - Stale condition cleanup
                                - Sort by priority
                                Helpers: SetStatusCondition,
                                FindStatusCondition, RemoveStatusCondition,
                                IsStatusConditionTrue.
    handlers/                   Watch event handlers: LabelToName,
                                AnnotationToName, ToNamed, RequestFromObject.
    predicates/                 Default predicates (DefaultPredicate,
                                DefaultDeploymentPredicate) plus sub-packages:
      dependent/                Dependent resource predicate.
      generation/               Generation-changed predicate.
      hash/                     Hash-changed predicate.
      label/                    Label-selector predicate.
      partial/                  Partial-object predicate.
      resources/                Resource-specific predicates (Deployment
                                status, created/deleted/named).
    types/                      ReconciliationRequest, ManifestInfo,
                                HelmChartInfo, TemplateInfo, Controller
                                interface, Hash/HashStr utilities.
  metadata/                     Annotation suffix constants used by
                                deploy/GC actions.
  resources/                    Resource helpers: GVK resolution,
                                apply-order sorting, owner references,
                                status apply, EnsureGroupVersionKind.
  rules/                        RBAC rule evaluation:
                                SelfSubjectRulesReview, permission checks,
                                authorized resource listing.
  utils/
    template/                   Template utilities.
    test/
      matchers/                 Gomega matchers and jq-based assertions
                                for integration tests.
```

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

## Build, Test, and Lint Commands

```bash
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

Do not introduce dependencies on `github.com/openshift/api` or on the
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
