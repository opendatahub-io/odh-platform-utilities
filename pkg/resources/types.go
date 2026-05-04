package resources

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Resource represents a Kubernetes API resource type with convenient methods
// for accessing common properties. It wraps meta.RESTMapping to provide a
// more intuitive interface for discovery and authorization workflows.
type Resource struct {
	meta.RESTMapping
}

// GroupVersionResource returns the GVR associated with this Resource.
func (r Resource) GroupVersionResource() schema.GroupVersionResource {
	return r.Resource
}

// GroupVersionKind returns the GVK associated with this Resource.
func (r Resource) GroupVersionKind() schema.GroupVersionKind {
	return r.RESTMapping.GroupVersionKind
}

// String returns a human-readable representation including both GVR and GVK.
func (r Resource) String() string {
	gv := r.Resource.Version

	if len(r.Resource.Group) > 0 {
		gv = r.Resource.Group + "/" + r.Resource.Version
	}

	return strings.Join(
		[]string{
			gv, "Resource=", r.Resource.Resource, "Kind=", r.RESTMapping.GroupVersionKind.Kind,
		},
		" ",
	)
}

// IsNamespaced returns true if this Resource exists within a Kubernetes namespace.
func (r Resource) IsNamespaced() bool {
	if r.Scope == nil {
		return false
	}

	return r.Scope.Name() == meta.RESTScopeNameNamespace
}

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
