// Package claim provides optional readiness helpers for DatabaseClaim and SchemaClaim consumers.
package claim

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
)

var (
	errMissingIdentity = errors.New("namespace and name are required")
	errInvalidTimeout  = errors.New("wait timeout must be positive")
)

// WaitForDatabaseClaimProvisioned blocks until the named DatabaseClaim reports
// Provisioned=True or the timeout expires. Missing claims and non-True
// conditions are retried; API and context errors are returned. The reader
// needs get permission on DatabaseClaims in the claim's namespace.
func WaitForDatabaseClaimProvisioned(
	ctx context.Context, reader client.Reader, namespace, name string, timeout time.Duration,
) error {
	return waitForProvisioned(ctx, reader, namespace, name, "DatabaseClaim", "database claim", timeout)
}

// WaitForSchemaClaimProvisioned blocks until the named SchemaClaim reports
// Provisioned=True or the timeout expires. Missing claims and non-True
// conditions are retried; API and context errors are returned. The reader
// needs get permission on SchemaClaims in the claim's namespace.
func WaitForSchemaClaimProvisioned(
	ctx context.Context, reader client.Reader, namespace, name string, timeout time.Duration,
) error {
	return waitForProvisioned(ctx, reader, namespace, name, "SchemaClaim", "schema claim", timeout)
}

func waitForProvisioned(
	ctx context.Context, reader client.Reader, namespace, name, kind, label string, timeout time.Duration,
) error {
	if namespace == "" || name == "" {
		return fmt.Errorf("%s: %w", label, errMissingIdentity)
	}

	if timeout <= 0 {
		return fmt.Errorf("%s: %w", label, errInvalidTimeout)
	}

	err := wait.PollUntilContextTimeout(ctx, time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		return isProvisioned(ctx, reader, namespace, name, kind, label)
	})
	if err == nil {
		return nil
	}

	if ctx.Err() != nil {
		return fmt.Errorf("wait for %s %s/%s: %w", label, namespace, name, ctx.Err())
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s %s/%s was not Provisioned within %s: %w", label, namespace, name, timeout, err)
	}

	return err
}

func isProvisioned(
	ctx context.Context, reader client.Reader, namespace, name, kind, label string,
) (bool, error) {
	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "infrastructure.opendatahub.io", Version: "v1alpha1", Kind: kind,
	})

	err := reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, object)
	if apierrors.IsNotFound(err) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("get %s %s/%s: %w", label, namespace, name, err)
	}

	statusRaw, found, err := unstructured.NestedMap(object.Object, "status")
	if err != nil {
		return false, fmt.Errorf("read %s %s/%s status: %w", label, namespace, name, err)
	}

	if !found {
		return false, nil
	}

	var status common.Status

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(statusRaw, &status)
	if err != nil {
		return false, fmt.Errorf("decode %s %s/%s status: %w", label, namespace, name, err)
	}

	return conditions.IsStatusConditionTrue(&status, "Provisioned"), nil
}
