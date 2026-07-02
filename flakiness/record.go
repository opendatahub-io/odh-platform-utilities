package flakiness

import (
	"errors"
	"fmt"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

// ErrTimestampRequired is returned by [RecordTestResult] when the
// [TestResult.Timestamp] is zero.
var ErrTimestampRequired = errors.New("record test result: timestamp is required")

// RecordTestResult appends two metric samples for a test execution:
// [MetricTestExecutionTotal] (value 1) and [MetricTestDurationSeconds].
// It does not call Commit.
func RecordTestResult(a SampleAppender, r TestResult) error {
	if r.Timestamp.IsZero() {
		return ErrTimestampRequired
	}

	ts := r.Timestamp.UnixMilli()

	executionLabels := labels.FromStrings(
		model.MetricNameLabel, MetricTestExecutionTotal,
		LabelTestName, r.Name,
		LabelSuite, r.Suite,
		LabelJob, r.Job,
		LabelBuildID, r.BuildID,
		LabelResult, string(r.Result),
	)

	_, err := a.Append(0, executionLabels, ts, 1)
	if err != nil {
		return fmt.Errorf("appending test execution metric: %w", err)
	}

	durationLabels := labels.FromStrings(
		model.MetricNameLabel, MetricTestDurationSeconds,
		LabelTestName, r.Name,
		LabelSuite, r.Suite,
		LabelJob, r.Job,
		LabelBuildID, r.BuildID,
	)

	_, err = a.Append(0, durationLabels, ts, r.Duration.Seconds())
	if err != nil {
		return fmt.Errorf("appending test duration metric: %w", err)
	}

	return nil
}
