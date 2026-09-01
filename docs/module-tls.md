# TLS Configuration for Module Controllers

Module operators run as their own Deployments. They cannot inherit the
opendatahub-operator's TLS settings. Each module is expected to configure TLS
for **its own process** (webhooks, metrics) and for **operands it deploys**
(kube-rbac-proxy, kube-auth-proxy, and similar).

This library (`pkg/tls`) supplies the shared resolution helpers. Wiring them
into `main.go`, RBAC, and templates is the module's job.

## Which helper to use

| What you are configuring | Helper | When |
|--------------------------|--------|------|
| Webhook server, metrics server | `Load` then `ConfigFromProfile` | Once, in `main.go` before `mgr.Start` |
| Reload when the cluster profile changes | `SecurityProfileWatcher` | Only if `Load` returns `Watchable` |
| kube-auth-proxy args (`TLS1.2`) | `FromAPIServer(..., FormatShort)` | Each reconcile (or whenever you render the Deployment) |
| kube-rbac-proxy args (`VersionTLS12`) | `FromAPIServer(..., FormatGo)` | Same |
| Explicit CR field / flag of type `TLSSecurityProfile` | `FromProfile` / `ConfigFromProfile` | When the module owns the knob and is not reading APIServer |

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

Baseline (Load / FromAPIServer):

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

Load / FromAPIServer only:

```go
//+kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=get
```

Also register `SecurityProfileWatcher`:

```go
//+kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=get;list;watch
```

## 1. Process TLS (webhook and metrics)

At process start, resolve the cluster profile **before** constructing the
manager, using a bootstrap client. `Load` already applies the startup fallback
policy:

| Cluster state | Spec used | Register watcher? |
|---------------|-----------|-------------------|
| APIServer readable | cluster profile | yes (`Watchable`) |
| Not OpenShift / CR missing | Intermediate | no |
| Transient API error | Intermediate | yes (self-heals) |
| Unexpected error (forbidden, …) | — | refuse to start |

```go
import (
    "context"
    "crypto/tls"
    "os"

    configv1 "github.com/openshift/api/config/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/rest"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
    ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

    pkgtls "github.com/opendatahub-io/odh-platform-utilities/pkg/tls"
)

func setupTLS(ctx context.Context, restConfig *rest.Config, scheme *runtime.Scheme) (
    []func(*tls.Config), pkgtls.LoadResult, error,
) {
    bootstrapClient, err := client.New(restConfig, client.Options{Scheme: scheme})
    if err != nil {
        return nil, pkgtls.LoadResult{}, err
    }

    result, err := pkgtls.Load(ctx, bootstrapClient)
    if err != nil {
        return nil, pkgtls.LoadResult{}, err
    }

    tlsOpts, unsupported := pkgtls.ConfigFromProfile(result.Spec)
    if len(unsupported) > 0 {
        // Log and continue. These names are not implemented by Go crypto/tls.
    }

    return []func(*tls.Config){tlsOpts}, result, nil
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
        Client:                mgr.GetClient(),
        InitialTLSProfileSpec: result.Spec,
        OnProfileChange: func(context.Context, configv1.TLSProfileSpec, configv1.TLSProfileSpec) {
            setupLog.Info("cluster TLS profile changed, shutting down to reload")
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
restart policy brings the process back, and `Load` reads the new profile.

Do **not** call `SetupWithManager` when `Watchable` is false. The APIServer GVK
is absent on vanilla Kubernetes and the watch will fail.

`ConfigFromProfile` applies the spec as given (including Old / TLS 1.0 if that
is the cluster profile). It does not set `NextProtos`; add `h2` / `http/1.1`
in your own `TLSOpts` if the metrics or webhook server needs them.

## 2. Operand / proxy TLS flags

Workloads that take `--tls-min-version` and cipher flags need **strings**, not
a `tls.Config`. Resolve them when you render the Deployment.

```go
minVersion, ciphers, err := pkgtls.FromAPIServer(ctx, r.Client, pkgtls.FormatShort)
if err != nil {
    return fmt.Errorf("resolve TLS profile: %w", err)
}

templateData["TLSMinVersion"] = minVersion
templateData["TLSCipherSuites"] = ciphers
```

Flag format:

| Operand | `VersionFormat` | Typical args |
|---------|-----------------|--------------|
| kube-auth-proxy | `FormatShort` | `--tls-min-version=TLS1.2`, `--tls-cipher-suite=<iana,...>` |
| kube-rbac-proxy | `FormatGo` | `--tls-min-version=VersionTLS12`, `--tls-cipher-suites=<iana,...>` |

`FromAPIServer` treats missing APIServer / non-OpenShift as Intermediate and
returns a nil error. Other errors (forbidden, transient) are returned so
reconcile can retry.

Unlike process TLS, proxy helpers **floor TLS 1.0/1.1 to TLS 1.2** (and replace
ciphers with Intermediate) because those operands cannot safely serve 1.0/1.1.

If the module already has a `*configv1.TLSSecurityProfile` from a CR field or
flag, skip the fetch:

```go
minVersion, ciphers := pkgtls.FromProfile(ctx, cr.Spec.TLSSecurityProfile, pkgtls.FormatGo)
```

## Vanilla Kubernetes (XKS)

No `apiservers.config.openshift.io` API. `Load` and `FromAPIServer` fall back
to Intermediate (TLS 1.2 + Mozilla Intermediate ciphers). The watcher stays
unregistered. Modules do not need a separate code path beyond honoring
`Watchable` and treating `FromAPIServer` errors as described above.

## Testing

Fake clients used with these helpers must install the config API:

```go
scheme := runtime.NewScheme()
require.NoError(t, configv1.Install(scheme))
cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build()
```

Hardcode expected IANA cipher strings in tests rather than calling
`CipherSuitesFromSpec` to produce the oracle. See `pkg/tls` tests for the
Intermediate/Old fixtures.

## Out of scope for the module (today)

- A shared `api/common` TLS embed type — wait for orchestrator projection
  (RHAISTRAT-1716) before standardizing CR fields across modules.
- TLS adherence policy — not exposed by this package.
- Changing TLS on a running listener without restart — cancel the manager
  context and let the pod restart.

## See also

- [pkg/tls GoDoc](https://pkg.go.dev/github.com/opendatahub-io/odh-platform-utilities/pkg/tls)
- [pkg/tls/AGENTS.md](../pkg/tls/AGENTS.md) — fallback table and the OpenShift API exception
- [Migration from the operator](./migration-from-operator.md#tls-pkgtls)
