package cluster_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"

	. "github.com/onsi/gomega"
)

// --- WithLabels ---

func TestWithLabelsOnEmptyObject(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	err := cluster.ApplyMetaOptions(obj, cluster.WithLabels("app", "test"))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetLabels()).Should(Equal(map[string]string{"app": "test"}))
}

func TestWithLabelsMerges(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	obj.SetLabels(map[string]string{"existing": "val"})

	err := cluster.ApplyMetaOptions(obj, cluster.WithLabels("new", "val"))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetLabels()).Should(Equal(map[string]string{
		"existing": "val",
		"new":      "val",
	}))
}

func TestWithLabelsMultiplePairs(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	err := cluster.ApplyMetaOptions(obj, cluster.WithLabels("a", "1", "b", "2"))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetLabels()).Should(Equal(map[string]string{"a": "1", "b": "2"}))
}

func TestWithLabelsOddPairsReturnsError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	err := cluster.ApplyMetaOptions(obj, cluster.WithLabels("key-only"))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("expected even number of key-value pairs"))
}

// --- WithAnnotations ---

func TestWithAnnotationsOnEmptyObject(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	err := cluster.ApplyMetaOptions(obj, cluster.WithAnnotations("note", "hello"))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetAnnotations()).Should(Equal(map[string]string{"note": "hello"}))
}

func TestWithAnnotationsMerges(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	obj.SetAnnotations(map[string]string{"old": "ann"})

	err := cluster.ApplyMetaOptions(obj, cluster.WithAnnotations("new", "ann"))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetAnnotations()).Should(Equal(map[string]string{
		"old": "ann",
		"new": "ann",
	}))
}

func TestWithAnnotationsOddPairsReturnsError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	err := cluster.ApplyMetaOptions(obj, cluster.WithAnnotations("orphan"))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("expected even number"))
}

// --- WithOwnerReference ---

func TestWithOwnerReferenceAppends(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	ref, err := cluster.OwnerRefRaw("v1", "ConfigMap", "my-cm", "uid-1", false)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = cluster.ApplyMetaOptions(obj, cluster.WithOwnerReference(ref))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetOwnerReferences()).Should(HaveLen(1))
	g.Expect(obj.GetOwnerReferences()[0].Name).Should(Equal("my-cm"))
}

func TestWithOwnerReferenceRejectsDifferentControllerDuplicate(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	obj.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Name:       "existing-controller",
			UID:        "uid-existing",
			Controller: ptr.To(true),
		},
	})

	ref, err := cluster.OwnerRefRaw("v1", "Secret", "my-secret", "uid-new", true)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = cluster.ApplyMetaOptions(obj, cluster.WithOwnerReference(ref))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("already has a controller owner reference"))
}

func TestWithOwnerReferenceIdempotentSameUID(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	ref, err := cluster.OwnerRefRaw("v1", "ConfigMap", "my-cm", "uid-1", false)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = cluster.ApplyMetaOptions(obj, cluster.WithOwnerReference(ref))
	g.Expect(err).ShouldNot(HaveOccurred())
	err = cluster.ApplyMetaOptions(obj, cluster.WithOwnerReference(ref))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetOwnerReferences()).Should(HaveLen(1))
}

func TestWithOwnerReferenceControllerReapplySameUID(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	ref, err := cluster.OwnerRefRaw("v1", "ConfigMap", "my-cm", "uid-1", true)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = cluster.ApplyMetaOptions(obj, cluster.WithOwnerReference(ref))
	g.Expect(err).ShouldNot(HaveOccurred())
	err = cluster.ApplyMetaOptions(obj, cluster.WithOwnerReference(ref))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetOwnerReferences()).Should(HaveLen(1))
}

func TestWithOwnerReferenceAllowsNonController(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	obj.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Name:       "existing-controller",
			UID:        "uid-existing",
			Controller: ptr.To(true),
		},
	})

	ref, err := cluster.OwnerRefRaw("v1", "Secret", "my-secret", "uid-new", false)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = cluster.ApplyMetaOptions(obj, cluster.WithOwnerReference(ref))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetOwnerReferences()).Should(HaveLen(2))
}

// --- OwnedBy ---

func TestOwnedByAddsNonControllerRef(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := newOwner("apps/v1", "Deployment", "my-deploy", "uid-2")

	err := cluster.ApplyMetaOptions(obj, cluster.OwnedBy(owner))
	g.Expect(err).ShouldNot(HaveOccurred())

	refs := obj.GetOwnerReferences()
	g.Expect(refs).Should(HaveLen(1))
	g.Expect(refs[0].Kind).Should(Equal("Deployment"))
	g.Expect(*refs[0].Controller).Should(BeFalse())
}

func TestOwnedByIdempotentSameOwner(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := newOwner("apps/v1", "Deployment", "my-deploy", "uid-2")

	err := cluster.ApplyMetaOptions(obj, cluster.OwnedBy(owner))
	g.Expect(err).ShouldNot(HaveOccurred())
	err = cluster.ApplyMetaOptions(obj, cluster.OwnedBy(owner))
	g.Expect(err).ShouldNot(HaveOccurred())

	g.Expect(obj.GetOwnerReferences()).Should(HaveLen(1))
}

func TestOwnedByErrorsWithoutKind(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "no-kind"},
		},
	}

	err := cluster.ApplyMetaOptions(obj, cluster.OwnedBy(owner))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no Kind set"))
}

// --- ControlledBy ---

func TestControlledByAddsControllerRef(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := newOwner("v1", "Service", "my-svc", "uid-3")

	err := cluster.ApplyMetaOptions(obj, cluster.ControlledBy(owner))
	g.Expect(err).ShouldNot(HaveOccurred())

	refs := obj.GetOwnerReferences()
	g.Expect(refs).Should(HaveLen(1))
	g.Expect(refs[0].Kind).Should(Equal("Service"))
	g.Expect(*refs[0].Controller).Should(BeTrue())
}

func TestControlledByErrorsWithoutKind(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "no-kind"},
		},
	}

	err := cluster.ApplyMetaOptions(obj, cluster.ControlledBy(owner))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no Kind set"))
}

func TestControlledByRejectsDifferentController(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	obj.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Name:       "existing-controller",
			UID:        "uid-existing",
			Controller: ptr.To(true),
		},
	})

	owner := newOwner("v1", "Service", "my-svc", "uid-3")
	err := cluster.ApplyMetaOptions(obj, cluster.ControlledBy(owner))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("already has a controller owner reference"))
}

func TestControlledByIdempotentSameOwner(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	owner := newOwner("v1", "Service", "my-svc", "uid-3")

	err := cluster.ApplyMetaOptions(obj, cluster.ControlledBy(owner))
	g.Expect(err).ShouldNot(HaveOccurred())
	err = cluster.ApplyMetaOptions(obj, cluster.ControlledBy(owner))
	g.Expect(err).ShouldNot(HaveOccurred())

	refs := obj.GetOwnerReferences()
	g.Expect(refs).Should(HaveLen(1))
	g.Expect(*refs[0].Controller).Should(BeTrue())
}

func TestControlledByAllowsAfterNonControllerRef(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	obj.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Name:       "non-controller",
			UID:        "uid-existing",
			Controller: ptr.To(false),
		},
	})

	owner := newOwner("v1", "Service", "my-svc", "uid-3")
	err := cluster.ApplyMetaOptions(obj, cluster.ControlledBy(owner))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetOwnerReferences()).Should(HaveLen(2))
}

// --- InNamespace ---

func TestInNamespace(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	err := cluster.ApplyMetaOptions(obj, cluster.InNamespace("target-ns"))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetNamespace()).Should(Equal("target-ns"))
}

// --- ApplyMetaOptions chaining ---

func TestApplyMetaOptionsChaining(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	err := cluster.ApplyMetaOptions(obj,
		cluster.WithLabels("app", "test"),
		cluster.WithAnnotations("note", "hi"),
		cluster.InNamespace("ns"),
	)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(obj.GetLabels()).Should(HaveKeyWithValue("app", "test"))
	g.Expect(obj.GetAnnotations()).Should(HaveKeyWithValue("note", "hi"))
	g.Expect(obj.GetNamespace()).Should(Equal("ns"))
}

func TestApplyMetaOptionsStopsOnError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := newObj()
	err := cluster.ApplyMetaOptions(obj,
		cluster.WithLabels("odd-number"),
		cluster.InNamespace("should-not-reach"),
	)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(obj.GetNamespace()).Should(BeEmpty())
}

// --- ExtractKeyValues ---

func TestExtractKeyValuesOddCount(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	_, err := cluster.ExtractKeyValues([]string{"a", "b", "c"})
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("expected even number of key-value pairs: got 3"))
}

func TestExtractKeyValuesEmpty(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	kv, err := cluster.ExtractKeyValues([]string{})
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(kv).Should(BeEmpty())
}

func TestExtractKeyValuesValid(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	kv, err := cluster.ExtractKeyValues([]string{"k1", "v1", "k2", "v2"})
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(kv).Should(Equal(map[string]string{"k1": "v1", "k2": "v2"}))
}

// --- OwnerRefFrom ---

func TestOwnerRefFromBuildsControllerRef(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	owner := newOwner("apps/v1", "Deployment", "my-deploy", "uid-10")
	ref, err := cluster.OwnerRefFrom(owner, true)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(ref.APIVersion).Should(Equal("apps/v1"))
	g.Expect(ref.Kind).Should(Equal("Deployment"))
	g.Expect(ref.Name).Should(Equal("my-deploy"))
	g.Expect(ref.UID).Should(Equal(types.UID("uid-10")))
	g.Expect(*ref.Controller).Should(BeTrue())
	g.Expect(*ref.BlockOwnerDeletion).Should(BeTrue())
}

func TestOwnerRefFromBuildsNonControllerRef(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	owner := newOwner("v1", "Service", "my-svc", "uid-11")
	ref, err := cluster.OwnerRefFrom(owner, false)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(*ref.Controller).Should(BeFalse())
	g.Expect(*ref.BlockOwnerDeletion).Should(BeTrue())
}

func TestOwnerRefFromErrorsWithoutKind(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	owner := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "no-kind", "uid": "some-uid"},
		},
	}

	_, err := cluster.OwnerRefFrom(owner, false)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no Kind set"))
}

func TestOwnerRefFromErrorsWithNilOwner(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	_, err := cluster.OwnerRefFrom(nil, false)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("owner is nil"))
}

func TestOwnerRefFromErrorsWithEmptyName(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	owner := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"uid": "some-uid"},
		},
	}
	owner.SetGroupVersionKind(schema.FromAPIVersionAndKind("v1", "ConfigMap"))

	_, err := cluster.OwnerRefFrom(owner, false)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no Name set"))
}

func TestOwnerRefFromErrorsWithEmptyUID(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	owner := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "my-cm"},
		},
	}
	owner.SetGroupVersionKind(schema.FromAPIVersionAndKind("v1", "ConfigMap"))

	_, err := cluster.OwnerRefFrom(owner, false)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no UID set"))
}

// --- OwnerRefRaw ---

func TestOwnerRefRawBuildsRef(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	ref, err := cluster.OwnerRefRaw("v1", "Secret", "my-secret", "uid-20", true)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(ref.APIVersion).Should(Equal("v1"))
	g.Expect(ref.Kind).Should(Equal("Secret"))
	g.Expect(ref.Name).Should(Equal("my-secret"))
	g.Expect(ref.UID).Should(Equal(types.UID("uid-20")))
	g.Expect(*ref.Controller).Should(BeTrue())
	g.Expect(*ref.BlockOwnerDeletion).Should(BeTrue())
}

func TestOwnerRefRawNonController(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	ref, err := cluster.OwnerRefRaw("v1", "ConfigMap", "cm", "uid-21", false)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(*ref.Controller).Should(BeFalse())
	g.Expect(*ref.BlockOwnerDeletion).Should(BeTrue())
}

func TestOwnerRefRawErrorsWithEmptyKind(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	_, err := cluster.OwnerRefRaw("v1", "", "my-cm", "uid-22", false)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no Kind set"))
}

func TestOwnerRefRawErrorsWithEmptyName(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	_, err := cluster.OwnerRefRaw("v1", "ConfigMap", "", "uid-23", false)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no Name set"))
}

func TestOwnerRefRawErrorsWithEmptyAPIVersion(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	_, err := cluster.OwnerRefRaw("", "ConfigMap", "my-cm", "uid-24", false)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no APIVersion set"))
}

func TestOwnerRefRawErrorsWithEmptyUID(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	_, err := cluster.OwnerRefRaw("v1", "ConfigMap", "my-cm", "", false)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("no UID set"))
}

// --- helpers ---

func newObj() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "test-obj"},
		},
	}
}

func newOwner(apiVersion, kind, name string, uid types.UID) *unstructured.Unstructured {
	u := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]any{
				"name": name,
				"uid":  string(uid),
			},
		},
	}
	u.SetGroupVersionKind(schema.FromAPIVersionAndKind(apiVersion, kind))
	u.SetUID(uid)

	return u
}
