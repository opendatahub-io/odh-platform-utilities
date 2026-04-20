package cluster_test

import (
	"context"
	"errors"
	"testing"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

var errGenericClient = errors.New("generic client error")

type erroringReader struct {
	client.Client

	err error
}

func (c *erroringReader) Get(
	ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption,
) error {
	if key.Name == "cluster-config-v1" || key.Name == "version" || key.Name == "cluster" {
		return c.err
	}

	return c.Client.Get(ctx, key, obj, opts...)
}

func TestDetectClusterType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected cluster.ClusterType
		objects  []client.Object
	}{
		{
			name: "OpenShift cluster (ClusterVersion present)",
			objects: []client.Object{
				newClusterVersion("4.15.3"),
			},
			expected: cluster.ClusterTypeOpenShift,
		},
		{
			name:     "Vanilla Kubernetes (no ClusterVersion CRD)",
			objects:  nil,
			expected: cluster.ClusterTypeKubernetes,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			builder := fake.NewClientBuilder().WithScheme(runtime.NewScheme())
			if len(tc.objects) > 0 {
				builder = builder.WithObjects(tc.objects...)
			}

			result, err := cluster.DetectClusterType(t.Context(), builder.Build())
			if err != nil {
				if result != cluster.ClusterTypeKubernetes {
					t.Fatalf("unexpected error: %v", err)
				}

				return
			}

			if result != tc.expected {
				t.Errorf("got %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestDetectClusterType_APIError(t *testing.T) {
	t.Parallel()

	cli := &erroringReader{
		Client: fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build(),
		err:    errGenericClient,
	}

	_, err := cluster.DetectClusterType(t.Context(), cli)
	if err == nil {
		t.Fatal("expected error from API failure, got nil")
	}
}

func TestDetectClusterInfo(t *testing.T) { //nolint:funlen // Table-driven test with many cases.
	t.Parallel()

	tests := []struct {
		name     string
		wantType cluster.ClusterType
		wantVer  string
		objects  []client.Object
		wantFips bool
		wantErr  bool
	}{
		{
			name: "OpenShift with version and FIPS ConfigMap (fips disabled)",
			objects: []client.Object{
				newClusterVersion("4.15.3"),
				newClusterConfigCM(false),
			},
			wantType: cluster.ClusterTypeOpenShift,
			wantVer:  "4.15.3",
		},
		{
			name: "OpenShift with FIPS enabled",
			objects: []client.Object{
				newClusterVersion("4.16.0"),
				newClusterConfigCM(true),
			},
			wantType: cluster.ClusterTypeOpenShift,
			wantVer:  "4.16.0",
			wantFips: true,
		},
		{
			name:     "Vanilla Kubernetes",
			objects:  nil,
			wantType: cluster.ClusterTypeKubernetes,
		},
		{
			name: "OpenShift with missing FIPS ConfigMap defaults to false",
			objects: []client.Object{
				newClusterVersion("4.15.3"),
			},
			wantType: cluster.ClusterTypeOpenShift,
			wantVer:  "4.15.3",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.objects...).Build()

			info, err := cluster.DetectClusterInfo(t.Context(), cli)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				if info.Type != cluster.ClusterTypeKubernetes {
					t.Fatalf("unexpected error: %v", err)
				}

				return
			}

			if info.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", info.Type, tc.wantType)
			}

			if info.Version != tc.wantVer {
				t.Errorf("Version = %q, want %q", info.Version, tc.wantVer)
			}

			if info.FipsEnabled != tc.wantFips {
				t.Errorf("FipsEnabled = %v, want %v", info.FipsEnabled, tc.wantFips)
			}
		})
	}
}

func TestIsFipsEnabled(t *testing.T) { //nolint:funlen // Table-driven test with many cases.
	t.Parallel()

	tests := []struct {
		clientErr error
		configMap *unstructured.Unstructured
		name      string
		want      bool
		wantErr   bool
	}{
		{
			name:      "FIPS enabled",
			configMap: newClusterConfigCM(true),
			want:      true,
		},
		{
			name:      "FIPS disabled",
			configMap: newClusterConfigCM(false),
			want:      false,
		},
		{
			name: "FIPS key missing",
			configMap: newClusterConfigCMData(map[string]string{
				"install-config": "apiVersion: v1\n",
			}),
			want: false,
		},
		{
			name: "Empty install-config",
			configMap: newClusterConfigCMData(map[string]string{
				"install-config": "",
			}),
			want: false,
		},
		{
			name:      "ConfigMap not found (vanilla K8s)",
			clientErr: k8serr.NewNotFound(schema.GroupResource{Resource: "configmaps"}, "cluster-config-v1"),
			want:      false,
		},
		{
			name:      "Client error",
			clientErr: errGenericClient,
			want:      false,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cli client.Client
			if tc.configMap != nil {
				cli = fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(tc.configMap).Build()
			} else {
				cli = fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
			}

			if tc.clientErr != nil {
				cli = &erroringReader{Client: cli, err: tc.clientErr}
			}

			result, err := cluster.IsFipsEnabled(t.Context(), cli)

			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tc.want {
				t.Errorf("IsFipsEnabled() = %v, want %v", result, tc.want)
			}
		})
	}
}

func TestIsFipsEnabled_InvalidYAMLReturnsError(t *testing.T) {
	t.Parallel()

	cm := newClusterConfigCMData(map[string]string{
		"install-config": "not: valid: yaml: [broken",
	})
	cli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(cm).Build()

	_, err := cluster.IsFipsEnabled(t.Context(), cli)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

// --- helpers ---

func newClusterVersion(version string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "ClusterVersion",
			"metadata": map[string]any{
				"name": "version",
			},
			"status": map[string]any{
				"history": []any{
					map[string]any{
						"version": version,
					},
				},
			},
		},
	}
}

func newClusterConfigCM(fipsEnabled bool) *unstructured.Unstructured {
	fipsStr := "false"
	if fipsEnabled {
		fipsStr = "true"
	}

	return newClusterConfigCMData(map[string]string{
		"install-config": "apiVersion: v1\nfips: " + fipsStr,
	})
}

func newClusterConfigCMData(data map[string]string) *unstructured.Unstructured {
	dataAny := make(map[string]any, len(data))
	for k, v := range data {
		dataAny[k] = v
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "cluster-config-v1",
				"namespace": "kube-system",
			},
			"data": dataAny,
		},
	}
}

// runtime.Object for fake client compatibility.
var _ runtime.Object = &unstructured.Unstructured{}
