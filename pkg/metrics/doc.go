// Package metrics defines the Prometheus metric schema and recording
// helpers for ODH module operators.
//
// # Metric Schema
//
// The package registers the following metrics with the controller-runtime
// global registry:
//
//   - [MetricPreconditionFailuresTotal] — counter of precondition failures
//   - [MetricBuildInfo] — gauge exposing build version metadata
//   - [MetricComponentRelease] — gauge tracking the last deployed release
//   - [MetricReconcilePhaseDurationSeconds] — histogram of action phase durations
//   - [MetricManagedResources] — gauge of managed resources per GVK
//   - [MetricConditionTransitionsTotal] — counter of condition status transitions
//
// # Recording Helpers
//
// Use [RecordPreconditionFailure] to increment the precondition failure
// counter. Use [RecordBuildInfo] and [RecordComponentRelease] to set
// info/release gauges; they retire stale label sets on version change.
// Use [RecordReconcilePhaseDuration] to observe action phase durations.
// Use [SetManagedResources] to replace the managed resource gauge per GVK.
// Use [RecordConditionTransition] to increment the condition transition
// counter.
//
// # Label Cardinality
//
// All label dimensions use closed typed enums ([PrerequisiteReason],
// [ReconcilePhase], [ConditionStatus]) or the platform contract type
// common.ConditionType to bound cardinality.
//
// For built-in controller-runtime metrics see:
// https://book.kubebuilder.io/reference/metrics-reference.html
package metrics
