package cluster

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// OLM GVKs are intentionally duplicated from cluster/olm to avoid an
// import cycle (olm imports cluster for OperatorInfo).
//
//nolint:gochecknoglobals // Immutable GVK constants.
var (
	platformOperatorConditionGVK = schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v2", Kind: "OperatorCondition",
	}
	platformCatalogSourceGVK = schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v1alpha1", Kind: "CatalogSource",
	}
)

// DetectPlatform determines the product distribution (platform variant)
// that is deploying the controller.
//
// Detection follows this precedence:
//  1. If platformType is a recognized non-empty string ("OpenDataHub",
//     "ManagedRHOAI", "SelfManagedRHOAI", "XKS"), the corresponding
//     [Platform] constant is returned immediately.
//  2. Otherwise, OLM resources are probed to auto-detect:
//     a. If an "addon-managed-odh-catalog" CatalogSource exists in
//     operatorNamespace → [ManagedRhoai].
//     b. If a "rhods-operator" OperatorCondition exists → [SelfManagedRhoai].
//     c. Fallback → [OpenDataHub].
//
// The platformType parameter typically comes from the ODH_PLATFORM_TYPE
// environment variable (set in the operator CSV). The operatorNamespace
// parameter is used for the ManagedRhoai CatalogSource lookup and may be
// empty (defaults to "redhat-ods-operator").
//
// This function is a transitional necessity. In the fully-realized module
// architecture, modules should prefer reading platform info from their
// projected CR config (set by the orchestrator) rather than calling
// detection directly.
func DetectPlatform(ctx context.Context, cli client.Reader, platformType, operatorNamespace string) (Platform, error) {
	switch platformType {
	case "OpenDataHub":
		return OpenDataHub, nil
	case "ManagedRHOAI":
		return ManagedRhoai, nil
	case "SelfManagedRHOAI":
		return SelfManagedRhoai, nil
	case string(XKS):
		return XKS, nil
	}

	if operatorNamespace == "" {
		operatorNamespace = "redhat-ods-operator"
	}

	managed, err := detectManagedRhoai(ctx, cli, operatorNamespace)
	if err != nil {
		return "", err
	}

	if managed {
		return ManagedRhoai, nil
	}

	return detectSelfManaged(ctx, cli)
}

func detectManagedRhoai(ctx context.Context, cli client.Reader, operatorNamespace string) (bool, error) {
	cs := &unstructured.Unstructured{}
	cs.SetGroupVersionKind(platformCatalogSourceGVK)

	err := cli.Get(ctx, client.ObjectKey{
		Name:      "addon-managed-odh-catalog",
		Namespace: operatorNamespace,
	}, cs)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return false, nil
		}

		return false, client.IgnoreNotFound(err)
	}

	return true, nil
}

func detectSelfManaged(ctx context.Context, cli client.Reader) (Platform, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(platformOperatorConditionGVK)

	err := cli.List(ctx, list)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return OpenDataHub, nil
		}

		return "", client.IgnoreNotFound(err)
	}

	for _, item := range list.Items {
		if strings.HasPrefix(item.GetName(), "rhods-operator.") {
			return SelfManagedRhoai, nil
		}
	}

	return OpenDataHub, nil
}
