// Package flakiness implements flaky test detection, quarantine management,
// and automated Jira ticket creation for ODH platform CI pipelines.
//
// # Quick Start
//
// The primary entry point is [Run], which executes the full pipeline:
//
//	cfg, err := flakiness.LoadConfig(".flakiness.yaml")
//	if err != nil { ... }
//
//	result, err := flakiness.Run(ctx, cfg)
//	// result.QuarantinedTests — tests to skip in CI
//	// result.NewlyQuarantined — freshly quarantined (Jira tickets filed)
//	// result.Unquarantined    — removed from quarantine (Jira resolved)
//
// # Pipeline Stages
//
// [Run] orchestrates four stages:
//
//  1. Scrape: Read JUnit XML artifacts from GCS via [Scraper]
//  2. Ingest: Write test metrics to an in-memory Prometheus TSDB ([Store])
//  3. Analyze: Compute flake rates and classify failure patterns ([Analyze])
//  4. Quarantine: Determine quarantine changes and file Jira tickets
//
// # Dependency Injection
//
// All external dependencies are injectable via [RunOption] functions:
//
//   - [WithBucketClient] — custom GCS client (default: anonymous public access)
//   - [WithJiraClient] — custom Jira client implementing [JiraFiler]
//   - [WithClock] — custom time source for analysis window
//   - [WithPriorEntries] — previous quarantine state for diff computation
//
// # Sink Architecture
//
// The default sink is an in-memory Prometheus TSDB requiring zero external
// infrastructure. The [Store] type wraps TSDB + PromQL engine and satisfies
// storage.Storage. Future remote sinks (VictoriaMetrics, Grafana Cloud) can
// be used by writing metrics via the [SampleAppender] interface and querying
// via standard Prometheus storage.Queryable.
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
