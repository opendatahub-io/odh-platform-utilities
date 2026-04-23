// Package cluster provides runtime helpers for working with Kubernetes
// clusters: singleton custom resource retrieval, metadata functional options,
// and stateless cluster/platform detection.
//
// # Singleton Helpers
//
// The ODH Onboarding Guide mandates that all module CRDs are cluster-scoped
// singletons with enforced naming. [GetSingleton] is a generic function that
// retrieves the single instance of a given type, returning an error if zero
// or more than one instance exists.
//
// # Cluster Detection
//
// Two conceptually separate detection layers are exposed:
//
//   - Cluster type detection (infrastructure layer): "Am I on OpenShift or
//     vanilla Kubernetes?" — see [DetectClusterType], [DetectClusterInfo].
//   - Platform variant detection (product layer): "Which product distribution
//     is deploying me?" — see [DetectPlatform].
//
// All detection functions are stateless: they accept a [client.Reader] (or
// [client.Client]) and [context.Context]. There are no package-level globals
// or Init() functions.
//
// Sub-packages provide additional detection helpers:
//
//   - cluster/openshift: OpenShift-specific queries (version, auth, SNO,
//     domain). Uses unstructured clients so no openshift/api import is required.
//   - cluster/olm: OLM-specific queries (operator existence, subscriptions).
//     Uses unstructured clients so no operator-framework/api import is required.
package cluster
