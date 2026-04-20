package cluster_test

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

var errPlatformAPI = errors.New("platform api error")

type erroringPlatformClient struct {
	client.Reader

	getErr  error
	listErr error
}

func (c *erroringPlatformClient) Get(
	ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption,
) error {
	if c.getErr != nil {
		return c.getErr
	}

	return c.Reader.Get(ctx, key, obj, opts...)
}

func (c *erroringPlatformClient) List(
	ctx context.Context, list client.ObjectList, opts ...client.ListOption,
) error {
	if c.listErr != nil {
		return c.listErr
	}

	return c.Reader.List(ctx, list, opts...)
}

func TestDetectPlatform_ExplicitType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		platformType string
		want         cluster.Platform
	}{
		{"OpenDataHub", "OpenDataHub", cluster.OpenDataHub},
		{"ManagedRHOAI", "ManagedRHOAI", cluster.ManagedRhoai},
		{"SelfManagedRHOAI", "SelfManagedRHOAI", cluster.SelfManagedRhoai},
		{"XKS", "XKS", cluster.XKS},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

			result, err := cluster.DetectPlatform(t.Context(), cli, tc.platformType, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tc.want {
				t.Errorf("DetectPlatform(%q) = %q, want %q", tc.platformType, result, tc.want)
			}
		})
	}
}

func TestDetectPlatform_AutoDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		want      cluster.Platform
		namespace string
		objects   []client.Object
	}{
		{
			name:      "ManagedRhoai detected via CatalogSource",
			namespace: "redhat-ods-operator",
			objects: []client.Object{
				newCatalogSource("addon-managed-odh-catalog", "redhat-ods-operator"),
			},
			want: cluster.ManagedRhoai,
		},
		{
			name: "SelfManagedRhoai detected via OperatorCondition",
			objects: []client.Object{
				newOperatorCondition("rhods-operator.v1.2.3"),
			},
			want: cluster.SelfManagedRhoai,
		},
		{
			name:    "Fallback to OpenDataHub",
			objects: nil,
			want:    cluster.OpenDataHub,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...).Build()

			result, err := cluster.DetectPlatform(t.Context(), cli, "", tc.namespace)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tc.want {
				t.Errorf("DetectPlatform() = %q, want %q", result, tc.want)
			}
		})
	}
}

func TestDetectPlatform_DefaultNamespace(t *testing.T) {
	t.Parallel()

	cs := newCatalogSource("addon-managed-odh-catalog", "redhat-ods-operator")
	cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(cs).Build()

	result, err := cluster.DetectPlatform(t.Context(), cli, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != cluster.ManagedRhoai {
		t.Errorf("DetectPlatform() = %q, want %q (default ns fallback)", result, cluster.ManagedRhoai)
	}
}

// --- helpers ---

func newCatalogSource(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "CatalogSource",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func newOperatorCondition(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "operators.coreos.com/v2",
			"kind":       "OperatorCondition",
			"metadata": map[string]any{
				"name": name,
			},
		},
	}
}

func TestDetectPlatform_APIErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		getErr  error
		listErr error
		name    string
	}{
		{
			name:   "CatalogSource Get error propagated",
			getErr: errPlatformAPI,
		},
		{
			name:    "OperatorCondition List error propagated",
			listErr: errPlatformAPI,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			baseCli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
			cli := &erroringPlatformClient{Reader: baseCli, getErr: tc.getErr, listErr: tc.listErr}

			_, err := cluster.DetectPlatform(t.Context(), cli, "", "redhat-ods-operator")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
