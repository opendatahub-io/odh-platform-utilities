package openshift_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster/openshift"
)

var (
	errConnectionRefused = errors.New("connection refused")
	errForbidden         = errors.New("forbidden")
)

type erroringClient struct {
	client.Client

	getErr  error
	listErr error
}

func (c *erroringClient) Get(
	ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption,
) error {
	if c.getErr != nil && (key.Name == "cluster" || key.Name == "version") {
		return c.getErr
	}

	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *erroringClient) List(
	ctx context.Context, list client.ObjectList, opts ...client.ListOption,
) error {
	if c.listErr != nil {
		return c.listErr
	}

	return c.Client.List(ctx, list, opts...)
}

func TestGetVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		want    string
		objects []client.Object
		wantErr bool
	}{
		{
			name: "returns version from ClusterVersion",
			objects: []client.Object{
				newClusterVersion("4.15.3"),
			},
			want: "4.15.3",
		},
		{
			name:    "error when ClusterVersion absent",
			objects: nil,
			wantErr: true,
		},
		{
			name: "error when history is empty",
			objects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "config.openshift.io/v1",
						"kind":       "ClusterVersion",
						"metadata":   map[string]any{"name": "version"},
						"status":     map[string]any{"history": []any{}},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...).Build()

			result, err := openshift.GetVersion(t.Context(), cli)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tc.want {
				t.Errorf("GetVersion() = %q, want %q", result, tc.want)
			}
		})
	}
}

func TestIsSingleNodeCluster(t *testing.T) { //nolint:funlen // Table-driven test with many cases.
	t.Parallel()

	noMatchErr := &meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "config.openshift.io", Kind: "Infrastructure"},
		SearchedVersions: []string{"v1"},
	}

	tests := []struct {
		name    string
		getErr  error
		listErr error
		objects []client.Object
		want    bool
		wantErr bool
	}{
		{
			name:    "SingleReplica topology on OpenShift",
			objects: []client.Object{newInfrastructure("SingleReplica")},
			want:    true,
		},
		{
			name:    "HighlyAvailable topology on OpenShift",
			objects: []client.Object{newInfrastructure("HighlyAvailable")},
			want:    false,
		},
		{
			name:    "empty topology defaults to multi-node",
			objects: []client.Object{newInfrastructure("")},
			want:    false,
		},
		{
			name:   "CRD absent (NoMatch) - fallback single node",
			getErr: noMatchErr,
			objects: []client.Object{
				newNode("node-1", false),
			},
			want: true,
		},
		{
			name:   "CRD absent (NoMatch) - fallback multiple nodes",
			getErr: noMatchErr,
			objects: []client.Object{
				newNode("node-1", false),
				newNode("node-2", false),
			},
			want: false,
		},
		{
			name:   "CRD absent - one schedulable, one unschedulable",
			getErr: k8serr.NewNotFound(schema.GroupResource{}, "cluster"),
			objects: []client.Object{
				newNode("node-1", false),
				newNode("node-2", true),
			},
			want: true,
		},
		{
			name:   "CRD absent - all unschedulable",
			getErr: k8serr.NewNotFound(schema.GroupResource{}, "cluster"),
			objects: []client.Object{
				newNode("node-1", true),
			},
			want: false,
		},
		{
			name:   "CRD absent - no nodes",
			getErr: k8serr.NewNotFound(schema.GroupResource{}, "cluster"),
			want:   false,
		},
		{
			name:    "other infrastructure error propagated",
			getErr:  errConnectionRefused,
			objects: []client.Object{newNode("node-1", false)},
			wantErr: true,
		},
		{
			name:    "infrastructure not found - node list error propagated",
			getErr:  k8serr.NewNotFound(schema.GroupResource{}, "cluster"),
			listErr: errForbidden,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			builder := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...)

			var cli client.Client = builder.Build()

			if tc.getErr != nil || tc.listErr != nil {
				cli = &erroringClient{Client: cli, getErr: tc.getErr, listErr: tc.listErr}
			}

			result, err := openshift.IsSingleNodeCluster(t.Context(), cli)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tc.want {
				t.Errorf("IsSingleNodeCluster() = %v, want %v", result, tc.want)
			}
		})
	}
}

func TestGetAuthenticationMode(t *testing.T) { //nolint:funlen // Table-driven test with many cases.
	t.Parallel()

	tests := []struct {
		name    string
		want    cluster.AuthenticationMode
		objects []client.Object
		wantErr bool
	}{
		{
			name:    "IntegratedOAuth type",
			objects: []client.Object{newAuthentication("IntegratedOAuth")},
			want:    cluster.AuthModeIntegratedOAuth,
		},
		{
			name:    "empty type defaults to IntegratedOAuth",
			objects: []client.Object{newAuthentication("")},
			want:    cluster.AuthModeIntegratedOAuth,
		},
		{
			name:    "OIDC type",
			objects: []client.Object{newAuthentication("OIDC")},
			want:    cluster.AuthModeOIDC,
		},
		{
			name:    "None type",
			objects: []client.Object{newAuthentication("None")},
			want:    cluster.AuthModeNone,
		},
		{
			name:    "custom type defaults to None",
			objects: []client.Object{newAuthentication("CustomAuth")},
			want:    cluster.AuthModeNone,
		},
		{
			name:    "Authentication not found",
			objects: nil,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...).Build()

			result, err := openshift.GetAuthenticationMode(t.Context(), cli)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tc.want {
				t.Errorf("GetAuthenticationMode() = %q, want %q", result, tc.want)
			}
		})
	}
}

func TestIsIntegratedOAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		objects []client.Object
		want    bool
		wantErr bool
	}{
		{
			name:    "IntegratedOAuth returns true",
			objects: []client.Object{newAuthentication("IntegratedOAuth")},
			want:    true,
		},
		{
			name:    "empty returns true",
			objects: []client.Object{newAuthentication("")},
			want:    true,
		},
		{
			name:    "OIDC returns false",
			objects: []client.Object{newAuthentication("OIDC")},
			want:    false,
		},
		{
			name:    "not found returns error",
			objects: nil,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...).Build()

			result, err := openshift.IsIntegratedOAuth(t.Context(), cli)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tc.want {
				t.Errorf("IsIntegratedOAuth() = %v, want %v", result, tc.want)
			}
		})
	}
}

func TestGetServiceAccountIssuer(t *testing.T) { //nolint:funlen // Table-driven test with many cases.
	t.Parallel()

	tests := []struct {
		name      string
		want      string
		clientErr error
		objects   []client.Object
		wantErr   bool
	}{
		{
			name: "HyperShift cluster with custom issuer",
			objects: []client.Object{
				newAuthenticationWithIssuer("https://rh-oidc.s3.us-east-1.amazonaws.com/abc123"),
			},
			want: "https://rh-oidc.s3.us-east-1.amazonaws.com/abc123",
		},
		{
			name:    "standard OpenShift (empty issuer)",
			objects: []client.Object{newAuthenticationWithIssuer("")},
			want:    "",
		},
		{
			name:    "not found returns error",
			objects: nil,
			wantErr: true,
		},
		{
			name:      "client error",
			clientErr: errForbidden,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...).Build()

			var c client.Client = cli

			if tc.clientErr != nil {
				c = &erroringClient{Client: cli, getErr: tc.clientErr}
			}

			result, err := openshift.GetServiceAccountIssuer(t.Context(), c)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tc.want {
				t.Errorf("GetServiceAccountIssuer() = %q, want %q", result, tc.want)
			}
		})
	}
}

func TestGetDomain(t *testing.T) { //nolint:funlen // Table-driven test with many cases.
	t.Parallel()

	tests := []struct {
		name      string
		want      string
		errMsg    string
		clientErr error
		objects   []client.Object
		wantErr   bool
	}{
		{
			name:    "appsDomain takes precedence",
			objects: []client.Object{newIngress("apps.custom.example.com", "example.com")},
			want:    "apps.custom.example.com",
		},
		{
			name:    "appsDomain empty falls back to domain",
			objects: []client.Object{newIngress("", "example.com")},
			want:    "example.com",
		},
		{
			name: "only domain field present",
			objects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "config.openshift.io/v1",
						"kind":       "Ingress",
						"metadata":   map[string]any{"name": "cluster"},
						"spec":       map[string]any{"domain": "example.com"},
					},
				},
			},
			want: "example.com",
		},
		{
			name: "domain empty returns error",
			objects: []client.Object{
				&unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "config.openshift.io/v1",
						"kind":       "Ingress",
						"metadata":   map[string]any{"name": "cluster"},
						"spec":       map[string]any{},
					},
				},
			},
			wantErr: true,
			errMsg:  "spec.domain not found or empty",
		},
		{
			name:      "ingress not found",
			clientErr: k8serr.NewNotFound(schema.GroupResource{}, "cluster"),
			wantErr:   true,
			errMsg:    "failed fetching cluster's ingress details",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...).Build()

			var c client.Client = cli

			if tc.clientErr != nil {
				c = &erroringClient{Client: cli, getErr: tc.clientErr}
			}

			result, err := openshift.GetDomain(t.Context(), c)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tc.errMsg != "" && !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("error = %v, want containing %q", err, tc.errMsg)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tc.want {
				t.Errorf("GetDomain() = %q, want %q", result, tc.want)
			}
		})
	}
}

// --- helpers ---

func newClusterVersion(version string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "ClusterVersion",
			"metadata":   map[string]any{"name": "version"},
			"status": map[string]any{
				"history": []any{
					map[string]any{"version": version},
				},
			},
		},
	}
}

func newInfrastructure(topology string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "Infrastructure",
			"metadata":   map[string]any{"name": "cluster"},
			"status": map[string]any{
				"controlPlaneTopology": topology,
			},
		},
	}
}

func newNode(name string, unschedulable bool) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{},
	}

	if unschedulable {
		obj["spec"] = map[string]any{"unschedulable": true}
	}

	return &unstructured.Unstructured{Object: obj}
}

func newAuthentication(authType string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "Authentication",
			"metadata":   map[string]any{"name": "cluster"},
			"spec": map[string]any{
				"type": authType,
			},
		},
	}
}

func newAuthenticationWithIssuer(issuer string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "Authentication",
			"metadata":   map[string]any{"name": "cluster"},
			"spec": map[string]any{
				"serviceAccountIssuer": issuer,
			},
		},
	}
}

func newIngress(appsDomain, domain string) *unstructured.Unstructured {
	spec := map[string]any{
		"domain": domain,
	}

	if appsDomain != "" {
		spec["appsDomain"] = appsDomain
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "Ingress",
			"metadata":   map[string]any{"name": "cluster"},
			"spec":       spec,
		},
	}
}
