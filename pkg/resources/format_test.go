package resources_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

	. "github.com/onsi/gomega"
)

func TestNamespacedNameFromObject(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm-1", "namespace": "ns-1"},
	}}

	nn := resources.NamespacedNameFromObject(obj)
	g.Expect(nn).Should(Equal(types.NamespacedName{Namespace: "ns-1", Name: "cm-1"}))
}

func TestFormatNamespacedNameWithNamespace(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nn := types.NamespacedName{Namespace: "ns", Name: "obj"}
	g.Expect(resources.FormatNamespacedName(nn)).Should(Equal("ns/obj"))
}

func TestFormatNamespacedNameClusterScoped(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nn := types.NamespacedName{Name: "obj"}
	g.Expect(resources.FormatNamespacedName(nn)).Should(Equal("obj"))
}

func TestFormatUnstructuredNameNamespaced(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm-1", "namespace": "ns-1"},
	}}

	g.Expect(resources.FormatUnstructuredName(obj)).Should(Equal("ns-1/cm-1"))
}

func TestFormatUnstructuredNameClusterScoped(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   map[string]any{"name": "my-ns"},
	}}

	g.Expect(resources.FormatUnstructuredName(obj)).Should(Equal("my-ns"))
}

func TestFormatObjectReferenceNamespaced(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm-1", "namespace": "ns-1"},
	}}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})

	result := resources.FormatObjectReference(obj)
	g.Expect(result).Should(ContainSubstring("ConfigMap"))
	g.Expect(result).Should(ContainSubstring("ns-1/cm-1"))
}

func TestFormatObjectReferenceClusterScoped(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   map[string]any{"name": "my-ns"},
	}}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"})

	result := resources.FormatObjectReference(obj)
	g.Expect(result).Should(ContainSubstring("Namespace"))
	g.Expect(result).Should(ContainSubstring("my-ns"))
	g.Expect(result).ShouldNot(ContainSubstring("my-ns/"))
}
