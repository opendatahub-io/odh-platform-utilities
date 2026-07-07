package flakiness_test

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

type appendCall struct {
	Labels labels.Labels
	Ref    storage.SeriesRef
	T      int64
	V      float64
}

type fakeAppender struct {
	appendErr  error
	calls      []appendCall
	nextRef    storage.SeriesRef
	failOnCall int // 1-indexed, 0 means all calls fail when appendErr is set
}

func (f *fakeAppender) Append(
	ref storage.SeriesRef,
	l labels.Labels,
	t int64,
	v float64,
) (storage.SeriesRef, error) {
	callNum := len(f.calls) + 1

	if f.appendErr != nil && (f.failOnCall == 0 || f.failOnCall == callNum) {
		return 0, f.appendErr
	}

	f.nextRef++

	f.calls = append(f.calls, appendCall{Ref: ref, Labels: l, T: t, V: v})

	return f.nextRef, nil
}

var _ flakiness.SampleAppender = (*fakeAppender)(nil)

func TestRecordTestResult(t *testing.T) { //nolint:funlen // Table-driven test with many assertions per case.
	t.Parallel()

	ts := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	tests := []struct { //nolint:govet // fieldalignment: test table struct
		name             string
		result           flakiness.TestResult
		expectedDurVal   float64
		expectedOutcome  string
		expectedTestName string
	}{
		{
			name: "passing test",
			result: flakiness.TestResult{
				Name:      "TestModelServing/basic_inference",
				Suite:     "e2e",
				Job:       "periodic-ci-main",
				BuildID:   "build-123",
				Result:    flakiness.OutcomePass,
				Duration:  5*time.Second + 200*time.Millisecond,
				Timestamp: ts,
			},
			expectedDurVal:   5.2,
			expectedOutcome:  "pass",
			expectedTestName: "TestModelServing/basic_inference",
		},
		{
			name: "failing test",
			result: flakiness.TestResult{
				Name:      "TestDashboard/login",
				Suite:     "integration",
				Job:       "presubmit-ci",
				BuildID:   "build-456",
				Result:    flakiness.OutcomeFail,
				Duration:  30 * time.Second,
				Timestamp: ts,
			},
			expectedDurVal:   30,
			expectedOutcome:  "fail",
			expectedTestName: "TestDashboard/login",
		},
		{
			name: "skipped test with zero duration",
			result: flakiness.TestResult{
				Name:      "TestPipelines/gpu",
				Suite:     "e2e",
				Job:       "nightly-gpu",
				BuildID:   "build-789",
				Result:    flakiness.OutcomeSkip,
				Duration:  0,
				Timestamp: ts,
			},
			expectedDurVal:   0,
			expectedOutcome:  "skip",
			expectedTestName: "TestPipelines/gpu",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fa := &fakeAppender{}
			err := flakiness.RecordTestResult(fa, tt.result)
			require.NoError(t, err)
			require.Len(t, fa.calls, 2)

			execCall := fa.calls[0]
			assert.InDelta(t, 1.0, execCall.V, 0.001)
			assert.Equal(t, ts.UnixMilli(), execCall.T)
			assert.Equal(t, flakiness.MetricTestExecutionTotal,
				execCall.Labels.Get(model.MetricNameLabel))
			assert.Equal(t, tt.expectedTestName,
				execCall.Labels.Get(flakiness.LabelTestName))
			assert.Equal(t, tt.result.Suite,
				execCall.Labels.Get(flakiness.LabelSuite))
			assert.Equal(t, tt.result.Job,
				execCall.Labels.Get(flakiness.LabelJob))
			assert.Equal(t, tt.result.BuildID,
				execCall.Labels.Get(flakiness.LabelBuildID))
			assert.Equal(t, tt.expectedOutcome,
				execCall.Labels.Get(flakiness.LabelResult))

			durCall := fa.calls[1]
			assert.InDelta(t, tt.expectedDurVal, durCall.V, 0.001)
			assert.Equal(t, ts.UnixMilli(), durCall.T)
			assert.Equal(t, flakiness.MetricTestDurationSeconds,
				durCall.Labels.Get(model.MetricNameLabel))
			assert.Equal(t, tt.expectedTestName,
				durCall.Labels.Get(flakiness.LabelTestName))
			assert.Empty(t, durCall.Labels.Get(flakiness.LabelResult),
				"duration metric should not carry result label")
		})
	}
}

func TestRecordTestResult_FailureClassification(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	t.Run("includes failure labels when set", func(t *testing.T) {
		t.Parallel()

		fa := &fakeAppender{}
		err := flakiness.RecordTestResult(fa, flakiness.TestResult{
			Name:               "TestModelServing/inference",
			Suite:              "e2e",
			Job:                "periodic-ci",
			BuildID:            "build-100",
			Result:             flakiness.OutcomeFail,
			Duration:           10 * time.Second,
			Timestamp:          ts,
			FailureCategory:    "infrastructure",
			FailureSubcategory: "cluster_timeout",
			FailureConfidence:  "0.95",
		})

		require.NoError(t, err)
		require.Len(t, fa.calls, 2)

		execCall := fa.calls[0]
		assert.Equal(t, "infrastructure",
			execCall.Labels.Get(flakiness.LabelFailureCategory))
		assert.Equal(t, "cluster_timeout",
			execCall.Labels.Get(flakiness.LabelFailureSubcategory))
		assert.Equal(t, "high",
			execCall.Labels.Get(flakiness.LabelFailureConfidence))

		durCall := fa.calls[1]
		assert.Empty(t, durCall.Labels.Get(flakiness.LabelFailureCategory),
			"duration metric should not carry failure labels")
	})

	t.Run("omits failure labels when empty", func(t *testing.T) {
		t.Parallel()

		fa := &fakeAppender{}
		err := flakiness.RecordTestResult(fa, flakiness.TestResult{
			Name:      "TestDashboard/login",
			Suite:     "e2e",
			Job:       "presubmit",
			BuildID:   "build-200",
			Result:    flakiness.OutcomePass,
			Duration:  2 * time.Second,
			Timestamp: ts,
		})

		require.NoError(t, err)
		require.Len(t, fa.calls, 2)

		execCall := fa.calls[0]
		assert.Empty(t, execCall.Labels.Get(flakiness.LabelFailureCategory))
		assert.Empty(t, execCall.Labels.Get(flakiness.LabelFailureSubcategory))
		assert.Empty(t, execCall.Labels.Get(flakiness.LabelFailureConfidence))
	})
}

func TestRecordTestResult_ZeroTimestamp(t *testing.T) {
	t.Parallel()

	fa := &fakeAppender{}
	err := flakiness.RecordTestResult(fa, flakiness.TestResult{
		Name:   "TestFoo",
		Suite:  "e2e",
		Result: flakiness.OutcomePass,
	})

	require.ErrorIs(t, err, flakiness.ErrTimestampRequired)
	assert.Empty(t, fa.calls, "no samples should be appended for zero timestamp")
}

func TestRecordTestResult_AppendError(t *testing.T) {
	t.Parallel()

	t.Run("first append fails", func(t *testing.T) {
		t.Parallel()

		fa := &fakeAppender{appendErr: assert.AnError, failOnCall: 1}
		err := flakiness.RecordTestResult(fa, flakiness.TestResult{
			Name:      "TestFoo",
			Suite:     "e2e",
			Result:    flakiness.OutcomePass,
			Timestamp: time.Now(),
		})

		require.Error(t, err)
		require.ErrorIs(t, err, assert.AnError)
		assert.Contains(t, err.Error(), "appending test execution metric")
	})

	t.Run("second append fails", func(t *testing.T) {
		t.Parallel()

		fa := &fakeAppender{appendErr: assert.AnError, failOnCall: 2}
		err := flakiness.RecordTestResult(fa, flakiness.TestResult{
			Name:      "TestFoo",
			Suite:     "e2e",
			Result:    flakiness.OutcomePass,
			Timestamp: time.Now(),
		})

		require.Error(t, err)
		require.ErrorIs(t, err, assert.AnError)
		assert.Contains(t, err.Error(), "appending test duration metric")
	})
}
