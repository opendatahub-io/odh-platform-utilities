package resources

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// ErrNilObject is returned when a nil runtime.Object is passed.
var ErrNilObject = errors.New("nil object")

// KindForObject returns the Kind string for obj. If the object already has a
// Kind set via its ObjectKind, that value is returned directly. Otherwise the
// scheme is consulted.
func KindForObject(scheme *runtime.Scheme, obj runtime.Object) (string, error) {
	if obj.GetObjectKind().GroupVersionKind().Kind != "" {
		return obj.GetObjectKind().GroupVersionKind().Kind, nil
	}

	gvk, err := apiutil.GVKForObject(obj, scheme)
	if err != nil {
		return "", fmt.Errorf("failed to get GVK: %w", err)
	}

	return gvk.Kind, nil
}

// GetGroupVersionKindForObject returns the GVK for obj. If the object already
// carries Version and Kind, that value is returned directly. Otherwise the
// scheme is consulted.
func GetGroupVersionKindForObject(s *runtime.Scheme, obj runtime.Object) (schema.GroupVersionKind, error) {
	if obj == nil {
		return schema.GroupVersionKind{}, ErrNilObject
	}

	if obj.GetObjectKind().GroupVersionKind().Version != "" && obj.GetObjectKind().GroupVersionKind().Kind != "" {
		return obj.GetObjectKind().GroupVersionKind(), nil
	}

	gvk, err := apiutil.GVKForObject(obj, s)
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("failed to get GVK: %w", err)
	}

	return gvk, nil
}

// EnsureGroupVersionKind looks up the GVK for obj in the scheme and sets it on
// the object's ObjectKind. This is a no-op if the GVK is already set.
func EnsureGroupVersionKind(s *runtime.Scheme, obj client.Object) error {
	gvk, err := GetGroupVersionKindForObject(s, obj)
	if err != nil {
		return err
	}

	obj.GetObjectKind().SetGroupVersionKind(gvk)

	return nil
}

// GvkToUnstructured creates an empty Unstructured object with the given GVK.
// This is useful for building lookup keys or list templates.
func GvkToUnstructured(gvk schema.GroupVersionKind) *unstructured.Unstructured {
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)

	return &u
}

// GvkToPartial creates a PartialObjectMetadata with the given GVK set as its
// TypeMeta. Useful for metadata-only list/watch operations.
func GvkToPartial(gvk schema.GroupVersionKind) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
		},
	}
}

// ObjectToUnstructured ensures the GVK is set on obj via the scheme and then
// converts it to an *unstructured.Unstructured.
func ObjectToUnstructured(s *runtime.Scheme, obj client.Object) (*unstructured.Unstructured, error) {
	err := EnsureGroupVersionKind(s, obj)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure GroupVersionKind: %w", err)
	}

	u, err := ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	return u, nil
}
