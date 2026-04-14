package resources

import (
	"maps"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetLabels merges the given label key-value pairs into the existing labels on
// obj. Existing labels with the same key are overwritten.
func SetLabels(obj client.Object, values map[string]string) {
	target := obj.GetLabels()
	if target == nil {
		target = make(map[string]string)
	}

	maps.Copy(target, values)

	obj.SetLabels(target)
}

// SetAnnotations merges the given annotation key-value pairs into the existing
// annotations on obj. Existing annotations with the same key are overwritten.
func SetAnnotations(obj client.Object, values map[string]string) {
	target := obj.GetAnnotations()
	if target == nil {
		target = make(map[string]string)
	}

	maps.Copy(target, values)

	obj.SetAnnotations(target)
}
