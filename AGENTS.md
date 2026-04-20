# ODH Platform Utilities — agent guide

This repository is a **Go library** of shared helpers for [Open Data Hub](https://opendatahub.io/) module controllers (manifest rendering, resource utilities, template helpers). It is consumed via standard Go modules, not deployed as a long-running service.

## Read first

| Document | Purpose |
|----------|---------|
| [README.md](./README.md) | Overview, packages table, basic usage snippets |
| [CONTRIBUTING.md](./CONTRIBUTING.md) | PR workflow, prerequisites, `make` targets |
| [docs/VERSIONING.md](./docs/VERSIONING.md) | SemVer and pre-v1 API expectations |
| [pkg/render/AGENTS.md](./pkg/render/AGENTS.md) | Helm / Kustomize / Go-template engines, caching, metrics, namespace rules |

When editing anything under `pkg/render/`, treat **`pkg/render/AGENTS.md` as required context** for behavior of `render.Hash`, action adapters, and cache semantics.

## Build and quality

The [Makefile](./Makefile) pins **`GOTOOLCHAIN` to the `go` version in `go.mod`** so `go test -race` stays consistent across machines. Prefer **`make test`** over raw `go test` unless you have a reason.

Common targets:

- `make fmt` — format (gofmt + golangci-lint fmt when installed)
- `make vet` — `go vet ./...`
- `make lint` — golangci-lint (installs linter via `make golangci-lint` if missing)
- `make test` — race + coverage → `cover.out`
- `make all` — fmt, vet, lint, test (full local gate)

CI runs lint, tests, `go mod tidy` verification, and formatting checks (see [.github/workflows/ci.yaml](./.github/workflows/ci.yaml)).

## Layout

- **`pkg/render/`** — shared `ReconciliationRequest` / `Fn`, `render.Hash`, Prometheus metrics, plus `helm`, `kustomize`, `template`, and `cacher` subpackages
- **`pkg/resources/`** — YAML decode and metadata helpers on unstructured objects
- **`pkg/template/`** — `text/template` funcmap (`indent`, `nindent`, `toYaml`)
- **`examples/`** — placeholder until runnable examples land (see [examples/README.md](./examples/README.md))
- **`docs/`** — versioning and other project docs

## Conventions for changes

1. **Prefer minimal, focused diffs** — this is a shared library; avoid unrelated refactors.
2. **Match existing style** — imports, naming, error wrapping (`fmt.Errorf` with `%w`), and test layout (Gomega/dot imports where already used).
3. **Tests** — add or extend tests under the same package (or `_test` package where the tree already does). Run `make test` before pushing.
4. **API changes** — pre-v1 releases may break callers; still document notable behavior changes in PR description and update GoDoc / subsystem docs as needed.
5. **RBAC / controllers** — this repo has no Kubernetes controllers; do not add `kubebuilder:rbac` markers here unless the project scope changes.

## Release notes

Tag-driven releases use [.github/workflows/release.yaml](./.github/workflows/release.yaml). Follow [docs/VERSIONING.md](./docs/VERSIONING.md) when choosing version bumps.
