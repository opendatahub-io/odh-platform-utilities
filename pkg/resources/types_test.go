package resources_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

	. "github.com/onsi/gomega"
)

func TestCloneNilList(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	var list resources.UnstructuredList
	g.Expect(list.Clone()).Should(BeNil())
}

func TestCloneEmptyList(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	list := resources.UnstructuredList{}
	g.Expect(list.Clone()).Should(BeNil())
}

func TestCloneDeepCopy(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	original := resources.UnstructuredList{
		{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "cm-1"},
		}},
		{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]any{"name": "svc-1"},
		}},
	}

	cloned := original.Clone()

	g.Expect(cloned).Should(HaveLen(2))
	g.Expect(cloned[0].GetName()).Should(Equal("cm-1"))
	g.Expect(cloned[1].GetName()).Should(Equal("svc-1"))

	cloned[0].SetName("mutated")
	g.Expect(original[0].GetName()).Should(Equal("cm-1"))
}

func TestCloneIndependentObjects(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	original := resources.UnstructuredList{
		{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "orig"},
			"data":       map[string]any{"key": "value"},
		}},
	}

	cloned := original.Clone()

	clonedData, ok := cloned[0].Object["data"].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	clonedData["key"] = "changed"

	origData, ok := original[0].Object["data"].(map[string]any)
	g.Expect(ok).Should(BeTrue())
	g.Expect(origData["key"]).Should(Equal("value"))
}

func TestClonePreservesContent(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	list := resources.UnstructuredList{
		{Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "deploy-1",
				"namespace": "test-ns",
				"labels":    map[string]any{"app": "test"},
			},
		}},
	}

	cloned := list.Clone()

	g.Expect(cloned[0]).Should(Equal(unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "deploy-1",
				"namespace": "test-ns",
				"labels":    map[string]any{"app": "test"},
			},
		},
	}))
}
