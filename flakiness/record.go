package flakiness

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

const maxLabelLen = 64

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

	execLabelPairs := []string{
		model.MetricNameLabel, MetricTestExecutionTotal,
		LabelTestName, r.Name,
		LabelSuite, r.Suite,
		LabelJob, r.Job,
		LabelBuildID, r.BuildID,
		LabelResult, string(r.Result),
	}

	if r.CommitSHA != "" {
		execLabelPairs = append(execLabelPairs, LabelCommitSHA, r.CommitSHA)
	}

	if r.PRNumber != "" {
		execLabelPairs = append(execLabelPairs, LabelPRNumber, r.PRNumber)
	}

	if r.FailureCategory != "" {
		execLabelPairs = append(execLabelPairs, LabelFailureCategory, truncateLabel(r.FailureCategory))
	}

	if r.FailureSubcategory != "" {
		execLabelPairs = append(execLabelPairs, LabelFailureSubcategory, truncateLabel(r.FailureSubcategory))
	}

	if r.FailureConfidence != "" {
		execLabelPairs = append(execLabelPairs, LabelFailureConfidence, bucketConfidence(r.FailureConfidence))
	}

	executionLabels := labels.FromStrings(execLabelPairs...)

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

func truncateLabel(v string) string {
	if len(v) <= maxLabelLen {
		return v
	}

	return v[:maxLabelLen]
}

func bucketConfidence(v string) string {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return "unknown"
	}

	switch {
	case f >= 0.9:
		return "high"
	case f >= 0.5:
		return "medium"
	default:
		return "low"
	}
}
