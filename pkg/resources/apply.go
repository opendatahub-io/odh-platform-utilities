package resources

import (
	"context"
	"fmt"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Apply patches an object using server-side apply. It converts the input to
// unstructured, strips managedFields/resourceVersion/status, applies the patch,
// and writes the result back into in so callers see the server response.
func Apply(ctx context.Context, cli client.Client, in client.Object, opts ...client.ApplyOption) error {
	err := EnsureGroupVersionKind(cli.Scheme(), in)
	if err != nil {
		return fmt.Errorf("failed to ensure GVK: %w", err)
	}

	u, err := ToUnstructured(in)
	if err != nil {
		return fmt.Errorf("failed to convert resource to unstructured: %w", err)
	}

	u = u.DeepCopy()

	unstructured.RemoveNestedField(u.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(u.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(u.Object, "status")

	err = cli.Apply(ctx, client.ApplyConfigurationFromUnstructured(u), opts...)
	if err != nil {
		objRef := FormatObjectReference(u)
		return fmt.Errorf("unable to patch %s: %w", objRef, err)
	}

	err = cli.Scheme().Convert(u, in, ctx)
	if err != nil {
		return fmt.Errorf("failed to write modified object: %w", err)
	}

	return nil
}

// ApplyStatus patches the status subresource of a Kubernetes object using
// server-side apply. NotFound errors are treated as success so that the caller
// does not need to distinguish "resource not yet created" from "status applied".
func ApplyStatus(
	ctx context.Context, cli client.Client, in client.Object, opts ...client.SubResourceApplyOption,
) error {
	err := EnsureGroupVersionKind(cli.Scheme(), in)
	if err != nil {
		return fmt.Errorf("failed to ensure GVK: %w", err)
	}

	u, err := ToUnstructured(in)
	if err != nil {
		return fmt.Errorf("failed to convert resource to unstructured: %w", err)
	}

	u = u.DeepCopy()

	unstructured.RemoveNestedField(u.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(u.Object, "metadata", "resourceVersion")

	err = cli.Status().Apply(ctx, client.ApplyConfigurationFromUnstructured(u), opts...)
	switch {
	case k8serr.IsNotFound(err):
		return nil
	case err != nil:
		objRef := FormatObjectReference(u)
		return fmt.Errorf("unable to patch %s status: %w", objRef, err)
	}

	err = cli.Scheme().Convert(u, in, ctx)
	if err != nil {
		return fmt.Errorf("failed to write modified object: %w", err)
	}

	return nil
}
