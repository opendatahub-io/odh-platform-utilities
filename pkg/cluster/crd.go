package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//nolint:gochecknoglobals // Immutable GVK constant.
var crdGVK = schema.GroupVersionKind{
	Group:   "apiextensions.k8s.io",
	Version: "v1",
	Kind:    "CustomResourceDefinition",
}

// CustomResourceDefinitionExists checks whether a CRD with the given
// GroupKind exists and has reached the Established condition.
//
// The function retries with exponential backoff (up to ~5 seconds) to
// account for CRDs that are being created concurrently. It works on any
// Kubernetes cluster — no OpenShift or OLM APIs are required.
//
// The CRD name is derived as lowercase(<kind>s.<group>). This matches the
// Kubernetes naming convention for CRDs with regular English plurals. Kinds
// with irregular plurals (e.g. Policy->policies) require a different lookup
// strategy; all CRDs used in the ODH ecosystem have regular plurals.
//
// Returns nil when the CRD exists and is Established, or a non-nil error
// when the CRD is not found, not yet established, or an API error occurs.
func CustomResourceDefinitionExists(ctx context.Context, cli client.Reader, crdGK schema.GroupKind) error {
	name := strings.ToLower(fmt.Sprintf("%ss.%s", crdGK.Kind, crdGK.Group))

	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.0,
		Steps:    5,
	}

	return wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		crd := &unstructured.Unstructured{}
		crd.SetGroupVersionKind(crdGVK)

		err := cli.Get(ctx, client.ObjectKey{Name: name}, crd)
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}

			return false, err
		}

		conditions, found, err := unstructured.NestedSlice(crd.Object, "status", "conditions")
		if err != nil {
			return false, err
		}

		if !found {
			return false, nil
		}

		for _, c := range conditions {
			cond, ok := c.(map[string]any)
			if !ok {
				continue
			}

			condType, _ := cond["type"].(string)
			condStatus, _ := cond["status"].(string)

			if condType == "Established" && condStatus == "True" {
				return true, nil
			}
		}

		return false, nil
	})
}
