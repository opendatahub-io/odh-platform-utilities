# TLS Configuration for Module Controllers

Module operators run as their own Deployments. They cannot inherit the
opendatahub-operator's TLS settings. Each module is expected to configure TLS
for **its own process** (webhooks, metrics) and for **operands it deploys**
(kube-rbac-proxy, kube-auth-proxy, and similar).

This library (`framework/tls`) supplies the shared resolution helpers. Wiring them
into `main.go`, RBAC, and templates is the module's job.

The package reuses `controller-runtime-common` for generic profile resolution,
TLS config construction, and APIServer watching. It adds ODH-specific adherence
handling, strict-profile validation, and TLS 1.3 suite filtering for TLS 1.2.

## Which helper to use

| What you are configuring | Helper | When |
|--------------------------|--------|------|
| Webhook server, metrics server | `LoadWithAdherence` then `ConfigFromProfileStrict` | Once, in `main.go` before `mgr.Start` |
| Reload when profile or adherence changes | `SecurityProfileWatcher` | Only if `LoadWithAdherence` returns `Watchable` |
| ALPN on process TLS | `controller-runtime-common` `SetNextProtos` and `HTTP2NextProtos` | Alongside `ConfigFromProfileStrict` when HTTP/2 is enabled |
| kube-auth-proxy args (`TLS1.2`) | `FromAPIServerWithCurvePreferences(..., FormatShort)` | Each reconcile (or whenever you render the Deployment) |
| kube-rbac-proxy args (`VersionTLS12`) | `FromAPIServerWithCurvePreferences(..., FormatGo)` | Same |
| Explicit CR field / flag of type `TLSSecurityProfile` | `FromProfile` / `FromProfileStrict` or their curve-preference variants | When the module owns the knob and is not reading APIServer |

Do not copy `configv1.TLSProfiles` into the module. Named profiles change with
OpenShift/Mozilla guidelines; this package reads the cluster types so you stay
aligned.

## Prerequisites

### Scheme

Typed Get/watch of `configv1.APIServer` requires the OpenShift config API in
the scheme:

```go
import configv1 "github.com/openshift/api/config/v1"

utilruntime.Must(configv1.Install(scheme))
```

Without this, Get fails even on OpenShift.

### RBAC

Grant the module service account `get` on `apiservers.config.openshift.io`.
Add `list` and `watch` only when registering `SecurityProfileWatcher`.

Baseline (`Load`, `LoadWithAdherence`, or `FromAPIServer`):

```text
apiGroups: ["config.openshift.io"]
resources: ["apiservers"]
verbs: ["get"]
```

With the watcher:

```text
apiGroups: ["config.openshift.io"]
resources: ["apiservers"]
verbs: ["get", "list", "watch"]
```

`Load` / `FromAPIServer` helpers only:

```go
//+kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=get
```

Also register `SecurityProfileWatcher`:

```go
//+kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=get;list;watch
```

## 1. Process TLS (webhook and metrics)

At process start, resolve the cluster profile and adherence policy **before**
constructing the manager, using a bootstrap client. `LoadWithAdherence` handles
startup fallbacks and validates the profile under Strict or unknown adherence:

| Cluster state | Result | Register watcher? |
|---------------|--------|-------------------|
| Strict/unknown adherence, valid profile | cluster profile + adherence policy | yes (`Watchable`) |
| NoOpinion/Legacy adherence | resolved profile + policy; caller uses its legacy defaults | yes (`Watchable`) |
| Not OpenShift / CR missing | Intermediate + NoOpinion | no |
| Transient API error | Intermediate + NoOpinion | yes (self-heals) |
| Invalid Strict profile or unexpected API error | error | refuse to start |

```go
import (
    "context"
    "crypto/tls"
    "fmt"
    "os"

    commonTLS "github.com/openshift/controller-runtime-common/pkg/tls"
    configv1 "github.com/openshift/api/config/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/rest"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
    ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

    pkgtls "github.com/opendatahub-io/odh-platform-utilities/framework/tls"
)

func setupTLS(ctx context.Context, restConfig *rest.Config, scheme *runtime.Scheme) (
    []func(*tls.Config), pkgtls.LoadResult, error,
) {
    bootstrapClient, err := client.New(restConfig, client.Options{Scheme: scheme})
    if err != nil {
        return nil, pkgtls.LoadResult{}, err
    }

    result, err := pkgtls.LoadWithAdherence(ctx, bootstrapClient)
    if err != nil {
        return nil, pkgtls.LoadResult{}, err
    }

    profile := result.Spec
    if !pkgtls.ShouldHonorClusterTLSProfile(result.AdherencePolicy) {
        profile = *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
    }
    tlsOpts, unsupported, err := pkgtls.ConfigFromProfileStrict(profile)
    if err != nil {
        return nil, result, fmt.Errorf("validate TLS profile: %w", err)
    }
    if len(unsupported) > 0 {
        // Log and continue. These names are not implemented by Go crypto/tls.
    }

    return []func(*tls.Config){
        tlsOpts,
        commonTLS.SetNextProtos(commonTLS.HTTP2NextProtos...),
    }, result, nil
}
```

Pass `tlsOpts` into both servers, then start the manager on a cancelable
context when the profile is watchable:

```go
tlsOpts, result, err := setupTLS(ctx, restConfig, scheme)
if err != nil {
    setupLog.Error(err, "unable to resolve TLS profile")
    os.Exit(1)
}

mgrCtx := ctx
var cancel context.CancelFunc
if result.Watchable {
    mgrCtx, cancel = context.WithCancel(ctx)
    defer cancel()
}

mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
    Scheme: scheme,
    Metrics: metricsserver.Options{
        TLSOpts: tlsOpts,
    },
    WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
        TLSOpts: tlsOpts,
    }),
})
if err != nil {
    os.Exit(1)
}

if result.Watchable {
    watcher := &pkgtls.SecurityProfileWatcher{
        Client:                    mgr.GetClient(),
        InitialTLSProfileSpec:     result.Spec,
        InitialTLSAdherencePolicy: result.AdherencePolicy,
        OnProfileChange: func(context.Context, configv1.TLSProfileSpec, configv1.TLSProfileSpec) {
            setupLog.Info("cluster TLS profile changed, shutting down to reload")
            cancel()
        },
        OnAdherencePolicyChange: func(context.Context, configv1.TLSAdherencePolicy, configv1.TLSAdherencePolicy) {
            setupLog.Info("cluster TLS adherence policy changed, shutting down to reload")
            cancel()
        },
    }
    if err := watcher.SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to register TLS profile watcher")
        os.Exit(1)
    }
}

if err := mgr.Start(mgrCtx); err != nil {
    setupLog.Error(err, "problem running manager")
    os.Exit(1)
}
```

Canceling the manager context is the supported reload: Go cannot change
`MinVersion` / `CipherSuites` on an already-listening server. The Deployment's
restart policy brings the process back, and `LoadWithAdherence` reads the new
profile and policy.

Do **not** call `SetupWithManager` when `Watchable` is false. The APIServer GVK
is absent on vanilla Kubernetes and the watch will fail.

`ConfigFromProfileStrict` validates the profile before applying it and does not
set `NextProtos`;
the `setupTLS` snippet uses `controller-runtime-common` to advertise `h2` with
`http/1.1` fallback. Under NoOpinion or Legacy adherence,
`LoadWithAdherence` uses the documented Intermediate compatibility fallback for
unknown profile types. Strict adherence rejects malformed profile types and
unsupported profiles before they reach `ConfigFromProfileStrict`.

## 2. Operand / proxy TLS flags

Workloads that take `--tls-min-version` and cipher flags need **strings**, not
a `tls.Config`. Resolve them when you render the Deployment.

```go
minVersion, ciphers, curves, err := pkgtls.FromAPIServerWithCurvePreferences(
    ctx, r.Client, pkgtls.FormatShort,
)
if err != nil {
    return fmt.Errorf("resolve TLS profile: %w", err)
}

templateData["TLSMinVersion"] = minVersion
templateData["TLSCipherSuites"] = ciphers
templateData["TLSCurvePreferences"] = curves
```

Flag format:

| Operand | `VersionFormat` | Typical args |
|---------|-----------------|--------------|
| kube-auth-proxy | `FormatShort` | `--tls-min-version=TLS1.2`, `--tls-cipher-suite=<iana,...>` |
| kube-rbac-proxy | `FormatGo` | `--tls-min-version=VersionTLS12`, `--tls-cipher-suites=<iana,...>` |

`FromAPIServerWithAdherence` and `FromAPIServerWithCurvePreferences` treat a
missing APIServer or a non-adhering policy as Intermediate defaults. Under
Strict or unknown adherence, they reject profiles the proxy cannot represent.
Other API errors are returned so reconcile can retry. The older `FromAPIServer`
helper remains available for callers that intentionally do not read adherence.

Legacy proxy helpers (`FromProfile`, `FromAPIServer`) **floor TLS 1.0/1.1 to
TLS 1.2** and replace ciphers with Intermediate. Under Strict or unknown
adherence, the new helpers reject unsupported versions instead of flooring.

If the module already has a `*configv1.TLSSecurityProfile` from a CR field or
flag, skip the fetch:

```go
minVersion, ciphers := pkgtls.FromProfile(ctx, cr.Spec.TLSSecurityProfile, pkgtls.FormatGo)
```

## Vanilla Kubernetes (XKS)

No `apiservers.config.openshift.io` API. `LoadWithAdherence` and the
adherence-aware `FromAPIServer` helpers fall back to Intermediate (TLS 1.2 +
Mozilla Intermediate ciphers). The watcher stays unregistered. Modules do not
need a separate code path beyond honoring `Watchable` and handling API errors
as described above.

## Testing

Fake clients used with these helpers must install the config API:

```go
scheme := runtime.NewScheme()
require.NoError(t, configv1.Install(scheme))
cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()
```

Hardcode expected IANA cipher strings in tests rather than calling
`CipherSuitesFromSpec` to produce the oracle. See `framework/tls` tests for the
Intermediate/Old fixtures.

## Out of scope for the module (today)

- A shared `api/common` TLS embed type — wait for orchestrator projection
  (RHAISTRAT-1716) before standardizing CR fields across modules.
- Changing TLS on a running listener without restart — cancel the manager
  context and let the pod restart.

## See also

- [framework/tls GoDoc](https://pkg.go.dev/github.com/opendatahub-io/odh-platform-utilities/framework/tls)
- [framework/tls/AGENTS.md](../framework/tls/AGENTS.md) — fallback table and the OpenShift API exception
- [Migration from the operator](./migration-from-operator.md#tls-frameworktls)
