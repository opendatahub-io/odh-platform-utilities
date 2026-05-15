package action_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/action"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
)

// testPlatformObject is a minimal PlatformObject for testing.
type testPlatformObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status testStatus `json:"status,omitempty"`
}

type testStatus struct {
	common.ComponentReleaseStatus `json:",inline"`
	common.Status                 `json:",inline"`
}

func (t *testPlatformObject) GetStatus() *common.Status {
	return &t.Status.Status
}

func (t *testPlatformObject) GetConditions() []common.Condition {
	return t.Status.Conditions
}

func (t *testPlatformObject) SetConditions(c []common.Condition) {
	t.Status.Conditions = c
}

func (t *testPlatformObject) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &t.Status.ComponentReleaseStatus
}

func (t *testPlatformObject) SetReleaseStatus(s common.ComponentReleaseStatus) {
	t.Status.ComponentReleaseStatus = s
}

func (t *testPlatformObject) DeepCopyObject() runtime.Object {
	if t == nil {
		return nil
	}

	out := *t
	out.ObjectMeta = *t.DeepCopy()
	t.Status.Status.DeepCopyInto(&out.Status.Status)
	t.Status.ComponentReleaseStatus.DeepCopyInto(&out.Status.ComponentReleaseStatus)

	return &out
}

var (
	_ common.PlatformObject = (*testPlatformObject)(nil)

	errExpectedResources = errors.New("expected 2 resources from previous step")
	errRenderFailed      = errors.New("render failed")
)

func TestReconciliationRequest_ZeroValue(t *testing.T) {
	t.Parallel()

	var rr action.ReconciliationRequest

	assert.Nil(t, rr.Client)
	assert.Nil(t, rr.Instance)
	assert.Nil(t, rr.Resources)
	assert.Nil(t, rr.Conditions)
}

func TestReconciliationRequest_ManualInitialization(t *testing.T) {
	t.Parallel()

	obj := &testPlatformObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	mgr := conditions.NewManager(obj, string(common.ConditionTypeReady))

	rr := &action.ReconciliationRequest{
		Instance:   obj,
		Conditions: mgr,
		Resources: []unstructured.Unstructured{
			{Object: map[string]interface{}{"kind": "ConfigMap"}},
		},
	}

	assert.Equal(t, "test-cr", rr.Instance.GetName())
	require.Len(t, rr.Resources, 1)
	assert.Equal(t, "ConfigMap", rr.Resources[0].Object["kind"])
	assert.NotNil(t, rr.Conditions)
}

func TestFn_SignatureCompatibility(t *testing.T) {
	t.Parallel()

	var fn action.Fn = func(_ context.Context, rr *action.ReconciliationRequest) error {
		rr.Resources = append(rr.Resources, unstructured.Unstructured{
			Object: map[string]interface{}{"kind": "Service"},
		})

		return nil
	}

	rr := &action.ReconciliationRequest{}

	err := fn(context.Background(), rr)
	require.NoError(t, err)
	require.Len(t, rr.Resources, 1)
	assert.Equal(t, "Service", rr.Resources[0].Object["kind"])
}

func TestFn_PipelineAccumulatesResources(t *testing.T) {
	t.Parallel()

	render := action.Fn(func(_ context.Context, rr *action.ReconciliationRequest) error {
		rr.Resources = append(rr.Resources,
			unstructured.Unstructured{Object: map[string]interface{}{"kind": "ConfigMap"}},
			unstructured.Unstructured{Object: map[string]interface{}{"kind": "Deployment"}},
		)

		return nil
	})

	verify := action.Fn(func(_ context.Context, rr *action.ReconciliationRequest) error {
		if len(rr.Resources) != 2 {
			return errExpectedResources
		}

		return nil
	})

	rr := &action.ReconciliationRequest{}
	pipeline := []action.Fn{render, verify}

	for _, step := range pipeline {
		err := step(context.Background(), rr)
		require.NoError(t, err)
	}

	require.Len(t, rr.Resources, 2)
	assert.Equal(t, "ConfigMap", rr.Resources[0].Object["kind"])
	assert.Equal(t, "Deployment", rr.Resources[1].Object["kind"])
}

func TestFn_ErrorStopsPipeline(t *testing.T) {
	t.Parallel()

	failing := action.Fn(func(_ context.Context, _ *action.ReconciliationRequest) error {
		return errRenderFailed
	})

	unreachable := action.Fn(func(_ context.Context, _ *action.ReconciliationRequest) error {
		t.Fatal("step after error must not execute")

		return nil
	})

	rr := &action.ReconciliationRequest{}
	pipeline := []action.Fn{failing, unreachable}

	for _, step := range pipeline {
		err := step(context.Background(), rr)
		if err != nil {
			assert.ErrorIs(t, err, errRenderFailed)

			return
		}
	}

	t.Fatal("pipeline should have returned an error")
}

func TestFn_ConditionsIntegration(t *testing.T) {
	t.Parallel()

	obj := &testPlatformObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	mgr := conditions.NewManager(obj,
		string(common.ConditionTypeReady),
		string(common.ConditionTypeProvisioningSucceeded),
	)

	rr := &action.ReconciliationRequest{
		Instance:   obj,
		Conditions: mgr,
	}

	step := action.Fn(func(_ context.Context, rr *action.ReconciliationRequest) error {
		rr.Conditions.MarkTrue(string(common.ConditionTypeProvisioningSucceeded))

		return nil
	})

	err := step(context.Background(), rr)
	require.NoError(t, err)

	assert.True(t, rr.Conditions.IsHappy())
}
