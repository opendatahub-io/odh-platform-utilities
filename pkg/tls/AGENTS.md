# TLS Package

This package resolves OpenShift TLS security profiles into `crypto/tls.Config`
options and proxy flag strings for module controllers.

## Why OpenShift APIs Are Allowed Here

Named profiles (Old, Intermediate, Modern) live in `configv1.TLSProfiles` and
change as Mozilla/OpenShift guidelines evolve. Copying those tables would drift
from `apiservers.config.openshift.io/cluster`.

`pkg/tls` is the **only** root-module package that may import:

- `github.com/openshift/api/config/v1`
- `github.com/openshift/library-go/pkg/crypto`

Do not import those modules from `pkg/cluster`, `pkg/cluster/openshift`, or
any other root package. `pkg/cluster/openshift` stays unstructured.

Do not import `opendatahub-operator` internals or
`openshift/controller-runtime-common`.

## Fallback Policy

| Situation | `FromAPIServer` | `Load` |
|---|---|---|
| Success | cluster profile strings | cluster spec, `Watchable=true` |
| `NotFound` / `NoMatchError` | Intermediate, no error | Intermediate, `Watchable=false` |
| Transient (unavailable, timeout, 429) | error | Intermediate, `Watchable=true` |
| Other errors (forbidden, etc.) | error | error (caller should refuse to start) |

Custom type with a nil spec falls back to Intermediate (do not fail closed).

Proxy flag helpers (`FromProfile`, `MinVersionFromSpec`) floor TLS 1.0/1.1 to
TLS 1.2. `ConfigFromProfile` applies the spec as given so process TLS can
follow the cluster profile.

## Caller Requirements

- `configv1.Install(scheme)` (or `AddToScheme`) so typed `APIServer` Get/watch works
- RBAC: get/list/watch `apiservers.config.openshift.io`
- Register `SecurityProfileWatcher` only when `Load` reports `Watchable`

Module-facing walkthrough (scheme, RBAC, `main.go`, proxy flags):
[docs/module-tls.md](../../docs/module-tls.md).

## Conventions

- Tests use the `_test` package suffix, `t.Parallel()`, testify assertions
- Fake clients must include `configv1` in the scheme
- Stateless fetch functions accept `context.Context` then `client.Reader`
