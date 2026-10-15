// Package ingressassignment resolves namespace ingress assignments to configured ingresses.
package ingressassignment

import (
	"context"
	"errors"
	"fmt"

	"github.com/opendatahub-io/odh-platform-utilities/framework/metadata/annotations"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Ingress is a configured ingress available for route assignment.
type Ingress struct {
	// Name is matched against the Namespace's opendatahub.io/ingress-name annotation.
	Name string `json:"name"`
	// GatewayName may differ from Name and is passed through to route consumers.
	GatewayName      string `json:"gatewayName"`
	GatewayNamespace string `json:"gatewayNamespace"`
	// Hostname is optional passthrough data.
	Hostname  string `json:"hostname,omitempty"`
	IsDefault bool   `json:"isDefault,omitempty"`
}

// ErrUnknownIngress means the requested ingress, or an absent default, is unavailable.
var ErrUnknownIngress = errors.New("unknown ingress")

// ErrInvalidIngresses means the configured ingress list fails validation.
var ErrInvalidIngresses = errors.New("invalid ingress configuration")

// Resolve reads the Namespace and selects the default ingress when its annotation
// is absent or names the default ingress, or resolves an exact ingress name.
// It returns the selected Ingress unchanged.
// It allows at most one default ingress and requires unique nonempty ingress
// names and Gateway names, with nonempty Gateway namespaces. Hostnames are optional.
// Gateway references come from the input list. Resolve does not read Gateways
// or check their existence or readiness.
// All errors return an empty Ingress. Invalid configuration wraps ErrInvalidIngresses
// before reading the Namespace. Namespace read failures wrap the underlying error.
// Unknown, removed, or empty annotated names, and an absent annotation without a
// default, wrap ErrUnknownIngress without fallback. Use errors.Is to match sentinels.
// A direct API client needs get permission on core namespaces. A controller
// using a cached client or watching Namespaces needs list and watch permission.
// Controllers should watch Namespace creation, deletion, and changes to the
// opendatahub.io/ingress-name annotation, mapping each event to the affected
// reconciliations. Configured ingress changes also require a separate requeue.
func Resolve(
	ctx context.Context,
	k8sClient client.Client,
	namespaceName string,
	ingresses []Ingress,
) (Ingress, error) {
	err := validateIngresses(ingresses)
	if err != nil {
		return Ingress{}, err
	}

	var namespace corev1.Namespace

	err = k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, &namespace)
	if err != nil {
		return Ingress{}, fmt.Errorf("get namespace %q: %w", namespaceName, err)
	}

	name, annotated := namespace.Annotations[annotations.IngressName]
	for _, configured := range ingresses {
		if (annotated && configured.Name == name) || (!annotated && configured.IsDefault) {
			return configured, nil
		}
	}

	if !annotated {
		return Ingress{}, fmt.Errorf("namespace %q has no default ingress: %w", namespaceName, ErrUnknownIngress)
	}

	return Ingress{}, fmt.Errorf("namespace %q references %w %q", namespaceName, ErrUnknownIngress, name)
}

func validateIngresses(ingresses []Ingress) error {
	names := make(map[string]struct{}, len(ingresses))
	gateways := make(map[string]struct{}, len(ingresses))
	defaultCount := 0

	for i, configured := range ingresses {
		if configured.Name == "" {
			return fmt.Errorf("%w: ingress at index %d has empty name", ErrInvalidIngresses, i)
		}

		if configured.GatewayName == "" {
			return fmt.Errorf("%w: ingress %q has empty Gateway name", ErrInvalidIngresses, configured.Name)
		}

		if configured.GatewayNamespace == "" {
			return fmt.Errorf("%w: ingress %q has empty Gateway namespace", ErrInvalidIngresses, configured.Name)
		}

		if _, found := names[configured.Name]; found {
			return fmt.Errorf("%w: duplicate ingress name %q", ErrInvalidIngresses, configured.Name)
		}

		if _, found := gateways[configured.GatewayName]; found {
			return fmt.Errorf("%w: duplicate Gateway name %q", ErrInvalidIngresses, configured.GatewayName)
		}

		names[configured.Name] = struct{}{}
		gateways[configured.GatewayName] = struct{}{}

		if configured.IsDefault {
			defaultCount++
		}
	}

	if defaultCount > 1 {
		return fmt.Errorf("%w: expected at most one default ingress, got %d", ErrInvalidIngresses, defaultCount)
	}

	return nil
}
