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
| `pkg/render/helm` | Helm chart renderer -- standalone function and action-pipeline adapter |
| `pkg/render/kustomize` | Kustomize overlay renderer with built-in namespace/label/annotation plugins |
| `pkg/render/template` | Go `text/template` renderer with dynamic data support |
| `pkg/render/cacher` | Generic render caching layer (skip re-render when inputs unchanged) |
| `pkg/render` | Shared types (`ReconciliationRequest`, `Fn`), Prometheus metrics |
| `pkg/resources` | Kubernetes resource helpers (`Decode`, `SetLabels`, `SetAnnotations`, `UnstructuredList`) |
| `pkg/template` | Template function map (`indent`, `nindent`, `toYaml`) |

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
