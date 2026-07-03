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
//   - [PreconditionFailuresTotal] — tracks when a required operator is missing
//   - [BuildInfo] — what version of the module is running
//   - [ComponentRelease] — what version was last successfully deployed
//   - [ReconcileTotal] — reconcile count with success/error result label
//   - [ReconcileDurationSeconds] — duration of the last reconcile
//
// All metrics are registered with the controller-runtime global registry
// on package init and are automatically exposed on the metrics endpoint.
//
// See [RecordReconcile], [RecordPreconditionFailure], [RecordBuildInfo],
// and [RecordComponentRelease].
package metrics
