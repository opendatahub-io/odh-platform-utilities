package webhook_test

import (
	"context"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/webhook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))

	return s
}

func TestValidateSingletonCreation_AllowFirst(t *testing.T) {
	t.Parallel()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	r := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	req := &admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
		},
	}

	resp := webhook.ValidateSingletonCreation(context.Background(), r, req, gvk)
	assert.True(t, resp.Allowed, "first creation should be allowed")
}

func TestValidateSingletonCreation_DenySecond(t *testing.T) {
	t.Parallel()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "existing", Namespace: "default"},
	}
	r := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(existing).
		Build()

	req := &admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
		},
	}

	resp := webhook.ValidateSingletonCreation(context.Background(), r, req, gvk)
	assert.False(t, resp.Allowed, "second creation should be denied")
	assert.Contains(t, resp.Result.Message, "only one instance")
}

func TestValidateSingletonCreation_AllowNonCreate(t *testing.T) {
	t.Parallel()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "existing", Namespace: "default"},
	}
	r := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(existing).
		Build()

	req := &admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
		},
	}

	resp := webhook.ValidateSingletonCreation(context.Background(), r, req, gvk)
	assert.True(t, resp.Allowed, "update operations should be allowed")
}

func TestCountObjects(t *testing.T) {
	t.Parallel()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		r := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()

		count, err := webhook.CountObjects(context.Background(), r, gvk)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("two objects", func(t *testing.T) {
		t.Parallel()

		r := fake.NewClientBuilder().
			WithScheme(newScheme(t)).
			WithObjects(
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns1"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns2"}},
			).
			Build()

		count, err := webhook.CountObjects(context.Background(), r, gvk)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}

func TestDenyCountGtZero(t *testing.T) {
	t.Parallel()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}

	t.Run("zero allows", func(t *testing.T) {
		t.Parallel()

		resp := webhook.DenyCountGtZero(0, gvk)
		assert.True(t, resp.Allowed)
	})

	t.Run("positive denies", func(t *testing.T) {
		t.Parallel()

		resp := webhook.DenyCountGtZero(1, gvk)
		assert.False(t, resp.Allowed)
	})
}
