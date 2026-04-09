# Contributing to ODH Platform Utilities

Thank you for your interest in contributing! This document provides guidelines
and instructions for contributing to this project.

## Prerequisites

- **Go 1.25+** — match the version in `go.mod`
- **golangci-lint v2.5.0+** — installed automatically by `make lint` if missing
- **pre-commit** — optional but recommended for local development

## Getting Started

```bash
git clone https://github.com/opendatahub-io/odh-platform-utilities.git
cd odh-platform-utilities

# Install pre-commit hooks (recommended)
pre-commit install
pre-commit install --hook-type pre-push
```

## Development Workflow

### Running Tests

```bash
make test
```

This runs all tests with the race detector enabled and generates a coverage
report at `cover.out`.

### Running the Linter

```bash
make lint       # check for issues
make lint-fix   # auto-fix where possible
```

### Formatting Code

```bash
make fmt
```

### Verifying Before Submitting

```bash
make all        # runs fmt, vet, lint, and test
```

## Pull Request Process

1. **Fork** the repository and create a feature branch from `main`.
2. Make your changes in small, focused commits.
3. Ensure all tests pass and the linter reports no issues.
4. Write or update tests for any new or changed functionality.
5. Update documentation (GoDoc comments, README, examples) as needed.
6. Open a pull request against `main` with a clear description of the change.

### Commit Messages

Use clear, descriptive commit messages. Preferred format:

```
<type>: <short summary>

<optional body explaining the "why">
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `ci`

Examples:
- `feat: add environment detection utilities`
- `fix: handle nil config map in manifest renderer`
- `docs: add examples for resource management package`

## Code Standards

### GoDoc Comments

Every exported symbol (function, type, constant, variable) **must** have a
GoDoc comment. Follow these conventions:

- Start with the name of the symbol: `// FunctionName does X.`
- Use complete sentences with proper punctuation.
- Include usage examples for non-trivial APIs using `Example` test functions.
- Document error conditions and edge cases.

```go
// DetectPlatform returns the platform type (OpenShift or Kubernetes) by
// inspecting the cluster's API resources. It returns an error if the
// cluster is unreachable.
func DetectPlatform(ctx context.Context) (Platform, error) {
    // ...
}
```

### Package Organization

- Public utility packages go under `pkg/`.
- Internal helpers that should not be imported by consumers go under `internal/`.
- Each package should have a focused, well-defined purpose.
- Avoid circular dependencies between packages.

### Testing

- Write table-driven tests where applicable.
- Use `t.Parallel()` in all test functions.
- Aim for meaningful coverage — test edge cases and error paths, not just the
  happy path.
- Place tests in a `_test` package (e.g., `package foo_test`) to test the
  public API surface.

## Breaking Changes

### Pre-v1 (v0.x.x)

Breaking changes are allowed in minor version bumps. When introducing a
breaking change:

1. Document it clearly in the PR description.
2. It will be highlighted in the release notes.

### Post-v1 (v1.0.0+)

Breaking changes require a major version bump. Prefer deprecation with a
migration path over breaking changes when possible.

## Dependencies

Keep external dependencies minimal. Before adding a new dependency:

1. Consider whether the functionality can be implemented with the standard
   library.
2. Evaluate the dependency's maintenance status and license compatibility.
3. Document the rationale in your PR.

## Questions?

Open an issue for any questions about contributing.
