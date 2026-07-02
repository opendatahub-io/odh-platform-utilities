// Package metrics provides helpers for recording ODH module operator metrics.
//
// # Built-in metrics (already available, no code needed)
//
// controller-runtime gives every operator these metrics for free:
//   - controller_runtime_reconcile_total — how many times reconcile ran
//   - controller_runtime_reconcile_time_seconds — how long each reconcile took
//   - controller_runtime_reconcile_errors_total — how many reconciles failed
//   - controller_runtime_active_workers — how many reconciles are running now
//   - workqueue_depth — how many items are waiting to be reconciled
//
// Full list: https://book.kubebuilder.io/reference/metrics-reference.html
//
// # Custom metrics (defined in this package)
//
// These fill the gaps that controller-runtime does not cover:
//   - [MetricPreconditionFailuresTotal] — tracks when a required operator is missing
//   - [MetricBuildInfo] — what version of the module is running
//   - [MetricComponentRelease] — what version was last successfully deployed
//
// # How to use
//
// All Record* functions take a [SampleAppender] interface (compatible with
// prometheus storage.Appender) and write metric samples to it.
// None of them call Commit — the caller is responsible for that.
//
// See [RecordReconcile], [RecordPreconditionFailure], [RecordBuildInfo],
// and [RecordComponentRelease].
package metrics
