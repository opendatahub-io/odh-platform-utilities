package resources_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

	. "github.com/onsi/gomega"
)

func TestSetLabelsNoExisting(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "test"},
		},
	}

	resources.SetLabels(obj, map[string]string{"app": "myapp", "env": "dev"})

	g.Expect(obj.GetLabels()).Should(Equal(map[string]string{
		"app": "myapp",
		"env": "dev",
	}))
}

func TestSetLabelsMerge(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":   "test",
				"labels": map[string]any{"existing": "label"},
			},
		},
	}

	resources.SetLabels(obj, map[string]string{"new": "label"})

	g.Expect(obj.GetLabels()).Should(Equal(map[string]string{
		"existing": "label",
		"new":      "label",
	}))
}

func TestSetLabelsOverwrite(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":   "test",
				"labels": map[string]any{"key": "old"},
			},
		},
	}

	resources.SetLabels(obj, map[string]string{"key": "new"})

	g.Expect(obj.GetLabels()).Should(Equal(map[string]string{
		"key": "new",
	}))
}

func TestSetAnnotationsNoExisting(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "test"},
		},
	}

	resources.SetAnnotations(obj, map[string]string{"note": "value"})

	g.Expect(obj.GetAnnotations()).Should(Equal(map[string]string{
		"note": "value",
	}))
}

func TestSetAnnotationsMerge(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":        "test",
				"annotations": map[string]any{"existing": "ann"},
			},
		},
	}

	resources.SetAnnotations(obj, map[string]string{"new": "ann"})

	g.Expect(obj.GetAnnotations()).Should(Equal(map[string]string{
		"existing": "ann",
		"new":      "ann",
	}))
}

func TestSetAnnotationsOverwrite(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":        "test",
				"annotations": map[string]any{"key": "old"},
			},
		},
	}

	resources.SetAnnotations(obj, map[string]string{"key": "new"})

	g.Expect(obj.GetAnnotations()).Should(Equal(map[string]string{
		"key": "new",
	}))
}
