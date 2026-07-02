package metrics_test

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/metrics"
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
	failOnCall int 
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

var _ metrics.SampleAppender = (*fakeAppender)(nil)

func TestRecordReconcile(t *testing.T) { //nolint:funlen
	t.Parallel()

	ts := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	tests := []struct { //nolint:govet
		name           string
		module         string
		duration       time.Duration
		reconcileErr   error
		expectedResult string
		expectedDurVal float64
	}{
		{
			name:           "successful reconcile",
			module:         "monitoring",
			duration:       50 * time.Millisecond,
			reconcileErr:   nil,
			expectedResult: "success",
			expectedDurVal: 0.05,
		},
		{
			name:           "failed reconcile",
			module:         "monitoring",
			duration:       2 * time.Second,
			reconcileErr:   assert.AnError,
			expectedResult: "error",
			expectedDurVal: 2.0,
		},
		{
			name:           "zero duration reconcile",
			module:         "trainer",
			duration:       0,
			reconcileErr:   nil,
			expectedResult: "success",
			expectedDurVal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fa := &fakeAppender{}
			err := metrics.RecordReconcile(fa, tt.module, ts, tt.duration, tt.reconcileErr)
			require.NoError(t, err)
			require.Len(t, fa.calls, 2)

			execCall := fa.calls[0]
			assert.InDelta(t, 1.0, execCall.V, 0.001)
			assert.Equal(t, ts.UnixMilli(), execCall.T)
			assert.Equal(t, metrics.MetricReconcileTotal,
				execCall.Labels.Get(model.MetricNameLabel))
			assert.Equal(t, tt.module,
				execCall.Labels.Get(metrics.LabelModule))
			assert.Equal(t, tt.expectedResult,
				execCall.Labels.Get(metrics.LabelResult))

			durCall := fa.calls[1]
			assert.InDelta(t, tt.expectedDurVal, durCall.V, 0.001)
			assert.Equal(t, ts.UnixMilli(), durCall.T)
			assert.Equal(t, metrics.MetricReconcileDurationSeconds,
				durCall.Labels.Get(model.MetricNameLabel))
			assert.Equal(t, tt.module,
				durCall.Labels.Get(metrics.LabelModule))
			assert.Empty(t, durCall.Labels.Get(metrics.LabelResult),
				"duration metric should not carry result label")
		})
	}
}

func TestRecordReconcile_ZeroTimestamp(t *testing.T) {
	t.Parallel()

	fa := &fakeAppender{}
	err := metrics.RecordReconcile(fa, "monitoring", time.Time{}, 10*time.Millisecond, nil)

	require.ErrorIs(t, err, metrics.ErrTimestampRequired)
	assert.Empty(t, fa.calls, "no samples should be appended for zero timestamp")
}

func TestRecordReconcile_AppendError(t *testing.T) {
	t.Parallel()

	t.Run("first append fails", func(t *testing.T) {
		t.Parallel()

		fa := &fakeAppender{appendErr: assert.AnError, failOnCall: 1}
		err := metrics.RecordReconcile(fa, "monitoring", time.Now(), 10*time.Millisecond, nil)

		require.Error(t, err)
		require.ErrorIs(t, err, assert.AnError)
		assert.Contains(t, err.Error(), "appending reconcile total metric")
	})

	t.Run("second append fails", func(t *testing.T) {
		t.Parallel()

		fa := &fakeAppender{appendErr: assert.AnError, failOnCall: 2}
		err := metrics.RecordReconcile(fa, "monitoring", time.Now(), 10*time.Millisecond, nil)

		require.Error(t, err)
		require.ErrorIs(t, err, assert.AnError)
		assert.Contains(t, err.Error(), "appending reconcile duration metric")
	})
}


func TestRecordPreconditionFailure(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	fa := &fakeAppender{}
	err := metrics.RecordPreconditionFailure(fa, "monitoring", metrics.PrerequisiteMissingDependency, ts)
	require.NoError(t, err)
	require.Len(t, fa.calls, 1)

	call := fa.calls[0]
	assert.InDelta(t, 1.0, call.V, 0.001)
	assert.Equal(t, ts.UnixMilli(), call.T)
	assert.Equal(t, metrics.MetricPreconditionFailuresTotal,
		call.Labels.Get(model.MetricNameLabel))
	assert.Equal(t, "monitoring",
		call.Labels.Get(metrics.LabelModule))
	assert.Equal(t, string(metrics.PrerequisiteMissingDependency),
		call.Labels.Get(metrics.LabelPrerequisite))
}

func TestRecordPreconditionFailure_ZeroTimestamp(t *testing.T) {
	t.Parallel()

	fa := &fakeAppender{}
	err := metrics.RecordPreconditionFailure(fa, "monitoring", metrics.PrerequisiteMissingDependency, time.Time{})

	require.ErrorIs(t, err, metrics.ErrTimestampRequired)
	assert.Empty(t, fa.calls)
}

func TestRecordPreconditionFailure_AppendError(t *testing.T) {
	t.Parallel()

	fa := &fakeAppender{appendErr: assert.AnError}
	err := metrics.RecordPreconditionFailure(fa, "monitoring", metrics.PrerequisiteMissingDependency, time.Now())

	require.Error(t, err)
	require.ErrorIs(t, err, assert.AnError)
	assert.Contains(t, err.Error(), "appending precondition failure metric")
}


func TestRecordBuildInfo(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	fa := &fakeAppender{}
	err := metrics.RecordBuildInfo(fa, "monitoring", "v0.3.1", "odh-observability", ts)
	require.NoError(t, err)
	require.Len(t, fa.calls, 1)

	call := fa.calls[0]
	assert.InDelta(t, 1.0, call.V, 0.001)
	assert.Equal(t, ts.UnixMilli(), call.T)
	assert.Equal(t, metrics.MetricBuildInfo,
		call.Labels.Get(model.MetricNameLabel))
	assert.Equal(t, "monitoring",
		call.Labels.Get(metrics.LabelModule))
	assert.Equal(t, "v0.3.1",
		call.Labels.Get(metrics.LabelVersion))
	assert.Equal(t, "odh-observability",
		call.Labels.Get(metrics.LabelRepo))
}

func TestRecordBuildInfo_ZeroTimestamp(t *testing.T) {
	t.Parallel()

	fa := &fakeAppender{}
	err := metrics.RecordBuildInfo(fa, "monitoring", "v0.3.1", "odh-observability", time.Time{})

	require.ErrorIs(t, err, metrics.ErrTimestampRequired)
	assert.Empty(t, fa.calls)
}

func TestRecordBuildInfo_AppendError(t *testing.T) {
	t.Parallel()

	fa := &fakeAppender{appendErr: assert.AnError}
	err := metrics.RecordBuildInfo(fa, "monitoring", "v0.3.1", "odh-observability", time.Now())

	require.Error(t, err)
	require.ErrorIs(t, err, assert.AnError)
	assert.Contains(t, err.Error(), "appending build info metric")
}


func TestRecordComponentRelease(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	fa := &fakeAppender{}
	err := metrics.RecordComponentRelease(fa, "monitoring", "v0.4.0", "odh-observability", ts)
	require.NoError(t, err)
	require.Len(t, fa.calls, 1)

	call := fa.calls[0]
	assert.InDelta(t, 1.0, call.V, 0.001)
	assert.Equal(t, ts.UnixMilli(), call.T)
	assert.Equal(t, metrics.MetricComponentRelease,
		call.Labels.Get(model.MetricNameLabel))
	assert.Equal(t, "monitoring",
		call.Labels.Get(metrics.LabelModule))
	assert.Equal(t, "v0.4.0",
		call.Labels.Get(metrics.LabelVersion))
	assert.Equal(t, "odh-observability",
		call.Labels.Get(metrics.LabelRepo))
}

func TestRecordComponentRelease_ZeroTimestamp(t *testing.T) {
	t.Parallel()

	fa := &fakeAppender{}
	err := metrics.RecordComponentRelease(fa, "monitoring", "v0.4.0", "odh-observability", time.Time{})

	require.ErrorIs(t, err, metrics.ErrTimestampRequired)
	assert.Empty(t, fa.calls)
}

func TestRecordComponentRelease_AppendError(t *testing.T) {
	t.Parallel()

	fa := &fakeAppender{appendErr: assert.AnError}
	err := metrics.RecordComponentRelease(fa, "monitoring", "v0.4.0", "odh-observability", time.Now())

	require.Error(t, err)
	require.ErrorIs(t, err, assert.AnError)
	assert.Contains(t, err.Error(), "appending component release metric")
}
