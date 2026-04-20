package cluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

//nolint:gochecknoglobals // Immutable GVK constant.
var clusterVersionGVK = schema.GroupVersionKind{
	Group:   "config.openshift.io",
	Version: "v1",
	Kind:    "ClusterVersion",
}

// DetectClusterType probes the API server for the ClusterVersion CRD to
// determine whether the cluster is OpenShift or vanilla Kubernetes.
//
// On OpenShift clusters the ClusterVersion CR named "version" is always
// present. On vanilla Kubernetes the CRD is absent and the API server
// returns a NoMatch error, which this function interprets as
// [ClusterTypeKubernetes].
//
// Returns an error only for unexpected API failures (network errors, RBAC
// issues). CRD-not-found is not an error — it means "not OpenShift".
func DetectClusterType(ctx context.Context, r client.Reader) (ClusterType, error) {
	cv := &unstructured.Unstructured{}
	cv.SetGroupVersionKind(clusterVersionGVK)

	err := r.Get(ctx, client.ObjectKey{Name: OpenShiftVersionObj}, cv)
	if err != nil {
		if meta.IsNoMatchError(err) || errors.IsNotFound(err) {
			return ClusterTypeKubernetes, nil
		}

		return "", fmt.Errorf("detecting cluster type: %w", err)
	}

	return ClusterTypeOpenShift, nil
}

// DetectClusterInfo combines cluster type detection, OpenShift version
// extraction, and FIPS status into a single [ClusterInfo] value.
//
// On vanilla Kubernetes clusters the returned ClusterInfo has
// Type=[ClusterTypeKubernetes], an empty Version, and FipsEnabled=false.
//
// On OpenShift clusters that are not in FIPS mode, FipsEnabled is false.
// Returns an error if the FIPS ConfigMap cannot be read or parsed.
func DetectClusterInfo(ctx context.Context, r client.Reader) (ClusterInfo, error) {
	info := ClusterInfo{
		Type: ClusterTypeOpenShift,
	}

	cv := &unstructured.Unstructured{}
	cv.SetGroupVersionKind(clusterVersionGVK)

	err := r.Get(ctx, client.ObjectKey{Name: OpenShiftVersionObj}, cv)
	if err != nil {
		if meta.IsNoMatchError(err) || errors.IsNotFound(err) {
			info.Type = ClusterTypeKubernetes

			return info, nil
		}

		return info, fmt.Errorf("detecting cluster type: %w", err)
	}

	info.Version = extractOCPVersion(cv)

	fips, err := IsFipsEnabled(ctx, r)
	if err != nil {
		return info, fmt.Errorf("detecting FIPS mode: %w", err)
	}

	info.FipsEnabled = fips

	return info, nil
}

// extractOCPVersion reads status.history[0].version from a ClusterVersion
// unstructured object. Returns empty string on any parse failure.
func extractOCPVersion(cv *unstructured.Unstructured) string {
	history, found, err := unstructured.NestedSlice(cv.Object, "status", "history")
	if err != nil || !found || len(history) == 0 {
		return ""
	}

	entry, ok := history[0].(map[string]any)
	if !ok {
		return ""
	}

	v, ok := entry["version"].(string)
	if !ok {
		return ""
	}

	return v
}

type installConfig struct {
	FIPS bool `json:"fips"`
}

// IsFipsEnabled reads the kube-system/cluster-config-v1 ConfigMap to
// determine whether the cluster was installed in FIPS mode.
//
// This function works on any Kubernetes cluster:
//   - On OpenShift, the ConfigMap is created by the installer and contains an
//     "install-config" key with a YAML document that may include "fips: true".
//   - On vanilla Kubernetes, the ConfigMap is absent; this returns (false, nil).
//
// Returns an error only for unexpected API failures (RBAC, network) or
// malformed YAML in the ConfigMap.
func IsFipsEnabled(ctx context.Context, r client.Reader) (bool, error) {
	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	})

	err := r.Get(ctx, types.NamespacedName{
		Name:      "cluster-config-v1",
		Namespace: "kube-system",
	}, cm)
	if err != nil {
		if errors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return false, nil
		}

		return false, fmt.Errorf("reading cluster-config-v1: %w", err)
	}

	data, _, err := unstructured.NestedStringMap(cm.Object, "data")
	if err != nil {
		return false, fmt.Errorf("reading ConfigMap data: %w", err)
	}

	raw := data["install-config"]

	if raw == "" {
		return false, nil
	}

	var ic installConfig

	err = yaml.Unmarshal([]byte(raw), &ic)
	if err != nil {
		return false, fmt.Errorf("parsing install-config YAML: %w", err)
	}

	return ic.FIPS, nil
}
