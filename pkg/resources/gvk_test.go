package resources_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

	. "github.com/onsi/gomega"
)

func TestKindForObjectWithKindSet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	cm := &corev1.ConfigMap{}
	cm.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})

	kind, err := resources.KindForObject(runtime.NewScheme(), cm)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(kind).Should(Equal("ConfigMap"))
}

func TestKindForObjectFromScheme(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	s := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(s)).Should(Succeed())

	cm := &corev1.ConfigMap{}

	kind, err := resources.KindForObject(s, cm)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(kind).Should(Equal("ConfigMap"))
}

func TestGetGroupVersionKindForObjectNil(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	_, err := resources.GetGroupVersionKindForObject(runtime.NewScheme(), nil)
	g.Expect(err).Should(HaveOccurred())
}

func TestGetGroupVersionKindForObjectAlreadySet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	cm := &corev1.ConfigMap{}
	expected := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	cm.SetGroupVersionKind(expected)

	gvk, err := resources.GetGroupVersionKindForObject(runtime.NewScheme(), cm)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(gvk).Should(Equal(expected))
}

func TestEnsureGroupVersionKind(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	s := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(s)).Should(Succeed())

	cm := &corev1.ConfigMap{}
	g.Expect(cm.GetObjectKind().GroupVersionKind().Kind).Should(BeEmpty())

	err := resources.EnsureGroupVersionKind(s, cm)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(cm.GetObjectKind().GroupVersionKind().Kind).Should(Equal("ConfigMap"))
}

func TestGvkToUnstructured(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	u := resources.GvkToUnstructured(gvk)

	g.Expect(u.GroupVersionKind()).Should(Equal(gvk))
	g.Expect(u.GetName()).Should(BeEmpty())
}

func TestGvkToPartial(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	p := resources.GvkToPartial(gvk)

	g.Expect(p.APIVersion).Should(Equal("apps/v1"))
	g.Expect(p.Kind).Should(Equal("Deployment"))
}

func TestObjectToUnstructured(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	s := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(s)).Should(Succeed())

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}

	u, err := resources.ObjectToUnstructured(s, cm)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(u.GetName()).Should(Equal("test-cm"))
	g.Expect(u.GetNamespace()).Should(Equal("default"))
	g.Expect(u.GroupVersionKind().Kind).Should(Equal("ConfigMap"))
}
