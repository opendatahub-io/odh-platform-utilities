package client_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientFake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	uclient "github.com/opendatahub-io/odh-platform-utilities/framework/client"

	. "github.com/onsi/gomega"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	return s
}

func newFakeClient(objs ...client.Object) client.Client {
	return clientFake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(objs...).
		Build()
}

// spyClient records what object types are passed to the inner client.
// Used to verify that the Client wrapper converts typed objects to
// unstructured before calling the inner client.
type spyClient struct {
	client.Client

	getCalls  []client.Object
	listCalls []client.ObjectList
}

func (s *spyClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	s.getCalls = append(s.getCalls, obj)
	return s.Client.Get(ctx, key, obj, opts...)
}

func (s *spyClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	s.listCalls = append(s.listCalls, list)
	return s.Client.List(ctx, list, opts...)
}

func TestClient_Get_Typed(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{"key": "value"},
	}

	spy := &spyClient{Client: newFakeClient(cm)}
	wrappedClient := uclient.New(spy)

	result := &corev1.ConfigMap{}
	err := wrappedClient.Get(t.Context(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, result)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Name).Should(Equal("test-cm"))
	g.Expect(result.Namespace).Should(Equal("default"))
	g.Expect(result.Data["key"]).Should(Equal("value"))

	g.Expect(spy.getCalls).Should(HaveLen(1))
	_, isUnstructured := spy.getCalls[0].(*unstructured.Unstructured)
	g.Expect(isUnstructured).Should(BeTrue(),
		"expected unstructured object to be passed to inner client for cache consistency")
}

func TestClient_Get_Unstructured(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{"key": "value"},
	}

	spy := &spyClient{Client: newFakeClient(cm)}
	wrappedClient := uclient.New(spy)

	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})
	err := wrappedClient.Get(t.Context(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, result)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.GetName()).Should(Equal("test-cm"))

	g.Expect(spy.getCalls).Should(HaveLen(1))
	passedObj, isUnstructured := spy.getCalls[0].(*unstructured.Unstructured)
	g.Expect(isUnstructured).Should(BeTrue())
	g.Expect(passedObj).Should(Equal(result), "unstructured input should pass through directly")
}

func TestClient_Get_NotFound(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	wrappedClient := uclient.New(newFakeClient())

	result := &corev1.ConfigMap{}
	err := wrappedClient.Get(t.Context(), types.NamespacedName{Name: "nonexistent", Namespace: "default"}, result)

	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("not found"))
}

func TestClient_List_Typed(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm-1", Namespace: "default"},
	}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm-2", Namespace: "default"},
	}

	spy := &spyClient{Client: newFakeClient(cm1, cm2)}
	wrappedClient := uclient.New(spy)

	result := &corev1.ConfigMapList{}
	err := wrappedClient.List(t.Context(), result, client.InNamespace("default"))

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Items).Should(HaveLen(2))

	g.Expect(spy.listCalls).Should(HaveLen(1))
	_, isUnstructuredList := spy.listCalls[0].(*unstructured.UnstructuredList)
	g.Expect(isUnstructuredList).Should(BeTrue(),
		"expected unstructured list to be passed to inner client for cache consistency")
}

func TestClient_List_Unstructured(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
	}

	spy := &spyClient{Client: newFakeClient(cm)}
	wrappedClient := uclient.New(spy)

	result := &unstructured.UnstructuredList{}
	result.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMapList"})
	err := wrappedClient.List(t.Context(), result, client.InNamespace("default"))

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Items).Should(HaveLen(1))
	g.Expect(result.Items[0].GetName()).Should(Equal("test-cm"))

	g.Expect(spy.listCalls).Should(HaveLen(1))
	_, isUnstructuredList := spy.listCalls[0].(*unstructured.UnstructuredList)
	g.Expect(isUnstructuredList).Should(BeTrue())
}

func TestClient_Create_Delegates(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	wrappedClient := uclient.New(newFakeClient())

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "new-cm", Namespace: "default"},
	}

	err := wrappedClient.Create(t.Context(), cm)
	g.Expect(err).ShouldNot(HaveOccurred())

	result := &corev1.ConfigMap{}
	err = wrappedClient.Get(t.Context(), types.NamespacedName{Name: "new-cm", Namespace: "default"}, result)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Name).Should(Equal("new-cm"))
}

func TestClient_Update_Delegates(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
		Data:       map[string]string{"key": "original"},
	}

	wrappedClient := uclient.New(newFakeClient(cm))

	toUpdate := &corev1.ConfigMap{}
	err := wrappedClient.Get(t.Context(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, toUpdate)
	g.Expect(err).ShouldNot(HaveOccurred())

	toUpdate.Data["key"] = "updated"
	err = wrappedClient.Update(t.Context(), toUpdate)
	g.Expect(err).ShouldNot(HaveOccurred())

	result := &corev1.ConfigMap{}
	err = wrappedClient.Get(t.Context(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, result)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.Data["key"]).Should(Equal("updated"))
}

func TestClient_Delete_Delegates(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
	}

	wrappedClient := uclient.New(newFakeClient(cm))

	err := wrappedClient.Delete(t.Context(), cm)
	g.Expect(err).ShouldNot(HaveOccurred())

	result := &corev1.ConfigMap{}
	err = wrappedClient.Get(t.Context(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, result)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("not found"))
}

func TestClient_MetadataPreserved(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cm",
			Namespace:       "default",
			UID:             "test-uid-123",
			ResourceVersion: "12345",
			Labels:          map[string]string{"app": "test"},
			Annotations:     map[string]string{"note": "important"},
		},
		Data: map[string]string{"key": "value"},
	}

	wrappedClient := uclient.New(newFakeClient(cm))

	result := &corev1.ConfigMap{}
	err := wrappedClient.Get(t.Context(), types.NamespacedName{Name: "test-cm", Namespace: "default"}, result)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(result.UID)).Should(Equal("test-uid-123"))
	g.Expect(result.ResourceVersion).Should(Equal("12345"))
	g.Expect(result.Labels["app"]).Should(Equal("test"))
	g.Expect(result.Annotations["note"]).Should(Equal("important"))
}

func TestClient_Scheme(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	fakeClient := newFakeClient()
	wrappedClient := uclient.New(fakeClient)

	g.Expect(wrappedClient.Scheme()).Should(Equal(fakeClient.Scheme()))
}
