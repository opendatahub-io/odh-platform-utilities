package resources_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

	. "github.com/onsi/gomega"
)

func TestHashDeterministic(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm-1", "namespace": "default"},
		"data":       map[string]any{"key": "value"},
	}}

	h1, err := resources.Hash(obj)
	g.Expect(err).ShouldNot(HaveOccurred())

	h2, err := resources.Hash(obj)
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(h1).Should(Equal(h2))
}

func TestHashIgnoresServerFields(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	base := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm-1"},
		"data":       map[string]any{"key": "value"},
	}}

	withServer := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":              "cm-1",
			"uid":               "abc-123",
			"resourceVersion":   "999",
			"deletionTimestamp": "2024-01-01T00:00:00Z",
			"managedFields":     []any{map[string]any{"manager": "test"}},
			"ownerReferences":   []any{map[string]any{"name": "owner"}},
		},
		"data":   map[string]any{"key": "value"},
		"status": map[string]any{"phase": "Active"},
	}}

	h1, err := resources.Hash(base)
	g.Expect(err).ShouldNot(HaveOccurred())

	h2, err := resources.Hash(withServer)
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(h1).Should(Equal(h2))
}

func TestHashDiffersOnContentChange(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj1 := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm-1"},
		"data":       map[string]any{"key": "value-a"},
	}}

	obj2 := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm-1"},
		"data":       map[string]any{"key": "value-b"},
	}}

	h1, err := resources.Hash(obj1)
	g.Expect(err).ShouldNot(HaveOccurred())

	h2, err := resources.Hash(obj2)
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(h1).ShouldNot(Equal(h2))
}

func TestHashDoesNotMutateInput(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":            "cm-1",
			"uid":             "test-uid",
			"resourceVersion": "42",
		},
	}}

	_, err := resources.Hash(obj)
	g.Expect(err).ShouldNot(HaveOccurred())

	meta, ok := obj.Object["metadata"].(map[string]any)
	g.Expect(ok).Should(BeTrue())
	g.Expect(meta).Should(HaveKey("uid"))
	g.Expect(meta).Should(HaveKey("resourceVersion"))
}

func TestEncodeToString(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	result := resources.EncodeToString([]byte("hello"))
	g.Expect(result).Should(HavePrefix("v"))
	g.Expect(len(result)).Should(BeNumerically(">", 1))
}

func TestStripServerMetadataNil(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	g.Expect(resources.StripServerMetadata(nil)).Should(BeNil())
}

func TestStripServerMetadataRemovesFields(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":              "cm-1",
			"uid":               "abc",
			"resourceVersion":   "1",
			"generation":        int64(3),
			"managedFields":     []any{},
			"creationTimestamp": "2024-01-01T00:00:00Z",
			"deletionTimestamp": "2024-01-02T00:00:00Z",
			"ownerReferences":   []any{},
		},
		"data":   map[string]any{"key": "value"},
		"status": map[string]any{"phase": "Active"},
	}}

	clean := resources.StripServerMetadata(obj)

	meta, ok := clean.Object["metadata"].(map[string]any)
	g.Expect(ok).Should(BeTrue())
	g.Expect(meta).Should(HaveKey("name"))
	g.Expect(meta).ShouldNot(HaveKey("uid"))
	g.Expect(meta).ShouldNot(HaveKey("resourceVersion"))
	g.Expect(meta).ShouldNot(HaveKey("generation"))
	g.Expect(meta).ShouldNot(HaveKey("managedFields"))
	g.Expect(meta).ShouldNot(HaveKey("creationTimestamp"))
	g.Expect(meta).ShouldNot(HaveKey("deletionTimestamp"))
	g.Expect(meta).ShouldNot(HaveKey("ownerReferences"))
	g.Expect(clean.Object).ShouldNot(HaveKey("status"))
	g.Expect(clean.Object).Should(HaveKey("data"))
}

func TestStripServerMetadataDoesNotMutateInput(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name": "cm-1",
			"uid":  "abc",
		},
	}}

	_ = resources.StripServerMetadata(obj)

	meta, ok := obj.Object["metadata"].(map[string]any)
	g.Expect(ok).Should(BeTrue())
	g.Expect(meta).Should(HaveKey("uid"))
}
