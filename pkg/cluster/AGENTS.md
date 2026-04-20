# Cluster Detection

This package provides stateless functions for discovering facts about the
Kubernetes cluster a controller is running on. It is extracted from the ODH
operator's `pkg/cluster/` and designed so that standalone module controllers
can use it without importing the operator.

## Two-Layer Model

The package separates two conceptually distinct detection concerns:

### 1. Cluster Type Detection (infrastructure layer)

**Question:** "What kind of Kubernetes am I running on?"

| Function | Package | Requires |
|---|---|---|
| `DetectClusterType` | `cluster` | Any K8s |
| `DetectClusterInfo` | `cluster` | Any K8s |
| `IsFipsEnabled` | `cluster` | Any K8s (returns error on vanilla K8s when ConfigMap absent) |
| `GetVersion` | `cluster/openshift` | OpenShift |
| `IsSingleNodeCluster` | `cluster/openshift` | Any K8s (OpenShift path preferred, node-count fallback) |
| `GetAuthenticationMode` | `cluster/openshift` | OpenShift |
| `IsIntegratedOAuth` | `cluster/openshift` | OpenShift |
| `GetServiceAccountIssuer` | `cluster/openshift` | OpenShift (returns NotFound/NoMatch on vanilla K8s) |
| `GetDomain` | `cluster/openshift` | OpenShift |

### 2. Platform Variant Detection (product layer)

**Question:** "Which product distribution is deploying me?"

| Function | Package | Requires |
|---|---|---|
| `DetectPlatform` | `cluster` | OLM for auto-detection; none for explicit `platformType` |

Platform variants: `OpenDataHub`, `SelfManagedRhoai`, `ManagedRhoai`, `XKS`.

**Transitional note:** In the fully-realized module architecture, modules
should prefer reading platform info from their projected CR config (set by
the orchestrator) rather than calling `DetectPlatform` directly. The
detection API is a transitional necessity and fallback mechanism.

### 3. Dependency Discovery

**Question:** "Is this CRD/operator available?"

| Function | Package | Requires |
|---|---|---|
| `CustomResourceDefinitionExists` | `cluster` | Any K8s |
| `OperatorExists` | `cluster/olm` | OLM |
| `SubscriptionExists` | `cluster/olm` | OLM |
| `GetSubscription` | `cluster/olm` | OLM |
| `CatalogSourceExists` | `cluster/olm` | OLM |

## Package Structure

```text
pkg/cluster/
├── types.go       # Platform, ClusterType, ClusterInfo, AuthenticationMode, OperatorInfo
├── detect.go      # DetectClusterType, DetectClusterInfo, IsFipsEnabled
├── crd.go         # CustomResourceDefinitionExists
├── platform.go    # DetectPlatform
├── openshift/
│   └── openshift.go  # GetVersion, IsSingleNodeCluster, GetAuthenticationMode, etc.
└── olm/
    └── olm.go        # OperatorExists, SubscriptionExists, GetSubscription, etc.
```

## API Dependency Strategy

All functions use **unstructured Kubernetes clients** internally. This means:

- Importing `pkg/cluster` does **not** pull in `github.com/openshift/api`
- Importing `pkg/cluster/openshift` does **not** pull in `github.com/openshift/api`
- Importing `pkg/cluster/olm` does **not** pull in `github.com/operator-framework/api`

The only dependencies are standard Kubernetes libraries (`k8s.io/apimachinery`,
`sigs.k8s.io/controller-runtime`).

## Stateless Function Convention

Every exported function follows the same signature pattern:

```go
func FunctionName(ctx context.Context, cli client.Reader, ...) (Result, error)
```

- First parameter: `context.Context`
- Second parameter: `client.Reader` or `client.Client`
- No package-level globals, singletons, or `Init()` functions
- No environment variable reads (callers pass values as parameters)

## Error Behavior

- **OpenShift-specific functions** on vanilla K8s: return errors satisfying
  `meta.IsNoMatchError` or `k8serr.IsNotFound`. Callers should check these.
- **OLM-specific functions** without OLM: return errors satisfying
  `meta.IsNoMatchError`.
- **`DetectClusterType`/`DetectClusterInfo`**: never error for "not OpenShift" —
  they return `ClusterTypeKubernetes` instead.

## Testing

Tests use `sigs.k8s.io/controller-runtime/pkg/client/fake` with unstructured
objects. No env-var globals are used — all configuration is passed as function
parameters.

To inject errors, tests use wrapper clients (e.g., `erroringClient`) that
override `Get` or `List` for specific object names.

## Migration Notes from ODH Operator

When migrating operator code to use this shared library, be aware of
these intentional API changes:

- **`IsSingleNodeCluster`** now returns `(bool, error)` instead of just
  `bool`. The operator version silently logged errors and returned `false`;
  the library exposes the error so callers can handle it explicitly. Update
  call sites from `isSNO := cluster.IsSingleNodeCluster(ctx, cli)` to
  `isSNO, err := openshift.IsSingleNodeCluster(ctx, cli)`.

- **`DetectPlatform`** accepts `platformType` and `operatorNamespace` as
  explicit parameters instead of reading `os.Getenv("ODH_PLATFORM_TYPE")`
  and calling the singleton `GetOperatorNamespace()`. Callers must pass
  these values.

- **`ClusterInfo.Version`** is a plain `string` instead of
  `version.OperatorVersion` (semver). Parse with `semver.ParseTolerant()`
  at the call site if structured version comparison is needed.

- **`GetSubscription`** returns `*unstructured.Unstructured` instead of
  `*v1alpha1.Subscription` to avoid importing OLM API types. Use
  `unstructured.NestedString()` and similar helpers to read fields.

## Adding New Detection Functions

1. Determine which layer (cluster type, platform, dependency) the function belongs to
2. Place it in the appropriate package (`cluster`, `cluster/openshift`, `cluster/olm`)
3. Use unstructured clients — never import typed OpenShift/OLM API packages
4. Accept `client.Reader` + `context.Context` (use `client.Client` only if List is needed)
5. Document what cluster types the function supports and error behavior
6. Write table-driven tests covering: resource present, resource absent, API errors
