package resources

import (
	"maps"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetLabels merges the given label key-value pairs into the existing labels on
// obj. Existing labels with the same key are overwritten.
func SetLabels(obj client.Object, values map[string]string) {
	if len(values) == 0 {
		return
	}

	target := obj.GetLabels()
	if target == nil {
		target = make(map[string]string)
	}

	maps.Copy(target, values)

	obj.SetLabels(target)
}

// SetLabel sets a single label on obj.
func SetLabel(obj client.Object, key, value string) {
	SetLabels(obj, map[string]string{key: value})
}

// GetLabel returns the value of the label with the given key, or "" if not
// present.
func GetLabel(obj client.Object, key string) string {
	l := obj.GetLabels()
	if l == nil {
		return ""
	}

	return l[key]
}

// HasLabel returns true if obj carries a label with the given key.
func HasLabel(obj client.Object, key string) bool {
	_, ok := obj.GetLabels()[key]
	return ok
}

// RemoveLabel removes a label by key. It is a no-op if the label is absent.
func RemoveLabel(obj client.Object, key string) {
	l := obj.GetLabels()
	if l == nil {
		return
	}

	delete(l, key)
	obj.SetLabels(l)
}

// SetAnnotations merges the given annotation key-value pairs into the existing
// annotations on obj. Existing annotations with the same key are overwritten.
func SetAnnotations(obj client.Object, values map[string]string) {
	if len(values) == 0 {
		return
	}

	target := obj.GetAnnotations()
	if target == nil {
		target = make(map[string]string)
	}

	maps.Copy(target, values)

	obj.SetAnnotations(target)
}

// SetAnnotation sets a single annotation on obj.
func SetAnnotation(obj client.Object, key, value string) {
	SetAnnotations(obj, map[string]string{key: value})
}

// GetAnnotation returns the value of the annotation with the given key, or ""
// if not present.
func GetAnnotation(obj client.Object, key string) string {
	a := obj.GetAnnotations()
	if a == nil {
		return ""
	}

	return a[key]
}

// HasAnnotation returns true if obj carries an annotation with the given key.
func HasAnnotation(obj client.Object, key string) bool {
	_, ok := obj.GetAnnotations()[key]
	return ok
}

// RemoveAnnotation removes an annotation by key. It is a no-op if the
// annotation is absent.
func RemoveAnnotation(obj client.Object, key string) {
	a := obj.GetAnnotations()
	if a == nil {
		return
	}

	delete(a, key)
	obj.SetAnnotations(a)
}
