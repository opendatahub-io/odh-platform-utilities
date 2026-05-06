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

## Package Structure

```text
api/
  common/          Platform contract: PlatformObject interface, Status,
                   Condition, ComponentRelease types, Phase/Condition
                   constants, ManagementSpec, accessor interfaces,
                   DeepCopy methods.

pkg/
  cluster/         Singleton enforcement, metadata options (GetSingleton[T],
                   MetaOptions, OwnerRefFrom, ControlledBy, OwnedBy).
  deploy/          Resource deployment utilities: SSA/patch deploy with
                   pluggable merge strategies, caching, ordering, metrics.
  metadata/        Well-known label and annotation constants.
    annotations/   Contract and recommended annotation keys.
    labels/        Contract and recommended label keys, NormalizePartOfValue.
  webhook/         Admission webhook helpers for singleton validation
                   (ValidateSingletonCreation, CountObjects, DenyCountGtZero).
  render/          Manifest rendering engines (Helm, Kustomize, Go template).
    cacher/        Render caching layer.
    helm/          Helm chart renderer.
    kustomize/     Kustomize overlay renderer.
    template/      Go text/template renderer.
  resources/       Kubernetes resource helpers (Decode, Hash, Apply, Sort,
                   SetLabels, FormatObjectReference, etc.).
  template/        Template function map (indent, nindent, toYaml).

docs/              Documentation beyond GoDoc.
examples/          Runnable usage examples.
```

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
| `GetSingleton[T]` | `pkg/cluster` | Retrieve the single CR instance |
| `ValidateSingletonCreation` | `pkg/webhook` | Admission webhook singleton guard |
| `Deployer` | `pkg/deploy` | Stateful deployer with cache, merge, ordering |
| `MergeFn` | `pkg/deploy` | Pluggable merge strategy per GVK |
| `Cache` | `pkg/deploy` | TTL-based deploy fingerprint cache |
| `MergeDeployments` | `pkg/deploy` | Preserve user-set replicas/resources |
| `Hash` | `pkg/resources` | SHA-256 content hash of unstructured resource |
| `Apply` | `pkg/resources` | Server-side apply wrapper |
| `SortByApplyOrder` | `pkg/resources` | Dependency-order resource sorting |

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
