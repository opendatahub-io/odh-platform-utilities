package resources_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

	. "github.com/onsi/gomega"
)

func TestIsOwnedByTypeTrue(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm-1"},
	}}
	obj.SetOwnerReferences([]metav1.OwnerReference{
		{APIVersion: "apps/v1", Kind: "Deployment", Name: "owner"},
	})

	owned, err := resources.IsOwnedByType(obj, schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "Deployment",
	})
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(owned).Should(BeTrue())
}

func TestIsOwnedByTypeFalse(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm-1"},
	}}
	obj.SetOwnerReferences([]metav1.OwnerReference{
		{APIVersion: "apps/v1", Kind: "Deployment", Name: "owner"},
	})

	owned, err := resources.IsOwnedByType(obj, schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "Service",
	})
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(owned).Should(BeFalse())
}

func TestIsOwnedByTypeNoRefs(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "cm-1"},
	}}

	owned, err := resources.IsOwnedByType(obj, schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "Deployment",
	})
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(owned).Should(BeFalse())
}
