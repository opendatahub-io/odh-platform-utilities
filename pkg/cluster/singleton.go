package cluster

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrNoInstance is returned by GetSingleton when no instance of the
	// requested type exists in the cluster.
	ErrNoInstance = errors.New("no instance found")

	// ErrMultipleInstances is returned by GetSingleton when more than one
	// instance of the requested type exists in the cluster.
	ErrMultipleInstances = errors.New("expected exactly one instance")

	// ErrUnregisteredType is returned when the target type has no GVK
	// registered in the client's scheme.
	ErrUnregisteredType = errors.New("type not registered in scheme")
)

// GetSingleton retrieves the single instance of a cluster-scoped CRD. It
// lists all objects of the target's GVK and returns an error if the count is
// not exactly one. On success the target pointer is populated with the
// singleton's data.
//
// The function derives the GVK from the client's scheme, so the target type
// must be registered. It uses an unstructured list internally to avoid
// requiring a typed ObjectList.
func GetSingleton[T client.Object](ctx context.Context, c client.Client, target T) error {
	gvk, err := gvkForObject(target, c.Scheme())
	if err != nil {
		return fmt.Errorf("determining GVK for %T: %w", target, err)
	}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind + "List",
	})

	err = c.List(ctx, list)
	if err != nil {
		return fmt.Errorf("listing %s: %w", gvk.Kind, err)
	}

	switch n := len(list.Items); {
	case n == 0:
		return fmt.Errorf("%s: %w", gvk.Kind, ErrNoInstance)
	case n > 1:
		return fmt.Errorf("%s (found %d): %w", gvk.Kind, n, ErrMultipleInstances)
	}

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(
		list.Items[0].Object, target,
	)
	if err != nil {
		return fmt.Errorf("converting unstructured %s: %w", gvk.Kind, err)
	}

	return nil
}

func gvkForObject(obj runtime.Object, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("looking up object kinds for %T: %w: %w", obj, ErrUnregisteredType, err)
	}

	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no GVK registered for %T: %w", obj, ErrUnregisteredType)
	}

	return gvks[0], nil
}
