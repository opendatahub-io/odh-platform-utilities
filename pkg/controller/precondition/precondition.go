package precondition

import (
	"context"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/action"
	cond "github.com/opendatahub-io/odh-platform-utilities/pkg/controller/conditions"
)

const (
	// ConditionTypeDependenciesAvailable is the default condition type
	// written by RunAll when no WithConditionType option is provided.
	ConditionTypeDependenciesAvailable = "DependenciesAvailable"

	// PreConditionFailedReason is the reason string set on conditions
	// when one or more precondition checks fail.
	PreConditionFailedReason = "PreConditionFailed"
)

// CheckResult holds the outcome of a check execution.
type CheckResult struct {
	Message string
	Pass    bool
}

// CheckFunc is the function signature for a pre-reconciliation check.
// Returned errors and CheckResult.Message values are written to logs
// and status conditions — never include sensitive data.
type CheckFunc func(ctx context.Context, rr *action.ReconciliationRequest) (CheckResult, error)

// Option configures a PreCondition.
type Option func(*PreCondition)

// SkipFunc is a runtime predicate evaluated on every reconciliation after
// WithClusterTypes filtering. Return true to skip the precondition (no
// condition is written and an Info-level log is emitted). Return an error
// to record ConditionUnknown and continue (same semantics as CheckFunc
// errors). Returned errors are written to logs and status conditions —
// never include sensitive data.
type SkipFunc func(ctx context.Context, rr *action.ReconciliationRequest) (bool, error)

// PreCondition composes a Check with framework configuration that controls
// how RunAll aggregates and writes Kubernetes status conditions.
type PreCondition struct {
	conditionType      string
	severity           common.ConditionSeverity
	message            string
	check              CheckFunc
	skipFunc           SkipFunc
	clusterTypes       []cluster.ClusterType
	stopReconciliation bool
}

// WithConditionType sets the condition type that will be written.
// Empty strings are ignored to preserve the constructor default.
func WithConditionType(t string) Option {
	return func(pc *PreCondition) {
		if t != "" {
			pc.conditionType = t
		}
	}
}

// WithSeverity sets the severity of the condition that will be written.
func WithSeverity(s common.ConditionSeverity) Option {
	return func(pc *PreCondition) {
		pc.severity = s
	}
}

// WithStopReconciliation flags the reconciliation to be stopped if the
// precondition is not met.
func WithStopReconciliation() Option {
	return func(pc *PreCondition) {
		pc.stopReconciliation = true
	}
}

// WithClusterTypes sets the cluster types on which the precondition will
// be checked. Preconditions are skipped on clusters whose type is not in
// this list.
func WithClusterTypes(types ...cluster.ClusterType) Option {
	return func(pc *PreCondition) {
		pc.clusterTypes = slices.Clone(types)
	}
}

// WithMessage sets the message that will be written to the condition.
func WithMessage(msg string) Option {
	return func(pc *PreCondition) {
		pc.message = msg
	}
}

// WithSkipFunc sets a runtime predicate evaluated on every reconciliation
// after WithClusterTypes filtering. See [SkipFunc] for return value semantics.
func WithSkipFunc(fn SkipFunc) Option {
	return func(pc *PreCondition) {
		pc.skipFunc = fn
	}
}

func newPreCondition(check CheckFunc, opts ...Option) PreCondition {
	pc := PreCondition{
		check:         check,
		conditionType: ConditionTypeDependenciesAvailable,
		severity:      common.ConditionSeverityError,
	}

	for _, opt := range opts {
		opt(&pc)
	}

	return pc
}

// conditionAggregate aggregates the check results for a given condition type.
type conditionAggregate struct {
	status     metav1.ConditionStatus
	severity   common.ConditionSeverity
	messages   []string
	shouldStop bool
}

// record merges a single check outcome into the aggregate.
// Priority: False > Unknown > True.
func (agg *conditionAggregate) record(s metav1.ConditionStatus, message string, pc *PreCondition) {
	switch {
	case s == metav1.ConditionFalse:
		agg.status = metav1.ConditionFalse
	case s == metav1.ConditionUnknown && agg.status != metav1.ConditionFalse:
		agg.status = metav1.ConditionUnknown
	}

	agg.messages = append(agg.messages, message)

	if pc.severity == common.ConditionSeverityError {
		agg.severity = common.ConditionSeverityError
	}

	if pc.stopReconciliation {
		agg.shouldStop = true
	}
}

func initAggregate(results map[string]*conditionAggregate, conditionType string) *conditionAggregate {
	if results[conditionType] == nil {
		results[conditionType] = &conditionAggregate{
			status:   metav1.ConditionTrue,
			severity: common.ConditionSeverityInfo,
		}
	}

	return results[conditionType]
}

// RunAll runs all the preconditions and returns true when the reconciliation
// should be stopped. The clusterType parameter controls WithClusterTypes
// filtering; pass an empty value to disable filtering.
//
// All preconditions are always executed, even when earlier checks fail or
// request a stop. This ensures all conditions are reported in a single
// reconciliation cycle.
func RunAll(
	ctx context.Context,
	rr *action.ReconciliationRequest,
	clusterType cluster.ClusterType,
	preConditions []PreCondition,
) bool {
	if len(preConditions) == 0 {
		return false
	}

	results := make(map[string]*conditionAggregate)

	for i := range preConditions {
		evalPreCondition(ctx, rr, &preConditions[i], clusterType, results)
	}

	return writeConditions(rr, results)
}

func evalPreCondition(
	ctx context.Context,
	rr *action.ReconciliationRequest,
	pc *PreCondition,
	clusterType cluster.ClusterType,
	results map[string]*conditionAggregate,
) {
	l := ctrlLog.FromContext(ctx)

	if len(pc.clusterTypes) > 0 && !slices.Contains(pc.clusterTypes, clusterType) {
		initAggregate(results, pc.conditionType)

		return
	}

	if pc.skipFunc != nil {
		skip, skipErr := pc.skipFunc(ctx, rr)
		if skipErr != nil {
			l.Info("Pre-condition skip function error", "conditionType", pc.conditionType, "error", skipErr.Error())
			initAggregate(results, pc.conditionType).record(metav1.ConditionUnknown, skipErr.Error(), pc)

			return
		}

		if skip {
			l.Info("Pre-condition skipped by runtime predicate", "conditionType", pc.conditionType)

			return
		}
	}

	agg := initAggregate(results, pc.conditionType)

	if pc.check == nil {
		l.Info("Pre-condition check function is nil", "conditionType", pc.conditionType)
		agg.record(metav1.ConditionUnknown, "precondition check function is nil", pc)

		return
	}

	result, err := pc.check(ctx, rr)
	if err != nil {
		l.Info("Pre-condition check error", "conditionType", pc.conditionType, "error", err.Error())
		agg.record(metav1.ConditionUnknown, err.Error(), pc)

		return
	}

	if !result.Pass {
		l.Info("Pre-condition not met", "conditionType", pc.conditionType, "message", result.Message)

		msg := result.Message
		if pc.message != "" {
			msg = pc.message
		}

		agg.record(metav1.ConditionFalse, msg, pc)
	}
}

func writeConditions(rr *action.ReconciliationRequest, results map[string]*conditionAggregate) bool {
	shouldStop := false

	for ct, agg := range results {
		opts := []cond.Option{
			cond.WithObservedGeneration(rr.Instance.GetGeneration()),
		}

		if agg.status != metav1.ConditionTrue {
			opts = append(opts,
				cond.WithReason(PreConditionFailedReason),
				cond.WithSeverity(agg.severity),
				cond.WithMessage("%s", strings.Join(agg.messages, "; ")),
			)
		}

		rr.Conditions.Mark(ct, agg.status, opts...)

		if agg.shouldStop {
			shouldStop = true
		}
	}

	return shouldStop
}
