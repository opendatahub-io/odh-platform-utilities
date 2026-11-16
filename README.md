# ODH Platform Utilities

Shared Go utilities for [Open Data Hub](https://opendatahub.io/) module controller development.

This library provides common functionality extracted from the
[ODH Operator](https://github.com/opendatahub-io/opendatahub-operator) to
accelerate module controller development and ensure consistency across modules.

## Installation

All product packages are published from one Go module:

```bash
go get github.com/opendatahub-io/odh-platform-utilities
```

The product module requires Go 1.26.0.

## Usage

```go
import (
    common "github.com/opendatahub-io/odh-platform-utilities/api/common"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/reconciler"
    "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/actions/deploy"

    // Retained helper package in the same root module.
    "github.com/opendatahub-io/odh-platform-utilities/framework/controller/conditions"
)
```

See the [examples](./examples/) directory for runnable usage examples and the
[GoDoc](https://pkg.go.dev/github.com/opendatahub-io/odh-platform-utilities)
for full API documentation.

## Repository Structure

The repository has one **product Go module** at the root. `api/`, `pkg/`,
`framework/`, and `testing/` are all part of
`github.com/opendatahub-io/odh-platform-utilities` and share root `v*` releases.
There are no nested framework or testing module versions.

The standalone `flakiness/` module and modules under `examples/` are tooling;
they are not additional product modules. CI checks the product module and the
flakiness tool module separately.

For local product development, consumers need only one replacement:

```go
replace github.com/opendatahub-io/odh-platform-utilities => /path/to/odh-platform-utilities
```

## Product Packages

| Package | Description |
|---------|-------------|
| `api/common` | Platform contract, including `PlatformObject`, `Status`, `Condition`, `ComponentRelease`, `Release`, `Platform`, and condition/phase constants |
| `pkg/controller/reconciler` | Generic reconciler with finalizers, action pipelines, condition aggregation, phase computation, and status SSA |
| `pkg/controller/actions` | Pipeline action types plus deploy, GC, render, status, caching, dynamic ownership, deletion, and sanity-check actions |
| `pkg/controller/handlers` | Watch event handlers |
| `pkg/controller/types` | Reconciliation requests, manifest descriptors, controller interfaces, and hashes |
| `pkg/controller/predicates` | Default and specialized controller-runtime event predicates |
| `pkg/client` | Shared Kubernetes client helpers |
| `pkg/manager` | Controller manager construction helpers |
| `pkg/cluster` | Cluster detection, singleton enforcement, metadata options, and ownership helpers |
| `pkg/cluster/gvk` | Well-known Kubernetes and monitoring `GroupVersionKind` constants |
| `pkg/rules` | RBAC rule evaluation and authorized-resource discovery |
| `pkg/tls` | OpenShift TLS profile resolution, process TLS configuration, proxy flags, and profile watching |
| `pkg/metadata` | Deploy/GC metadata plus platform label and annotation packages |
| `pkg/webhook` | Admission webhook builder and singleton validation helpers |
| `pkg/ingressassignment` | Namespace ingress-assignment resolution |
| `pkg/testing` | Gomega matchers and jq-based test assertions |
| `testing/integration` | Live-cluster integration PR gate (`integration.Run`) |
| `pkg/render` | Helm, Kustomize, and Go-template rendering with caching and pipeline adapters |
| `pkg/resources` | Kubernetes resource decoding, hashing, applying, sorting, and metadata helpers |
| `pkg/deploy` | Stateful resource deployment with caching and merge strategies |
| `pkg/status` | Conflict-safe status updates |

Some behaviorally distinct compatibility/helper packages intentionally remain
under `framework/`: `framework/controller/conditions`, `framework/cluster`,
`framework/resources`, `framework/render/kustomize`, and
`framework/utils/template`. They are packages in the same root module—not
separate modules—and remain there pending separate behavior deduplication.

## Import Path Changes

| Previous public path | Current public path |
|----------------------|---------------------|
| `framework/controller/actions` | `pkg/controller/actions` |
| `framework/controller/reconciler` | `pkg/controller/reconciler` |
| `framework/controller/handlers` | `pkg/controller/handlers` |
| `framework/controller/types` | `pkg/controller/types` |
| `framework/controller/predicates` | `pkg/controller/predicates` |
| `framework/client` | `pkg/client` |
| `framework/manager` | `pkg/manager` |
| `framework/tls` | `pkg/tls` |
| `framework/rules` | `pkg/rules` |
| `framework/cluster/gvk` | `pkg/cluster/gvk` |
| `framework/metadata` | `pkg/metadata` |
| `framework/webhook` | `pkg/webhook` |
| `framework/utils/ingressassignment` | `pkg/ingressassignment` |
| `framework/utils/test` | `pkg/testing` |
| `framework/testing` | `testing` |

`framework/api` was removed. Import contract types, including `Release` and
`Platform`, from `api/common`.

The canonical `common.PlatformObject` includes `WithReleases`, so implementations
must provide `GetReleaseStatus` and `SetReleaseStatus` even when they do not use
the releases status action. `common.Status.Phase` is the typed `common.Phase`;
convert phase values sourced from `string` variables explicitly.

## Platform Contract

Module controllers that participate in the ODH platform must implement the
`PlatformObject` interface. See [docs/platform-object-contract.md](./docs/platform-object-contract.md)
for the full contract specification and a copy-pasteable implementation example.

To add a live-cluster PR gate in a module repo, import
`github.com/opendatahub-io/odh-platform-utilities/testing/integration`
and follow [docs/integration-testing.md](./docs/integration-testing.md).

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

## Cluster Detection

Module controllers need to discover facts about the cluster they're running
on. This library provides two detection layers:

### Am I on OpenShift?

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"

clusterType, err := cluster.DetectClusterType(ctx, client)
if clusterType == cluster.ClusterTypeOpenShift {
    // OpenShift-specific logic
}

// Or get full cluster info (type + version + FIPS)
info, err := cluster.DetectClusterInfo(ctx, client)
```

### What platform variant am I?

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"

// platformType typically comes from ODH_PLATFORM_TYPE env var
platform, err := cluster.DetectPlatform(ctx, client, os.Getenv("ODH_PLATFORM_TYPE"), operatorNs)
switch platform {
case cluster.OpenDataHub:
case cluster.SelfManagedRhoai:
case cluster.ManagedRhoai:
case cluster.XKS:
}
```

### Is this CRD available?

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"

err := cluster.CustomResourceDefinitionExists(ctx, client, schema.GroupKind{
    Group: "serving.kserve.io",
    Kind:  "InferenceService",
})
```

### OpenShift-specific queries

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster/openshift"

version, err := openshift.GetVersion(ctx, client)
isSNO, err := openshift.IsSingleNodeCluster(ctx, client)
authMode, err := openshift.GetAuthenticationMode(ctx, client)
domain, err := openshift.GetDomain(ctx, client)
```

### OLM queries

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster/olm"

info, err := olm.OperatorExists(ctx, client, "rhods-operator")
requested, err := olm.OperatorPackageRequested(ctx, client, "my-operator") // Catalog package name
```

`OperatorPackageRequested` matches the package in Subscription `spec.name` or
ClusterExtension `spec.source.catalog.packageName`, regardless of resource name.
It does not check installation success or readiness. Use it for cross-version
dependency detection by package name. `SubscriptionExists` is deprecated: it
checks Subscription resource names, not package names, and sees only OLMv0.

All functions are stateless (no singletons, no `Init()`), accept
`client.Reader` + `context.Context`, and use unstructured clients internally
so they do not pull in `github.com/openshift/api` or
`github.com/operator-framework/api`.

See [pkg/cluster/AGENTS.md](./pkg/cluster/AGENTS.md) for detailed
documentation on the two-layer model, error behavior, and dependency
strategy.

### TLS profile resolution

Module controllers can resolve the cluster profile and adherence policy from
`apiservers.config.openshift.io/cluster` with
`github.com/opendatahub-io/odh-platform-utilities/pkg/tls`. Register
`configv1.Install(scheme)` and grant `get` on that resource. Grant `list` and
`watch` only when registering `SecurityProfileWatcher`.

```go
import (
	configv1 "github.com/openshift/api/config/v1"
	pkgtls "github.com/opendatahub-io/odh-platform-utilities/pkg/tls"
)

result, err := pkgtls.LoadWithAdherence(ctx, client)
if err != nil {
    return fmt.Errorf("load TLS profile: %w", err)
}
profile := result.Spec
if !pkgtls.ShouldHonorClusterTLSProfile(result.AdherencePolicy) {
    profile = *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
}
tlsOpts, unsupported := pkgtls.ConfigFromProfile(profile)
if len(unsupported) > 0 {
    setupLog.Info("dropping unsupported TLS settings", "settings", unsupported)
}

minVersion, ciphers, curves, err := pkgtls.FromAPIServerWithCurvePreferences(ctx, client, pkgtls.FormatShort)
if err != nil {
    return fmt.Errorf("resolve TLS profile: %w", err)
}
```

On vanilla Kubernetes, `LoadWithAdherence` and the adherence-aware proxy helper
fall back to the Intermediate profile. Register `SecurityProfileWatcher` only
when `result.Watchable` is true. Initialize its adherence state from
`result.AdherencePolicy` when setting `OnAdherencePolicyChange`.

See [docs/module-tls.md](./docs/module-tls.md) for the expected module wiring
(scheme, RBAC, manager startup, proxy flags). See
[pkg/tls/AGENTS.md](./pkg/tls/AGENTS.md) for fallback policy and the OpenShift
API exception.

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

- Go 1.26.0
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

# Run all product-module checks (includes api/, pkg/, framework/, and testing/)
make all

# Include the standalone flakiness tool module
make all-modules

# See all available targets
make help
```

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines on how to contribute.

## License

Apache License 2.0 — see [LICENSE](./LICENSE) for details.
