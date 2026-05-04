package conditions_test

import (
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
)

var (
	errDeploymentFailed = errors.New("deployment failed")
	errTestErrorMessage = errors.New("test error message")
)

// testAccessor is a minimal ConditionsAccessor implementation for testing.
type testAccessor struct {
	conditions []common.Condition
}

func (t *testAccessor) GetConditions() []common.Condition {
	return t.conditions
}

func (t *testAccessor) SetConditions(c []common.Condition) {
	t.conditions = c
}

// assertDependentsInitialized verifies all dependents are initialized to Unknown.
func assertDependentsInitialized(t *testing.T, accessor common.ConditionsAccessor, dependents []string) {
	t.Helper()

	for _, dep := range dependents {
		depCond := conditions.FindStatusCondition(accessor, dep)
		if depCond == nil {
			t.Errorf("dependent %s not created", dep)
			continue
		}

		if depCond.Status != metav1.ConditionUnknown {
			t.Errorf("expected dependent %s status Unknown, got %s", dep, depCond.Status)
		}
	}
}

// assertHappyCondition verifies the happy condition matches expected values.
func assertHappyCondition(
	t *testing.T, mgr *conditions.Manager, status metav1.ConditionStatus, reason, message string,
) {
	t.Helper()

	happy := mgr.GetTopLevelCondition()
	if happy == nil {
		t.Fatal("happy condition not found")
	}

	if happy.Status != status {
		t.Errorf("happy status = %s, want %s", happy.Status, status)
	}

	if happy.Reason != reason {
		t.Errorf("happy reason = %s, want %s", happy.Reason, reason)
	}

	if message != "" && happy.Message != message {
		t.Errorf("happy message = %s, want %s", happy.Message, message)
	}
}

func TestNewManager(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		happy      string
		dependents []string
		wantHappy  bool
		wantCount  int
	}{
		{
			name:       "creates manager with happy and two dependents",
			happy:      "Ready",
			dependents: []string{"ProvisioningSucceeded", "Degraded"},
			wantHappy:  false,
			wantCount:  3, // happy + 2 dependents
		},
		{
			name:      "creates manager with only happy condition",
			happy:     "Ready",
			wantHappy: false,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			accessor := &testAccessor{}
			mgr := conditions.NewManager(accessor, tt.happy, tt.dependents...)

			if mgr == nil {
				t.Fatal("NewManager returned nil")
			}

			conds := accessor.GetConditions()
			if len(conds) != tt.wantCount {
				t.Errorf("expected %d conditions, got %d", tt.wantCount, len(conds))
			}

			happyCond := conditions.FindStatusCondition(accessor, tt.happy)
			if happyCond == nil || happyCond.Status != metav1.ConditionUnknown {
				t.Fatal("happy condition not properly initialized")
			}

			assertDependentsInitialized(t, accessor, tt.dependents)

			if mgr.IsHappy() != tt.wantHappy {
				t.Errorf("IsHappy() = %v, want %v", mgr.IsHappy(), tt.wantHappy)
			}
		})
	}
}

func TestManager_MarkTrue(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "ProvisioningSucceeded")

	mgr.MarkTrue("ProvisioningSucceeded",
		conditions.WithReason("Success"), conditions.WithMessage("All resources deployed"))

	cond := conditions.FindStatusCondition(accessor, "ProvisioningSucceeded")
	if cond == nil {
		t.Fatal("condition not found")
	}

	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected status True, got %s", cond.Status)
	}

	if cond.Reason != "Success" {
		t.Errorf("expected reason 'Success', got %s", cond.Reason)
	}

	if cond.Message != "All resources deployed" {
		t.Errorf("expected message 'All resources deployed', got %s", cond.Message)
	}
}

func TestManager_MarkFalse(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "ProvisioningSucceeded")

	mgr.MarkFalse("ProvisioningSucceeded", conditions.WithError(errDeploymentFailed))

	cond := conditions.FindStatusCondition(accessor, "ProvisioningSucceeded")
	if cond == nil {
		t.Fatal("condition not found")
	}

	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected status False, got %s", cond.Status)
	}

	if cond.Severity != common.ConditionSeverityError {
		t.Errorf("expected severity Error, got %s", cond.Severity)
	}

	if cond.Message != errDeploymentFailed.Error() {
		t.Errorf("expected message '%s', got %s", errDeploymentFailed.Error(), cond.Message)
	}
}

func TestManager_RecomputeHappiness_AllHealthy(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "Dep1", "Dep2")
	mgr.MarkTrue("Dep1", conditions.WithReason("Ready"))
	mgr.MarkTrue("Dep2", conditions.WithReason("Ready"))
	mgr.RecomputeHappiness("")

	assertHappyCondition(t, mgr, metav1.ConditionTrue, "AllDependentsHealthy", "")
}

func TestManager_RecomputeHappiness_OneFalse(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "Dep1", "Dep2")
	mgr.MarkTrue("Dep1", conditions.WithReason("Ready"))
	mgr.MarkFalse("Dep2", conditions.WithReason("Failed"), conditions.WithMessage("Something went wrong"))
	mgr.RecomputeHappiness("")

	assertHappyCondition(t, mgr, metav1.ConditionFalse, "Failed", "Something went wrong")
}

func TestManager_RecomputeHappiness_FalsePriority(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "Dep1", "Dep2")
	mgr.MarkUnknown("Dep1", conditions.WithReason("Pending"))
	mgr.MarkFalse("Dep2", conditions.WithReason("Failed"), conditions.WithMessage("Error occurred"))
	mgr.RecomputeHappiness("")

	assertHappyCondition(t, mgr, metav1.ConditionFalse, "Failed", "Error occurred")
}

func TestManager_RecomputeHappiness_InfoSeverity(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "Dep1", "Dep2")
	mgr.MarkTrue("Dep1", conditions.WithReason("Ready"))
	mgr.MarkFalse("Dep2", conditions.WithReason("Info"), conditions.WithSeverity(common.ConditionSeverityInfo))
	mgr.RecomputeHappiness("")

	assertHappyCondition(t, mgr, metav1.ConditionTrue, "AllDependentsHealthy", "")
}

func TestManager_RecomputeHappiness_EmptySeverity(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "Dep1")
	mgr.MarkFalse("Dep1", conditions.WithReason("Failed"), conditions.WithMessage("Deployment failed"))

	cond := conditions.FindStatusCondition(accessor, "Dep1")
	cond.Severity = ""
	conditions.SetStatusCondition(accessor, *cond)

	mgr.RecomputeHappiness("")

	assertHappyCondition(t, mgr, metav1.ConditionFalse, "Failed", "Deployment failed")
}

func TestManager_MarkFrom(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "SubcomponentReady")

	// Simulate a sub-component condition
	source := &common.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "SubcomponentFailed",
		Message:            "Subcomponent deployment failed",
		Severity:           common.ConditionSeverityError,
		ObservedGeneration: 5,
	}

	mgr.MarkFrom("SubcomponentReady", source)

	cond := conditions.FindStatusCondition(accessor, "SubcomponentReady")
	if cond == nil {
		t.Fatal("condition not found")
	}

	if cond.Status != source.Status {
		t.Errorf("status = %s, want %s", cond.Status, source.Status)
	}

	if cond.Reason != source.Reason {
		t.Errorf("reason = %s, want %s", cond.Reason, source.Reason)
	}

	if cond.Message != source.Message {
		t.Errorf("message = %s, want %s", cond.Message, source.Message)
	}

	if cond.Severity != source.Severity {
		t.Errorf("severity = %s, want %s", cond.Severity, source.Severity)
	}

	if cond.ObservedGeneration != 0 {
		t.Errorf("observedGeneration = %d, want 0 (not copied from source)", cond.ObservedGeneration)
	}
}

func TestManager_MarkFrom_NilSource(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "SubcomponentReady")

	mgr.MarkFrom("SubcomponentReady", nil)

	cond := conditions.FindStatusCondition(accessor, "SubcomponentReady")
	if cond == nil {
		t.Fatal("condition not found")
	}

	if cond.Status != metav1.ConditionUnknown {
		t.Errorf("expected status Unknown after nil MarkFrom, got %s", cond.Status)
	}
}

func TestManager_Sort(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "ProvisioningSucceeded", "Degraded")

	mgr.MarkTrue("AnotherCondition", conditions.WithReason("Ready"))
	mgr.MarkTrue("ZebraCondition", conditions.WithReason("Ready"))

	mgr.Sort()

	conds := accessor.GetConditions()
	if len(conds) < 3 {
		t.Fatal("not enough conditions")
	}

	expectedOrder := []string{"Ready", "ProvisioningSucceeded", "Degraded", "AnotherCondition", "ZebraCondition"}
	for i, expected := range expectedOrder {
		if i >= len(conds) {
			t.Errorf("missing condition at index %d, expected %s", i, expected)
			continue
		}

		if conds[i].Type != expected {
			t.Errorf("condition[%d] = %s, want %s", i, conds[i].Type, expected)
		}
	}
}

func TestManager_Reset(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "Dep1", "Dep2")

	mgr.MarkTrue("Dep1")
	mgr.MarkTrue("Dep2")

	if len(accessor.GetConditions()) == 0 {
		t.Fatal("expected conditions before reset")
	}

	mgr.Reset()

	if len(accessor.GetConditions()) != 0 {
		t.Errorf("expected 0 conditions after reset, got %d", len(accessor.GetConditions()))
	}
}

func TestManager_ClearCondition(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready", "Dep1")

	mgr.MarkTrue("Dep1")

	if conditions.FindStatusCondition(accessor, "Dep1") == nil {
		t.Fatal("Dep1 should exist before clear")
	}

	mgr.ClearCondition("Dep1")

	if conditions.FindStatusCondition(accessor, "Dep1") != nil {
		t.Error("Dep1 should not exist after clear")
	}
}

func TestWithOptions(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready")

	mgr.MarkTrue("TestCondition",
		conditions.WithReason("CustomReason"),
		conditions.WithMessage("Message with %s", "formatting"),
		conditions.WithObservedGeneration(42),
		conditions.WithSeverity(common.ConditionSeverityInfo),
	)

	cond := conditions.FindStatusCondition(accessor, "TestCondition")
	if cond == nil {
		t.Fatal("condition not found")
	}

	if cond.Reason != "CustomReason" {
		t.Errorf("reason = %s, want CustomReason", cond.Reason)
	}

	if cond.Message != "Message with formatting" {
		t.Errorf("message = %s, want 'Message with formatting'", cond.Message)
	}

	if cond.ObservedGeneration != 42 {
		t.Errorf("observedGeneration = %d, want 42", cond.ObservedGeneration)
	}

	if cond.Severity != common.ConditionSeverityInfo {
		t.Errorf("severity = %s, want Info", cond.Severity)
	}
}

func TestWithError(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready")

	mgr.MarkFalse("TestCondition", conditions.WithError(errTestErrorMessage))

	cond := conditions.FindStatusCondition(accessor, "TestCondition")
	if cond == nil {
		t.Fatal("condition not found")
	}

	if cond.Severity != common.ConditionSeverityError {
		t.Errorf("severity = %s, want Error", cond.Severity)
	}

	if cond.Reason != "Error" {
		t.Errorf("reason = %s, want Error", cond.Reason)
	}

	if cond.Message != "test error message" {
		t.Errorf("message = %s, want 'test error message'", cond.Message)
	}
}

func TestWithError_NilError(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}
	mgr := conditions.NewManager(accessor, "Ready")

	mgr.MarkFalse("TestCondition", conditions.WithError(nil))

	cond := conditions.FindStatusCondition(accessor, "TestCondition")
	if cond == nil {
		t.Fatal("condition not found")
	}

	if cond.Message != "" {
		t.Errorf("expected empty message for nil error, got %s", cond.Message)
	}
}

func TestManager_IsHappy_NilAccessor(t *testing.T) {
	t.Parallel()

	mgr := conditions.NewManager(nil, "Ready")

	if mgr.IsHappy() {
		t.Error("IsHappy() should return false for nil accessor")
	}
}
