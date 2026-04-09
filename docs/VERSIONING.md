# Versioning Strategy

This project follows [Semantic Versioning 2.0.0](https://semver.org/).

## Version Format

```
vMAJOR.MINOR.PATCH[-prerelease]
```

- **MAJOR**: Incompatible API changes
- **MINOR**: New functionality in a backward-compatible manner
- **PATCH**: Backward-compatible bug fixes

## Phases

### Pre-v1 (v0.x.x) — Current

The API is under active development. Consumers should expect changes.

- **Minor bumps** (v0.1.0 → v0.2.0) may contain breaking changes, documented
  in release notes.
- **Patch bumps** (v0.1.0 → v0.1.1) are backward-compatible bug fixes only.
- Pin to a specific minor version if you need stability:
  `go get github.com/opendatahub-io/odh-platform-utilities@v0.1.0`

### Post-v1 (v1.0.0+) — Future

Once the API is considered stable:

- **Major bumps** are required for any breaking changes.
- Deprecated APIs will carry a deprecation notice for at least one minor
  release before removal.
- The module path will include the major version for v2+:
  `github.com/opendatahub-io/odh-platform-utilities/v2`

## Creating a Release

1. Ensure `main` is in a releasable state (CI green, docs updated).

2. Tag the release:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

3. The [release workflow](./../.github/workflows/release.yaml) will
   automatically create a GitHub Release with auto-generated release notes.

4. The Go module proxy will pick up the new version automatically. Verify at:
   `https://pkg.go.dev/github.com/opendatahub-io/odh-platform-utilities@v0.1.0`

## Retraction

If a version is published with a critical bug, it can be retracted by adding a
`retract` directive to `go.mod`:

```go
retract v0.1.0 // contains critical bug in X
```

Then tag and release a new patch version that includes the retraction.

## Go Module Proxy

Published versions are cached by the Go module proxy (`proxy.golang.org`).
Once a version is published, its contents are immutable — you cannot overwrite
a tag. If you need to fix a released version, publish a new patch release.
