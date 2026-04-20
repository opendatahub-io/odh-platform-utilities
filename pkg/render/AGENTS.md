# Manifest Rendering Engines

This package provides three rendering engines for converting embedded manifest
sources into `[]unstructured.Unstructured` resources ready for deployment.

## Engines

### Helm (`pkg/render/helm`)

Renders Helm charts using `k8s-manifest-kit/renderer-helm`. Best for teams
that maintain Helm charts as their packaging format.

- Wraps the external `renderer-helm` library (no Helm lifecycle -- templates
  only, no releases or hooks).
- Supports label/annotation injection via `k8s-manifest-kit` transformers.
- Post-rendering automatically applies Kubernetes apply-order sorting.

### Kustomize (`pkg/render/kustomize`)

Renders Kustomize overlays using `sigs.k8s.io/kustomize/api`. Best for teams
that use Kustomize bases and overlays.

- Built-in plugins for namespace, label, and annotation injection.
- Supports custom `resmap.Transformer` plugins and `FilterFn` callbacks.
- Configurable filesystem (use `WithEngineFS` for in-memory testing).

### Template (`pkg/render/template`)

Renders Go `text/template` files from `fs.FS` sources. Best for dynamic YAML
generation that depends on runtime data (instance metadata, computed values).

- Built-in template functions: `indent`, `nindent`, `toYaml` (extensible via
  `WithFuncMap`).
- Supports static data (`WithData`) and dynamic data functions (`WithDataFn`).
- Template data automatically includes `Component` (the Kubernetes instance)
  and `AppNamespace` (the target namespace).

## Usage Patterns

### Standalone Functions

For module teams that do NOT use the action pipeline:

```go
// Helm
resources, err := helm.Render(ctx, chartSources, helm.WithLabel("app", "mine"))

// Kustomize
resources, err := kustomize.Render(path, engineOpts, kustomize.WithNamespace(ns))

// Template
resources, err := template.Render(ctx, scheme, sources, data, template.WithLabel("app", "mine"))
```

### Action Pipeline Adapters

For teams using the reconciler builder with `[]render.Fn` actions:

```go
actions := []render.Fn{
    helm.NewAction([]helm.Option{helm.WithLabel("app", "mine")}),
    kustomize.NewAction(engineOpts, kustomize.WithActionNamespace(ns)),
    template.NewAction(template.WithData(data), template.WithNamespace(ns)),
}

for _, action := range actions {
    if err := action(ctx, &rr); err != nil {
        return err
    }
}
```

The action adapters read inputs from `ReconciliationRequest` (HelmCharts,
Manifests, Templates) and write rendered resources to `rr.Resources`.

## Caching

All action adapters cache rendered resources by default. A render is skipped
when the cache key is unchanged. The base key, `render.Hash(ctx, rr)`, covers:

- Instance UID and generation
- Kustomize manifest paths (`ManifestInfo`)
- Template paths and per-template labels and annotations (`TemplateInfo`)
- Helm chart identity and values from each chart’s `Values(ctx)` loader

**Helm (`helm.NewAction`)** extends the base key with a digest of render
options (labels, annotations, and transformer identity). Pass `ctx` into
`render.Hash` so Helm values loading respects cancellation and deadlines.

**Kustomize (`kustomize.NewAction`)** extends the base key with the resolved
action namespace (from `WithActionNamespace` or `WithActionNamespaceFn`).
Content changes under a fixed on-disk path are **not** detected; use embedded
manifests, bump instance generation, or disable caching when directories are
mutable.

**Template (`template.NewAction`)** extends the base key with the resolved
template data map (excluding `Component`, which mirrors the instance already
hashed) and with a digest of action-level `WithAction*` label/annotation
options plus each `text/template` function identity. Template data is built
once per reconciliation (`buildData`) and reused for both keying and
rendering.

- Cache is keyed per-action instance (each `NewAction()` call has its own cache).
- `Cacher` / `ResourceCacher` are intended for single-threaded use per action
  (typical controller-runtime reconcile).
- Disable with `WithCache(false)`.
- When served from cache, `rr.Generated` remains `false`; downstream GC actions
  can use this to skip unnecessary work.

## Namespace Injection

Namespace is always explicit -- callers provide it, the library never reads it
from the cluster:

- **Kustomize**: `WithNamespace(ns)` at render-option level, or
  `WithActionNamespace(ns)` / `WithActionNamespaceFn(fn)` at action level.
- **Template**: `WithNamespace(ns)` / `WithNamespaceFn(fn)` at action level,
  or set `AppNamespaceKey` in template data for standalone usage.
- **Helm**: Namespace is set in the chart's `values.yaml` by convention.

## Metrics

`render.RenderedResourcesTotal` is a Prometheus counter with labels
`controller` (lowercase Kind of the instance) and `engine` (helm, kustomize,
template). It is registered with the controller-runtime metrics registry.

## Relationship to Deploy

Rendering produces `[]unstructured.Unstructured`. Deploying (applying to the
cluster) is a separate concern handled by deploy actions. Render output feeds
deploy input.
