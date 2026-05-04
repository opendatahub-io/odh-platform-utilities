package conditions

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
)

// findConditionIndex returns the index of the condition with the given type, or -1 if not found.
func findConditionIndex(conditions []common.Condition, conditionType string) int {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i
		}
	}

	return -1
}

// conditionsEqual returns true if all relevant fields are equal.
func conditionsEqual(a, b common.Condition) bool {
	return a.Status == b.Status &&
		a.Reason == b.Reason &&
		a.Message == b.Message &&
		a.ObservedGeneration == b.ObservedGeneration &&
		a.Severity == b.Severity
}

// ensureTransitionTime sets LastTransitionTime if it's zero.
func ensureTransitionTime(cond *common.Condition) {
	if cond.LastTransitionTime.IsZero() {
		cond.LastTransitionTime = metav1.NewTime(time.Now())
	}
}

// SetStatusCondition upserts a condition in the accessor's condition slice.
// For new conditions, LastTransitionTime is set to now if not already provided.
// For existing conditions, LastTransitionTime is only updated when status
// changes (not on reason/message-only changes).
//
// Returns true if the condition was actually modified (either added or updated).
func SetStatusCondition(accessor common.ConditionsAccessor, newCondition common.Condition) bool {
	if accessor == nil {
		return false
	}

	conditions := accessor.GetConditions()

	ensureTransitionTime(&newCondition)

	existingIdx := findConditionIndex(conditions, newCondition.Type)

	// No existing condition, add it
	if existingIdx == -1 {
		conditions = append(conditions, newCondition)
		accessor.SetConditions(conditions)

		return true
	}

	// Existing condition found, check if update is needed
	existing := &conditions[existingIdx]

	if conditionsEqual(*existing, newCondition) {
		return false
	}

	// Preserve LastTransitionTime if status hasn't changed
	if existing.Status == newCondition.Status {
		newCondition.LastTransitionTime = existing.LastTransitionTime
	}

	conditions[existingIdx] = newCondition
	accessor.SetConditions(conditions)

	return true
}

// RemoveStatusCondition removes a condition from the accessor's condition slice.
// Returns true if the condition was found and removed.
func RemoveStatusCondition(accessor common.ConditionsAccessor, conditionType string) bool {
	if accessor == nil {
		return false
	}

	conditions := accessor.GetConditions()

	idx := findConditionIndex(conditions, conditionType)
	if idx == -1 {
		return false
	}

	newConditions := make([]common.Condition, 0, len(conditions)-1)
	newConditions = append(newConditions, conditions[:idx]...)
	newConditions = append(newConditions, conditions[idx+1:]...)

	accessor.SetConditions(newConditions)

	return true
}

// FindStatusCondition returns a deep copy of the named condition, or nil if
// not found. Deep copy prevents accidental mutation of the backing slice.
func FindStatusCondition(accessor common.ConditionsAccessor, conditionType string) *common.Condition {
	if accessor == nil {
		return nil
	}

	conditions := accessor.GetConditions()
	for i := range conditions {
		if conditions[i].Type == conditionType {
			// Return a deep copy to prevent mutation
			cond := conditions[i].DeepCopy()
			return cond
		}
	}

	return nil
}

// IsStatusConditionTrue returns true if the named condition exists and has
// status True.
func IsStatusConditionTrue(accessor common.ConditionsAccessor, conditionType string) bool {
	cond := FindStatusCondition(accessor, conditionType)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// IsStatusConditionFalse returns true if the named condition exists and has
// status False.
func IsStatusConditionFalse(accessor common.ConditionsAccessor, conditionType string) bool {
	cond := FindStatusCondition(accessor, conditionType)
	return cond != nil && cond.Status == metav1.ConditionFalse
}

// IsStatusConditionPresentAndEqual returns true if the named condition exists
// and has the given status.
func IsStatusConditionPresentAndEqual(
	accessor common.ConditionsAccessor,
	conditionType string,
	status metav1.ConditionStatus,
) bool {
	cond := FindStatusCondition(accessor, conditionType)
	return cond != nil && cond.Status == status
}
