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
| *Coming soon* | Utility packages will be added as they are extracted from the ODH Operator |

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
