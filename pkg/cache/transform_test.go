package cache_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
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

func TestStripDataRemovesTypedPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		obj  metav1.Object
		name string
	}{
		{
			name: "ConfigMap",
			obj: &corev1.ConfigMap{
				Data:       map[string]string{"key": "value"},
				BinaryData: map[string][]byte{"bin": []byte("value")},
			},
		},
		{
			name: "Secret",
			obj: &corev1.Secret{
				Data: map[string][]byte{"tls.crt": []byte("cert")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			tt.obj.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "manager"}})
			tt.obj.SetAnnotations(map[string]string{
				cachepkg.AnnotationLastAppliedConfiguration: "{}",
				"keep": "value",
			})

			out, err := cachepkg.StripData()(tt.obj)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(out).Should(BeIdenticalTo(tt.obj))

			switch o := tt.obj.(type) {
			case *corev1.ConfigMap:
				g.Expect(o.Data).Should(BeNil())
				g.Expect(o.BinaryData).Should(BeNil())
			case *corev1.Secret:
				g.Expect(o.Data).Should(BeNil())
			}

			g.Expect(tt.obj.GetManagedFields()).Should(BeNil())
			g.Expect(tt.obj.GetAnnotations()).Should(Equal(map[string]string{"keep": "value"}))
		})
	}
}

func TestStripDataRemovesUnstructuredPayload(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"ConfigMap", "Secret"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			obj := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "v1",
				"kind":       kind,
				"metadata":   map[string]any{"name": "obj", "namespace": "default"},
				"data":       map[string]any{"key": "dmFsdWU="},
				"binaryData": map[string]any{"bin": "dmFsdWU="},
				"type":       "Opaque",
			}}
			obj.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "manager"}})

			out, err := cachepkg.StripData()(obj)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(out).Should(BeIdenticalTo(obj))

			g.Expect(obj.Object).ShouldNot(HaveKey("data"))
			g.Expect(obj.Object).ShouldNot(HaveKey("binaryData"))
			g.Expect(obj.Object).Should(HaveKeyWithValue("type", "Opaque"))
			g.Expect(obj.GetName()).Should(Equal("obj"))
			g.Expect(obj.GetManagedFields()).Should(BeNil())
		})
	}
}

func TestStripDataKeepsDataOfOtherKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		apiVersion string
		kind       string
	}{
		{name: "core kind without payload", apiVersion: "v1", kind: "Service"},
		{name: "ConfigMap kind in another group", apiVersion: "example.com/v1", kind: "ConfigMap"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			obj := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": tt.apiVersion,
				"kind":       tt.kind,
				"metadata":   map[string]any{"name": "obj"},
				"data":       map[string]any{"key": "value"},
			}}
			obj.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "manager"}})

			_, err := cachepkg.StripData()(obj)
			g.Expect(err).ShouldNot(HaveOccurred())

			g.Expect(obj.Object).Should(HaveKeyWithValue("data", map[string]any{"key": "value"}))
			g.Expect(obj.GetManagedFields()).Should(BeNil())
		})
	}
}

func TestStripDataPassesThroughNonKubernetesObjects(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := &nonKubernetesObject{value: "unchanged"}

	out, err := cachepkg.StripData()(obj)
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
