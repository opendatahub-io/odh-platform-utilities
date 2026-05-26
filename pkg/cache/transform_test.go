package cache_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	cachepkg "github.com/opendatahub-io/odh-platform-utilities/pkg/cache"

	. "github.com/onsi/gomega"
)

func TestStripUnusedFieldsRemovesManagedFieldsAndLastAppliedAnnotation(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newTransformObject(map[string]string{
		cachepkg.AnnotationLastAppliedConfiguration: `{"kind":"ConfigMap"}`,
		"keep": "value",
	})

	transform := cachepkg.StripUnusedFields()
	out, err := transform(obj)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(out).Should(BeIdenticalTo(obj))

	g.Expect(obj.GetManagedFields()).Should(BeNil())
	g.Expect(obj.GetAnnotations()).ShouldNot(HaveKey(cachepkg.AnnotationLastAppliedConfiguration))
	g.Expect(obj.GetAnnotations()).Should(HaveKeyWithValue("keep", "value"))
}

func TestStripUnusedFieldsPreservesObjectsWithoutAnnotations(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newTransformObject(nil)

	transform := cachepkg.StripUnusedFields()
	out, err := transform(obj)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(out).Should(BeIdenticalTo(obj))

	g.Expect(obj.GetManagedFields()).Should(BeNil())
	g.Expect(obj.GetAnnotations()).Should(BeNil())
}

func TestStripUnusedFieldsLeavesOtherAnnotationsIntact(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newTransformObject(map[string]string{
		"first":  "value-1",
		"second": "value-2",
	})

	transform := cachepkg.StripUnusedFields()
	_, err := transform(obj)
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(obj.GetAnnotations()).Should(HaveLen(2))
	g.Expect(obj.GetAnnotations()).Should(HaveKeyWithValue("first", "value-1"))
	g.Expect(obj.GetAnnotations()).Should(HaveKeyWithValue("second", "value-2"))
}

func TestStripUnusedFieldsPassesThroughNonKubernetesObjects(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &nonKubernetesObject{value: "unchanged"}

	transform := cachepkg.StripUnusedFields()
	out, err := transform(obj)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(out).Should(BeIdenticalTo(obj))
	g.Expect(obj.value).Should(Equal("unchanged"))
}

func newTransformObject(annotations map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "cm-1",
			"namespace": "default",
		},
	}}

	obj.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "manager"}})
	obj.SetAnnotations(annotations)

	return obj
}

type nonKubernetesObject struct {
	value string
}
