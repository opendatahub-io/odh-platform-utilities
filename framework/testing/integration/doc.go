// Package integration is a reusable PR gate for ODH module controllers
// against a real cluster.
//
// Call [Run] from a test (typically //go:build integration). Required vs
// optional inputs, skip behavior, and defaults are on [Run], the With*
// options, and the Default* constants. DSC field names live beside
// [DSCSpec].
//
// The installed operator must already know the module: its DataScienceCluster
// CRD must declare the component field, and its reconciler must create and
// watch the module CR. This package does not register components. A
// brand-new module needs that operator onboarding first; an onboarded
// module can use this gate to verify pinned version changes.
//
// The cluster must be reachable via KUBECONFIG (a kubeconfig file path, or
// [WithKubeconfig]) with permission to create/delete a DataScienceCluster
// and to get/patch the module CR and its Deployment. Creating CRDs is only
// required if [WithOperatorManifest] is set.
//
// The operator under test is whoever is already on the cluster. Install it
// in CI (OLM) before [Run]. See [WithOperatorManifest] only for disposable
// clusters with local YAML — not the PR-gate path.
//
// Module-repo setup (go.mod, test file, CI): docs/integration-testing.md
// in this repository.
package integration
