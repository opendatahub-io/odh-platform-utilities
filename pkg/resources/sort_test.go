package resources_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

	. "github.com/onsi/gomega"
)

func TestSortByApplyOrderCRDsFirst(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	input := []unstructured.Unstructured{
		newUnstructuredWithGVK(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}),
		newUnstructuredWithGVK(schema.GroupVersionKind{
			Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition",
		}),
		newUnstructuredWithGVK(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}),
	}

	sorted, err := resources.SortByApplyOrder(t.Context(), input)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(sorted).Should(HaveLen(3))

	g.Expect(sorted[0].GetKind()).Should(Equal("Namespace"))
	g.Expect(sorted[1].GetKind()).Should(Equal("CustomResourceDefinition"))
	g.Expect(sorted[2].GetKind()).Should(Equal("Deployment"))
}

func TestSortByApplyOrderWebhooksLast(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	input := []unstructured.Unstructured{
		newUnstructuredWithGVK(schema.GroupVersionKind{
			Group: "admissionregistration.k8s.io", Version: "v1", Kind: "ValidatingWebhookConfiguration",
		}),
		newUnstructuredWithGVK(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}),
		newUnstructuredWithGVK(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}),
	}

	sorted, err := resources.SortByApplyOrder(t.Context(), input)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(sorted).Should(HaveLen(3))

	g.Expect(sorted[len(sorted)-1].GetKind()).Should(Equal("ValidatingWebhookConfiguration"))
}

func TestSortByApplyOrderEmpty(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	sorted, err := resources.SortByApplyOrder(t.Context(), nil)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(sorted).Should(BeEmpty())
}

func newUnstructuredWithGVK(gvk schema.GroupVersionKind) unstructured.Unstructured {
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName("test-" + gvk.Kind)

	return u
}
