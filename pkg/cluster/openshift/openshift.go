// Package openshift provides stateless detection functions for OpenShift-specific
// cluster properties: version, single-node topology, authentication mode,
// service account issuer, and ingress domain.
//
// All functions use unstructured Kubernetes clients so that importing this
// package does not pull in github.com/openshift/api. Callers on vanilla
// Kubernetes clusters will receive appropriate errors (typically
// [meta.IsNoMatchError] or [k8serr.IsNotFound]) when the required OpenShift
// CRDs are absent.
//
// Every function is stateless — it accepts a [client.Reader] (or
// [client.Client]) and [context.Context]. There are no package-level
// globals, singletons, or Init() functions.
package openshift

import (
	"context"
	"errors"
	"fmt"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

var (
	// Sentinel errors for ClusterVersion parsing failures.
	errHistoryEmpty        = errors.New("ClusterVersion status.history is empty or unreadable")
	errHistoryEntryInvalid = errors.New("ClusterVersion status.history[0] is not an object")
	errVersionEmpty        = errors.New("ClusterVersion status.history[0].version is empty")
	errDomainEmpty         = errors.New("spec.domain not found or empty")
)

//nolint:gochecknoglobals // Immutable GVK constants.
var (
	clusterVersionGVK = schema.GroupVersionKind{
		Group: "config.openshift.io", Version: "v1", Kind: "ClusterVersion",
	}
	infrastructureGVK = schema.GroupVersionKind{
		Group: "config.openshift.io", Version: "v1", Kind: "Infrastructure",
	}
	authenticationGVK = schema.GroupVersionKind{
		Group: "config.openshift.io", Version: "v1", Kind: "Authentication",
	}
	ingressGVK = schema.GroupVersionKind{
		Group: "config.openshift.io", Version: "v1", Kind: "Ingress",
	}
	nodeGVK = schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "Node",
	}
)

// GetVersion reads the ClusterVersion CR and returns the version string
// from status.history[0].version.
//
// Requires OpenShift. On vanilla Kubernetes where the ClusterVersion CRD is
// absent, returns an error satisfying [meta.IsNoMatchError].
func GetVersion(ctx context.Context, r client.Reader) (string, error) {
	cv := &unstructured.Unstructured{}
	cv.SetGroupVersionKind(clusterVersionGVK)

	err := r.Get(ctx, client.ObjectKey{Name: cluster.OpenShiftVersionObj}, cv)
	if err != nil {
		return "", fmt.Errorf("unable to get OCP version: %w", err)
	}

	history, found, err := unstructured.NestedSlice(cv.Object, "status", "history")
	if err != nil || !found || len(history) == 0 {
		return "", errHistoryEmpty
	}

	entry, ok := history[0].(map[string]any)
	if !ok {
		return "", errHistoryEntryInvalid
	}

	v, ok := entry["version"].(string)
	if !ok || v == "" {
		return "", errVersionEmpty
	}

	return v, nil
}

// IsSingleNodeCluster determines whether the cluster uses a single-node
// topology.
//
// On OpenShift it reads the Infrastructure CR's
// status.controlPlaneTopology field. If that resource is unavailable (non-
// OpenShift clusters, RBAC restrictions), it falls back to counting
// schedulable nodes — a single schedulable node means SNO.
//
// Returns an error when the topology cannot be determined because of an
// unexpected API failure.
func IsSingleNodeCluster(ctx context.Context, cli client.Reader) (bool, error) {
	infra := &unstructured.Unstructured{}
	infra.SetGroupVersionKind(infrastructureGVK)

	err := cli.Get(ctx, client.ObjectKey{Name: "cluster"}, infra)
	if err == nil {
		topology, _, _ := unstructured.NestedString(infra.Object, "status", "controlPlaneTopology")

		return topology == "SingleReplica", nil
	}

	if !k8serr.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return false, err
	}

	nodeList := &unstructured.UnstructuredList{}
	nodeList.SetGroupVersionKind(nodeGVK)

	err = cli.List(ctx, nodeList)
	if err != nil {
		return false, err
	}

	schedulable := 0

	for _, node := range nodeList.Items {
		unschedulable, _, _ := unstructured.NestedBool(node.Object, "spec", "unschedulable")
		if !unschedulable {
			schedulable++
		}
	}

	return schedulable == 1, nil
}

// GetAuthenticationMode reads the OpenShift Authentication CR to determine
// the cluster's authentication mode.
//
// Requires OpenShift. On vanilla Kubernetes where the Authentication CRD
// is absent, returns a NotFound error. Callers should check with
// [k8serr.IsNotFound].
//
// Mapping:
//   - "" or "IntegratedOAuth" → [cluster.AuthModeIntegratedOAuth]
//   - "OIDC"                  → [cluster.AuthModeOIDC]
//   - "None"                  → [cluster.AuthModeNone]
//   - any other value         → [cluster.AuthModeNone]
func GetAuthenticationMode(ctx context.Context, r client.Reader) (cluster.AuthenticationMode, error) {
	auth := &unstructured.Unstructured{}
	auth.SetGroupVersionKind(authenticationGVK)

	err := r.Get(ctx, client.ObjectKey{Name: cluster.ClusterAuthenticationObj}, auth)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return "", k8serr.NewNotFound(
				schema.GroupResource{Group: authenticationGVK.Group, Resource: "authentications"},
				cluster.ClusterAuthenticationObj,
			)
		}

		return "", fmt.Errorf("failed to get cluster authentication config: %w", err)
	}

	authType, _, _ := unstructured.NestedString(auth.Object, "spec", "type")

	switch authType {
	case "OIDC":
		return cluster.AuthModeOIDC, nil
	case "None":
		return cluster.AuthModeNone, nil
	case "", "IntegratedOAuth":
		return cluster.AuthModeIntegratedOAuth, nil
	default:
		return cluster.AuthModeNone, nil
	}
}

// IsIntegratedOAuth returns true if the cluster uses IntegratedOAuth
// authentication mode, which is the default on OpenShift.
//
// Requires OpenShift. Returns an error on vanilla Kubernetes.
func IsIntegratedOAuth(ctx context.Context, r client.Reader) (bool, error) {
	mode, err := GetAuthenticationMode(ctx, r)
	if err != nil {
		return false, err
	}

	return mode == cluster.AuthModeIntegratedOAuth, nil
}

// GetServiceAccountIssuer reads the serviceAccountIssuer field from the
// OpenShift Authentication CR. This is used for kubernetesTokenReview
// audiences on HyperShift/ROSA clusters which use a custom OIDC provider URL.
//
// Returns empty string with nil error when the field is empty on OpenShift.
// On vanilla Kubernetes, returns a NotFound/NoMatch-style error.
func GetServiceAccountIssuer(ctx context.Context, r client.Reader) (string, error) {
	auth := &unstructured.Unstructured{}
	auth.SetGroupVersionKind(authenticationGVK)

	err := r.Get(ctx, client.ObjectKey{Name: cluster.ClusterAuthenticationObj}, auth)
	if err != nil {
		if meta.IsNoMatchError(err) || k8serr.IsNotFound(err) {
			return "", err
		}

		return "", fmt.Errorf("failed to get cluster authentication config: %w", err)
	}

	issuer, _, _ := unstructured.NestedString(auth.Object, "spec", "serviceAccountIssuer")

	return issuer, nil
}

// GetDomain reads the OpenShift Ingress CR to determine the cluster's
// apps domain. It first checks spec.appsDomain (custom override), then
// falls back to spec.domain (default wildcard domain).
//
// Requires OpenShift. On vanilla Kubernetes where the Ingress CRD
// (config.openshift.io) is absent, returns an error.
func GetDomain(ctx context.Context, r client.Reader) (string, error) {
	ingress := &unstructured.Unstructured{}
	ingress.SetGroupVersionKind(ingressGVK)

	err := r.Get(ctx, client.ObjectKey{Name: "cluster"}, ingress)
	if err != nil {
		return "", fmt.Errorf("failed fetching cluster's ingress details: %w", err)
	}

	appsDomain, found, err := unstructured.NestedString(ingress.Object, "spec", "appsDomain")
	if err != nil {
		return "", fmt.Errorf("failed to read spec.appsDomain: %w", err)
	}

	if found && len(appsDomain) > 0 {
		return appsDomain, nil
	}

	domain, found, err := unstructured.NestedString(ingress.Object, "spec", "domain")
	if err != nil {
		return "", fmt.Errorf("failed to read spec.domain: %w", err)
	}

	if !found || len(domain) == 0 {
		return "", errDomainEmpty
	}

	return domain, nil
}
