# Module Operator Scenario Catalog

This document catalogs concrete scenarios module controller teams encounter when
building out-of-tree operators against the shared utilities library. It is not a
prescriptive reference implementation — your operator does not have to look
exactly like this. Instead, each scenario shows what the library offers, when to
reach for each piece, and what trade-offs each path carries.

Each scenario is self-contained. Read only the sections relevant to your module.

**Conventions used in this document:**

- `root` refers to the root Go module (`github.com/opendatahub-io/odh-platform-utilities`)
- `framework` refers to the framework Go module (`github.com/opendatahub-io/odh-platform-utilities/framework`)
- Code snippets are illustrative, not compilable standalone programs
- See [GoDoc](https://pkg.go.dev/github.com/opendatahub-io/odh-platform-utilities) for full API signatures

---

## Table of Contents

1. [Reconciliation Strategy Selection](#1-reconciliation-strategy-selection)
2. [Manifest Rendering](#2-manifest-rendering)
3. [Resource Deployment and Ownership](#3-resource-deployment-and-ownership)
4. [Garbage Collection](#4-garbage-collection)
5. [Status and Condition Management](#5-status-and-condition-management)
6. [CRD Design and PlatformObject Contract](#6-crd-design-and-platformobject-contract)
7. [Environment Detection](#7-environment-detection)
8. [Dependency Management Between Modules](#8-dependency-management-between-modules)
9. [Upgrade and Migration](#9-upgrade-and-migration)
10. [Testing Patterns](#10-testing-patterns)

---

## 1. Reconciliation Strategy Selection

**Context:** You're starting a new module controller and need to decide how much
of the framework to adopt. The library supports a spectrum from fully managed
pipeline to bare controller-runtime with cherry-picked utilities.

### Option A: Pipeline Approach (ReconcilerBuilder)

Use the framework's `ReconcilerBuilder` to get an ordered action pipeline with
built-in finalizer management, condition aggregation, phase computation, and
status SSA writes.

```go
import (
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/reconciler"
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/gc"
    renderhelm "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/helm"
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/deployments"
)

func SetupController(mgr ctrl.Manager, release api.Release) error {
    _, err := reconciler.ReconcilerFor(mgr, &MyModuleCR{}).
        WithReconcilerOpts(
            reconciler.WithRelease(release),
            reconciler.WithFinalizerName("mymodule.opendatahub.io/finalizer"),
        ).
        WithConditions("DeploymentsAvailable").
        WithAction(initAction).                      // populate rr.HelmCharts
        WithAction(renderhelm.NewAction()).           // render → rr.Resources
        WithAction(deploy.NewAction(                  // apply to cluster
            deploy.WithMode(deploy.ModeSSA),
            deploy.WithCache(),
        )).
        WithAction(deployments.NewAction(             // check deployment health
            deployments.InNamespaceFn(namespaceFn),
        )).
        WithAction(gc.NewAction(namespaceFn)).        // delete stale resources
        Build(ctx)

    return err
}
```

The builder handles:
- Finalizer add/remove lifecycle
- Sequential action execution with error propagation
- Condition aggregation from dependents to the `Ready` condition
- Phase computation (`Ready` / `Not Ready`)
- Status write via server-side apply

### Option B: Standalone Approach

Use raw controller-runtime and import individual library functions. This suits
teams with unusual reconciliation flows or minimal dependency requirements.

```go
import (
    "github.com/opendatahub-io/odh-platform-utilities/pkg/render/helm"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/deploy"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/gc"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/status"
)

func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    instance := &MyModuleCR{}
    if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    resources, err := helm.Render(ctx, chartSources,
        helm.WithLabel("app.kubernetes.io/part-of", "my-module"),
    )
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("render: %w", err)
    }

    deployer := deploy.NewDeployer(
        deploy.WithMode(deploy.ModeSSA),
        deploy.WithApplyOrder(),
    )
    if err := deployer.Deploy(ctx, deploy.DeployInput{
        Client:    r.Client,
        Owner:     instance,
        Resources: resources,
        Release:   deploy.ReleaseInfo{Version: "1.0.0"},
    }); err != nil {
        return ctrl.Result{}, fmt.Errorf("deploy: %w", err)
    }

    collector := gc.New(gc.InNamespace("my-namespace"))
    if err := collector.Run(ctx, gc.RunParams{
        Client:          r.Client,
        DynamicClient:   r.DynamicClient,
        DiscoveryClient: r.DiscoveryClient,
        Owner:           instance,
        Version:         "1.0.0",
    }); err != nil {
        return ctrl.Result{}, fmt.Errorf("gc: %w", err)
    }

    mgr := conditions.NewManager(instance.GetStatus(), "Ready", "ProvisioningSucceeded")
    mgr.MarkTrue("ProvisioningSucceeded")
    // mgr aggregates to Ready automatically

    return ctrl.Result{}, status.Update(ctx, r.Client, instance, func(obj *MyModuleCR) {
        obj.Status = *instance.GetStatus()
    })
}
```

### Option C: Hybrid

Use the builder for structure but inject custom actions or skip standard ones.
For example, use the pipeline for lifecycle management but implement a custom
deploy step that conditionally deploys subsets of resources.

```go
reconciler.ReconcilerFor(mgr, &MyModuleCR{}).
    WithAction(renderKustomize.NewAction(
        renderKustomize.WithEngine(engine),
    )).
    WithAction(customFilterAction).    // your logic: remove resources based on spec
    WithAction(deploy.NewAction()).    // deploy only what remains in rr.Resources
    WithAction(gc.NewAction(nsFn)).
    Build(ctx)
```

Actions communicate through the shared `ReconciliationRequest`: earlier actions
mutate `rr.Resources`, `rr.Extensions`, or `rr.SkipDeploy` for later ones.

### Trade-offs

| Dimension | Pipeline | Standalone | Hybrid |
|-----------|----------|------------|--------|
| Boilerplate | Minimal — lifecycle handled | You manage finalizers, status writes, condition aggregation | Medium |
| Flexibility | Constrained to sequential action model | Full control over reconcile flow | Best of both |
| Testability | Test individual `actions.Fn` in isolation | Test your reconciler directly | Mix both |
| Learning curve | Must understand action pipeline semantics | Familiar controller-runtime patterns | Both |
| Condition management | Automatic aggregation + stale cleanup | Manual via `conditions.Manager` | Automatic |
| Status writes | SSA write handled by reconciler | Manual via `status.Update` or SSA | Automatic |

**Recommendation:** Start with the pipeline. If you hit a case it can't express,
inject a custom action rather than dropping to standalone. The pipeline's
finalizer management and condition aggregation eliminate a class of bugs that
standalone controllers commonly have.

---

## 2. Manifest Rendering

**Context:** Your module embeds Kubernetes manifests (YAML, Helm charts,
Kustomize overlays, or Go templates) and needs to render them into
`[]unstructured.Unstructured` before applying to the cluster.

### Option A: Kustomize Overlays

Most existing ODH components use Kustomize. The library wraps `sigs.k8s.io/kustomize`
with namespace/label/annotation injection plugins.

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/render/kustomize"

//go:embed manifests
var manifestsFS embed.FS

resources, err := kustomize.Render("manifests/overlays/production",
    []kustomize.EngineOptsFn{kustomize.WithEngineFS(manifestsFS)},
    kustomize.WithNamespace("my-namespace"),
    kustomize.WithLabel("app.kubernetes.io/part-of", "my-module"),
    kustomize.WithAnnotation("platform.opendatahub.io/version", "1.0.0"),
)
```

In the action pipeline, the kustomize action reads from `rr.Manifests`:

```go
import (
    renderKustomize "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/kustomize"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/render/kustomize"
)

// Populate rr.Manifests in an init action:
func initAction(ctx context.Context, rr *types.ReconciliationRequest) error {
    rr.Manifests = []types.ManifestInfo{
        {Path: rr.ManifestsBasePath, ContextDir: "overlays/production"},
    }
    return nil
}

// Wire it:
engine := kustomize.NewEngine(kustomize.WithEngineFS(manifestsFS))

renderKustomize.NewAction(
    renderKustomize.WithEngine(engine),
    renderKustomize.WithNamespaceFn(namespaceFn),
    renderKustomize.WithCache(true),
)
```

### Option B: Helm Chart Rendering

For modules that ship Helm charts. The library wraps
[k8s-manifest-kit](https://github.com/k8s-manifest-kit/renderer-helm) and
applies post-renderers for apply ordering.

```go
import (
    "github.com/opendatahub-io/odh-platform-utilities/pkg/render/helm"
    helmRenderer "github.com/k8s-manifest-kit/renderer-helm/pkg"
)

resources, err := helm.Render(ctx, []helmRenderer.Source{
    {
        Chart:       "charts/my-module",
        ReleaseName: "my-module",
        Values:      helmRenderer.Values(map[string]any{"replicas": 3}),
    },
},
    helm.WithLabel("app.kubernetes.io/part-of", "my-module"),
)
```

In the action pipeline:

```go
import renderHelm "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/helm"

// Populate rr.HelmCharts in an init action:
func initAction(ctx context.Context, rr *types.ReconciliationRequest) error {
    rr.HelmCharts = []types.HelmChartInfo{
        {
            Source: helmRenderer.Source{
                Chart:       filepath.Join(rr.ChartsBasePath, "my-module"),
                ReleaseName: "my-module",
                Values:      helmRenderer.Values(valuesFromSpec(rr.Instance)),
            },
        },
    }
    return nil
}

renderHelm.NewAction(
    renderHelm.WithLabel("app.kubernetes.io/part-of", "my-module"),
    renderHelm.WithCache(true),
)
```

`HelmChartInfo` also supports `PreApply` and `PostApply` hooks for per-chart
logic that runs around the render step.

### Option C: Go Template Rendering

For modules that need dynamic value injection into YAML templates using Go's
`text/template`. The library provides `indent`, `nindent`, and `toYaml`
template functions by default.

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/render/template"

resources, err := template.Render(ctx, scheme, []template.TemplateSource{
    {FS: embedFS, Path: "templates/*.yaml"},
}, map[string]any{
    "Namespace": "my-namespace",
    "Replicas":  3,
},
    template.WithLabel("app", "my-module"),
)
```

In the action pipeline, template data can be static or computed per-reconcile:

```go
import renderTemplate "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/template"

renderTemplate.NewAction(
    renderTemplate.WithDataFn(func(ctx context.Context, rr *types.ReconciliationRequest) (map[string]any, error) {
        return map[string]any{"FeatureGate": isEnabled(ctx)}, nil
    }),
    renderTemplate.WithNamespaceFn(namespaceFn),
    renderTemplate.WithCache(true),
)
```

The action automatically injects `Component` (the CR instance) and
`AppNamespace` into the template data.

### Option D: Plain YAML with Runtime Patching

For simple manifests that need minimal transformation:

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

content, err := embedFS.ReadFile("manifests/deployment.yaml")
if err != nil {
    return fmt.Errorf("reading deployment manifest: %w", err)
}
decoded, err := resources.Decode(scheme.Codecs.UniversalDeserializer(), content)
for i := range decoded {
    resources.SetLabels(&decoded[i], map[string]string{"app": "my-module"})
    resources.SetAnnotation(&decoded[i], "custom-key", "custom-value")
}
```

### Combining Approaches

You can mix render engines in a single pipeline. Each render action appends to
`rr.Resources`; they don't overwrite each other.

```go
reconciler.ReconcilerFor(mgr, &MyModuleCR{}).
    WithAction(initAction).
    WithAction(renderKustomize.NewAction(                 // base infra
        renderKustomize.WithEngine(engine),
    )).
    WithAction(renderTemplate.NewAction(                  // dynamic config
        renderTemplate.WithDataFn(dynamicValues),
    )).
    WithAction(deploy.NewAction()).
    WithAction(gc.NewAction(nsFn)).
    Build(ctx)
```

### Render Caching

All three action-pipeline render engines support caching. The cache key is a
SHA-256 hash of the instance UID, generation, release metadata, and
manifest/chart/template input identifiers. On cache hit, `rr.Generated` stays
`false`, and the GC action skips its expensive API discovery pass.

Caching is enabled by default for all render actions. Disable it only if your
render inputs change without a generation bump (e.g. external ConfigMaps
that affect rendering).

### Trade-offs

| Engine | Strengths | Weaknesses | Best for |
|--------|-----------|------------|----------|
| Kustomize | Familiar to most teams; overlay model for env-specific config | Complex for highly dynamic values | Migrating from in-tree components |
| Helm | Rich templating; dependency management; values-driven | Heavier dependency; chart structure overhead | New components; complex resource graphs |
| Go template | Lightweight; full Go expressiveness | Raw string templating; easy to produce invalid YAML | Simple dynamic injection; config-heavy modules |
| Plain YAML | Simplest; no engine dependency | No parameterization; manual patching | Static resources; one-off CRDs |

---

## 3. Resource Deployment and Ownership

**Context:** After rendering manifests, you need to apply them to the cluster.
The library handles server-side apply, owner references, merge strategies, and
the "unmanaged" opt-out convention.

### Standard Deployment with Owner References

The deploy action (or standalone `Deployer`) stamps lifecycle annotations and
sets owner references on resources in the same namespace as the CR.

```go
import "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"

deploy.NewAction(
    deploy.WithMode(deploy.ModeSSA),
    deploy.WithFieldOwner("my-module"),
    deploy.WithCache(deploy.WithTTL(10 * time.Minute)),
    deploy.WithApplyOrder(),
)
```

Resources deployed this way get:
- Controller owner reference (establishes GC ownership; enables child-change
  routing when paired with an `Owns`/`EnqueueRequestForOwner` watch)
- Lifecycle annotations (`instance.generation`, `instance.name`, `instance.uid`,
  `version`, `type`) used by GC to detect stale resources
- Part-of label (used by GC's label selector)

### Cross-Namespace Resources

Owner references are namespace-scoped in Kubernetes — they don't work across
namespaces. For resources your module deploys into a different namespace
(e.g. monitoring namespace, user namespace), use annotation-based ownership:

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"

cluster.ApplyMetaOptions(obj,
    cluster.WithOwnerAnnotations(instance),     // stamps name/ns/uid/generation annotations
    cluster.InNamespace("monitoring-namespace"),
)
```

Then watch these resources with annotation-based routing:

```go
// In the pipeline builder:
reconciler.ReconcilerFor(mgr, &MyModuleCR{}).
    WatchesGVK(
        schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
        // Default handler is AnnotationToName — reads the instance name annotation
        // and enqueues a reconcile for that CR
    ).
    // ...
```

Or with standalone controller-runtime:

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"

ctrl.NewControllerManagedBy(mgr).
    Watches(&corev1.ConfigMap{}, cluster.EnqueueByOwnerAnnotation()).
    // ...
```

### Annotation-Based "Unmanaged" Resources

Some resources should be created by the operator but not updated on subsequent
reconciles (e.g. user-editable ConfigMaps). Mark them with the managed-by
annotation set to `"false"`:

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"

resources.SetAnnotation(obj, annotations.ManagedByODHOperator, "false")
```

The deploy action treats `managed=false` resources as create-only:
- First reconcile: creates the resource without an owner reference
- Subsequent reconciles: skips the resource entirely
- GC: does not delete `managed=false` resources
- Dynamic ownership: watches with delete-only predicate (cleanup on CR deletion)

### Merge Strategies

For resources where the cluster state should partially override the desired
state (e.g. user-set replica counts on Deployments), the deploy action supports
per-GVK merge strategies.

Built-in merge strategies:

| GVK | Behavior |
|-----|----------|
| Deployment | Preserves user-set `replicas` and container `resources` unless `managed=true` |
| ClusterRole | Strips `rules` when `aggregationRule` is present (let Kubernetes aggregate) |
| Observability CRs | Merges fields from the live cluster state for MonitoringStack, TempoStack, etc. |

Custom merge strategies (standalone `Deployer`):

```go
deployer := deploy.NewDeployer(
    deploy.WithMergeStrategy(
        schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
        func(existing, desired *unstructured.Unstructured) error {
            // Preserve PVC templates from existing
            pvcs, found, err := unstructured.NestedSlice(existing.Object, "spec", "volumeClaimTemplates")
            if err != nil {
                return fmt.Errorf("reading volumeClaimTemplates: %w", err)
            }
            if found {
                if err := unstructured.SetNestedSlice(desired.Object, pvcs, "spec", "volumeClaimTemplates"); err != nil {
                    return fmt.Errorf("setting volumeClaimTemplates: %w", err)
                }
            }
            return nil
        },
    ),
)
```

In the pipeline, use `WithApplyCustomizer` or `WithPatchCustomizer` for
per-GVK hooks:

```go
deploy.NewAction(
    deploy.WithApplyCustomizer(myGVK, func(existing, desired *unstructured.Unstructured) error {
        // custom merge logic
        return nil
    }),
)
```

### Partial Deployment

To conditionally deploy a subset of rendered resources, filter `rr.Resources`
in a custom action before the deploy action runs:

```go
func featureGateFilter(ctx context.Context, rr *types.ReconciliationRequest) error {
    if !isFeatureEnabled(rr.Instance) {
        rr.RemoveResources(func(obj *unstructured.Unstructured) bool {
            return obj.GetKind() == "ServiceMonitor"
        })
    }
    return nil
}
```

### Deploy Caching

The deploy cache uses content fingerprints to skip resources that haven't
changed since the last apply. The cache key includes the resource's GVK, name,
namespace, resource version, and a SHA-256 hash of the desired content. Cache
entries expire after a configurable TTL (default 10 minutes).

### Trade-offs

| Approach | Scope | Cleanup | Watch routing |
|----------|-------|---------|---------------|
| Owner references | Same namespace | Automatic (Kubernetes GC) | `EnqueueRequestForOwner` |
| Annotation-based ownership | Cross-namespace | Manual (your GC or finalizer) | `EnqueueByOwnerAnnotation` |
| Unmanaged (`managed=false`) | Any | Skipped by GC | Delete-only predicate |

---

## 4. Garbage Collection

**Context:** When manifests change between versions (resources renamed, removed,
or moved to a different namespace), stale resources must be deleted from the
cluster. The library provides label-selector-based GC that discovers deletable
resource types via RBAC and compares lifecycle annotations.

### Pipeline GC Action

The GC action **must be the last user action** in the pipeline (before
auto-appended dynamic watch/ownership actions). It depends on:
- `rr.Resources` being fully populated by prior render + deploy actions
- `rr.Generated == true` (set by render cache miss) — on cache hit, GC skips
  its expensive API discovery pass
- Lifecycle annotations stamped by the deploy action on cluster resources

```go
import "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/gc"

gc.NewAction(namespaceFn,
    gc.WithPartOfLabel("platform.opendatahub.io/part-of"),
    gc.WithUnremovables(gvk.CustomResourceDefinition, gvk.Lease),
)
```

**Why must GC be last?** GC lists cluster resources by label selector, then
compares their lifecycle annotations against the current instance's metadata. If
GC ran before deploy, newly rendered resources wouldn't have their annotations
updated yet and would be incorrectly marked stale.

### GC Flow

1. Check preconditions: skip if `rr.SkipDeploy` or `!rr.Generated`
2. Discover API resources the controller has RBAC `delete` permission for
   (via `SelfSubjectRulesReview`)
3. Filter by type predicate (exclude unremovable GVKs)
4. List resources matching the label selector
5. For each resource, evaluate the object predicate:
   - Missing lifecycle annotations → stale (delete)
   - Annotation values don't match current instance → stale (delete)
   - `managed=false` → skip
   - Not owned by this CR (when `onlyOwned=true`) → skip
6. Delete stale resources with configured propagation policy

### Standalone GC

Outside the pipeline, use the root-module `gc.Collector` directly:

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/gc"

collector := gc.New(
    gc.WithLabel("app.kubernetes.io/part-of", "my-module"),
    gc.InNamespace("my-namespace"),
    gc.WithMetrics(),
)

err := collector.Run(ctx, gc.RunParams{
    Client:          r.Client,
    DynamicClient:   r.DynamicClient,
    DiscoveryClient: r.DiscoveryClient,
    Owner:           instance,
    Version:         "1.0.0",
    PlatformType:    "OpenDataHub",
})
```

### Resources That Must Survive CR Deletion

Some resources (e.g. PVCs with user data, CRDs with live instances) must not be
deleted when the module CR is removed. Handle this with:

**Unremovable GVKs:** CRDs and Leases are unremovable by default. Add more:

```go
gc.WithUnremovables(
    schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
)
```

**Object predicates:** Custom logic per resource:

```go
gc.WithObjectPredicate(func(rr *types.ReconciliationRequest, obj unstructured.Unstructured) (bool, error) {
    if obj.GetKind() == "PersistentVolumeClaim" {
        return false, nil // never delete PVCs
    }
    return gc.DefaultObjectPredicate(/* ... */)
})
```

**Finalizers:** For cleanup that requires ordering (e.g. delete data-plane
resources before control-plane), use finalizer actions in the pipeline:

```go
reconciler.ReconcilerFor(mgr, &MyModuleCR{}).
    WithFinalizer(cleanupDataPlane).    // runs on CR deletion
    WithFinalizer(cleanupControlPlane).
    // ...
```

Finalizer actions run sequentially. A `StopError` from any finalizer action
halts the remaining finalizer actions without returning an error (graceful
stop for progressive cleanup across reconciles).

### Trade-offs

| Approach | Scope | Safety | Performance |
|----------|-------|--------|-------------|
| Pipeline GC | Automatic; label + annotation based | Safe with `onlyOwned` | Skips on cache hit (`!rr.Generated`) |
| Standalone GC | Manual invocation | Same predicate model | Always runs discovery |
| Unremovable GVKs | Type-level exclusion | Broad; entire GVK never deleted | No overhead |
| Object predicate | Per-resource exclusion | Fine-grained | Evaluated per listed resource |

---

## 5. Status and Condition Management

**Context:** The ODH orchestrator reads module CR status through the
`PlatformObject` interface to aggregate health, detect versions, and gate
upgrade progression. Your module must maintain standard conditions and phases.

### Required Conditions

Every module CR must support at minimum:

| Condition Type | Purpose |
|----------------|---------|
| `Ready` | Top-level health (aggregated from dependents) |
| `ProvisioningSucceeded` | Whether the latest reconcile completed successfully |

The orchestrator evaluates these conditions to determine module health. See
[platform-object-contract.md](./platform-object-contract.md) for the full
contract specification.

### Using the Conditions Manager

The conditions manager (inspired by Knative) tracks a "happy" condition and
dependent conditions, automatically recomputing the happy condition when
dependents change.

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"

mgr := conditions.NewManager(instance.GetStatus(), "Ready",
    "ProvisioningSucceeded",
    "DeploymentsAvailable",
)

// Mark a dependent condition
mgr.MarkTrue("DeploymentsAvailable",
    conditions.WithReason("AllReplicasReady"),
    conditions.WithObservedGeneration(instance.Generation),
)

// Propagate an error to a condition
mgr.MarkFalse("ProvisioningSucceeded",
    conditions.WithError(err),  // sets Reason="Error", Message=err.Error()
)

// Ready is recomputed automatically:
// - All dependents True → Ready=True
// - Any dependent False → Ready=False (copies reason/message from worst dependent)
// - Any dependent Unknown → Ready=Unknown
```

In the pipeline, the reconciler creates the manager automatically from the
`WithConditions(...)` builder call and injects it as `rr.Conditions`. Actions
use it directly:

```go
func myAction(ctx context.Context, rr *types.ReconciliationRequest) error {
    // ...
    if degraded {
        rr.Conditions.MarkFalse("MyDependentCondition",
            conditions.WithReason("BackendUnavailable"),
            conditions.WithMessage("endpoint %s returned %d", url, code),
        )
    } else {
        rr.Conditions.MarkTrue("MyDependentCondition")
    }
    return nil
}
```

### Aggregating Status from Sub-Resources

For modules that deploy Deployments and need to reflect their availability:

```go
import "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/deployments"

deployments.NewAction(
    deployments.InNamespaceFn(namespaceFn),
    deployments.WithConditionType("DeploymentsAvailable"),
)
```

This action checks all Deployments matching the part-of label and sets the
condition based on their combined availability status.

### Low-Level Condition Helpers

For standalone controllers that don't use the Manager:

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"

// Upsert a condition (manages LastTransitionTime automatically)
conditions.SetStatusCondition(instance.GetStatus(), common.Condition{
    Type:    "Ready",
    Status:  metav1.ConditionTrue,
    Reason:  "Reconciled",
    Message: "all resources deployed successfully",
})

// Query
cond := conditions.FindStatusCondition(instance.GetStatus(), "Ready")
isReady := conditions.IsStatusConditionTrue(instance.GetStatus(), "Ready")
```

### Writing Status

**Pipeline:** The reconciler handles status writes automatically via SSA after
all actions complete.

**Standalone:** Use `status.Update` for safe writes with automatic conflict
retry:

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/status"

err := status.Update(ctx, r.Client, instance, func(obj *MyModuleCR) {
    obj.Status.Phase = common.PhaseReady
    obj.Status.ObservedGeneration = obj.Generation
})
// Retries up to 5 times on conflict (re-reads object, re-applies mutateFn)
```

### Stale Condition Cleanup

The framework conditions manager tracks which conditions were set during the
current reconcile via `activeTypes`. After all actions, the reconciler calls
`CleanupStaleConditions()`:
- Registered dependents not set this reconcile → marked `False` with reason
  `ConditionNotSet`
- Non-dependent conditions not set this reconcile → removed entirely

This prevents stale conditions from a previous code version persisting
indefinitely.

### Trade-offs

| Approach | Aggregation | Stale cleanup | Status write |
|----------|-------------|---------------|--------------|
| Pipeline + Manager | Automatic | Automatic | SSA by reconciler |
| Standalone + Manager | Automatic | Manual (call `CleanupStaleConditions`) | Manual via `status.Update` |
| Low-level helpers | Manual | Manual | Manual |

---

## 6. CRD Design and PlatformObject Contract

**Context:** Every module CRD that participates in the ODH platform must
implement the `PlatformObject` interface so the orchestrator can read its status
generically.

### Implementing the PlatformObject Interface

```go
import common "github.com/opendatahub-io/odh-platform-utilities/api/common"

type MyModuleSpec struct {
    common.ManagementSpec `json:",inline"`
    // module-specific fields...
}

type MyModuleStatus struct {
    common.Status               `json:",inline"`
    common.ComponentReleaseStatus `json:",inline"`
    // module-specific status fields...
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
type MyModule struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec   MyModuleSpec   `json:"spec,omitempty"`
    Status MyModuleStatus `json:"status,omitempty"`
}

// Compile-time interface verification
var _ common.PlatformObject = (*MyModule)(nil)

func (m *MyModule) GetStatus() *common.Status           { return &m.Status.Status }
func (m *MyModule) GetConditions() []common.Condition    { return m.Status.GetConditions() }
func (m *MyModule) SetConditions(c []common.Condition)   { m.Status.SetConditions(c) }
func (m *MyModule) GetReleaseStatus() *common.ComponentReleaseStatus { return &m.Status.ComponentReleaseStatus }
func (m *MyModule) SetReleaseStatus(s common.ComponentReleaseStatus) { m.Status.ComponentReleaseStatus = s }
```

Use `validation.ValidatePlatformObject(t, &MyModule{})` in your tests to verify
the contract is correctly implemented (checks pointer liveness, round-trip
persistence, and mandatory condition types).

### Singleton CR Pattern

Module CRDs are cluster-scoped singletons. Enforce this at two levels:

**CEL validation rule on the CRD** (preferred — no webhook deployment needed):

```yaml
# +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default-mymodule'",message="only the 'default-mymodule' instance is allowed"
```

**Admission webhook** (defense in depth):

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/webhook"

func (w *MyModuleWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
    resp := webhook.ValidateSingletonCreation(ctx, w.Client, w.Request, myModuleGVK)
    if !resp.Allowed {
        return nil, errors.New(resp.Result.Message)
    }
    return nil, nil
}
```

**Runtime retrieval:**

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"

instance := &MyModule{}
err := cluster.GetSingleton(ctx, r.Client, instance)
// Returns ErrNoInstance if none exist, ErrMultipleInstances if more than one
```

### ManagementSpec: Managed vs. Removed

The orchestrator writes the management state annotation on your CR. Your
controller reads it to decide whether to deploy or undeploy:

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"

state := resources.GetAnnotation(instance, annotations.ManagementStateAnnotation)
switch common.ManagementState(state) {
case common.Managed:
    // normal reconcile
case common.Removed:
    // undeploy all resources, set phase accordingly
}
```

In the pipeline, the `preApplyFn` option can gate the entire action pipeline:

```go
reconciler.WithPreApplyFn(func(ctx context.Context, rr *types.ReconciliationRequest) bool {
    state := resources.GetAnnotation(rr.Instance, annotations.ManagementStateAnnotation)
    return common.ManagementState(state) == common.Removed
})
```

When `preApplyFn` returns `true`, all actions are skipped and
`ProvisioningSucceeded` is set to `False` with a configurable reason.

### Release Metadata

Report your component version through the `ComponentReleaseStatus`:

```go
instance.GetReleaseStatus().SetRelease(common.ComponentRelease{
    Name:    "my-module",
    Version: "1.2.0",
})
instance.GetReleaseStatus().SetPlatformRelease("2.19.0")
```

The orchestrator reads this to detect version mismatches during upgrades.

### Trade-offs

| Design Decision | Option A | Option B |
|-----------------|----------|----------|
| Singleton enforcement | CEL only (simpler, no webhook infra) | CEL + webhook (defense in depth) |
| Management state | Annotation-based (orchestrator-controlled) | Spec field (user-controlled) |
| Status embedding | Inline `common.Status` (simple) | Wrapper with extra fields (extensible) |
| Release tracking | Single component release | Multiple releases (for multi-component modules) |

---

## 7. Environment Detection

**Context:** Your module needs to behave differently on OpenShift vs. vanilla
Kubernetes, detect FIPS mode, handle single-node clusters, or check for
optional CRDs before creating resources that depend on them.

### Detecting OpenShift vs. Kubernetes

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"

clusterType, err := cluster.DetectClusterType(ctx, r.Client)
if err != nil {
    return ctrl.Result{}, fmt.Errorf("detecting cluster type: %w", err)
}
switch clusterType {
case cluster.ClusterTypeOpenShift:
    // create Routes, ConsoleLinks, etc.
case cluster.ClusterTypeKubernetes:
    // create Ingresses instead
}
```

For full cluster info (type + version + FIPS):

```go
info, err := cluster.DetectClusterInfo(ctx, r.Client)
// info.Type, info.Version, info.FipsEnabled
```

### Platform Variant Detection

The ODH platform distinguishes between several deployment modes:

```go
platform, err := cluster.DetectPlatform(ctx, r.Client,
    os.Getenv("ODH_PLATFORM_TYPE"),
    operatorNamespace,
)
if err != nil {
    return ctrl.Result{}, fmt.Errorf("detecting platform: %w", err)
}
switch platform {
case cluster.OpenDataHub:        // community ODH
case cluster.SelfManagedRhoai:   // RHOAI self-managed
case cluster.ManagedRhoai:       // RHOAI cloud service
case cluster.XKS:                // non-OpenShift Kubernetes
}
```

Detection precedence: explicit env var → OLM auto-detection (CatalogSource /
OperatorCondition) → fallback to `OpenDataHub`.

### Handling OpenShift-Specific Resources

Use the three-way fallback pattern:

1. Check if the CRD exists
2. If yes, create OpenShift-specific resources
3. If no, fall back to Kubernetes-native equivalents

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"

exists, err := cluster.HasCRD(ctx, r.Client, schema.GroupKind{
    Group: "route.openshift.io",
    Kind:  "Route",
})
if err != nil {
    return fmt.Errorf("checking Route CRD: %w", err)
}
if exists {
    // CRD exists — create Route
} else {
    // CRD doesn't exist — create Ingress instead
}
```

In the pipeline, the sanity check action can block reconciliation when
deprecated or conflicting CRDs are present:

```go
import "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/sanitycheck"

sanitycheck.NewAction(
    sanitycheck.WithUnwantedResource(deprecatedGVK,
        "deprecated CRD still has instances — remove them before upgrading"),
)
```

### OpenShift-Specific Queries

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster/openshift"

version, err := openshift.GetVersion(ctx, r.Client)           // e.g. "4.16.2"
isSNO, err := openshift.IsSingleNodeCluster(ctx, r.Client)    // topology detection
authMode, err := openshift.GetAuthenticationMode(ctx, r.Client) // IntegratedOAuth, OIDC, None
domain, err := openshift.GetDomain(ctx, r.Client)             // apps domain for Routes
```

### FIPS Mode Detection

```go
fips, err := cluster.IsFipsEnabled(ctx, r.Client)
if err != nil {
    return ctrl.Result{}, fmt.Errorf("detecting FIPS mode: %w", err)
}
if fips {
    // use FIPS-compliant crypto, disable non-compliant features
}
```

Reads the `cluster-config-v1` ConfigMap in `kube-system` and parses the
install-config. Returns `false` with no error when the ConfigMap is absent
(non-OpenShift clusters).

### OLM Queries

Check if another operator is installed (for soft dependencies):

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster/olm"

info, err := olm.OperatorExists(ctx, r.Client, "servicemeshoperator")
if errors.Is(err, olm.ErrOperatorNotInstalled) {
    // operator not found — handle gracefully
} else if err != nil {
    return fmt.Errorf("checking operator: %w", err)
}
// info.Version is available when found
```

### Trade-offs

All detection functions are stateless (no singletons, no `Init()`) and use
unstructured clients internally to avoid importing `github.com/openshift/api`.
This means:
- **No compile-time dependency** on OpenShift types
- **Runtime errors** if the expected API isn't available (handle gracefully)
- **Repeated API calls** on each reconcile (consider caching results in
  `rr.Extensions` or a controller-level variable if detection is expensive)

---

## 8. Dependency Management Between Modules

**Context:** Your module depends on another module being healthy before it can
operate (e.g. Model Serving requires Service Mesh). You need to monitor
dependency status and handle missing prerequisites.

### Monitoring Dependency Conditions

Watch the dependency CR and check its conditions:

```go
import (
    "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
)

depInstance := &DependencyCR{}
if err := cluster.GetSingleton(ctx, r.Client, depInstance); err != nil {
    if errors.Is(err, cluster.ErrNoInstance) {
        // dependency not installed yet
        rr.Conditions.MarkFalse("DependencyReady",
            conditions.WithReason("DependencyMissing"),
            conditions.WithMessage("dependency CR not found"),
        )
        return nil
    }
    return fmt.Errorf("fetching dependency: %w", err)
}

if !conditions.IsStatusConditionTrue(depInstance.GetStatus(), "Ready") {
    rr.Conditions.MarkFalse("DependencyReady",
        conditions.WithReason("DependencyNotReady"),
        conditions.WithMessage("waiting for dependency to become ready"),
    )
    return nil
}

rr.Conditions.MarkTrue("DependencyReady")
```

### Ordering Constraints in the Pipeline

Use `rr.SkipDeploy` to block the deploy/GC pipeline while dependencies are
unmet:

```go
func checkDependencies(ctx context.Context, rr *types.ReconciliationRequest) error {
    depReady, err := isDependencyReady(ctx, rr.Client)
    if err != nil {
        return fmt.Errorf("checking dependency: %w", err)
    }
    if !depReady {
        rr.SkipDeploy = true
        rr.Conditions.MarkFalse("DependencyReady",
            conditions.WithReason("WaitingForDependency"),
            conditions.WithMessage("Service Mesh operator is not ready"),
        )
    }
    return nil
}
```

When `rr.SkipDeploy = true`, the deploy and GC actions become no-ops, but
condition management and status writes still proceed. This lets your CR report
*why* it's not ready.

### Watching Dependency CRs

In the pipeline builder, watch the dependency CR to trigger reconciliation when
it changes:

```go
reconciler.ReconcilerFor(mgr, &MyModuleCR{}).
    WatchesGVK(dependencyGVK,
        // Routes events from the dependency CR to your module's reconcile
    ).
    WithEventFilter(predicates.DefaultPredicate).
    // ...
```

### Handling Missing Prerequisites

Two strategies:

**Block with informative status (recommended):** Set `SkipDeploy` and report
the missing dependency in conditions. The orchestrator sees the module as "Not
Ready" and can display the reason.

**Degraded operation:** Deploy what you can, skip features that require the
dependency, and set a degraded condition:

```go
rr.Conditions.MarkFalse("OptionalFeatureAvailable",
    conditions.WithSeverity(common.ConditionSeverityInfo), // Info, not Error
    conditions.WithReason("OptionalDependencyMissing"),
    conditions.WithMessage("monitoring dashboards disabled: Grafana operator not found"),
)
```

Using `ConditionSeverityInfo` prevents this condition from dragging `Ready` to
`False` during aggregation — only `ConditionSeverityError` (the default)
affects the happy condition.

### Trade-offs

| Strategy | Module behavior | Orchestrator view | User experience |
|----------|----------------|-------------------|-----------------|
| Block | No resources deployed | Not Ready + clear reason | Clean; no partial state |
| Degraded | Core features work | Ready (Info-severity condition) | Functional but incomplete |

---

## 9. Upgrade and Migration

**Context:** Your module needs to handle CRD schema changes, data migrations,
and ensure data-plane stability across version upgrades. Migration logic is
encapsulated within the module operator — the orchestrator gates progression
but does not run migrations.

### Schema Migration: Field Additions

New optional fields with defaults are safe across versions. Use kubebuilder
markers to set defaults in the CRD schema:

```go
// +kubebuilder:default:=3
// +kubebuilder:validation:Minimum=1
Replicas *int32 `json:"replicas,omitempty"`
```

Your reconciler should handle the zero-value case for older CRs that predate
the field:

```go
func reconcileReplicas(spec MyModuleSpec) int32 {
    if spec.Replicas != nil {
        return *spec.Replicas
    }
    return 3 // default for CRs created before this field existed
}
```

### Schema Migration: Field Removals and Renames

These are breaking changes. Keep the deprecated field served in the CRD schema
throughout the migration window — once a field is pruned from the served schema,
the apiserver strips it before your controller sees it and any reconcile-time
migration becomes a no-op.

Use a migration action early in the pipeline to copy values during the
transition period while both fields coexist:

```go
func migrateV1ToV2(ctx context.Context, rr *types.ReconciliationRequest) error {
    instance := rr.Instance.(*MyModuleCR)

    // Both fields must still be served in the CRD schema for this to work.
    // Remove DeprecatedField from the schema only after all existing CRs
    // have been migrated (e.g. after one full reconcile cycle).
    if instance.Spec.DeprecatedField != "" && instance.Spec.NewField == "" {
        instance.Spec.NewField = instance.Spec.DeprecatedField
        instance.Spec.DeprecatedField = ""

        if err := rr.Client.Update(ctx, instance); err != nil {
            return fmt.Errorf("migrating spec: %w", err)
        }
    }
    return nil
}
```

> **Note:** The `Client.Update` call triggers a new reconcile. This is
> intentional — the migration action is idempotent and will be a no-op on the
> second pass once `NewField` is populated. For complex migrations involving
> multiple fields or storage versions, consider a
> [conversion webhook](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/#webhook-conversion)
> instead.

### Data Migration Between Versions

For stateful components (databases, model registries), encapsulate migration
logic in a dedicated action:

```go
func migrateDatabase(ctx context.Context, rr *types.ReconciliationRequest) error {
    currentVersion := rr.Instance.(*MyModuleCR).Status.GetReleaseStatus().GetRelease("my-module")
    if currentVersion == nil {
        return nil // fresh install, no migration needed
    }

    if semver.Compare(currentVersion.Version, "2.0.0") < 0 {
        if err := runDatabaseMigration(ctx, rr.Client); err != nil {
            rr.Conditions.MarkFalse("ProvisioningSucceeded",
                conditions.WithReason("MigrationFailed"),
                conditions.WithError(err),
            )
            return errors.NewStopError("migration failed: %v", err)
        }
    }
    return nil
}
```

### Ensuring Data-Plane Stability

To avoid unnecessary workload restarts during upgrades, the deploy action's
merge strategies preserve user-set values:

- **Deployment replicas:** Preserved unless `managed=true` annotation is set
- **Container resources:** Preserved to avoid triggering rolling updates
- **Deploy cache:** Skips resources whose content hash hasn't changed

For additional control, use the `preApplyFn` to gate upgrades:

```go
reconciler.WithPreApplyFn(func(ctx context.Context, rr *types.ReconciliationRequest) bool {
    return upgradeInProgress(rr.Instance) && !migrationComplete(rr.Instance)
})
```

### Admin Acknowledgment Gates

For breaking changes that require admin intervention before proceeding, use
conditions to communicate the requirement:

```go
func checkAdminAck(ctx context.Context, rr *types.ReconciliationRequest) error {
    if needsAck(rr.Instance) && !hasAck(rr.Instance) {
        rr.SkipDeploy = true
        rr.Conditions.MarkFalse("ProvisioningSucceeded",
            conditions.WithReason("AdminAcknowledgmentRequired"),
            conditions.WithMessage(
                "breaking change in v2.0: set annotation '%s=true' to proceed",
                ackAnnotation,
            ),
        )
    }
    return nil
}
```

### Trade-offs

| Migration Type | Risk | Rollback | Complexity |
|----------------|------|----------|------------|
| Field addition (optional) | Low | Drop field | Minimal |
| Field rename/removal | Medium | Dual-write period | Moderate |
| Data migration | High | Backup + reverse migration | High |
| Breaking CRD change | High | Major version bump | High |

---

## 10. Testing Patterns

**Context:** Module controllers need unit tests for individual actions, tests
for the reconciliation pipeline, and integration tests with a real API server.

### Unit Testing Actions

Each `actions.Fn` is a standalone function that takes a context and
`ReconciliationRequest`. Test them in isolation with a fake client:

```go
func TestMyAction(t *testing.T) {
    t.Parallel()

    tests := []struct {
        name      string
        instance  *MyModuleCR
        resources []unstructured.Unstructured
        wantErr   bool
    }{
        {
            name:     "happy path",
            instance: newTestInstance(withSpec(MyModuleSpec{Replicas: ptr.To(int32(3))})),
        },
        {
            name:     "missing required field",
            instance: newTestInstance(),
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()

            cli := fake.NewClientBuilder().
                WithScheme(testScheme).
                WithObjects(tt.instance).
                Build()

            rr := &types.ReconciliationRequest{
                Client:   cli,
                Instance: tt.instance,
                // Populate fields the action reads
            }

            err := myAction(context.Background(), rr)
            if tt.wantErr {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)

            // Assert on rr.Resources, conditions, etc.
            assert.Len(t, rr.Resources, 3)
        })
    }
}
```

Conventions from this repository:
- `t.Parallel()` in all tests and subtests
- Table-driven tests for input variations
- `_test` package suffix to exercise the public API
- `testify/assert` for non-fatal checks, `require` for fatal preconditions

### Testing with envtest

For integration tests that need a real API server (CRD registration, webhook
validation, SSA):

```go
func TestReconciler(t *testing.T) {
    t.Parallel()

    env := &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "config", "crd", "bases"),
        },
    }

    cfg, err := env.Start()
    require.NoError(t, err)
    t.Cleanup(func() { _ = env.Stop() })

    mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: testScheme})
    require.NoError(t, err)

    // Set up your reconciler against the real API server
    // Create CR, wait for conditions, assert on deployed resources
}
```

### Testing the PlatformObject Contract

Use the validation helper to verify your CR correctly implements the interface:

```go
func TestMyModulePlatformObjectContract(t *testing.T) {
    t.Parallel()
    validation.ValidatePlatformObject(t, &MyModule{})
}
```

This verifies that the status pointer is live (not a copy), conditions
round-trip correctly, mandatory condition types are storable, release status
persists, and phases are writable.

For live-cluster conformance testing:

```go
validation.ValidatePlatformContract(t, k8sClient, validation.ContractOptions{
    GVK:          myModuleGVK,
    InstanceName: "default-mymodule",
})
```

### JQ-Based Assertions

The framework provides jq-based Gomega matchers for asserting on complex
Kubernetes objects without extracting every nested field:

```go
import "github.com/opendatahub-io/odh-platform-utilities/framework/utils/test/matchers/jq"

// Assert on unstructured resources
g.Expect(deployment).Should(jq.Match(`.spec.replicas == 3`))
g.Expect(deployment).Should(jq.Match(
    `.spec.template.spec.containers[0].image == "my-image:v1.0"`,
))

// Extract and compose
g.Expect(resource).Should(
    WithTransform(jq.Extract(`.status`),
        And(
            jq.Match(`.conditions | length > 0`),
            jq.Match(`.phase == "Ready"`),
        ),
    ),
)
```

### Testing Conditions

Use `ExtractStatusCondition` to assert on individual conditions:

```go
import "github.com/opendatahub-io/odh-platform-utilities/framework/utils/test/matchers"

g.Eventually(func() (api.Condition, error) {
    if err := k8sClient.Get(ctx, key, instance); err != nil {
        return api.Condition{}, err
    }
    return matchers.ExtractStatusCondition("Ready")(instance), nil
}).Should(And(
    HaveField("Status", Equal(metav1.ConditionTrue)),
    HaveField("Reason", Equal("Reconciled")),
))
```

### Avoiding Tautological Tests

The test oracle must be structurally independent from the production code:

```go
// BAD: test computes expected value the same way production code does
expected := computeHash(input)
assert.Equal(t, expected, result)

// GOOD: hardcode the expected value or derive it independently
assert.Equal(t, "sha256:abc123...", result)
```

### Trade-offs

| Test Level | Speed | Fidelity | Dependencies |
|------------|-------|----------|--------------|
| Unit (fake client) | Fast (~ms) | No real API server behavior (no SSA, no webhooks) | None |
| envtest | Medium (~seconds) | Real etcd + apiserver; no controllers | etcd + kube-apiserver binaries |
| E2E (real cluster) | Slow (~minutes) | Full cluster behavior | Running cluster |

---

## Appendix: Quick Reference

### Import Paths

```go
// Root module — low-level utilities
import (
    common   "github.com/opendatahub-io/odh-platform-utilities/api/common"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster/openshift"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster/olm"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/gc"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/deploy"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/render/helm"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/render/kustomize"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/render/template"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/status"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/webhook"
)

// Framework module — opinionated controller framework
import (
    "github.com/opendatahub-io/odh-platform-utilities/framework/api"
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/reconciler"
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/gc"
    renderHelm "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/helm"
    renderKustomize "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/kustomize"
    renderTemplate "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/template"
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/status/deployments"
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/sanitycheck"
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
    "github.com/opendatahub-io/odh-platform-utilities/framework/utils/test/matchers/jq"
)
```

### Typical Pipeline Action Order

```text
1. sanitycheck        — block if unwanted CRDs present
2. init / migration   — populate rr inputs, run schema migrations
3. dependency check   — set rr.SkipDeploy if dependencies unmet
4. render (×N)        — kustomize / helm / template → rr.Resources
5. filter / transform — remove or modify resources based on spec
6. deploy             — apply resources to cluster
7. status checks      — deployment availability, custom health
8. gc                 — delete stale resources (MUST BE LAST user action)
```

### Related Documents

- [PlatformObject Contract](./platform-object-contract.md) — full contract
  specification with implementation examples
- [Migration from Operator](./migration-from-operator.md) — import path
  mapping from the ODH Operator monorepo
- [Versioning](./VERSIONING.md) — semantic versioning policy
