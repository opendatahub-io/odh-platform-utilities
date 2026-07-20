package flakiness_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

func TestAnalyzeBudget_PipelineUtilisation(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	ingestBudgetData(t, store, ctx, ts)
	analyzer := flakiness.NewRuntimeAnalyzer(store)

	t.Run("computes pipeline utilisation", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.TimeoutConfig{
			PipelineTimeout: time.Hour,
		}

		report, err := analyzer.AnalyzeBudget(
			ctx, cfg, flakiness.DefaultNearTimeoutThreshold,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		require.NotNil(t, report.Pipeline)

		assert.Equal(t, "pipeline", report.Pipeline.Name)
		assert.Equal(t, time.Hour, report.Pipeline.ConfiguredTimeout)
		assert.Greater(t, report.Pipeline.ActualDuration, time.Duration(0))
		assert.Greater(t, report.Pipeline.Utilisation, 0.0)
	})

	t.Run("nil pipeline when no timeout configured", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.TimeoutConfig{}

		report, err := analyzer.AnalyzeBudget(
			ctx, cfg, flakiness.DefaultNearTimeoutThreshold,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		assert.Nil(t, report.Pipeline)
	})
}

func TestAnalyzeBudget_SuiteUtilisation(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	ingestBudgetData(t, store, ctx, ts)
	analyzer := flakiness.NewRuntimeAnalyzer(store)

	t.Run("computes per-suite budgets", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.TimeoutConfig{
			SuiteTimeouts: map[string]time.Duration{
				"e2e":         30 * time.Minute,
				"integration": 10 * time.Minute,
			},
		}

		report, err := analyzer.AnalyzeBudget(
			ctx, cfg, flakiness.DefaultNearTimeoutThreshold,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		require.Len(t, report.Suites, 2)

		for _, s := range report.Suites {
			assert.Greater(t, s.ActualDuration, time.Duration(0))
			assert.Greater(t, s.ConfiguredTimeout, time.Duration(0))
			assert.Greater(t, s.Utilisation, 0.0)
		}
	})

	t.Run("sorted by utilisation descending", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.TimeoutConfig{
			SuiteTimeouts: map[string]time.Duration{
				"e2e":         30 * time.Minute,
				"integration": 10 * time.Minute,
			},
		}

		report, err := analyzer.AnalyzeBudget(
			ctx, cfg, flakiness.DefaultNearTimeoutThreshold,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		require.Len(t, report.Suites, 2)

		assert.GreaterOrEqual(t, report.Suites[0].Utilisation,
			report.Suites[1].Utilisation)
	})

	t.Run("nil when no suite timeouts configured", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.TimeoutConfig{
			PipelineTimeout: time.Hour,
		}

		report, err := analyzer.AnalyzeBudget(
			ctx, cfg, flakiness.DefaultNearTimeoutThreshold,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		assert.Nil(t, report.Suites)
	})
}

func TestAnalyzeBudget_NearTimeout(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	ingestBudgetData(t, store, ctx, ts)
	analyzer := flakiness.NewRuntimeAnalyzer(store)

	t.Run("detects tests near their timeout", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.TimeoutConfig{
			TestTimeouts: map[string]time.Duration{
				"TestNearTimeout/tight": 5 * time.Minute,
				"TestFast/quick":        10 * time.Minute,
			},
		}

		report, err := analyzer.AnalyzeBudget(
			ctx, cfg, flakiness.DefaultNearTimeoutThreshold,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		require.NotEmpty(t, report.NearTimeout)

		found := false
		for _, nt := range report.NearTimeout {
			if nt.Name == "TestNearTimeout/tight" {
				found = true
				assert.GreaterOrEqual(t, nt.Utilisation, 0.80)
				assert.Equal(t, 5*time.Minute, nt.Timeout)
			}
		}

		assert.True(t, found, "expected TestNearTimeout/tight to be flagged")
	})

	t.Run("respects custom threshold", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.TimeoutConfig{
			TestTimeouts: map[string]time.Duration{
				"TestNearTimeout/tight": 5 * time.Minute,
				"TestFast/quick":        10 * time.Minute,
			},
		}

		report, err := analyzer.AnalyzeBudget(
			ctx, cfg, 0.99,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)

		for _, nt := range report.NearTimeout {
			assert.GreaterOrEqual(t, nt.Utilisation, 0.99)
		}
	})

	t.Run("nil when no test timeouts configured", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.TimeoutConfig{
			PipelineTimeout: time.Hour,
		}

		report, err := analyzer.AnalyzeBudget(
			ctx, cfg, flakiness.DefaultNearTimeoutThreshold,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		assert.Nil(t, report.NearTimeout)
	})
}

func TestAnalyzeBudget_Recommendations(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	ingestBudgetData(t, store, ctx, ts)
	analyzer := flakiness.NewRuntimeAnalyzer(store)

	t.Run("recommends raise for near-timeout tests", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.TimeoutConfig{
			TestTimeouts: map[string]time.Duration{
				"TestNearTimeout/tight": 5 * time.Minute,
			},
		}

		report, err := analyzer.AnalyzeBudget(
			ctx, cfg, flakiness.DefaultNearTimeoutThreshold,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)

		var raise *flakiness.TimeoutRecommendation
		for i := range report.Recommendations {
			if report.Recommendations[i].Name == "TestNearTimeout/tight" {
				raise = &report.Recommendations[i]

				break
			}
		}

		require.NotNil(t, raise)
		assert.Equal(t, flakiness.RecommendRaise, raise.Action)
		assert.Greater(t, raise.SuggestedTimeout, raise.CurrentTimeout)
	})

	t.Run("recommends lower for over-provisioned tests", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.TimeoutConfig{
			TestTimeouts: map[string]time.Duration{
				"TestFast/quick": 10 * time.Minute,
			},
		}

		report, err := analyzer.AnalyzeBudget(
			ctx, cfg, flakiness.DefaultNearTimeoutThreshold,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)

		var lower *flakiness.TimeoutRecommendation
		for i := range report.Recommendations {
			if report.Recommendations[i].Name == "TestFast/quick" {
				lower = &report.Recommendations[i]

				break
			}
		}

		require.NotNil(t, lower)
		assert.Equal(t, flakiness.RecommendLower, lower.Action)
		assert.Less(t, lower.SuggestedTimeout, lower.CurrentTimeout)
	})

	t.Run("nil when no test timeouts configured", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.TimeoutConfig{}

		report, err := analyzer.AnalyzeBudget(
			ctx, cfg, flakiness.DefaultNearTimeoutThreshold,
			ts.Add(-time.Minute), ts.Add(5*time.Minute),
		)
		require.NoError(t, err)
		assert.Nil(t, report.Recommendations)
	})
}

func TestAnalyzeBudget_EmptyStore(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	analyzer := flakiness.NewRuntimeAnalyzer(store)

	cfg := flakiness.TimeoutConfig{
		PipelineTimeout: time.Hour,
		SuiteTimeouts:   map[string]time.Duration{"e2e": 30 * time.Minute},
		TestTimeouts:    map[string]time.Duration{"TestFoo": 5 * time.Minute},
	}

	now := time.Now()
	report, err := analyzer.AnalyzeBudget(
		ctx, cfg, flakiness.DefaultNearTimeoutThreshold,
		now.Add(-time.Hour), now,
	)
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.NotNil(t, report.Pipeline)
	assert.Equal(t, time.Duration(0), report.Pipeline.ActualDuration)
	assert.InDelta(t, 0.0, report.Pipeline.Utilisation, 0.001)
}

func ingestBudgetData(
	t *testing.T,
	store *flakiness.Store,
	ctx context.Context,
	ts time.Time,
) {
	t.Helper()

	results := []flakiness.TestResult{
		{
			Name:      "TestNearTimeout/tight",
			Suite:     "e2e",
			Job:       "periodic-ci",
			BuildID:   "b1",
			Result:    flakiness.OutcomePass,
			Duration:  4*time.Minute + 30*time.Second,
			Timestamp: ts,
		},
		{
			Name:      "TestNearTimeout/tight",
			Suite:     "e2e",
			Job:       "periodic-ci",
			BuildID:   "b2",
			Result:    flakiness.OutcomePass,
			Duration:  4*time.Minute + 45*time.Second,
			Timestamp: ts.Add(time.Minute),
		},
		{
			Name:      "TestNearTimeout/tight",
			Suite:     "e2e",
			Job:       "periodic-ci",
			BuildID:   "b3",
			Result:    flakiness.OutcomePass,
			Duration:  4*time.Minute + 50*time.Second,
			Timestamp: ts.Add(2 * time.Minute),
		},
		{
			Name:      "TestNearTimeout/tight",
			Suite:     "e2e",
			Job:       "periodic-ci",
			BuildID:   "b4",
			Result:    flakiness.OutcomeFail,
			Duration:  5 * time.Minute,
			Timestamp: ts.Add(3 * time.Minute),
		},
		{
			Name:      "TestNearTimeout/tight",
			Suite:     "e2e",
			Job:       "periodic-ci",
			BuildID:   "b5",
			Result:    flakiness.OutcomePass,
			Duration:  4*time.Minute + 40*time.Second,
			Timestamp: ts.Add(4 * time.Minute),
		},
		{
			Name:      "TestFast/quick",
			Suite:     "integration",
			Job:       "periodic-ci",
			BuildID:   "b1",
			Result:    flakiness.OutcomePass,
			Duration:  3 * time.Second,
			Timestamp: ts,
		},
		{
			Name:      "TestFast/quick",
			Suite:     "integration",
			Job:       "periodic-ci",
			BuildID:   "b2",
			Result:    flakiness.OutcomePass,
			Duration:  2 * time.Second,
			Timestamp: ts.Add(time.Minute),
		},
		{
			Name:      "TestFast/quick",
			Suite:     "integration",
			Job:       "periodic-ci",
			BuildID:   "b3",
			Result:    flakiness.OutcomePass,
			Duration:  4 * time.Second,
			Timestamp: ts.Add(2 * time.Minute),
		},
		{
			Name:      "TestFast/quick",
			Suite:     "integration",
			Job:       "periodic-ci",
			BuildID:   "b4",
			Result:    flakiness.OutcomePass,
			Duration:  3 * time.Second,
			Timestamp: ts.Add(3 * time.Minute),
		},
		{
			Name:      "TestFast/quick",
			Suite:     "integration",
			Job:       "periodic-ci",
			BuildID:   "b5",
			Result:    flakiness.OutcomePass,
			Duration:  2 * time.Second,
			Timestamp: ts.Add(4 * time.Minute),
		},
	}

	app := store.Appender(ctx)

	for _, r := range results {
		require.NoError(t, flakiness.RecordTestResult(app, r))
	}

	require.NoError(t, app.Commit())
}
