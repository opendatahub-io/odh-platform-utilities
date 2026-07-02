package flakiness

import "time"

// Metric name constants (used as the __name__ label value).
const (
	MetricTestExecutionTotal  = "test_execution_total"
	MetricTestDurationSeconds = "test_duration_seconds"
)

const (
	LabelTestName = "test_name"
	LabelSuite    = "suite"
	LabelJob      = "job"
	LabelBuildID  = "build_id"
	LabelResult   = "result"
)

type TestOutcome string

const (
	OutcomePass  TestOutcome = "pass"
	OutcomeFail  TestOutcome = "fail"
	OutcomeError TestOutcome = "error"
	OutcomeSkip  TestOutcome = "skip"
)

// TestResult - use [RecordTestResult] to translate results into metric samples.
type TestResult struct {
	Timestamp time.Time
	Name      string
	Suite     string
	Job       string
	BuildID   string
	Result    TestOutcome
	Duration  time.Duration
}
