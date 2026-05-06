package conditions_test

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
)

func TestSetStatusCondition_Add(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	cond := common.Condition{
		Type:    "TestCondition",
		Status:  metav1.ConditionTrue,
		Reason:  "Success",
		Message: "Test message",
	}

	modified := conditions.SetStatusCondition(accessor, cond)
	if !modified {
		t.Error("expected SetStatusCondition to return true when adding new condition")
	}

	conds := accessor.GetConditions()
	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conds))
	}

	if conds[0].Type != "TestCondition" {
		t.Errorf("expected type TestCondition, got %s", conds[0].Type)
	}

	if conds[0].LastTransitionTime.IsZero() {
		t.Error("expected LastTransitionTime to be set")
	}
}

func TestSetStatusCondition_Update(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	// Add initial condition
	initial := common.Condition{
		Type:               "TestCondition",
		Status:             metav1.ConditionTrue,
		Reason:             "Initial",
		Message:            "Initial message",
		LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
	}
	conditions.SetStatusCondition(accessor, initial)

	// Update with different status (should update LastTransitionTime)
	updated := common.Condition{
		Type:    "TestCondition",
		Status:  metav1.ConditionFalse,
		Reason:  "Updated",
		Message: "Updated message",
	}

	modified := conditions.SetStatusCondition(accessor, updated)
	if !modified {
		t.Error("expected SetStatusCondition to return true when updating condition")
	}

	conds := accessor.GetConditions()
	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conds))
	}

	result := conds[0]
	if result.Status != metav1.ConditionFalse {
		t.Errorf("expected status False, got %s", result.Status)
	}

	if result.Reason != "Updated" {
		t.Errorf("expected reason Updated, got %s", result.Reason)
	}

	if result.LastTransitionTime.Equal(&initial.LastTransitionTime) {
		t.Error("expected LastTransitionTime to be updated when status changes")
	}
}

func TestSetStatusCondition_UpdateReasonOnly(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	// Add initial condition
	initial := common.Condition{
		Type:               "TestCondition",
		Status:             metav1.ConditionTrue,
		Reason:             "Initial",
		Message:            "Initial message",
		LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
	}
	conditions.SetStatusCondition(accessor, initial)

	initialTime := accessor.GetConditions()[0].LastTransitionTime

	// Small sleep to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Update with same status but different reason (should NOT update LastTransitionTime)
	updated := common.Condition{
		Type:    "TestCondition",
		Status:  metav1.ConditionTrue,
		Reason:  "UpdatedReason",
		Message: "Updated message",
	}

	modified := conditions.SetStatusCondition(accessor, updated)
	if !modified {
		t.Error("expected SetStatusCondition to return true when updating reason/message")
	}

	conds := accessor.GetConditions()
	result := conds[0]

	if result.Reason != "UpdatedReason" {
		t.Errorf("expected reason UpdatedReason, got %s", result.Reason)
	}

	if !result.LastTransitionTime.Equal(&initialTime) {
		t.Error("expected LastTransitionTime to be preserved when status doesn't change")
	}
}

func TestSetStatusCondition_NoChange(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	initial := common.Condition{
		Type:               "TestCondition",
		Status:             metav1.ConditionTrue,
		Reason:             "Success",
		Message:            "Test message",
		ObservedGeneration: 1,
		Severity:           common.ConditionSeverityInfo,
	}
	conditions.SetStatusCondition(accessor, initial)

	// Try to set the exact same condition
	modified := conditions.SetStatusCondition(accessor, initial)
	if modified {
		t.Error("expected SetStatusCondition to return false when condition is unchanged")
	}
}

func TestSetStatusCondition_NilAccessor(t *testing.T) {
	t.Parallel()

	cond := common.Condition{
		Type:   "TestCondition",
		Status: metav1.ConditionTrue,
	}

	modified := conditions.SetStatusCondition(nil, cond)
	if modified {
		t.Error("expected SetStatusCondition to return false for nil accessor")
	}
}

func TestRemoveStatusCondition(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	// Add some conditions
	conditions.SetStatusCondition(accessor, common.Condition{Type: "Cond1", Status: metav1.ConditionTrue})
	conditions.SetStatusCondition(accessor, common.Condition{Type: "Cond2", Status: metav1.ConditionTrue})
	conditions.SetStatusCondition(accessor, common.Condition{Type: "Cond3", Status: metav1.ConditionTrue})

	// Remove Cond2
	removed := conditions.RemoveStatusCondition(accessor, "Cond2")
	if !removed {
		t.Error("expected RemoveStatusCondition to return true")
	}

	conds := accessor.GetConditions()
	if len(conds) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(conds))
	}

	// Verify Cond2 is gone
	for _, cond := range conds {
		if cond.Type == "Cond2" {
			t.Error("Cond2 should have been removed")
		}
	}

	// Verify others remain
	if !conditionExists(conds, "Cond1") {
		t.Error("Cond1 should still exist")
	}

	if !conditionExists(conds, "Cond3") {
		t.Error("Cond3 should still exist")
	}
}

func TestRemoveStatusCondition_NotFound(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	conditions.SetStatusCondition(accessor, common.Condition{Type: "Cond1", Status: metav1.ConditionTrue})

	removed := conditions.RemoveStatusCondition(accessor, "NonExistent")
	if removed {
		t.Error("expected RemoveStatusCondition to return false for non-existent condition")
	}

	if len(accessor.GetConditions()) != 1 {
		t.Error("expected existing conditions to remain unchanged")
	}
}

func TestRemoveStatusCondition_NilAccessor(t *testing.T) {
	t.Parallel()

	removed := conditions.RemoveStatusCondition(nil, "TestCondition")
	if removed {
		t.Error("expected RemoveStatusCondition to return false for nil accessor")
	}
}

func TestFindStatusCondition(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	original := common.Condition{
		Type:               "TestCondition",
		Status:             metav1.ConditionTrue,
		Reason:             "Success",
		Message:            "Test message",
		Severity:           common.ConditionSeverityInfo,
		ObservedGeneration: 42,
	}
	conditions.SetStatusCondition(accessor, original)

	found := conditions.FindStatusCondition(accessor, "TestCondition")
	if found == nil {
		t.Fatal("expected to find condition")
	}

	if found.Type != original.Type {
		t.Errorf("type = %s, want %s", found.Type, original.Type)
	}

	if found.Status != original.Status {
		t.Errorf("status = %s, want %s", found.Status, original.Status)
	}

	if found.Reason != original.Reason {
		t.Errorf("reason = %s, want %s", found.Reason, original.Reason)
	}

	if found.Message != original.Message {
		t.Errorf("message = %s, want %s", found.Message, original.Message)
	}

	if found.Severity != original.Severity {
		t.Errorf("severity = %s, want %s", found.Severity, original.Severity)
	}

	if found.ObservedGeneration != original.ObservedGeneration {
		t.Errorf("observedGeneration = %d, want %d", found.ObservedGeneration, original.ObservedGeneration)
	}
}

func TestFindStatusCondition_DeepCopy(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	conditions.SetStatusCondition(accessor, common.Condition{
		Type:    "TestCondition",
		Status:  metav1.ConditionTrue,
		Reason:  "Original",
		Message: "Original message",
	})

	found := conditions.FindStatusCondition(accessor, "TestCondition")
	if found == nil {
		t.Fatal("expected to find condition")
	}

	// Mutate the returned condition
	found.Reason = "Mutated"
	found.Message = "Mutated message"

	// Verify the original in the accessor is unchanged
	original := accessor.GetConditions()[0]
	if original.Reason == "Mutated" {
		t.Error("mutation of returned condition affected the original")
	}

	if original.Message == "Mutated message" {
		t.Error("mutation of returned condition affected the original")
	}
}

func TestFindStatusCondition_NotFound(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	conditions.SetStatusCondition(accessor, common.Condition{Type: "Cond1", Status: metav1.ConditionTrue})

	found := conditions.FindStatusCondition(accessor, "NonExistent")
	if found != nil {
		t.Error("expected nil for non-existent condition")
	}
}

func TestFindStatusCondition_NilAccessor(t *testing.T) {
	t.Parallel()

	found := conditions.FindStatusCondition(nil, "TestCondition")
	if found != nil {
		t.Error("expected nil for nil accessor")
	}
}

func TestIsStatusConditionTrue(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	conditions.SetStatusCondition(accessor, common.Condition{Type: "TrueCond", Status: metav1.ConditionTrue})
	conditions.SetStatusCondition(accessor, common.Condition{Type: "FalseCond", Status: metav1.ConditionFalse})
	conditions.SetStatusCondition(accessor, common.Condition{Type: "UnknownCond", Status: metav1.ConditionUnknown})

	if !conditions.IsStatusConditionTrue(accessor, "TrueCond") {
		t.Error("expected TrueCond to be True")
	}

	if conditions.IsStatusConditionTrue(accessor, "FalseCond") {
		t.Error("expected FalseCond to not be True")
	}

	if conditions.IsStatusConditionTrue(accessor, "UnknownCond") {
		t.Error("expected UnknownCond to not be True")
	}

	if conditions.IsStatusConditionTrue(accessor, "NonExistent") {
		t.Error("expected non-existent condition to not be True")
	}
}

func TestIsStatusConditionFalse(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	conditions.SetStatusCondition(accessor, common.Condition{Type: "TrueCond", Status: metav1.ConditionTrue})
	conditions.SetStatusCondition(accessor, common.Condition{Type: "FalseCond", Status: metav1.ConditionFalse})
	conditions.SetStatusCondition(accessor, common.Condition{Type: "UnknownCond", Status: metav1.ConditionUnknown})

	if conditions.IsStatusConditionFalse(accessor, "TrueCond") {
		t.Error("expected TrueCond to not be False")
	}

	if !conditions.IsStatusConditionFalse(accessor, "FalseCond") {
		t.Error("expected FalseCond to be False")
	}

	if conditions.IsStatusConditionFalse(accessor, "UnknownCond") {
		t.Error("expected UnknownCond to not be False")
	}

	if conditions.IsStatusConditionFalse(accessor, "NonExistent") {
		t.Error("expected non-existent condition to not be False")
	}
}

func TestIsStatusConditionPresentAndEqual(t *testing.T) {
	t.Parallel()

	accessor := &testAccessor{}

	conditions.SetStatusCondition(accessor, common.Condition{Type: "TestCond", Status: metav1.ConditionTrue})

	if !conditions.IsStatusConditionPresentAndEqual(accessor, "TestCond", metav1.ConditionTrue) {
		t.Error("expected TestCond to be present and True")
	}

	if conditions.IsStatusConditionPresentAndEqual(accessor, "TestCond", metav1.ConditionFalse) {
		t.Error("expected TestCond to not be False")
	}

	if conditions.IsStatusConditionPresentAndEqual(accessor, "NonExistent", metav1.ConditionTrue) {
		t.Error("expected non-existent condition to return false")
	}
}

// conditionExists checks if a condition exists in a slice.
func conditionExists(conds []common.Condition, condType string) bool {
	for _, c := range conds {
		if c.Type == condType {
			return true
		}
	}

	return false
}
