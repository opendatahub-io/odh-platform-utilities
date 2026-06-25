// Package flakiness defines the metric schema and convenience helpers for
// the flaky test detection and quarantine system.
//
// # Interface Contract
//
// The data sink contract is based on Prometheus storage interfaces
// (github.com/prometheus/prometheus/storage):
//
//   - Write: storage.Appendable / storage.Appender
//   - Read: storage.Queryable / storage.Querier
//   - Combined: storage.Storage
//
// [SampleAppender] is a minimal subset of storage.Appender used by
// [RecordTestResult]. Any storage.Appender satisfies it.
//
// # Metric Schema
//
// See [MetricTestExecutionTotal], [MetricTestDurationSeconds], and the
// Label* constants.
//
// # Convenience Wrapper
//
// [RecordTestResult] translates a [TestResult] into two Append calls.
// It does not call Commit, callers manage the transaction lifecycle.
package flakiness
