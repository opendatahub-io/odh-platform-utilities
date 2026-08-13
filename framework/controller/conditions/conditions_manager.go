package conditions

import (
	"fmt"
	"slices"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ConditionReasonNotSet = "ConditionNotSet"

type Manager struct {
	aggregator  *Aggregator
	accessor    api.ConditionsAccessor
	activeTypes map[api.ConditionType]struct{}
}

func NewManager(
	accessor api.ConditionsAccessor,
	aggregator *Aggregator,
) *Manager {
	m := &Manager{
		accessor:   accessor,
		aggregator: aggregator,
	}

	m.initializeConditions()

	return m
}

func (r *Manager) initializeConditions() {
	if r.aggregator == nil {
		return
	}

	target := r.aggregator.Target()
	if r.GetCondition(target) == nil {
		r.SetCondition(api.Condition{
			Type:   string(target),
			Status: metav1.ConditionUnknown,
		})
	}

	for _, dependent := range r.aggregator.Dependents() {
		if r.GetCondition(dependent.Type) != nil {
			continue
		}

		r.SetCondition(api.Condition{
			Type:   string(dependent.Type),
			Status: metav1.ConditionUnknown,
		})
	}
}

func (r *Manager) IsHappy() bool {
	if r.accessor == nil || r.aggregator == nil {
		return false
	}

	return IsStatusConditionTrue(r.accessor, string(r.aggregator.Target()))
}

func (r *Manager) GetTopLevelCondition() *api.Condition {
	if r.aggregator == nil {
		return nil
	}

	return r.GetCondition(r.aggregator.Target())
}

func (r *Manager) GetCondition(conditionType api.ConditionType) *api.Condition {
	return FindStatusCondition(r.accessor, string(conditionType))
}

// SetCondition writes a condition through the manager.
// Callers may set the happy/target condition directly, but it remains derived
// state and can be overwritten by a later RecomputeHappiness call.
func (r *Manager) SetCondition(condition api.Condition) {
	if r.accessor == nil {
		return
	}

	if r.activeTypes != nil {
		r.activeTypes[api.ConditionType(condition.Type)] = struct{}{}
	}

	if !SetStatusCondition(r.accessor, condition) {
		return
	}

	if r.aggregator == nil || api.ConditionType(condition.Type) == r.aggregator.Target() {
		return
	}

	r.RecomputeHappiness()
}

func (r *Manager) ClearCondition(conditionType api.ConditionType) {
	if r.accessor == nil {
		return
	}

	if !RemoveStatusCondition(r.accessor, string(conditionType)) {
		return
	}

	r.RecomputeHappiness()
}

// Mark writes a condition status via SetCondition and follows the same
// temporary-override semantics for the happy/target condition.
func (r *Manager) Mark(
	conditionType api.ConditionType,
	status metav1.ConditionStatus,
	opts ...Option,
) {
	condition := api.Condition{
		Type:   string(conditionType),
		Status: status,
	}

	applyOpts(&condition, opts...)
	r.SetCondition(condition)
}

func (r *Manager) MarkTrue(conditionType api.ConditionType, opts ...Option) {
	r.Mark(conditionType, metav1.ConditionTrue, opts...)
}

func (r *Manager) MarkFalse(conditionType api.ConditionType, opts ...Option) {
	r.Mark(conditionType, metav1.ConditionFalse, opts...)
}

func (r *Manager) MarkUnknown(conditionType api.ConditionType, opts ...Option) {
	r.Mark(conditionType, metav1.ConditionUnknown, opts...)
}

func (r *Manager) MarkHealthy(conditionType api.ConditionType, opts ...Option) {
	polarity := HealthyWhenTrue
	if r.aggregator != nil {
		polarity = r.aggregator.polarityFor(conditionType)
	}

	switch polarity {
	case HealthyWhenFalse:
		r.MarkFalse(conditionType, opts...)
	default:
		r.MarkTrue(conditionType, opts...)
	}
}

func (r *Manager) MarkUnhealthy(conditionType api.ConditionType, opts ...Option) {
	polarity := HealthyWhenTrue
	if r.aggregator != nil {
		polarity = r.aggregator.polarityFor(conditionType)
	}

	switch polarity {
	case HealthyWhenFalse:
		r.MarkTrue(conditionType, opts...)
	default:
		r.MarkFalse(conditionType, opts...)
	}
}

func (r *Manager) MarkFrom(conditionType api.ConditionType, condition api.Condition) {
	r.SetCondition(api.Condition{
		Type:     string(conditionType),
		Status:   condition.Status,
		Reason:   condition.Reason,
		Message:  condition.Message,
		Severity: condition.Severity,
	})
}

// RecomputeHappiness derives the happy/target condition from the current
// dependent conditions and overwrites any prior manual value.
func (r *Manager) RecomputeHappiness() {
	if r.aggregator == nil {
		return
	}

	condition := r.aggregator.Aggregate(r.accessor)
	if condition == nil {
		return
	}

	SetStatusCondition(r.accessor, *condition)
}

func (r *Manager) Reset() {
	r.activeTypes = make(map[api.ConditionType]struct{})
}

func (r *Manager) CleanupStaleConditions() {
	if r.accessor == nil || r.activeTypes == nil || r.aggregator == nil {
		return
	}

	var toRemove []string
	changed := false

	for _, dependent := range r.aggregator.Dependents() {
		if _, active := r.activeTypes[dependent.Type]; active {
			continue
		}

		status := metav1.ConditionFalse
		if r.aggregator.polarityFor(dependent.Type) == HealthyWhenFalse {
			status = metav1.ConditionTrue
		}

		if SetStatusCondition(r.accessor, api.Condition{
			Type:     string(dependent.Type),
			Status:   status,
			Severity: api.ConditionSeverityError,
			Reason:   ConditionReasonNotSet,
			Message:  fmt.Sprintf("condition %s was not set during reconciliation", dependent.Type),
		}) {
			changed = true
		}
	}

	for _, condition := range slices.Clone(r.accessor.GetConditions()) {
		conditionType := api.ConditionType(condition.Type)
		if conditionType == r.aggregator.Target() {
			continue
		}

		if r.aggregator.hasDependent(conditionType) {
			continue
		}

		if _, active := r.activeTypes[conditionType]; active {
			continue
		}

		toRemove = append(toRemove, condition.Type)
	}

	for _, conditionType := range toRemove {
		RemoveStatusCondition(r.accessor, conditionType)
	}

	if len(toRemove) > 0 || changed {
		r.RecomputeHappiness()
	}
}

func (r *Manager) Sort() {
	conditions := r.accessor.GetConditions()
	if len(conditions) <= 1 {
		return
	}

	slices.SortStableFunc(conditions, r.aggregator.compareConditions)
	r.accessor.SetConditions(conditions)
}
