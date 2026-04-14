package resources

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// UnstructuredList is a slice of unstructured Kubernetes resources with
// convenience methods for deep-copying.
type UnstructuredList []unstructured.Unstructured

// Clone returns a deep copy of every element in the list. Returns nil for
// an empty or nil list.
func (l UnstructuredList) Clone() []unstructured.Unstructured {
	if len(l) == 0 {
		return nil
	}

	result := make([]unstructured.Unstructured, len(l))

	for i := range l {
		result[i] = *l[i].DeepCopy()
	}

	return result
}
