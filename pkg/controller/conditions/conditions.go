package conditions

import (
	"cmp"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
)

// ConditionReasonError is the default reason string used when marking a
// condition as failed via WithError.
const ConditionReasonError = "Error"

// Manager provides a Knative-inspired condition management interface that
// tracks a "happy" condition (typically Ready) and its dependents. The
// manager automatically aggregates dependent condition states into the happy
// condition using severity-based filtering and sort ordering.
//
// Manager is not safe for concurrent use; callers must provide external
// synchronization if accessed from multiple goroutines.
//
// This is an optional convenience utility. Module teams can manage conditions
// manually using raw SetConditions/GetConditions on the ConditionsAccessor
// interface if preferred.
type Manager struct {
	accessor   common.ConditionsAccessor
	happy      string
	dependents []string
}

// NewManager creates a new condition manager bound to the given accessor.
// The happy condition is initialized to Unknown, and dependent conditions are
// seeded based on the happy condition's current status.
//
// Parameters:
//   - accessor: The object implementing ConditionsAccessor (typically a module CR status)
//   - happy: The top-level condition type (typically ConditionTypeReady)
//   - dependents: Condition types that should be aggregated into the happy condition
func NewManager(accessor common.ConditionsAccessor, happy string, dependents ...string) *Manager {
	// Deduplicate dependents and exclude happy from dependents list
	uniqueDeps := make([]string, 0, len(dependents))

	seen := make(map[string]bool)
	for _, dep := range dependents {
		if dep == happy || seen[dep] {
			continue
		}

		seen[dep] = true
		uniqueDeps = append(uniqueDeps, dep)
	}

	m := &Manager{
		accessor:   accessor,
		happy:      happy,
		dependents: uniqueDeps,
	}

	m.initializeConditions()

	return m
}

// IsHappy returns true if the happy condition is True.
func (m *Manager) IsHappy() bool {
	if m.accessor == nil {
		return false
	}

	return IsStatusConditionTrue(m.accessor, m.happy)
}

// GetTopLevelCondition returns a deep copy of the happy condition.
func (m *Manager) GetTopLevelCondition() *common.Condition {
	return FindStatusCondition(m.accessor, m.happy)
}

// GetCondition returns a deep copy of the named condition, or nil if not found.
func (m *Manager) GetCondition(conditionType string) *common.Condition {
	return FindStatusCondition(m.accessor, conditionType)
}

// SetCondition upserts a condition and triggers happiness recomputation.
func (m *Manager) SetCondition(condition common.Condition) {
	if m.accessor == nil {
		return
	}

	if !SetStatusCondition(m.accessor, condition) {
		return
	}

	m.RecomputeHappiness(condition.Type)
}

// ClearCondition removes a condition and recomputes happiness.
func (m *Manager) ClearCondition(conditionType string) {
	if m.accessor == nil {
		return
	}

	if !RemoveStatusCondition(m.accessor, conditionType) {
		return
	}

	m.RecomputeHappiness(conditionType)
}

// Mark sets a condition with the given status and applies optional mutations.
func (m *Manager) Mark(conditionType string, status metav1.ConditionStatus, opts ...Option) {
	condition := common.Condition{
		Type:   conditionType,
		Status: status,
	}

	// Apply all options
	for _, opt := range opts {
		opt(&condition)
	}

	m.SetCondition(condition)
}

// MarkTrue marks a condition as True with optional mutations.
func (m *Manager) MarkTrue(conditionType string, opts ...Option) {
	m.Mark(conditionType, metav1.ConditionTrue, opts...)
}

// MarkFalse marks a condition as False with optional mutations.
func (m *Manager) MarkFalse(conditionType string, opts ...Option) {
	m.Mark(conditionType, metav1.ConditionFalse, opts...)
}

// MarkUnknown marks a condition as Unknown with optional mutations.
func (m *Manager) MarkUnknown(conditionType string, opts ...Option) {
	m.Mark(conditionType, metav1.ConditionUnknown, opts...)
}

// MarkFrom copies the status, reason, message, and severity from the source
// condition to a new condition with the given type. This is heavily used for
// propagating status from sub-component CRs up to parent resources.
//
// Note: ObservedGeneration is intentionally NOT copied to avoid confusing
// generation tracking between different condition types.
//
// If the source condition is nil, this is a no-op.
func (m *Manager) MarkFrom(conditionType string, source *common.Condition) {
	if source == nil {
		return
	}

	m.SetCondition(common.Condition{
		Type:     conditionType,
		Status:   source.Status,
		Reason:   source.Reason,
		Message:  source.Message,
		Severity: source.Severity,
	})
}

// RecomputeHappiness scans dependent conditions for unhappy states (severity =
// Error, status = False or Unknown). If any unhappy dependents are found, the
// happy condition mirrors the first unhappy dependent (sorted by
// LastTransitionTime descending, False before Unknown). If all dependents are
// healthy, the happy condition becomes True.
//
// Only dependents with ConditionSeverityError (or empty severity, which
// defaults to Error) participate in happiness computation. Conditions with
// ConditionSeverityInfo are ignored.
//
// Parameters:
//   - conditionType: The type of condition that triggered this recomputation.
//     If this equals the happy condition type, the happy condition is not
//     automatically set to True (prevents infinite recursion).
func (m *Manager) RecomputeHappiness(conditionType string) {
	unhappy := m.findUnhappyDependent()
	if unhappy != nil {
		// Propagate the unhappy dependent to the happy condition.
		// Use SetStatusCondition directly to avoid infinite recursion.
		// ObservedGeneration is intentionally omitted to avoid confusing
		// generation tracking between different condition types.
		SetStatusCondition(m.accessor, common.Condition{
			Type:    m.happy,
			Status:  unhappy.Status,
			Reason:  unhappy.Reason,
			Message: unhappy.Message,
		})
	} else if conditionType != m.happy {
		// All dependents are healthy or Info severity
		// Only mark happy=True if we're not already setting the happy condition
		// (prevents infinite recursion)
		SetStatusCondition(m.accessor, common.Condition{
			Type:   m.happy,
			Status: metav1.ConditionTrue,
			Reason: "AllDependentsHealthy",
		})
	}
}

// Reset clears all conditions. Used at the start of a reconcile cycle to
// clean stale conditions.
func (m *Manager) Reset() {
	if m.accessor == nil {
		return
	}

	m.accessor.SetConditions([]common.Condition{})
}

// Sort performs a stable sort on the conditions slice:
// - Happy condition first
// - Dependents in registration order
// - Remaining conditions alphabetically by type.
func (m *Manager) Sort() {
	if m.accessor == nil {
		return
	}

	conditions := m.accessor.GetConditions()
	if len(conditions) == 0 {
		return
	}

	// Build sort priority map
	priority := make(map[string]int)
	priority[m.happy] = 0

	for i, dep := range m.dependents {
		priority[dep] = i + 1
	}

	slices.SortStableFunc(conditions, func(a, b common.Condition) int {
		pi, foundI := priority[a.Type]
		pj, foundJ := priority[b.Type]

		if foundI && foundJ {
			return cmp.Compare(pi, pj)
		}

		if foundI {
			return -1
		}

		if foundJ {
			return 1
		}

		return cmp.Compare(a.Type, b.Type)
	})

	m.accessor.SetConditions(conditions)
}

// initializeConditions ensures that the conditions for the manager and its dependents
// are properly initialized. The happy condition is initialized to Unknown if it doesn't
// exist. Dependent conditions are seeded based on the happy condition's status: if happy
// is True, dependents are set to True; otherwise they are set to Unknown.
func (m *Manager) initializeConditions() {
	happy := m.GetCondition(m.happy)
	if happy == nil {
		happy = &common.Condition{
			Type:   m.happy,
			Status: metav1.ConditionUnknown,
		}
		SetStatusCondition(m.accessor, *happy)
	}

	// Seed dependents based on happy status
	status := metav1.ConditionUnknown
	if happy.Status == metav1.ConditionTrue {
		status = metav1.ConditionTrue
	}

	for _, dep := range m.dependents {
		if m.GetCondition(dep) != nil {
			continue
		}

		SetStatusCondition(m.accessor, common.Condition{
			Type:   dep,
			Status: status,
		})
	}
}

// findUnhappyDependent returns the first unhappy dependent condition, or nil
// if all dependents are healthy. An unhappy dependent is one with:
//   - Severity = Error (or empty, which defaults to Error)
//   - Status = False or Unknown
//
// Conditions are sorted by LastTransitionTime (descending), with False
// conditions prioritized over Unknown.
func (m *Manager) findUnhappyDependent() *common.Condition {
	var unhappyCandidates []common.Condition

	for _, depType := range m.dependents {
		cond := FindStatusCondition(m.accessor, depType)
		if cond == nil {
			continue
		}

		// Only consider conditions with Error severity (empty string defaults to Error)
		if cond.Severity == common.ConditionSeverityInfo {
			continue
		}

		// Only consider False or Unknown conditions
		if cond.Status == metav1.ConditionFalse || cond.Status == metav1.ConditionUnknown {
			unhappyCandidates = append(unhappyCandidates, *cond)
		}
	}

	if len(unhappyCandidates) == 0 {
		return nil
	}

	// Sort by status (False before Unknown), then by LastTransitionTime (descending)
	slices.SortStableFunc(unhappyCandidates, func(a, b common.Condition) int {
		if a.Status != b.Status {
			if a.Status == metav1.ConditionFalse {
				return -1
			}

			return 1
		}

		return b.LastTransitionTime.Compare(a.LastTransitionTime.Time)
	})

	return &unhappyCandidates[0]
}

// Option is a functional option for condition mutations.
type Option func(*common.Condition)

// WithReason sets the condition's reason field.
func WithReason(value string) Option {
	return func(c *common.Condition) {
		c.Reason = value
	}
}

// WithMessage sets the condition's message field with optional fmt.Sprintf formatting.
func WithMessage(msg string, opts ...any) Option {
	value := msg
	if len(opts) != 0 {
		value = fmt.Sprintf(msg, opts...)
	}

	return func(c *common.Condition) {
		c.Message = value
	}
}

// WithObservedGeneration stamps the generation that was reconciled.
func WithObservedGeneration(value int64) Option {
	return func(c *common.Condition) {
		c.ObservedGeneration = value
	}
}

// WithSeverity sets the condition's severity (ConditionSeverityError or ConditionSeverityInfo).
// Controls whether a dependent condition participates in happiness recomputation.
func WithSeverity(value common.ConditionSeverity) Option {
	return func(c *common.Condition) {
		c.Severity = value
	}
}

// WithError is a convenience option that sets severity to Error, reason to
// ConditionReasonError, and message to err.Error(). This is the most common
// option for marking failures.
func WithError(err error) Option {
	return func(c *common.Condition) {
		c.Severity = common.ConditionSeverityError

		c.Reason = ConditionReasonError
		if err != nil {
			c.Message = err.Error()
		}
	}
}
