package resources

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NamespacedNameFromObject returns the namespace/name key for obj.
func NamespacedNameFromObject(obj client.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

// FormatNamespacedName returns the string representation of nn. If the
// namespace is empty, only the name is returned.
func FormatNamespacedName(nn types.NamespacedName) string {
	if nn.Namespace == "" {
		return nn.Name
	}

	return nn.String()
}

// FormatUnstructuredName returns "namespace/name" for namespaced objects or
// just "name" for cluster-scoped objects.
func FormatUnstructuredName(obj *unstructured.Unstructured) string {
	if obj.GetNamespace() == "" {
		return obj.GetName()
	}

	return obj.GetNamespace() + string(types.Separator) + obj.GetName()
}

// FormatObjectReference returns a human-readable string identifying an
// unstructured resource by its GVK and namespace/name (e.g.
// "/v1, Kind=ConfigMap default/my-cm").
func FormatObjectReference(u *unstructured.Unstructured) string {
	gvk := u.GroupVersionKind().String()
	name := u.GetName()
	ns := u.GetNamespace()

	if ns != "" {
		return gvk + " " + ns + "/" + name
	}

	return gvk + " " + name
}
