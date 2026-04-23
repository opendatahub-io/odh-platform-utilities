# Deploy Package

## What This Package Does

`pkg/deploy` provides the deploy step of the render-then-deploy pipeline. It
takes `[]unstructured.Unstructured` resources (typically produced by
`pkg/render`) and applies them to the cluster with:

- Server-Side Apply (SSA) as default mode, with configurable field ownership
- Patch mode as an alternative
- Pluggable merge strategies per GVK to preserve user customisations
- Deploy caching to skip unchanged resources
- CRD-specific handling (platform field owner, no controller owner references)
- Annotation stamping for GC integration

## Key Types

| Type | Purpose |
|------|---------|
| `Deployer` | Stateful deployer with cached merge strategies and options |
| `DeployInput` | Per-invocation parameters (client, resources, owner, release) |
| `MergeFn` | Merge strategy: `func(existing, desired *unstructured.Unstructured) error` |
| `SortFn` | Resource ordering: `func(ctx, resources) (resources, error)` |
| `Cache` | TTL-based deploy fingerprint cache |
| `ReleaseInfo` | Platform type and version for annotation stamping |
| `Mode` | `ModeSSA` or `ModePatch` |

## Built-in Merge Strategies

- `MergeDeployments` — preserves user-set container resources and replicas
- `MergeObservabilityResources` — preserves `spec.resources` for monitoring types
- `RemoveDeploymentResources` — strips resources/replicas for patch mode
- `RevertDeploymentDrift` — SMP to clear user drift on managed Deployments

## Conventions

- All exported functions have GoDoc comments.
- Tests use `_test` package suffix, `t.Parallel()`, Gomega assertions.
- No imports from `github.com/opendatahub-io/opendatahub-operator`.
- Annotation/label constants are imported from `pkg/metadata/`.

## Deploy Loop Outline

```text
for each resource:
  1. Stamp labels + annotations (PlatformPartOf, instance metadata, release)
  2. Check managed annotation on existing resource → skip if "false"
  3. Check cache → skip if unchanged
  4. If CRD → deploy with platform field owner, no controller ref
  5. Else → apply merge strategy, set controller ref, SSA/patch
  6. Update cache
  7. Increment metrics counter
```
