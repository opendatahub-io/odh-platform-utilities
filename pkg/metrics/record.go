package metrics

import (
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

// ErrTimestampRequired is returned by [RecordReconcile] when the
// timestamp is zero.
var ErrTimestampRequired = errors.New("record reconcile: timestamp is required")

// RecordReconcile appends two metric samples for a reconcile invocation:
// [MetricReconcileTotal] (value 1) and [MetricReconcileDurationSeconds].
// It does not call Commit; callers manage the transaction lifecycle.
func RecordReconcile(
	a SampleAppender,
	module string,
	ts time.Time,
	duration time.Duration,
	reconcileErr error,
) error {
	if ts.IsZero() {
		return ErrTimestampRequired
	}

	result := ResultSuccess
	if reconcileErr != nil {
		result = ResultError
	}

	tsMs := ts.UnixMilli()

	totalLabels := labels.FromStrings(
		model.MetricNameLabel, MetricReconcileTotal,
		LabelModule, module,
		LabelResult, string(result),
	)

	if _, err := a.Append(0, totalLabels, tsMs, 1); err != nil {
		return fmt.Errorf("appending reconcile total metric: %w", err)
	}

	durationLabels := labels.FromStrings(
		model.MetricNameLabel, MetricReconcileDurationSeconds,
		LabelModule, module,
	)

	if _, err := a.Append(0, durationLabels, tsMs, duration.Seconds()); err != nil {
		return fmt.Errorf("appending reconcile duration metric: %w", err)
	}

	return nil
}


// RecordPreconditionFailure appends one metric sample when a module detects
// a missing prerequisite operator (e.g. Cert Manager, Cluster Observability
// Operator). It does not call Commit.
func RecordPreconditionFailure(
	a SampleAppender,
	module string,
	prerequisite PrerequisiteReason,
	ts time.Time,
) error {
	if ts.IsZero() {
		return ErrTimestampRequired
	}

	failureLabels := labels.FromStrings(
		model.MetricNameLabel, MetricPreconditionFailuresTotal,
		LabelModule, module,
		LabelPrerequisite, string(prerequisite),
	)

	if _, err := a.Append(0, failureLabels, ts.UnixMilli(), 1); err != nil {
		return fmt.Errorf("appending precondition failure metric: %w", err)
	}

	return nil
}

// RecordBuildInfo appends one metric sample with the module's version and
// source repository. Typically called once at startup. It does not call Commit.
func RecordBuildInfo(
	a SampleAppender,
	module string,
	version string,
	repo string,
	ts time.Time,
) error {
	if ts.IsZero() {
		return ErrTimestampRequired
	}

	buildInfoLabels := labels.FromStrings(
		model.MetricNameLabel, MetricBuildInfo,
		LabelModule, module,
		LabelVersion, version,
		LabelRepo, repo,
	)

	if _, err := a.Append(0, buildInfoLabels, ts.UnixMilli(), 1); err != nil {
		return fmt.Errorf("appending build info metric: %w", err)
	}

	return nil
}

// RecordComponentRelease appends one metric sample tracking the last
// successfully deployed component version. Useful during upgrades to verify
// all modules progressed. It does not call Commit.
func RecordComponentRelease(
	a SampleAppender,
	module string,
	version string,
	repo string,
	ts time.Time,
) error {
	if ts.IsZero() {
		return ErrTimestampRequired
	}

	releaseLabels := labels.FromStrings(
		model.MetricNameLabel, MetricComponentRelease,
		LabelModule, module,
		LabelVersion, version,
		LabelRepo, repo,
	)

	if _, err := a.Append(0, releaseLabels, ts.UnixMilli(), 1); err != nil {
		return fmt.Errorf("appending component release metric: %w", err)
	}

	return nil
}