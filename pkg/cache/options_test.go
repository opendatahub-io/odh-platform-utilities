package cache_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	cachepkg "github.com/opendatahub-io/odh-platform-utilities/pkg/cache"

	. "github.com/onsi/gomega"
)

func TestNewOptionsDefaults(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	opts := cachepkg.NewOptions()

	g.Expect(opts.ReaderFailOnMissingInformer).Should(BeFalse())
	g.Expect(opts.ByObject).Should(BeNil())
	g.Expect(opts.DefaultNamespaces).Should(BeNil())
	g.Expect(opts.DefaultTransform).ShouldNot(BeNil())

	// Default transform strips managedFields and last-applied-configuration.
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm"},
		"data":       map[string]any{"key": "value"},
	}}
	obj.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "manager"}})
	obj.SetAnnotations(map[string]string{cachepkg.AnnotationLastAppliedConfiguration: "{}"})

	_, err := opts.DefaultTransform(obj)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetManagedFields()).Should(BeNil())
	g.Expect(obj.GetAnnotations()).ShouldNot(HaveKey(cachepkg.AnnotationLastAppliedConfiguration))
	g.Expect(obj.Object).Should(HaveKeyWithValue("data", map[string]any{"key": "value"}))
}

func TestNewOptionsWithStrictInformers(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	opts := cachepkg.NewOptions(cachepkg.WithStrictInformers())

	g.Expect(opts.ReaderFailOnMissingInformer).Should(BeTrue())
	g.Expect(opts.DefaultTransform).ShouldNot(BeNil())
}
