package precondition_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/action"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/precondition"
)

//nolint:gochecknoglobals // Immutable test fixtures.
var (
	testCRDGK = schema.GroupKind{
		Group: "testprecondition.opendatahub.io",
		Kind:  "TestPreConditionResource",
	}

	testCRDGK2 = schema.GroupKind{
		Group: "testprecondition.opendatahub.io",
		Kind:  "TestPreConditionResource2",
	}
)

//nolint:unparam // established varies across test files sharing this helper pattern.
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

func TestMonitorCRD_Present(t *testing.T) {
	t.Parallel()

	cli := fake.NewClientBuilder().
		WithScheme(runtime.NewScheme()).
		WithObjects(newCRD("testpreconditionresources.testprecondition.opendatahub.io", true)).
		Build()

	instance := &testPlatformObject{ObjectMeta: metav1.ObjectMeta{Name: "test-obj"}}
	rr := &action.ReconciliationRequest{
		Client:   cli,
		Instance: instance,
		Conditions: conditions.NewManager(
			instance, string(common.ConditionTypeReady),
			precondition.ConditionTypeDependenciesAvailable,
		),
	}

	pcs := []precondition.PreCondition{
		precondition.MonitorCRD(testCRDGK),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionTrue, got.Status)
}

func TestMonitorCRDs_AllPresent(t *testing.T) {
	t.Parallel()

	cli := fake.NewClientBuilder().
		WithScheme(runtime.NewScheme()).
		WithObjects(
			newCRD("testpreconditionresources.testprecondition.opendatahub.io", true),
			newCRD("testpreconditionresource2s.testprecondition.opendatahub.io", true),
		).
		Build()

	instance := &testPlatformObject{ObjectMeta: metav1.ObjectMeta{Name: "test-obj"}}
	rr := &action.ReconciliationRequest{
		Client:   cli,
		Instance: instance,
		Conditions: conditions.NewManager(
			instance, string(common.ConditionTypeReady),
			precondition.ConditionTypeDependenciesAvailable,
		),
	}

	pcs := []precondition.PreCondition{
		precondition.MonitorCRDs([]schema.GroupKind{testCRDGK, testCRDGK2}),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionTrue, got.Status)
}

func TestMonitorCRD_Absent(t *testing.T) {
	t.Parallel()

	cli := fake.NewClientBuilder().
		WithScheme(runtime.NewScheme()).
		Build()

	instance := &testPlatformObject{ObjectMeta: metav1.ObjectMeta{Name: "test-obj"}}
	rr := &action.ReconciliationRequest{
		Client:   cli,
		Instance: instance,
		Conditions: conditions.NewManager(
			instance, string(common.ConditionTypeReady),
			precondition.ConditionTypeDependenciesAvailable,
		),
	}

	absentGK := schema.GroupKind{
		Group: "absent.opendatahub.io",
		Kind:  "AbsentResource",
	}

	pcs := []precondition.PreCondition{
		precondition.MonitorCRD(absentGK),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
	assert.Contains(t, got.Message, "AbsentResource")
	assert.Contains(t, got.Message, "CRD not found")
}

func TestMonitorCRDs_SomeAbsent(t *testing.T) {
	t.Parallel()

	cli := fake.NewClientBuilder().
		WithScheme(runtime.NewScheme()).
		WithObjects(newCRD("testpreconditionresources.testprecondition.opendatahub.io", true)).
		Build()

	instance := &testPlatformObject{ObjectMeta: metav1.ObjectMeta{Name: "test-obj"}}
	rr := &action.ReconciliationRequest{
		Client:   cli,
		Instance: instance,
		Conditions: conditions.NewManager(
			instance, string(common.ConditionTypeReady),
			precondition.ConditionTypeDependenciesAvailable,
		),
	}

	absentGK := schema.GroupKind{
		Group: "absent.opendatahub.io",
		Kind:  "AbsentResource",
	}

	pcs := []precondition.PreCondition{
		precondition.MonitorCRDs([]schema.GroupKind{testCRDGK, absentGK}),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
	assert.Contains(t, got.Message, "AbsentResource")
	assert.NotContains(t, got.Message, "TestPreConditionResource")
}

func TestMonitorCRDs_EmptySlice(t *testing.T) {
	t.Parallel()

	cli := fake.NewClientBuilder().
		WithScheme(runtime.NewScheme()).
		Build()

	instance := &testPlatformObject{ObjectMeta: metav1.ObjectMeta{Name: "test-obj"}}
	rr := &action.ReconciliationRequest{
		Client:   cli,
		Instance: instance,
		Conditions: conditions.NewManager(
			instance, string(common.ConditionTypeReady),
			precondition.ConditionTypeDependenciesAvailable,
		),
	}

	pcs := []precondition.PreCondition{
		precondition.MonitorCRDs(nil),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionUnknown, got.Status)
	assert.Contains(t, got.Message, "empty GroupKind list")
}

func TestMonitorCRD_IntegrationWithRunAll(t *testing.T) {
	t.Parallel()

	cli := fake.NewClientBuilder().
		WithScheme(runtime.NewScheme()).
		WithObjects(newCRD("testpreconditionresources.testprecondition.opendatahub.io", true)).
		Build()

	absentGK := schema.GroupKind{
		Group: "absent.opendatahub.io",
		Kind:  "AbsentResource",
	}

	instance := &testPlatformObject{ObjectMeta: metav1.ObjectMeta{Name: "test-obj"}}
	condManager := conditions.NewManager(
		instance, string(common.ConditionTypeReady),
		precondition.ConditionTypeDependenciesAvailable,
	)
	rr := &action.ReconciliationRequest{Client: cli, Instance: instance, Conditions: condManager}

	pcs := []precondition.PreCondition{
		precondition.MonitorCRD(testCRDGK),
		precondition.MonitorCRD(absentGK),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)
	assert.False(t, shouldStop)

	got := condManager.GetCondition(precondition.ConditionTypeDependenciesAvailable)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
	assert.Contains(t, got.Message, "AbsentResource")
	assert.NotContains(t, got.Message, "TestPreConditionResource")
}
