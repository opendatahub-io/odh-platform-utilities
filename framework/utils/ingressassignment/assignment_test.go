package ingressassignment_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/framework/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/framework/utils/ingressassignment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testClient(t *testing.T, namespace *corev1.Namespace) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	builder := fake.NewClientBuilder().WithScheme(scheme)
	if namespace != nil {
		builder = builder.WithObjects(namespace)
	}

	return builder.Build()
}

func ingresses() []ingressassignment.Ingress {
	return []ingressassignment.Ingress{
		{
			Name: "default", GatewayName: "default-gateway", GatewayNamespace: "gateways",
			Hostname: "default.example.com", IsDefault: true,
		},
		{Name: "alpha", GatewayName: "alpha-gateway", GatewayNamespace: "gateways", Hostname: "alpha.example.com"},
		{Name: "beta", GatewayName: "beta-gateway", GatewayNamespace: "other-gateways"},
	}
}

func TestIngressJSON(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		payload string
		ingress ingressassignment.Ingress
	}{
		{
			name:    "all fields",
			payload: `{"name":"default","gatewayName":"default-gateway","gatewayNamespace":"gateways","hostname":"default.example.com","isDefault":true}`,
			ingress: ingressassignment.Ingress{
				Name: "default", GatewayName: "default-gateway", GatewayNamespace: "gateways",
				Hostname: "default.example.com", IsDefault: true,
			},
		},
		{
			name:    "optional fields omitted",
			payload: `{"name":"beta","gatewayName":"beta-gateway","gatewayNamespace":"other-gateways"}`,
			ingress: ingressassignment.Ingress{Name: "beta", GatewayName: "beta-gateway", GatewayNamespace: "other-gateways"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var decoded ingressassignment.Ingress
			require.NoError(t, json.Unmarshal([]byte(tt.payload), &decoded))
			assert.Equal(t, tt.ingress, decoded)

			encoded, err := json.Marshal(tt.ingress)
			require.NoError(t, err)
			assert.JSONEq(t, tt.payload, string(encoded))
		})
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		annotation string
		annotated  bool
		wantIndex  int
	}{
		{name: "absent annotation selects default"},
		{name: "explicit default", annotation: "default", annotated: true},
		{name: "named ingress", annotation: "alpha", annotated: true, wantIndex: 1},
		{name: "empty hostname remains valid", annotation: "beta", annotated: true, wantIndex: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team-a"}}
			if tt.annotated {
				namespace.Annotations = map[string]string{annotations.IngressName: tt.annotation}
			}

			selected, err := ingressassignment.Resolve(
				context.Background(), testClient(t, namespace), namespace.Name, ingresses(),
			)
			require.NoError(t, err)
			assert.Equal(t, ingresses()[tt.wantIndex], selected)
		})
	}
}

func TestResolveUnknownIngress(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		annotation string
		removed    bool
	}{
		{name: "unknown", annotation: "unknown-gateway"},
		{name: "Gateway name is not ingress name", annotation: "alpha-gateway"},
		{name: "removed", annotation: "alpha", removed: true},
		{name: "empty annotation"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name: "team-a", Annotations: map[string]string{annotations.IngressName: tt.annotation},
			}}

			available := ingresses()
			if tt.removed {
				available = available[:1]
			}

			selected, err := ingressassignment.Resolve(context.Background(), testClient(t, namespace), namespace.Name, available)
			require.ErrorIs(t, err, ingressassignment.ErrUnknownIngress)
			require.ErrorContains(t, err, fmt.Sprintf(
				"namespace %q references unknown ingress %q", namespace.Name, tt.annotation,
			))
			assert.Equal(t, ingressassignment.Ingress{}, selected)
		})
	}
}

func TestResolveAnnotationChange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "team-a", Annotations: map[string]string{annotations.IngressName: "alpha"},
	}}
	k8sClient := testClient(t, namespace)

	selected, err := ingressassignment.Resolve(ctx, k8sClient, namespace.Name, ingresses())
	require.NoError(t, err)
	assert.Equal(t, ingresses()[1], selected)

	updated := &corev1.Namespace{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: namespace.Name}, updated))
	updated.Annotations[annotations.IngressName] = "beta"
	require.NoError(t, k8sClient.Update(ctx, updated))

	selected, err = ingressassignment.Resolve(ctx, k8sClient, namespace.Name, ingresses())
	require.NoError(t, err)
	assert.Equal(t, ingresses()[2], selected)

	delete(updated.Annotations, annotations.IngressName)
	require.NoError(t, k8sClient.Update(ctx, updated))

	selected, err = ingressassignment.Resolve(ctx, k8sClient, namespace.Name, ingresses())
	require.NoError(t, err)
	assert.Equal(t, ingresses()[0], selected)
}

func TestResolveWithoutDefault(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "team-a", Annotations: map[string]string{annotations.IngressName: "alpha"},
	}}
	k8sClient := testClient(t, namespace)
	available := ingresses()[1:]

	selected, err := ingressassignment.Resolve(ctx, k8sClient, namespace.Name, available)
	require.NoError(t, err)
	assert.Equal(t, available[0], selected)

	updated := &corev1.Namespace{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Name: namespace.Name}, updated))
	delete(updated.Annotations, annotations.IngressName)
	require.NoError(t, k8sClient.Update(ctx, updated))

	selected, err = ingressassignment.Resolve(ctx, k8sClient, namespace.Name, available)
	require.ErrorIs(t, err, ingressassignment.ErrUnknownIngress)
	require.ErrorContains(t, err, "no default ingress")
	assert.Equal(t, ingressassignment.Ingress{}, selected)
}

func TestResolveNamespaceNotFound(t *testing.T) {
	t.Parallel()

	selected, err := ingressassignment.Resolve(context.Background(), testClient(t, nil), "missing", ingresses())
	assert.Equal(t, ingressassignment.Ingress{}, selected)
	require.Error(t, err)
	assert.True(t, apierrors.IsNotFound(err))
	assert.NotErrorIs(t, err, ingressassignment.ErrUnknownIngress)
}

func TestResolveInvalidIngresses(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		wantError string
		ingresses []ingressassignment.Ingress
	}{
		{
			name: "multiple defaults", wantError: "at most one default",
			ingresses: []ingressassignment.Ingress{
				{Name: "a", GatewayName: "gateway-a", GatewayNamespace: "gateways", IsDefault: true},
				{Name: "b", GatewayName: "gateway-b", GatewayNamespace: "gateways", IsDefault: true},
			},
		},
		{
			name: "empty name", wantError: "empty name",
			ingresses: []ingressassignment.Ingress{{GatewayName: "gateway-a", GatewayNamespace: "gateways", IsDefault: true}},
		},
		{
			name: "empty Gateway name", wantError: "empty Gateway name",
			ingresses: []ingressassignment.Ingress{{Name: "a", GatewayNamespace: "gateways", IsDefault: true}},
		},
		{
			name: "empty Gateway namespace", wantError: "empty Gateway namespace",
			ingresses: []ingressassignment.Ingress{{Name: "a", GatewayName: "gateway-a", IsDefault: true}},
		},
		{
			name: "duplicate ingress name", wantError: "duplicate ingress name",
			ingresses: []ingressassignment.Ingress{
				{Name: "a", GatewayName: "gateway-a", GatewayNamespace: "gateways", IsDefault: true},
				{Name: "a", GatewayName: "gateway-b", GatewayNamespace: "other"},
			},
		},
		{
			name: "duplicate Gateway name", wantError: "duplicate Gateway name",
			ingresses: []ingressassignment.Ingress{
				{Name: "a", GatewayName: "gateway-a", GatewayNamespace: "gateways", IsDefault: true},
				{Name: "b", GatewayName: "gateway-a", GatewayNamespace: "other"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			selected, err := ingressassignment.Resolve(context.Background(), testClient(t, nil), "missing", tt.ingresses)
			require.ErrorContains(t, err, tt.wantError)
			assert.Equal(t, ingressassignment.Ingress{}, selected)
			require.NotErrorIs(t, err, ingressassignment.ErrUnknownIngress)
			assert.False(t, apierrors.IsNotFound(err), "validation must precede Namespace lookup")
		})
	}
}
