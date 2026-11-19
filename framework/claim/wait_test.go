package claim_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/framework/claim"
)

var (
	errUnexpectedRequest = errors.New("unexpected claim request")
	errUnavailable       = errors.New("API unavailable")
)

type waitFn func(context.Context, client.Reader, string, string, time.Duration) error

type claimReader struct {
	get  func(*unstructured.Unstructured) error
	kind string
}

func (r claimReader) Get(_ context.Context, key types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
	if key != (types.NamespacedName{Namespace: "consumer", Name: "app-claim"}) {
		return errUnexpectedRequest
	}

	if obj.GetObjectKind().GroupVersionKind() != (schema.GroupVersionKind{
		Group: "infrastructure.opendatahub.io", Version: "v1alpha1", Kind: r.kind,
	}) {
		return errUnexpectedRequest
	}

	object, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return errUnexpectedRequest
	}

	err := r.get(object)

	return err
}

func (claimReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return errUnexpectedRequest
}

func setProvisioned(obj *unstructured.Unstructured, status string) {
	obj.Object["status"] = map[string]any{"conditions": []any{map[string]any{
		"type": "Provisioned", "status": status,
	}}}
}

func TestWaitForClaimProvisioned(t *testing.T) {
	t.Parallel()

	t.Run("DatabaseClaim", func(t *testing.T) {
		t.Parallel()
		testClaimWait(t, "DatabaseClaim", "databaseclaims", "database claim", claim.WaitForDatabaseClaimProvisioned)
	})
	t.Run("SchemaClaim", func(t *testing.T) {
		t.Parallel()
		testClaimWait(t, "SchemaClaim", "schemaclaims", "schema claim", claim.WaitForSchemaClaimProvisioned)
	})
}

func testClaimWait(t *testing.T, kind, resource, label string, wait waitFn) {
	t.Helper()

	for _, status := range []string{"True", "False"} {
		t.Run("Provisioned="+status, func(t *testing.T) {
			t.Parallel()

			reader := claimReader{kind: kind, get: func(obj *unstructured.Unstructured) error {
				setProvisioned(obj, status)
				return nil
			}}

			err := wait(t.Context(), reader, "consumer", "app-claim", 20*time.Millisecond)
			if status == "True" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, label+" consumer/app-claim was not Provisioned within")
			}
		})
	}

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		reader := claimReader{kind: kind, get: func(*unstructured.Unstructured) error {
			return apierrors.NewNotFound(schema.GroupResource{
				Group: "infrastructure.opendatahub.io", Resource: resource,
			}, "app-claim")
		}}

		err := wait(t.Context(), reader, "consumer", "app-claim", 20*time.Millisecond)
		require.ErrorContains(t, err, label+" consumer/app-claim was not Provisioned within")
	})

	t.Run("API error", func(t *testing.T) {
		t.Parallel()

		reader := claimReader{kind: kind, get: func(*unstructured.Unstructured) error {
			return errUnavailable
		}}

		err := wait(t.Context(), reader, "consumer", "app-claim", time.Second)
		require.ErrorIs(t, err, errUnavailable)
		require.ErrorContains(t, err, "get "+label+" consumer/app-claim")
	})
}

func TestWaitForSchemaClaimProvisionedRetries(t *testing.T) {
	t.Parallel()

	var reads atomic.Int32

	reader := claimReader{kind: "SchemaClaim", get: func(obj *unstructured.Unstructured) error {
		if reads.Add(1) == 1 {
			setProvisioned(obj, "False")
		} else {
			setProvisioned(obj, "True")
		}

		return nil
	}}

	err := claim.WaitForSchemaClaimProvisioned(t.Context(), reader, "consumer", "app-claim", 5*time.Second)
	require.NoError(t, err)
	require.Equal(t, int32(2), reads.Load())
}

func TestWaitForSchemaClaimProvisionedInputAndCancellation(t *testing.T) {
	t.Parallel()

	err := claim.WaitForSchemaClaimProvisioned(t.Context(), nil, "", "app-claim", time.Second)
	require.ErrorContains(t, err, "namespace and name are required")

	err = claim.WaitForSchemaClaimProvisioned(t.Context(), nil, "consumer", "app-claim", 0)
	require.ErrorContains(t, err, "wait timeout must be positive")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reader := claimReader{kind: "SchemaClaim", get: func(*unstructured.Unstructured) error {
		return errUnexpectedRequest
	}}

	err = claim.WaitForSchemaClaimProvisioned(ctx, reader, "consumer", "app-claim", time.Second)
	require.ErrorIs(t, err, context.Canceled)
}
