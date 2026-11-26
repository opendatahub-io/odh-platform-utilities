package cache

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	toolscache "k8s.io/client-go/tools/cache"
)

const (
	// AnnotationLastAppliedConfiguration is the annotation added by kubectl apply
	// that is unnecessary when using server-side apply.
	AnnotationLastAppliedConfiguration = "kubectl.kubernetes.io/last-applied-configuration"
)

// StripUnusedFields returns a cache transform that removes managedFields
// and the last-applied-configuration annotation from cached objects.
// Both are unnecessary when using server-side apply.
func StripUnusedFields() toolscache.TransformFunc {
	return func(obj any) (any, error) {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			// non-k8s objects pass through unchanged
			//nolint:nilerr
			return obj, nil
		}

		accessor.SetManagedFields(nil)

		annotations := accessor.GetAnnotations()
		if annotations != nil {
			delete(annotations, AnnotationLastAppliedConfiguration)
			accessor.SetAnnotations(annotations)
		}

		return obj, nil
	}
}

// StripData returns a cache transform that applies StripUnusedFields and also
// removes .data and .binaryData from ConfigMaps and Secrets. Use it per type
// (WithTransform) only for objects whose payload is never read from the cache.
//
// WARNING: an Update built from a stripped object erases the live payload.
func StripData() toolscache.TransformFunc {
	stripUnused := StripUnusedFields()

	return func(obj any) (any, error) {
		switch o := obj.(type) {
		case *corev1.ConfigMap:
			o.Data = nil
			o.BinaryData = nil
		case *corev1.Secret:
			o.Data = nil
		case *unstructured.Unstructured:
			if isConfigMapOrSecret(o) {
				unstructured.RemoveNestedField(o.Object, "data")
				unstructured.RemoveNestedField(o.Object, "binaryData")
			}
		}

		return stripUnused(obj)
	}
}

func isConfigMapOrSecret(u *unstructured.Unstructured) bool {
	gvk := u.GroupVersionKind()

	return gvk == corev1.SchemeGroupVersion.WithKind("ConfigMap") ||
		gvk == corev1.SchemeGroupVersion.WithKind("Secret")
}
