# ODH Platform Utilities

Shared Go utilities for [Open Data Hub](https://opendatahub.io/) module controller development.

This library provides common functionality extracted from the
[ODH Operator](https://github.com/opendatahub-io/opendatahub-operator) to
accelerate module controller development and ensure consistency across modules.

## Installation

```bash
go get github.com/opendatahub-io/odh-platform-utilities
```

## Usage

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/<package>"
```

Packages are organized under `pkg/` with each utility in its own sub-package.
See the [examples](./examples/) directory for runnable usage examples and the
[GoDoc](https://pkg.go.dev/github.com/opendatahub-io/odh-platform-utilities)
for full API documentation.

## Packages

| Package | Description |
|---------|-------------|
| `api/common` | Platform contract: `PlatformObject` interface, `Status`, `Condition`, `ComponentRelease` types, condition/phase constants |
| `pkg/metadata/labels` | Platform label key constants and `NormalizePartOfValue` helper |
| `pkg/metadata/annotations` | Platform annotation key constants |
| `pkg/cluster` | Functional options for setting labels, annotations, owner references, and namespace on `client.Object`; singleton enforcement (`GetSingleton[T]`) |
| `pkg/webhook` | Admission webhook helpers for singleton validation (`ValidateSingletonCreation`) |
| `pkg/render/helm` | Helm chart renderer -- standalone function and action-pipeline adapter |
| `pkg/render/kustomize` | Kustomize overlay renderer with built-in namespace/label/annotation plugins |
| `pkg/render/template` | Go `text/template` renderer with dynamic data support |
| `pkg/render/cacher` | Generic render caching layer (skip re-render when inputs unchanged) |
| `pkg/render` | Shared types (`ReconciliationRequest`, `Fn`), Prometheus metrics |
| `pkg/resources` | Kubernetes resource helpers (`Decode`, `SetLabels`, `SetAnnotations`, `UnstructuredList`) |
| `pkg/template` | Template function map (`indent`, `nindent`, `toYaml`) |

## Platform Contract

Module controllers that participate in the ODH platform must implement the
`PlatformObject` interface. See [docs/platform-object-contract.md](./docs/platform-object-contract.md)
for the full contract specification and a copy-pasteable implementation example.

## Metadata Conventions

The platform uses a set of well-known labels and annotations as conventions
between the orchestrator, module controllers, and deployed resources. The
`pkg/metadata` and `pkg/cluster` packages give module teams a single import
for all platform metadata, avoiding string duplication and drift across repos.

### Contract (orchestrator requires these)

| Kind | Constant | Value | Purpose |
|------|----------|-------|---------|
| Label | `labels.ManagedBy` | `components.platform.opendatahub.io/managed-by` | Orchestrator discovery -- must be present on bootstrap resources |
| Annotation | `annotations.ManagementStateAnnotation` | `component.opendatahub.io/management-state` | Orchestrator writes `Managed` / `Removed` on module CRs |
| Annotation | `annotations.ManagedByODHOperator` | `opendatahub.io/managed` | Set to `"false"` to opt a resource out of updates (create-only) |

### Recommended standard (consistency across modules)

| Kind | Constant | Value | Purpose |
|------|----------|-------|---------|
| Label | `labels.PlatformPartOf` | `platform.opendatahub.io/part-of` | Controller ownership -- used by GC label selector |
| Label | `labels.PlatformDependency` | `platform.opendatahub.io/dependency` | Dependency relationships |
| Label | `labels.InfrastructurePartOf` | `infrastructure.opendatahub.io/part-of` | Infrastructure-layer ownership |
| Annotation | `annotations.PlatformVersion` | `platform.opendatahub.io/version` | Release version at deploy time |
| Annotation | `annotations.PlatformType` | `platform.opendatahub.io/type` | Platform type at deploy time |
| Annotation | `annotations.InstanceGeneration` | `platform.opendatahub.io/instance.generation` | CR generation at deploy time |
| Annotation | `annotations.InstanceName` | `platform.opendatahub.io/instance.name` | CR name for event routing |
| Annotation | `annotations.InstanceUID` | `platform.opendatahub.io/instance.uid` | CR UID for stale-resource detection |

See [pkg/metadata/AGENTS.md](./pkg/metadata/AGENTS.md) for detailed semantics,
the deploy/GC annotation lifecycle, and the opt-out convention.

```go
import (
    "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

// Use constants directly
partOf, err := labels.NormalizePartOfValue("MyComponent")
if err != nil {
    return err
}
obj.SetLabels(map[string]string{
    labels.PlatformPartOf: partOf,
    labels.ManagedBy:      "my-controller",
})

// Or use the functional-option helpers
cluster.ApplyMetaOptions(obj,
    cluster.WithLabels(labels.PlatformPartOf, partOf),
    cluster.WithAnnotations(annotations.InstanceName, "my-cr"),
    cluster.InNamespace("target-ns"),
)
```

## Manifest Rendering

Module controllers embed their own manifests and use these utilities to render
them into `[]unstructured.Unstructured` before applying to the cluster.

### Standalone usage

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/render/helm"

resources, err := helm.Render(ctx, chartSources,
    helm.WithLabel("app.kubernetes.io/part-of", "my-component"),
    helm.WithAnnotation("platform.opendatahub.io/release", "1.0.0"),
)
```

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/render/kustomize"

resources, err := kustomize.Render(manifestPath,
    []kustomize.EngineOptsFn{kustomize.WithEngineFS(embeddedFS)},
    kustomize.WithNamespace("my-namespace"),
    kustomize.WithLabel("app", "my-component"),
)
```

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/render/template"

resources, err := template.Render(ctx, scheme, sources, templateData,
    template.WithLabel("app", "my-component"),
)
```

### Action pipeline usage

For teams using the reconciler builder pattern:

```go
import (
    "github.com/opendatahub-io/odh-platform-utilities/pkg/render"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/render/helm"
)

action := helm.NewAction(
    []helm.Option{helm.WithLabel("app", "my-component")},
    helm.WithCache(true),
)

// action is a render.Fn that reads rr.HelmCharts and writes to rr.Resources
err := action(ctx, &rr)
```

See [pkg/render/AGENTS.md](./pkg/render/AGENTS.md) for detailed documentation
on each engine, caching behavior, and namespace injection.

## Versioning

This project follows [Semantic Versioning](https://semver.org/).

- **Pre-v1** (`v0.x.x`): The API is under active development. Breaking changes
  may occur in minor version bumps and will be documented in release notes.
- **Post-v1** (`v1.0.0+`): The public API is stable. Breaking changes require a
  major version bump.

See [docs/VERSIONING.md](./docs/VERSIONING.md) for the full versioning strategy.

## Development

### Prerequisites

- Go 1.25+
- [golangci-lint](https://golangci-lint.run/) v2.5.0+
- [pre-commit](https://pre-commit.com/) (optional but recommended)

### Quick start

```bash
# Clone the repository
git clone https://github.com/opendatahub-io/odh-platform-utilities.git
cd odh-platform-utilities

# Install pre-commit hooks
pre-commit install
pre-commit install --hook-type pre-push

# Run tests
make test

# Run linter
make lint

# See all available targets
make help
```

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines on how to contribute.

## License

Apache License 2.0 — see [LICENSE](./LICENSE) for details.
