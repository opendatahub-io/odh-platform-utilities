package validation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
)

const (
	defaultTimeout      = 60 * time.Second
	defaultPollInterval = 2 * time.Second
)

// ContractOptions configures the platform contract conformance suite.
type ContractOptions struct {
	GVK          schema.GroupVersionKind
	InstanceName string        // e.g. "default-dashboard"
	Timeout      time.Duration // default: 60s
	PollInterval time.Duration // default: 2s

	// SkipReleases skips the releases check for modules that do not yet
	// embed ComponentReleaseStatus in their status type.
	SkipReleases bool
}

func (o *ContractOptions) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}

	return defaultTimeout
}

func (o *ContractOptions) pollInterval() time.Duration {
	if o.PollInterval > 0 {
		return o.PollInterval
	}

	return defaultPollInterval
}

// ValidatePlatformContract runs a conformance suite that verifies a module CR
// on a live cluster satisfies the platform orchestration contract.
// The CR must already exist and be reconciled before calling this.
func ValidatePlatformContract(t *testing.T, c client.Client, opts ContractOptions) {
	t.Helper()

	t.Run("ObservedGeneration matches generation", func(t *testing.T) {
		t.Parallel()
		checkObservedGeneration(t, c, opts)
	})

	t.Run("Ready condition is present and True", func(t *testing.T) {
		t.Parallel()
		checkReadyCondition(t, c, opts)
	})

	t.Run("ProvisioningSucceeded condition is present", func(t *testing.T) {
		t.Parallel()
		checkConditionExists(t, c, opts, common.ConditionTypeProvisioningSucceeded)
	})

	t.Run("Singleton enforcement rejects duplicate", func(t *testing.T) {
		t.Parallel()
		checkSingletonEnforcement(t, c, opts)
	})

	if !opts.SkipReleases {
		t.Run("Releases are populated", func(t *testing.T) {
			t.Parallel()
			checkReleasesPopulated(t, c, opts)
		})
	}
}

func getCR(ctx context.Context, c client.Client, opts ContractOptions) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(opts.GVK)

	nn := types.NamespacedName{Name: opts.InstanceName}
	err := c.Get(ctx, nn, obj)

	return obj, err
}

func checkObservedGeneration(t *testing.T, c client.Client, opts ContractOptions) {
	t.Helper()

	err := wait.PollUntilContextTimeout(t.Context(), opts.pollInterval(), opts.timeout(), true,
		func(ctx context.Context) (bool, error) {
			obj, getErr := getCR(ctx, c, opts)
			if getErr != nil {
				return false, nil //nolint:nilerr // retry on transient errors
			}

			gen := obj.GetGeneration()

			observedGen, found, nestedErr := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
			if nestedErr != nil || !found {
				return false, nil //nolint:nilerr // retry until field appears
			}

			return observedGen == gen, nil
		},
	)

	require.NoError(t, err, "observedGeneration should match metadata.generation within %s", opts.timeout())
}

func checkReadyCondition(t *testing.T, c client.Client, opts ContractOptions) {
	t.Helper()

	err := wait.PollUntilContextTimeout(t.Context(), opts.pollInterval(), opts.timeout(), true,
		func(ctx context.Context) (bool, error) {
			obj, getErr := getCR(ctx, c, opts)
			if getErr != nil {
				return false, nil //nolint:nilerr // retry on transient errors
			}

			return hasConditionWithStatus(obj, string(common.ConditionTypeReady), metav1.ConditionTrue), nil
		},
	)

	require.NoError(t, err, "Ready condition should be True within %s", opts.timeout())
}

func checkConditionExists(t *testing.T, c client.Client, opts ContractOptions, condType common.ConditionType) {
	t.Helper()

	err := wait.PollUntilContextTimeout(t.Context(), opts.pollInterval(), opts.timeout(), true,
		func(ctx context.Context) (bool, error) {
			obj, getErr := getCR(ctx, c, opts)
			if getErr != nil {
				return false, nil //nolint:nilerr // retry on transient errors
			}

			return hasCondition(obj, string(condType)), nil
		},
	)

	require.NoError(t, err, "%s condition should exist within %s", condType, opts.timeout())
}

func checkSingletonEnforcement(t *testing.T, c client.Client, opts ContractOptions) {
	t.Helper()

	dup := &unstructured.Unstructured{}
	dup.SetGroupVersionKind(opts.GVK)
	dup.SetName(opts.InstanceName + "-duplicate")

	err := c.Create(t.Context(), dup)
	if err == nil {
		_ = c.Delete(t.Context(), dup)

		t.Fatal("singleton enforcement failed: duplicate CR was created without error")
	}

	assert.True(t,
		k8serr.IsForbidden(err) || k8serr.IsInvalid(err) || k8serr.IsBadRequest(err),
		"expected webhook/CEL rejection (Forbidden, Invalid, or BadRequest), got: %v", err,
	)
}

func checkReleasesPopulated(t *testing.T, c client.Client, opts ContractOptions) {
	t.Helper()

	err := wait.PollUntilContextTimeout(t.Context(), opts.pollInterval(), opts.timeout(), true,
		func(ctx context.Context) (bool, error) {
			obj, getErr := getCR(ctx, c, opts)
			if getErr != nil {
				return false, nil //nolint:nilerr // retry on transient errors
			}

			releases, found, nestedErr := unstructured.NestedSlice(obj.Object, "status", "releases")
			if nestedErr != nil || !found {
				return false, nil //nolint:nilerr // retry until field appears
			}

			return len(releases) > 0, nil
		},
	)

	require.NoError(t, err, "status.releases should be populated within %s", opts.timeout())
}

func hasCondition(obj *unstructured.Unstructured, condType string) bool {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}

	for _, c := range conditions {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}

		if ct, _ := cm["type"].(string); ct == condType {
			return true
		}
	}

	return false
}

func hasConditionWithStatus(obj *unstructured.Unstructured, condType string, status metav1.ConditionStatus) bool {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}

	for _, c := range conditions {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}

		ct, _ := cm["type"].(string)
		cs, _ := cm["status"].(string)

		if ct == condType && cs == string(status) {
			return true
		}
	}

	return false
}
