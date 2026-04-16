package common_test

import (
	"testing"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// --- Test struct implementing PlatformObject ---

type testComponentStatus struct {
	common.ComponentReleaseStatus `json:",inline"`
	common.Status                 `json:",inline"`
}

type testComponent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   testComponentSpec   `json:"spec,omitempty"`
	Status testComponentStatus `json:"status,omitempty"`
}

type testComponentSpec struct {
	common.ManagementSpec `json:",inline"`
}

func (t *testComponent) GetStatus() *common.Status {
	return &t.Status.Status
}

func (t *testComponent) GetConditions() []common.Condition {
	return t.Status.Conditions
}

func (t *testComponent) SetConditions(conditions []common.Condition) {
	t.Status.Conditions = conditions
}

func (t *testComponent) GetReleaseStatus() *common.ComponentReleaseStatus {
	return &t.Status.ComponentReleaseStatus
}

func (t *testComponent) SetReleaseStatus(status common.ComponentReleaseStatus) {
	t.Status.ComponentReleaseStatus = status
}

func (t *testComponent) DeepCopyObject() runtime.Object {
	if t == nil {
		return nil
	}

	out := new(testComponent)
	t.deepCopyInto(out)

	return out
}

func (t *testComponent) deepCopyInto(out *testComponent) {
	*out = *t
	out.ObjectMeta = *t.DeepCopy()
	t.Status.Status.DeepCopyInto(&out.Status.Status)
	t.Status.ComponentReleaseStatus.DeepCopyInto(&out.Status.ComponentReleaseStatus)
}

// Compile-time interface compliance: testComponent must satisfy PlatformObject.
var _ common.PlatformObject = (*testComponent)(nil)

func TestWithStatus(t *testing.T) {
	t.Parallel()

	obj := &testComponent{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "default-test",
			Generation: 3,
		},
	}

	status := obj.GetStatus()
	require.NotNil(t, status)
	assert.Equal(t, common.Phase(""), status.Phase)

	status.Phase = common.PhaseReady
	status.ObservedGeneration = 3

	assert.Equal(t, common.PhaseReady, obj.GetStatus().Phase)
	assert.Equal(t, int64(3), obj.GetStatus().ObservedGeneration)
}

func TestConditionsAccessor(t *testing.T) {
	t.Parallel()

	obj := &testComponent{}
	assert.Empty(t, obj.GetConditions())

	conditions := []common.Condition{
		{
			Type:               string(common.ConditionTypeReady),
			Status:             metav1.ConditionTrue,
			Reason:             "AllComponentsReady",
			Message:            "All components are running",
			Severity:           common.ConditionSeverityInfo,
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: 1,
		},
		{
			Type:               string(common.ConditionTypeDegraded),
			Status:             metav1.ConditionFalse,
			Reason:             "NotDegraded",
			Severity:           common.ConditionSeverityInfo,
			LastTransitionTime: metav1.Now(),
		},
	}

	obj.SetConditions(conditions)
	got := obj.GetConditions()
	assert.Len(t, got, 2)
	assert.Equal(t, string(common.ConditionTypeReady), got[0].Type)
	assert.Equal(t, metav1.ConditionTrue, got[0].Status)
	assert.Equal(t, string(common.ConditionTypeDegraded), got[1].Type)
}

func TestWithReleases(t *testing.T) {
	t.Parallel()

	obj := &testComponent{}
	releases := obj.GetReleaseStatus()
	require.NotNil(t, releases)
	assert.Empty(t, releases.Releases)

	newReleases := common.ComponentReleaseStatus{
		Releases: []common.ComponentRelease{
			{Name: "dashboard", Version: "v2.16.0", RepoURL: "https://github.com/opendatahub-io/odh-dashboard"},
			{Name: "notebook-controller", Version: "v1.9.0"},
		},
	}
	obj.SetReleaseStatus(newReleases)

	got := obj.GetReleaseStatus()
	assert.Len(t, got.Releases, 2)
	assert.Equal(t, "dashboard", got.Releases[0].Name)
	assert.Equal(t, "v2.16.0", got.Releases[0].Version)
}

func TestManagementStateConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, common.Managed, common.ManagementState("Managed"))
	assert.Equal(t, common.Removed, common.ManagementState("Removed"))
}

func TestConditionSeverityConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, common.ConditionSeverityError, common.ConditionSeverity(""))
	assert.Equal(t, common.ConditionSeverityInfo, common.ConditionSeverity("Info"))
}

func TestConditionTypeConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, common.ConditionTypeReady, common.ConditionType("Ready"))
	assert.Equal(t, common.ConditionTypeProvisioningSucceeded, common.ConditionType("ProvisioningSucceeded"))
	assert.Equal(t, common.ConditionTypeDegraded, common.ConditionType("Degraded"))
}

func TestPhaseConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, common.PhaseReady, common.Phase("Ready"))
	assert.Equal(t, common.PhaseNotReady, common.Phase("Not Ready"))
}

func TestDeepCopyStatus(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	original := &common.Status{
		Phase:              common.PhaseReady,
		ObservedGeneration: 5,
		Conditions: []common.Condition{
			{Type: string(common.ConditionTypeReady), Status: metav1.ConditionTrue, LastTransitionTime: now},
		},
	}

	copied := original.DeepCopy()
	require.NotNil(t, copied)
	assert.Equal(t, original, copied)

	copied.Conditions[0].Status = metav1.ConditionFalse
	assert.Equal(t, metav1.ConditionTrue, original.Conditions[0].Status,
		"modifying copy must not affect original")
}

func TestDeepCopyCondition(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	original := &common.Condition{
		Type:               string(common.ConditionTypeReady),
		Status:             metav1.ConditionTrue,
		Reason:             "Test",
		LastTransitionTime: now,
	}

	copied := original.DeepCopy()
	require.NotNil(t, copied)
	assert.Equal(t, original, copied)
}

func TestDeepCopyComponentRelease(t *testing.T) {
	t.Parallel()

	original := &common.ComponentRelease{
		Name:    "dashboard",
		Version: "v2.0.0",
		RepoURL: "https://github.com/example/dashboard",
	}

	copied := original.DeepCopy()
	require.NotNil(t, copied)
	assert.Equal(t, original, copied)
}

func TestDeepCopyComponentReleaseStatus(t *testing.T) {
	t.Parallel()

	original := &common.ComponentReleaseStatus{
		Releases: []common.ComponentRelease{
			{Name: "a", Version: "v1"},
			{Name: "b", Version: "v2"},
		},
	}

	copied := original.DeepCopy()
	require.NotNil(t, copied)
	assert.Equal(t, original, copied)

	copied.Releases[0].Version = "v99"
	assert.Equal(t, "v1", original.Releases[0].Version,
		"modifying copy must not affect original")
}

func TestDeepCopyManagementSpec(t *testing.T) {
	t.Parallel()

	original := &common.ManagementSpec{ManagementState: common.Managed}

	copied := original.DeepCopy()
	require.NotNil(t, copied)
	assert.Equal(t, original, copied)
}

func TestDeepCopyNil(t *testing.T) {
	t.Parallel()

	var s *common.Status
	assert.Nil(t, s.DeepCopy())

	var c *common.Condition
	assert.Nil(t, c.DeepCopy())

	var cr *common.ComponentRelease
	assert.Nil(t, cr.DeepCopy())

	var crs *common.ComponentReleaseStatus
	assert.Nil(t, crs.DeepCopy())

	var ms *common.ManagementSpec
	assert.Nil(t, ms.DeepCopy())
}

func TestClientObjectCompliance(t *testing.T) {
	t.Parallel()

	obj := &testComponent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "test.example.com/v1",
			Kind:       "TestComponent",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "default-test",
		},
	}

	assert.Equal(t, "default-test", obj.GetName())
	assert.Equal(t, schema.GroupVersionKind{Group: "test.example.com", Version: "v1", Kind: "TestComponent"},
		obj.GetObjectKind().GroupVersionKind())

	clone := obj.DeepCopyObject()
	require.NotNil(t, clone)

	cloned, ok := clone.(*testComponent)
	require.True(t, ok)
	assert.Equal(t, "default-test", cloned.GetName())
}
