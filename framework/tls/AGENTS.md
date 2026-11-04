# Framework TLS Package

This package resolves OpenShift TLS security profiles into `crypto/tls.Config`
options and proxy flag strings for module controllers.

## Why OpenShift APIs Are Allowed Here

Named profiles (Old, Intermediate, Modern) live in `configv1.TLSProfiles` and
change as Mozilla/OpenShift guidelines evolve. Copying those tables would drift
from `apiservers.config.openshift.io/cluster`.

`framework/tls` is the scoped framework package that may import:

- `github.com/openshift/api/config/v1`
- `github.com/openshift/controller-runtime-common/pkg/tls`
- `github.com/openshift/library-go/pkg/crypto`

Do not import those modules from other framework packages. The framework
module's depguard also blocks imports from the root `pkg/*` and `api/*` trees.

Reuse `openshift/controller-runtime-common` for generic profile resolution,
`tls.Config` options, and the APIServer watcher. Keep ODH-specific adherence,
strict validation, and TLS 1.3 filtering for TLS 1.2 in this package. Modules
should use its `SetNextProtos` helper directly for ALPN configuration.

Do not import `opendatahub-operator` internals.

## Fallback Policy

| Situation | `FromAPIServer` | `Load` |
|---|---|---|
| Success | cluster profile strings | cluster spec, `Watchable=true` |
| `NotFound` (API served, object missing) | Intermediate, no error | Intermediate, `Watchable=true` (watcher detects creation) |
| `NoMatchError` (API not served) | Intermediate, no error | Intermediate, `Watchable=false` |
| Transient (unavailable, timeout, 429, context deadline) | error | Intermediate, `Watchable=true` |
| Other errors (forbidden, etc.) | error | error (caller should refuse to start) |

`FromAPIServer` and `Load` retain profile-only behavior for compatibility.
`FromAPIServerWithAdherence`, `FromAPIServerWithCurvePreferences`, and
`LoadWithAdherence` honor `tlsAdherence`: NoOpinion/Legacy use the caller's
legacy defaults, while Strict and unknown policies validate and apply the
cluster profile. Missing APIs use Intermediate; transient `LoadWithAdherence`
errors use Intermediate and remain watchable; other errors are returned.

The profile-only resolver falls back to Intermediate for a custom type with a
nil spec. Strict resolvers reject a custom type with no spec or an unknown
type; an empty profile type uses Intermediate.

Proxy flag helpers (`FromProfile`, `MinVersionFromSpec`) floor TLS 1.0/1.1 to
TLS 1.2. `ConfigFromProfile` applies the spec as given so process TLS can
follow the cluster profile. Call `ValidateStrictTLSProfile` before applying a
Strict profile with `ConfigFromProfile`.

## Caller Requirements

- `configv1.Install(scheme)` (or `AddToScheme`) so typed `APIServer` Get/watch works
- RBAC: `get` on `apiservers.config.openshift.io`; add `list`/`watch` only for `SecurityProfileWatcher`
- Register profile-only watching from `Load`; when setting
  `OnAdherencePolicyChange`, initialize the watcher from `LoadWithAdherence`
- Register `SecurityProfileWatcher` only when the selected loader reports `Watchable`
- `github.com/openshift/library-go` pulls `k8s.io/apiserver` transitively; do not import it from this package

Module-facing walkthrough (scheme, RBAC, `main.go`, proxy flags):
[docs/module-tls.md](../../docs/module-tls.md).

## Conventions

- Tests use the `_test` package suffix, `t.Parallel()`, testify assertions
- Fake clients must include `configv1` in the scheme
- Stateless fetch functions accept `context.Context` then `client.Reader`
