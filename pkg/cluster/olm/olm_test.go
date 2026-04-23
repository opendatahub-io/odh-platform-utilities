package olm_test

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster/olm"
)

var errAPIFailure = errors.New("api failure")

type erroringOLMClient struct {
	client.Reader

	listErr error
	getErr  error
}

func (c *erroringOLMClient) Get(
	ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption,
) error {
	if c.getErr != nil {
		return c.getErr
	}

	return c.Reader.Get(ctx, key, obj, opts...)
}

func (c *erroringOLMClient) List(
	ctx context.Context, list client.ObjectList, opts ...client.ListOption,
) error {
	if c.listErr != nil {
		return c.listErr
	}

	return c.Reader.List(ctx, list, opts...)
}

func TestOperatorExists(t *testing.T) { //nolint:funlen // Table-driven test with many cases.
	t.Parallel()

	tests := []struct {
		name     string
		prefix   string
		wantVer  string
		objects  []client.Object
		wantInfo bool
	}{
		{
			name:   "operator found with version",
			prefix: "rhods-operator",
			objects: []client.Object{
				newOperatorCondition("rhods-operator.v1.2.3"),
			},
			wantInfo: true,
			wantVer:  "v1.2.3",
		},
		{
			name:   "operator found without v prefix",
			prefix: "rhods-operator",
			objects: []client.Object{
				newOperatorCondition("rhods-operator.1.2.3"),
			},
			wantInfo: true,
			wantVer:  "v1.2.3",
		},
		{
			name:   "operator found with empty version",
			prefix: "rhods-operator",
			objects: []client.Object{
				newOperatorCondition("rhods-operator."),
			},
			wantInfo: true,
			wantVer:  "",
		},
		{
			name:     "operator not found",
			prefix:   "rhods-operator",
			objects:  nil,
			wantInfo: false,
		},
		{
			name:   "different operator present, target not found",
			prefix: "rhods-operator",
			objects: []client.Object{
				newOperatorCondition("other-operator.v1.0.0"),
			},
			wantInfo: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...).Build()

			info, err := olm.OperatorExists(t.Context(), cli, tc.prefix)

			if !tc.wantInfo {
				if !errors.Is(err, olm.ErrOperatorNotInstalled) {
					t.Errorf("expected ErrOperatorNotInstalled, got %v", err)
				}

				if info != nil {
					t.Errorf("expected nil OperatorInfo, got %+v", info)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if info == nil {
				t.Fatal("expected OperatorInfo, got nil")
			}

			if info.Version != tc.wantVer {
				t.Errorf("Version = %q, want %q", info.Version, tc.wantVer)
			}
		})
	}
}

func TestSubscriptionExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		subName string
		objects []client.Object
		want    bool
	}{
		{
			name:    "subscription found",
			subName: "my-operator",
			objects: []client.Object{
				newSubscription("my-operator", "operators"),
			},
			want: true,
		},
		{
			name:    "subscription not found",
			subName: "my-operator",
			objects: nil,
			want:    false,
		},
		{
			name:    "different subscription name",
			subName: "my-operator",
			objects: []client.Object{
				newSubscription("other-operator", "operators"),
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...).Build()

			result, err := olm.SubscriptionExists(t.Context(), cli, tc.subName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tc.want {
				t.Errorf("SubscriptionExists(%q) = %v, want %v", tc.subName, result, tc.want)
			}
		})
	}
}

func TestGetSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		namespace string
		subName   string
		objects   []client.Object
		wantErr   bool
	}{
		{
			name:      "subscription found",
			namespace: "operators",
			subName:   "my-operator",
			objects: []client.Object{
				newSubscription("my-operator", "operators"),
			},
		},
		{
			name:      "subscription not found",
			namespace: "operators",
			subName:   "missing",
			objects:   nil,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...).Build()

			sub, err := olm.GetSubscription(t.Context(), cli, tc.namespace, tc.subName)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if sub.GetName() != tc.subName {
				t.Errorf("subscription name = %q, want %q", sub.GetName(), tc.subName)
			}
		})
	}
}

func TestCatalogSourceExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		csName    string
		namespace string
		objects   []client.Object
		want      bool
	}{
		{
			name:      "catalog source found",
			csName:    "addon-managed-odh-catalog",
			namespace: "redhat-ods-operator",
			objects: []client.Object{
				newCatalogSource("addon-managed-odh-catalog", "redhat-ods-operator"),
			},
			want: true,
		},
		{
			name:      "catalog source not found",
			csName:    "addon-managed-odh-catalog",
			namespace: "redhat-ods-operator",
			objects:   nil,
			want:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...).Build()

			result, err := olm.CatalogSourceExists(t.Context(), cli, tc.namespace, tc.csName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tc.want {
				t.Errorf("CatalogSourceExists() = %v, want %v", result, tc.want)
			}
		})
	}
}

// --- helpers ---

func newOperatorCondition(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "operators.coreos.com/v2",
			"kind":       "OperatorCondition",
			"metadata":   map[string]any{"name": name},
		},
	}
}

func newSubscription(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "Subscription",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

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

func TestOperatorExists_APIError(t *testing.T) {
	t.Parallel()

	baseCli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	cli := &erroringOLMClient{Reader: baseCli, listErr: errAPIFailure}

	_, err := olm.OperatorExists(t.Context(), cli, "rhods-operator")
	if err == nil {
		t.Fatal("expected error from List failure, got nil")
	}
}

func TestSubscriptionExists_APIError(t *testing.T) {
	t.Parallel()

	baseCli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	cli := &erroringOLMClient{Reader: baseCli, listErr: errAPIFailure}

	_, err := olm.SubscriptionExists(t.Context(), cli, "my-operator")
	if err == nil {
		t.Fatal("expected error from List failure, got nil")
	}
}

func TestGetSubscription_APIError(t *testing.T) {
	t.Parallel()

	baseCli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	cli := &erroringOLMClient{Reader: baseCli, getErr: errAPIFailure}

	_, err := olm.GetSubscription(t.Context(), cli, "operators", "my-operator")
	if err == nil {
		t.Fatal("expected error from Get failure, got nil")
	}
}

func TestCatalogSourceExists_APIError(t *testing.T) {
	t.Parallel()

	baseCli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	cli := &erroringOLMClient{Reader: baseCli, getErr: errAPIFailure}

	_, err := olm.CatalogSourceExists(t.Context(), cli, "redhat-ods-operator", "addon-managed-odh-catalog")
	if err == nil {
		t.Fatal("expected error from Get failure, got nil")
	}
}
