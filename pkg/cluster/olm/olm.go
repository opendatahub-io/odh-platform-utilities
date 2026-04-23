// Package olm provides stateless detection functions for OLM (Operator
// Lifecycle Manager) resources: operator existence, subscription queries.
//
// All functions use unstructured Kubernetes clients so that importing this
// package does not pull in github.com/operator-framework/api. When OLM is
// not installed on the cluster, API calls return errors satisfying
// [meta.IsNoMatchError].
//
// Every function is stateless — it accepts a [client.Reader] and
// [context.Context]. There are no package-level globals or Init() functions.
package olm

import (
	"context"
	"errors"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

// ErrOperatorNotInstalled is returned by [OperatorExists] when no
// OperatorCondition matching the given prefix is found.
var ErrOperatorNotInstalled = errors.New("operator not installed")

//nolint:gochecknoglobals // Immutable GVK constants.
var (
	operatorConditionGVK = schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v2", Kind: "OperatorCondition",
	}
	subscriptionGVK = schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription",
	}
	catalogSourceGVK = schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v1alpha1", Kind: "CatalogSource",
	}
)

// OperatorExists checks whether an OLM-managed operator whose
// OperatorCondition name starts with operatorPrefix is installed on the
// cluster.
//
// If found, it returns an [cluster.OperatorInfo] with the version extracted from
// the OperatorCondition name (format: "<prefix>.<version>"). If the
// operator is not installed, it returns (nil, [ErrOperatorNotInstalled]).
//
// Requires OLM. When OLM is absent, returns an error satisfying
// [meta.IsNoMatchError].
func OperatorExists(
	ctx context.Context, cli client.Reader, operatorPrefix string,
) (*cluster.OperatorInfo, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(operatorConditionGVK)

	err := cli.List(ctx, list)
	if err != nil {
		return nil, err
	}

	expectedPrefix := operatorPrefix + "."

	for _, item := range list.Items {
		if !strings.HasPrefix(item.GetName(), expectedPrefix) {
			continue
		}

		version := strings.TrimPrefix(item.GetName(), expectedPrefix)
		if version != "" && !strings.HasPrefix(version, "v") {
			version = "v" + version
		}

		return &cluster.OperatorInfo{Version: version}, nil
	}

	return nil, ErrOperatorNotInstalled
}

// SubscriptionExists checks whether an OLM Subscription with the given
// name exists anywhere on the cluster.
//
// Requires OLM. When OLM is absent, returns an error satisfying
// [meta.IsNoMatchError].
func SubscriptionExists(ctx context.Context, cli client.Reader, name string) (bool, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(subscriptionGVK)

	err := cli.List(ctx, list)
	if err != nil {
		return false, err
	}

	for _, item := range list.Items {
		if item.GetName() == name {
			return true, nil
		}
	}

	return false, nil
}

// GetSubscription retrieves a specific OLM Subscription by namespace and
// name. The returned object is unstructured to avoid importing OLM API
// types.
//
// Requires OLM. Returns a standard Kubernetes NotFound or NoMatch error
// when the Subscription or the CRD is absent.
func GetSubscription(
	ctx context.Context, cli client.Reader, namespace, name string,
) (*unstructured.Unstructured, error) {
	sub := &unstructured.Unstructured{}
	sub.SetGroupVersionKind(subscriptionGVK)

	err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, sub)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

// CatalogSourceExists checks whether a CatalogSource with the given name
// exists in the given namespace.
//
// Requires OLM. Returns false with nil error when the CatalogSource is not
// found. Returns an error when the CatalogSource CRD is absent (OLM not
// installed) or for other API failures.
func CatalogSourceExists(ctx context.Context, cli client.Reader, namespace, name string) (bool, error) {
	cs := &unstructured.Unstructured{}
	cs.SetGroupVersionKind(catalogSourceGVK)

	err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cs)
	if err != nil {
		return false, client.IgnoreNotFound(err)
	}

	return true, nil
}
