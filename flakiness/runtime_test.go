package flakiness_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

func TestRuntimeAnalyzer_TopSlowest(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	ingestRuntimeData(t, store, ctx, ts)
	analyzer := flakiness.NewRuntimeAnalyzer(store)

	t.Run("returns top N by max duration", func(t *testing.T) {
		t.Parallel()

		results, err := analyzer.TopSlowest(
			ctx, 2,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		require.Len(t, results, 2)

		assert.Equal(t, "TestSlow/very_slow", results[0].Name)
		assert.Equal(t, "TestMedium/moderate", results[1].Name)
	})

	t.Run("respects N limit", func(t *testing.T) {
		t.Parallel()

		results, err := analyzer.TopSlowest(
			ctx, 1,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "TestSlow/very_slow", results[0].Name)
	})

	t.Run("returns all when N exceeds count", func(t *testing.T) {
		t.Parallel()

		results, err := analyzer.TopSlowest(
			ctx, 100,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})

	t.Run("returns nil for zero N", func(t *testing.T) {
		t.Parallel()

		results, err := analyzer.TopSlowest(
			ctx, 0,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		assert.Nil(t, results)
	})

	t.Run("returns empty for no data in window", func(t *testing.T) {
		t.Parallel()

		far := ts.Add(24 * time.Hour)
		results, err := analyzer.TopSlowest(
			ctx, 5,
			far, far.Add(time.Hour),
		)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("includes percentile statistics", func(t *testing.T) {
		t.Parallel()

		results, err := analyzer.TopSlowest(
			ctx, 3,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)

		slow := results[0]
		assert.Equal(t, "TestSlow/very_slow", slow.Name)
		assert.Equal(t, "e2e", slow.Suite)
		assert.Greater(t, slow.P50, time.Duration(0))
		assert.GreaterOrEqual(t, slow.P95, slow.P50)
		assert.GreaterOrEqual(t, slow.P99, slow.P95)
		assert.GreaterOrEqual(t, slow.Max, slow.P99)
		assert.Equal(t, 3, slow.Samples)
	})
}

func TestRuntimeAnalyzer_SuiteRuntimes(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	ingestRuntimeData(t, store, ctx, ts)
	analyzer := flakiness.NewRuntimeAnalyzer(store)

	t.Run("returns per-suite totals sorted descending", func(t *testing.T) {
		t.Parallel()

		results, err := analyzer.SuiteRuntimes(
			ctx,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		require.Len(t, results, 2)

		assert.Equal(t, "e2e", results[0].Suite)
		assert.Equal(t, "integration", results[1].Suite)
		assert.Greater(t, results[0].Total, results[1].Total)
	})

	t.Run("includes test count and samples", func(t *testing.T) {
		t.Parallel()

		results, err := analyzer.SuiteRuntimes(
			ctx,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)

		e2e := results[0]
		assert.Equal(t, "e2e", e2e.Suite)
		assert.Equal(t, 2, e2e.TestCount)
		assert.Equal(t, 5, e2e.Samples)
	})

	t.Run("returns empty for no data in window", func(t *testing.T) {
		t.Parallel()

		far := ts.Add(24 * time.Hour)
		results, err := analyzer.SuiteRuntimes(
			ctx, far, far.Add(time.Hour),
		)
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func TestRuntimeAnalyzer_RuntimeTrend(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	ingestRuntimeData(t, store, ctx, ts)
	analyzer := flakiness.NewRuntimeAnalyzer(store)

	t.Run("returns chronological points", func(t *testing.T) {
		t.Parallel()

		points, err := analyzer.RuntimeTrend(
			ctx, "TestSlow/very_slow",
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		require.Len(t, points, 3)

		for i := 1; i < len(points); i++ {
			assert.False(t, points[i].Timestamp.Before(points[i-1].Timestamp),
				"points should be in chronological order")
		}
	})

	t.Run("returns empty for unknown test", func(t *testing.T) {
		t.Parallel()

		points, err := analyzer.RuntimeTrend(
			ctx, "TestNonExistent",
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		assert.Empty(t, points)
	})

	t.Run("durations match ingested values", func(t *testing.T) {
		t.Parallel()

		points, err := analyzer.RuntimeTrend(
			ctx, "TestMedium/moderate",
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		require.Len(t, points, 2)

		assert.InDelta(t, 30.0, points[0].Duration.Seconds(), 0.001)
		assert.InDelta(t, 35.0, points[1].Duration.Seconds(), 0.001)
	})
}

func ingestRuntimeData(
	t *testing.T,
	store *flakiness.Store,
	ctx context.Context,
	ts time.Time,
) {
	t.Helper()

	results := []flakiness.TestResult{
		{
			Name:      "TestSlow/very_slow",
			Suite:     "e2e",
			Job:       "periodic-ci",
			BuildID:   "b1",
			Result:    flakiness.OutcomePass,
			Duration:  120 * time.Second,
			Timestamp: ts,
		},
		{
			Name:      "TestSlow/very_slow",
			Suite:     "e2e",
			Job:       "periodic-ci",
			BuildID:   "b2",
			Result:    flakiness.OutcomePass,
			Duration:  130 * time.Second,
			Timestamp: ts.Add(time.Minute),
		},
		{
			Name:      "TestSlow/very_slow",
			Suite:     "e2e",
			Job:       "periodic-ci",
			BuildID:   "b3",
			Result:    flakiness.OutcomeFail,
			Duration:  150 * time.Second,
			Timestamp: ts.Add(2 * time.Minute),
		},
		{
			Name:      "TestMedium/moderate",
			Suite:     "e2e",
			Job:       "periodic-ci",
			BuildID:   "b1",
			Result:    flakiness.OutcomePass,
			Duration:  30 * time.Second,
			Timestamp: ts,
		},
		{
			Name:      "TestMedium/moderate",
			Suite:     "e2e",
			Job:       "periodic-ci",
			BuildID:   "b2",
			Result:    flakiness.OutcomePass,
			Duration:  35 * time.Second,
			Timestamp: ts.Add(time.Minute),
		},
		{
			Name:      "TestFast/quick",
			Suite:     "integration",
			Job:       "periodic-ci",
			BuildID:   "b1",
			Result:    flakiness.OutcomePass,
			Duration:  2 * time.Second,
			Timestamp: ts,
		},
	}

	app := store.Appender(ctx)

	for _, r := range results {
		require.NoError(t, flakiness.RecordTestResult(app, r))
	}

	require.NoError(t, app.Commit())
}
