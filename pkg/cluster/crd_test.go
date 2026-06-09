package cluster_test

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
)

var errSyntheticAPI = errors.New("synthetic")

type erroringCRDClient struct {
	client.Reader

	err        error
	targetName string
}

func (c *erroringCRDClient) Get(
	ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption,
) error {
	if key.Name == c.targetName {
		return c.err
	}

	return c.Reader.Get(ctx, key, obj, opts...)
}

func TestCustomResourceDefinitionExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		crdGK   schema.GroupKind
		cli     client.Reader
		objects []*unstructured.Unstructured
		wantErr bool
	}{
		{
			name:  "CRD exists and is established",
			crdGK: schema.GroupKind{Group: "serving.kserve.io", Kind: "InferenceService"},
			objects: []*unstructured.Unstructured{
				newCRD("inferenceservices.serving.kserve.io", true),
			},
			wantErr: false,
		},
		{
			name:    "CRD does not exist",
			crdGK:   schema.GroupKind{Group: "serving.kserve.io", Kind: "InferenceService"},
			objects: nil,
			wantErr: true,
		},
		{
			name:  "CRD exists but not established",
			crdGK: schema.GroupKind{Group: "serving.kserve.io", Kind: "InferenceService"},
			objects: []*unstructured.Unstructured{
				newCRD("inferenceservices.serving.kserve.io", false),
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cli client.Reader

			if tc.cli != nil {
				cli = tc.cli
			} else {
				builder := fake.NewClientBuilder().WithScheme(runtime.NewScheme())
				for _, obj := range tc.objects {
					builder = builder.WithObjects(obj)
				}

				cli = builder.Build()
			}

			err := cluster.CustomResourceDefinitionExists(t.Context(), cli, tc.crdGK)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}

			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestHasCRD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		crdGK   schema.GroupKind
		objects []*unstructured.Unstructured
		wantHas bool
	}{
		{
			name:  "CRD exists and is established",
			crdGK: schema.GroupKind{Group: "serving.kserve.io", Kind: "InferenceService"},
			objects: []*unstructured.Unstructured{
				newCRD("inferenceservices.serving.kserve.io", true),
			},
			wantHas: true,
		},
		{
			name:    "CRD does not exist",
			crdGK:   schema.GroupKind{Group: "serving.kserve.io", Kind: "InferenceService"},
			objects: nil,
			wantHas: false,
		},
		{
			name:  "CRD exists but not established",
			crdGK: schema.GroupKind{Group: "serving.kserve.io", Kind: "InferenceService"},
			objects: []*unstructured.Unstructured{
				newCRD("inferenceservices.serving.kserve.io", false),
			},
			wantHas: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			builder := fake.NewClientBuilder().WithScheme(runtime.NewScheme())
			for _, obj := range tc.objects {
				builder = builder.WithObjects(obj)
			}

			cli := builder.Build()

			has, err := cluster.HasCRD(t.Context(), cli, tc.crdGK)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if has != tc.wantHas {
				t.Errorf("HasCRD = %v, want %v", has, tc.wantHas)
			}
		})
	}
}

func TestHasCRD_APIError(t *testing.T) {
	t.Parallel()

	baseCli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	cli := &erroringCRDClient{
		Reader:     baseCli,
		targetName: "inferenceservices.serving.kserve.io",
		err:        apierrors.NewInternalError(errSyntheticAPI),
	}

	gk := schema.GroupKind{Group: "serving.kserve.io", Kind: "InferenceService"}

	has, err := cluster.HasCRD(t.Context(), cli, gk)
	if err == nil {
		t.Fatal("expected error from API failure, got nil")
	}

	if has {
		t.Error("HasCRD should be false on API error")
	}
}

func TestCustomResourceDefinitionExists_APIError(t *testing.T) {
	t.Parallel()

	baseCli := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	cli := &erroringCRDClient{
		Reader:     baseCli,
		targetName: "inferenceservices.serving.kserve.io",
		err:        apierrors.NewInternalError(errSyntheticAPI),
	}

	gk := schema.GroupKind{Group: "serving.kserve.io", Kind: "InferenceService"}

	err := cluster.CustomResourceDefinitionExists(t.Context(), cli, gk)
	if err == nil {
		t.Fatal("expected error from API failure, got nil")
	}
}

//nolint:unparam // name kept as parameter for readability in test table cases.
func newCRD(name string, established bool) *unstructured.Unstructured {
	status := "False"
	if established {
		status = "True"
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]any{
				"name": name,
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":   "Established",
						"status": status,
					},
				},
			},
		},
	}
}
