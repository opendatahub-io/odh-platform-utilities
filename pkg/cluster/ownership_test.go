package cluster_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

	. "github.com/onsi/gomega"
)

// --- WithDynamicOwner ---

func TestWithDynamicOwnerSetsLabelsAndAnnotations(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := newOwner("example.com/v1", "Dashboard", "my-dashboard", "uid-100")
	owner.SetGeneration(5)

	err := cluster.ApplyMetaOptions(obj, cluster.WithDynamicOwner(owner))
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(resources.GetLabel(obj, labels.PlatformPartOf)).Should(Equal("dashboard"))
	g.Expect(resources.GetAnnotation(obj, annotations.InstanceName)).Should(Equal("my-dashboard"))
	g.Expect(resources.GetAnnotation(obj, annotations.InstanceNamespace)).Should(BeEmpty())
	g.Expect(resources.GetAnnotation(obj, annotations.InstanceUID)).Should(Equal("uid-100"))
	g.Expect(resources.GetAnnotation(obj, annotations.InstanceGeneration)).Should(Equal("5"))
}

func TestWithDynamicOwnerNamespacedOwner(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := newOwner("example.com/v1", "Widget", "my-widget", "uid-200")
	owner.SetNamespace("widget-ns")
	owner.SetGeneration(3)

	err := cluster.ApplyMetaOptions(obj, cluster.WithDynamicOwner(owner))
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(resources.GetAnnotation(obj, annotations.InstanceNamespace)).Should(Equal("widget-ns"))
	g.Expect(resources.GetAnnotation(obj, annotations.InstanceName)).Should(Equal("my-widget"))
}

func TestWithDynamicOwnerNormalizesKind(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := newOwner("example.com/v1", "DataScienceCluster", "dsc", "uid-300")

	err := cluster.ApplyMetaOptions(obj, cluster.WithDynamicOwner(owner))
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(resources.GetLabel(obj, labels.PlatformPartOf)).Should(Equal("datasciencecluster"))
}

func TestWithDynamicOwnerIdempotent(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := newOwner("example.com/v1", "Dashboard", "my-dashboard", "uid-100")
	owner.SetGeneration(1)

	err := cluster.ApplyMetaOptions(obj, cluster.WithDynamicOwner(owner))
	g.Expect(err).ShouldNot(HaveOccurred())
	err = cluster.ApplyMetaOptions(obj, cluster.WithDynamicOwner(owner))
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(resources.GetLabel(obj, labels.PlatformPartOf)).Should(Equal("dashboard"))
	g.Expect(resources.GetAnnotation(obj, annotations.InstanceName)).Should(Equal("my-dashboard"))
}

func TestWithDynamicOwnerPreservesExistingLabels(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	obj.SetLabels(map[string]string{"existing": "label"})

	owner := newOwner("example.com/v1", "Dashboard", "my-dashboard", "uid-100")
	err := cluster.ApplyMetaOptions(obj, cluster.WithDynamicOwner(owner))
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(obj.GetLabels()).Should(HaveKeyWithValue("existing", "label"))
	g.Expect(obj.GetLabels()).Should(HaveKeyWithValue(labels.PlatformPartOf, "dashboard"))
}

func TestWithDynamicOwnerErrorsNilOwner(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	err := cluster.ApplyMetaOptions(obj, cluster.WithDynamicOwner(nil))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("owner is nil"))
}

func TestWithDynamicOwnerErrorsMissingKind(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name": "no-kind",
				"uid":  "some-uid",
			},
		},
	}

	err := cluster.ApplyMetaOptions(obj, cluster.WithDynamicOwner(owner))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no Kind set"))
}

func TestWithDynamicOwnerErrorsMissingName(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"uid": "some-uid"},
		},
	}
	owner.SetGroupVersionKind(schema.FromAPIVersionAndKind("v1", "ConfigMap"))

	err := cluster.ApplyMetaOptions(obj, cluster.WithDynamicOwner(owner))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no Name set"))
}

func TestWithDynamicOwnerErrorsInvalidKindForLabel(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := newOwner("example.com/v1", ".InvalidKind", "my-cr", "uid-400")

	err := cluster.ApplyMetaOptions(obj, cluster.WithDynamicOwner(owner))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("normalizing owner kind"))
}

func TestWithDynamicOwnerErrorsMissingUID(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "my-cm"},
		},
	}
	owner.SetGroupVersionKind(schema.FromAPIVersionAndKind("v1", "ConfigMap"))

	err := cluster.ApplyMetaOptions(obj, cluster.WithDynamicOwner(owner))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no UID set"))
}

// --- EnqueueOwner ---

func TestEnqueueOwnerReturnsRequestFromAnnotations(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	resources.SetAnnotation(obj, annotations.InstanceName, "my-cr")
	resources.SetAnnotation(obj, annotations.InstanceNamespace, "my-ns")

	mapFn := cluster.EnqueueOwner()
	reqs := mapFn(context.Background(), obj)
	g.Expect(reqs).Should(HaveLen(1))
	g.Expect(reqs[0].Name).Should(Equal("my-cr"))
	g.Expect(reqs[0].Namespace).Should(Equal("my-ns"))
}

func TestEnqueueOwnerClusterScopedOwner(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	resources.SetAnnotation(obj, annotations.InstanceName, "singleton-cr")

	mapFn := cluster.EnqueueOwner()
	reqs := mapFn(context.Background(), obj)
	g.Expect(reqs).Should(HaveLen(1))
	g.Expect(reqs[0].Name).Should(Equal("singleton-cr"))
	g.Expect(reqs[0].Namespace).Should(BeEmpty())
}

func TestEnqueueOwnerMissingAnnotationReturnsNil(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	mapFn := cluster.EnqueueOwner()
	reqs := mapFn(context.Background(), obj)
	g.Expect(reqs).Should(BeNil())
}

func TestEnqueueOwnerRoundTripsWithDynamicOwner(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	owner := newOwner("example.com/v1", "Dashboard", "my-dashboard", "uid-rt")
	owner.SetNamespace("dashboard-ns")
	owner.SetGeneration(7)

	child := newObj()
	err := cluster.ApplyMetaOptions(child, cluster.WithDynamicOwner(owner))
	g.Expect(err).ShouldNot(HaveOccurred())

	mapFn := cluster.EnqueueOwner()
	reqs := mapFn(context.Background(), child)
	g.Expect(reqs).Should(HaveLen(1))
	g.Expect(reqs[0].Name).Should(Equal("my-dashboard"))
	g.Expect(reqs[0].Namespace).Should(Equal("dashboard-ns"))
}
