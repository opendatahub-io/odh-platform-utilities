# Integration Test Harness

Module repos import [`framework/testing/integration`](../framework/testing/integration)
and call `Run` as a live-cluster PR gate: enable the module on a
DataScienceCluster, wait until the module CR is `Ready=True` **and**
`ProvisioningSucceeded=True`, wait until a named Deployment has
`readyReplicas >= 1`.

**The operator is a cluster fixture.** CI (or a human) installs it the same
way your other e2e already does. This library does not install OLM. `Run`
creates a DSC, asserts, and deletes that DSC. The operator stays.

API, defaults, and skip behavior are documented in
[`doc.go`](../framework/testing/integration/doc.go) and the GoDoc on `Run`
and the `With*` options.

## PR gate

```text
CI job (separate from unit tests)
  1. Dedicated cluster (this job deletes DSC "default-dsc")
  2. ODH operator installed and Ready (OLM)
  3. DSCInitialization already exists (creates namespace opendatahub)
  4. go test -tags integration  →  Run()
  5. teardown: module CR Removed, delete DSC  (operator stays)
```

`Run` does not create DSCI or the `opendatahub` namespace. Install the
operator **before** the test. Omit `WithOperatorManifest`.

`KUBECONFIG` must be a **file path**. GitHub secrets are kubeconfig
**bytes** — write them to a file first. Do not add `-tags integration` to
unit/envtest `go test`.

```yaml
- name: Write kubeconfig
  env:
    KUBECONFIG_DATA: ${{ secrets.INTEGRATION_KUBECONFIG }}
  run: |
    install -m 600 /dev/null /tmp/kubeconfig
    printf '%s' "$KUBECONFIG_DATA" > /tmp/kubeconfig
- name: Integration test
  env:
    KUBECONFIG: /tmp/kubeconfig
  run: go test -tags integration -v -timeout 20m ./test/e2e/...
```

The operator version must already know your module (DSC
`spec.components.<field>` + reconciler). This package does not onboard
components.

## Prerequisites

- Dedicated real cluster (`KUBECONFIG` file path, or `WithKubeconfig`)
- Operator Ready, DSCI present, namespace `opendatahub` (or override with
  `WithModuleNamespace`)
- Service account permissions: create/patch/delete `DataScienceCluster`, get
  `Deployment`, get/patch your cluster-scoped module CR

## Values to fill in

| Argument | What it is | How to find it |
|----------|------------|----------------|
| First argument to `Run` | **Module controller** Deployment name | `oc get deploy -n opendatahub` — not a workload you create later |
| `Component("…")` | DSC `spec.components` field name, not the CR Kind | `oc explain datasciencecluster.spec.components` |
| `WithModuleCR` GVK | Your singleton **module** CR (PlatformObject) | Your module CRD, usually `components.platform.opendatahub.io` |
| `WithModuleCR` name | Singleton instance name | Often `default`; `oc get <kind>` |

A namespaced workload CR (for example `MCPServer`) is not the module CR.

## 1. Depend on the testing module

Nested module — not `…/framework`.

Once a `framework/testing/vX.Y.Z` tag has been published (see
[VERSIONING.md](VERSIONING.md)):

```bash
go get github.com/opendatahub-io/odh-platform-utilities/framework/testing@vX.Y.Z
```

Until then, or for local development, point all three modules at a local
checkout via `replace` in your `go.mod`:

```
replace (
    github.com/opendatahub-io/odh-platform-utilities         => /path/to/odh-platform-utilities
    github.com/opendatahub-io/odh-platform-utilities/framework => /path/to/odh-platform-utilities/framework
    github.com/opendatahub-io/odh-platform-utilities/framework/testing => /path/to/odh-platform-utilities/framework/testing
)
```

## 2. Add the test

Create `test/e2e/integration_test.go` in the **module** repo. Keep
`//go:build integration` and use a **different** build tag than your
existing e2e suite so this job does not run those tests.

Substitute your own: deployment name, DSC component field, module CR GVK,
and singleton name.

```go
//go:build integration

package e2e_test

import (
    "testing"
    "time"

    "k8s.io/apimachinery/pkg/runtime/schema"

    "github.com/opendatahub-io/odh-platform-utilities/framework/testing/integration"
)

// Full scenario: DSC-registered module component.
func TestIntegration(t *testing.T) {
    integration.Run(t,
        "my-operator", // controller Deployment name in opendatahub namespace
        integration.WithDSCSpec(
            integration.NewDSCSpec().Component("my-component", integration.Managed),
        ),
        integration.WithModuleCR(
            schema.GroupVersionKind{
                Group:   "components.platform.opendatahub.io",
                Version: "v1alpha1",
                Kind:    "MyComponent", // replace with your CR Kind
            },
            "default-mycomponent",
        ),
        integration.WithTimeout(10*time.Minute),
    )
}

// Minimal scenario: independent operator, no DSC.
func TestIntegrationNoDSC(t *testing.T) {
    integration.Run(t,
        "my-operator-controller-manager",
        integration.WithTimeout(10*time.Minute),
    )
}
```

## 3. Run it

```bash
go test -tags integration -v -timeout 20m ./test/e2e/...
```

Use a timeout longer than `WithTimeout` (default 5 m is often short).

## WithOperatorManifest (not the PR gate)

SSA-applies multi-document YAML before creating the DSC. Teardown does
**not** uninstall the operator — do not use this on a shared gate cluster.

Accepts any [`resources.Source`](../framework/resources/source.go):
[`NewFileSource`](../framework/resources/source.go) (absolute path) or
[`NewURLSource`](../framework/resources/source.go) (HTTPS URL; load errors
surface as `t.Fatalf`). Not a channel, quay image, GitHub tag, or PR.
Operator releases do not ship `install.yaml`.
