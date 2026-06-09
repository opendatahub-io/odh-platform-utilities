package precondition_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/action"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/precondition"
)

//nolint:gochecknoglobals // Immutable test fixtures.
var (
	testGVK = schema.GroupVersionKind{
		Group:   "test.opendatahub.io",
		Version: "v1",
		Kind:    "TestResource",
	}

	testGVK2 = schema.GroupVersionKind{
		Group:   "test.opendatahub.io",
		Version: "v1",
		Kind:    "TestResource2",
	}

	absentGVK = schema.GroupVersionKind{
		Group:   "absent.opendatahub.io",
		Version: "v1",
		Kind:    "AbsentResource",
	}
)

type erroringMapper struct {
	meta.RESTMapper

	errGVK schema.GroupVersionKind
}

func (m *erroringMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	if gk == m.errGVK.GroupKind() {
		return nil, errTest
	}

	return m.RESTMapper.RESTMapping(gk, versions...)
}

func crdClient(gvks ...schema.GroupVersionKind) client.Client {
	mapper := meta.NewDefaultRESTMapper(nil)
	for _, gvk := range gvks {
		mapper.Add(gvk, meta.RESTScopeNamespace)
	}

	return fake.NewClientBuilder().WithRESTMapper(mapper).Build()
}

func crdClientWithError(errGVK schema.GroupVersionKind, gvks ...schema.GroupVersionKind) client.Client {
	base := meta.NewDefaultRESTMapper(nil)
	for _, gvk := range gvks {
		base.Add(gvk, meta.RESTScopeNamespace)
	}

	return fake.NewClientBuilder().
		WithRESTMapper(&erroringMapper{RESTMapper: base, errGVK: errGVK}).
		Build()
}

func newMonitorRR(cli client.Client) *action.ReconciliationRequest {
	instance := &testPlatformObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test-obj"},
	}

	return &action.ReconciliationRequest{
		Client:   cli,
		Instance: instance,
		Conditions: conditions.NewManager(
			instance, string(common.ConditionTypeReady),
			precondition.ConditionTypeDependenciesAvailable,
		),
	}
}

func TestMonitorCRD_Present(t *testing.T) {
	t.Parallel()

	rr := newMonitorRR(crdClient(testGVK))

	pcs := []precondition.PreCondition{
		precondition.MonitorCRD(testGVK),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(
		precondition.ConditionTypeDependenciesAvailable,
	)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionTrue, got.Status)
}

func TestMonitorCRDs_AllPresent(t *testing.T) {
	t.Parallel()

	rr := newMonitorRR(crdClient(testGVK, testGVK2))

	pcs := []precondition.PreCondition{
		precondition.MonitorCRDs(
			[]schema.GroupVersionKind{testGVK, testGVK2},
		),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(
		precondition.ConditionTypeDependenciesAvailable,
	)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionTrue, got.Status)
}

func TestMonitorCRD_Absent(t *testing.T) {
	t.Parallel()

	rr := newMonitorRR(crdClient())

	pcs := []precondition.PreCondition{
		precondition.MonitorCRD(absentGVK),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(
		precondition.ConditionTypeDependenciesAvailable,
	)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
	assert.Contains(t, got.Message, "AbsentResource")
	assert.Contains(t, got.Message, "CRD not found")
}

func TestMonitorCRDs_SomeAbsent(t *testing.T) {
	t.Parallel()

	rr := newMonitorRR(crdClient(testGVK))

	pcs := []precondition.PreCondition{
		precondition.MonitorCRDs(
			[]schema.GroupVersionKind{testGVK, absentGVK},
		),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(
		precondition.ConditionTypeDependenciesAvailable,
	)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
	assert.Contains(t, got.Message, "AbsentResource")
	assert.NotContains(t, got.Message, "TestResource")
}

func TestMonitorCRDs_EmptySlice(t *testing.T) {
	t.Parallel()

	rr := newMonitorRR(crdClient())

	pcs := []precondition.PreCondition{
		precondition.MonitorCRDs(nil),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(
		precondition.ConditionTypeDependenciesAvailable,
	)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionUnknown, got.Status)
	assert.Contains(t, got.Message, "empty GroupVersionKind list")
}

func TestMonitorCRD_IntegrationWithRunAll(t *testing.T) {
	t.Parallel()

	rr := newMonitorRR(crdClient(testGVK))

	pcs := []precondition.PreCondition{
		precondition.MonitorCRD(testGVK),
		precondition.MonitorCRD(absentGVK),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)
	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(
		precondition.ConditionTypeDependenciesAvailable,
	)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
	assert.Contains(t, got.Message, "AbsentResource")
	assert.NotContains(t, got.Message, "TestResource")
}

func TestMonitorCRD_UnexpectedMapperError(t *testing.T) {
	t.Parallel()

	rr := newMonitorRR(crdClientWithError(testGVK))

	pcs := []precondition.PreCondition{
		precondition.MonitorCRD(testGVK),
	}

	shouldStop := precondition.RunAll(t.Context(), rr, "", pcs)

	assert.False(t, shouldStop)

	got := rr.Conditions.GetCondition(
		precondition.ConditionTypeDependenciesAvailable,
	)
	require.NotNil(t, got)
	assert.Equal(t, metav1.ConditionUnknown, got.Status)
	assert.Contains(t, got.Message, "failed to check CRD presence")
}
