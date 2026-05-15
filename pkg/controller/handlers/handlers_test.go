package handlers_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/handlers"
)

func TestEnqueueOwner_ReturnsNonNil(t *testing.T) {
	t.Parallel()

	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))

	h := handlers.EnqueueOwner(s, newFakeRESTMapper(), &corev1.ConfigMap{})
	assert.NotNil(t, h)
}

func TestEnqueueOwner_AcceptsOnlyControllerOwner(t *testing.T) {
	t.Parallel()

	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))

	h := handlers.EnqueueOwner(s, newFakeRESTMapper(), &corev1.ConfigMap{}, handler.OnlyControllerOwner())
	assert.NotNil(t, h)
}

func TestEnqueueOwner_EnqueuesOwnerOnCreate(t *testing.T) {
	t.Parallel()

	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))

	h := handlers.EnqueueOwner(s, newFakeRESTMapper(), &corev1.ConfigMap{})

	ownerGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	pod := podOwnedBy("my-configmap", "uid-1", ownerGVK, true)

	q := workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
	)
	defer q.ShutDown()

	h.Create(context.Background(), event.TypedCreateEvent[client.Object]{Object: pod}, q)

	require.Equal(t, 1, q.Len())

	item, _ := q.Get()
	assert.Equal(t, "my-configmap", item.Name)
}

func TestEnqueueOwner_OnlyControllerOwnerFiltersNonController(t *testing.T) {
	t.Parallel()

	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))

	h := handlers.EnqueueOwner(s, newFakeRESTMapper(), &corev1.ConfigMap{}, handler.OnlyControllerOwner())

	ownerGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	pod := podOwnedBy("my-configmap", "uid-1", ownerGVK, false)

	q := workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
	)
	defer q.ShutDown()

	h.Create(context.Background(), event.TypedCreateEvent[client.Object]{Object: pod}, q)

	assert.Equal(t, 0, q.Len(), "non-controller owner should not enqueue")
}

// --- helpers ---

func podOwnedBy(ownerName string, ownerUID types.UID, ownerGVK schema.GroupVersionKind, isController bool) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: ownerGVK.GroupVersion().String(),
				Kind:       ownerGVK.Kind,
				Name:       ownerName,
				UID:        ownerUID,
				Controller: &isController,
			}},
		},
	}
}

// fakeRESTMapper is a minimal RESTMapper for constructing handlers in tests.
type fakeRESTMapper struct{}

func newFakeRESTMapper() *fakeRESTMapper { return &fakeRESTMapper{} }

func (f *fakeRESTMapper) KindFor(schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}

func (f *fakeRESTMapper) KindsFor(schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return nil, nil
}

func (f *fakeRESTMapper) ResourceFor(schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{}, nil
}

func (f *fakeRESTMapper) ResourcesFor(schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return nil, nil
}

func (f *fakeRESTMapper) RESTMapping(gk schema.GroupKind, _ ...string) (*meta.RESTMapping, error) {
	return &meta.RESTMapping{
		Resource: schema.GroupVersionResource{
			Group:    gk.Group,
			Version:  "v1",
			Resource: gk.Kind,
		},
		GroupVersionKind: schema.GroupVersionKind{
			Group:   gk.Group,
			Version: "v1",
			Kind:    gk.Kind,
		},
		Scope: meta.RESTScopeNamespace,
	}, nil
}

func (f *fakeRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	m, err := f.RESTMapping(gk, versions...)
	if err != nil {
		return nil, err
	}

	return []*meta.RESTMapping{m}, nil
}

func (f *fakeRESTMapper) ResourceSingularizer(resource string) (string, error) {
	return resource, nil
}
