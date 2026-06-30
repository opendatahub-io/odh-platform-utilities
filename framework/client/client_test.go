package client_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	opclient "github.com/opendatahub-io/odh-platform-utilities/framework/client"

	. "github.com/onsi/gomega"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()

	g := NewWithT(t)
	g.Expect(corev1.AddToScheme(s)).Should(Succeed())

	return s
}

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()

	return fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(objs...).
		Build()
}

func TestClient_ImplementsClientInterface(t *testing.T) {
	var _ client.Client = (*opclient.Client)(nil)
}

func TestGet_TypedObject_ReturnsTypedResult(t *testing.T) {
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}

	inner := newFakeClient(t, cm)
	c := opclient.New(inner)

	result := &corev1.ConfigMap{}
	err := c.Get(context.Background(), client.ObjectKeyFromObject(cm), result)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Name).Should(Equal("test"))
	g.Expect(result.Data).Should(HaveKeyWithValue("key", "value"))
}

func TestGet_UnstructuredObject_PassesThrough(t *testing.T) {
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}

	inner := newFakeClient(t, cm)
	c := opclient.New(inner)

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))

	err := c.Get(context.Background(), client.ObjectKeyFromObject(cm), u)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(u.GetName()).Should(Equal("test"))
}

func TestList_TypedList_ReturnsTypedResult(t *testing.T) {
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	inner := newFakeClient(t, cm)
	c := opclient.New(inner)

	result := &corev1.ConfigMapList{}
	err := c.List(context.Background(), result, client.InNamespace("default"))

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Items).Should(HaveLen(1))
	g.Expect(result.Items[0].Name).Should(Equal("test"))
}

func TestList_UnstructuredList_PassesThrough(t *testing.T) {
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	inner := newFakeClient(t, cm)
	c := opclient.New(inner)

	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMapList"))

	err := c.List(context.Background(), ul, client.InNamespace("default"))

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(ul.Items).Should(HaveLen(1))
}

func TestCreate_DelegatesDirectly(t *testing.T) {
	g := NewWithT(t)

	inner := newFakeClient(t)
	c := opclient.New(inner)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "new", Namespace: "default"},
	}

	err := c.Create(context.Background(), cm)
	g.Expect(err).ShouldNot(HaveOccurred())

	result := &corev1.ConfigMap{}
	err = inner.Get(context.Background(), client.ObjectKeyFromObject(cm), result)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Name).Should(Equal("new"))
}

func TestUpdate_DelegatesDirectly(t *testing.T) {
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Data:       map[string]string{"key": "old"},
	}

	inner := newFakeClient(t, cm)
	c := opclient.New(inner)

	cm.Data["key"] = "new"
	err := c.Update(context.Background(), cm)
	g.Expect(err).ShouldNot(HaveOccurred())

	result := &corev1.ConfigMap{}
	err = inner.Get(context.Background(), client.ObjectKeyFromObject(cm), result)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Data).Should(HaveKeyWithValue("key", "new"))
}

func TestDelete_DelegatesDirectly(t *testing.T) {
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	inner := newFakeClient(t, cm)
	c := opclient.New(inner)

	err := c.Delete(context.Background(), cm)
	g.Expect(err).ShouldNot(HaveOccurred())
}
