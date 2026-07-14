package flakiness_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

func makeRecord(base time.Time, failures []bool) flakiness.TestRecord {
	history := make([]flakiness.RunOutcome, len(failures))
	failed, passed := 0, 0

	for i, f := range failures {
		history[i] = flakiness.RunOutcome{
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Failed:    f,
		}

		if f {
			failed++
		} else {
			passed++
		}
	}

	return flakiness.TestRecord{
		TotalRuns:  len(failures),
		FailedRuns: failed,
		PassedRuns: passed,
		History:    history,
	}
}

func scatteredFlakyRecord(base time.Time) flakiness.TestRecord {
	return makeRecord(base, []bool{false, true, false, false, true, false, false, true, false, false})
}

func highFlakyRecord(base time.Time) flakiness.TestRecord {
	return makeRecord(base, []bool{true, false, true, false, false, true, false, false, true, false})
}

func lowFlakyRecord(base time.Time) flakiness.TestRecord {
	return makeRecord(base, []bool{true, false, false, false, false, false, false, false, false, false})
}

func regressionRecord(base time.Time) flakiness.TestRecord {
	return makeRecord(base, []bool{false, false, false, true, true, true})
}

func persistentRecord(base time.Time) flakiness.TestRecord {
	return makeRecord(base, []bool{true, true, true, true, true})
}

func TestFlakeRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		total    int
		failed   int
		expected float64
	}{
		{"no runs", 0, 0, 0},
		{"all pass", 10, 0, 0},
		{"all fail", 10, 10, 1.0},
		{"half fail", 10, 5, 0.5},
		{"one of three", 3, 1, 1.0 / 3.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := &flakiness.TestRecord{
				TotalRuns:  tc.total,
				FailedRuns: tc.failed,
			}
			assert.InDelta(t, tc.expected, r.FlakeRate(), 0.001)
		})
	}
}

func TestClassifyPattern(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		record   flakiness.TestRecord
		expected flakiness.FailurePattern
	}{
		{
			name:     "no runs is healthy",
			record:   flakiness.TestRecord{TotalRuns: 0},
			expected: flakiness.PatternHealthy,
		},
		{
			name: "no failures is healthy",
			record: flakiness.TestRecord{
				TotalRuns:  5,
				FailedRuns: 0,
				PassedRuns: 5,
			},
			expected: flakiness.PatternHealthy,
		},
		{
			name: "all failures is persistent",
			record: flakiness.TestRecord{
				TotalRuns:  5,
				FailedRuns: 5,
				PassedRuns: 0,
				History: []flakiness.RunOutcome{
					{Timestamp: base, Failed: true},
					{Timestamp: base.Add(time.Hour), Failed: true},
					{Timestamp: base.Add(2 * time.Hour), Failed: true},
					{Timestamp: base.Add(3 * time.Hour), Failed: true},
					{Timestamp: base.Add(4 * time.Hour), Failed: true},
				},
			},
			expected: flakiness.PatternPersistent,
		},
		{
			name:     "scattered failures is flaky",
			record:   scatteredFlakyRecord(base),
			expected: flakiness.PatternFlaky,
		},
		{
			name: "trailing failures with clean history is regression",
			record: flakiness.TestRecord{
				TotalRuns:  8,
				FailedRuns: 3,
				PassedRuns: 5,
				History: []flakiness.RunOutcome{
					{Timestamp: base, Failed: false},
					{Timestamp: base.Add(time.Hour), Failed: false},
					{Timestamp: base.Add(2 * time.Hour), Failed: false},
					{Timestamp: base.Add(3 * time.Hour), Failed: false},
					{Timestamp: base.Add(4 * time.Hour), Failed: false},
					{Timestamp: base.Add(5 * time.Hour), Failed: true, BuildID: "500", CommitSHA: "abc123"},
					{Timestamp: base.Add(6 * time.Hour), Failed: true, BuildID: "501"},
					{Timestamp: base.Add(7 * time.Hour), Failed: true, BuildID: "502"},
				},
			},
			expected: flakiness.PatternRegression,
		},
		{
			name: "trailing failures with flaky history stays flaky",
			record: flakiness.TestRecord{
				TotalRuns:  8,
				FailedRuns: 5,
				PassedRuns: 3,
				History: []flakiness.RunOutcome{
					{Timestamp: base, Failed: true},
					{Timestamp: base.Add(time.Hour), Failed: false},
					{Timestamp: base.Add(2 * time.Hour), Failed: true},
					{Timestamp: base.Add(3 * time.Hour), Failed: false},
					{Timestamp: base.Add(4 * time.Hour), Failed: false},
					{Timestamp: base.Add(5 * time.Hour), Failed: true},
					{Timestamp: base.Add(6 * time.Hour), Failed: true},
					{Timestamp: base.Add(7 * time.Hour), Failed: true},
				},
			},
			expected: flakiness.PatternFlaky,
		},
		{
			name: "fewer than 3 trailing failures is flaky",
			record: flakiness.TestRecord{
				TotalRuns:  5,
				FailedRuns: 2,
				PassedRuns: 3,
				History: []flakiness.RunOutcome{
					{Timestamp: base, Failed: false},
					{Timestamp: base.Add(time.Hour), Failed: false},
					{Timestamp: base.Add(2 * time.Hour), Failed: false},
					{Timestamp: base.Add(3 * time.Hour), Failed: true},
					{Timestamp: base.Add(4 * time.Hour), Failed: true},
				},
			},
			expected: flakiness.PatternFlaky,
		},
		{
			name: "unsorted history is sorted before classification",
			record: flakiness.TestRecord{
				TotalRuns:  6,
				FailedRuns: 3,
				PassedRuns: 3,
				History: []flakiness.RunOutcome{
					{Timestamp: base.Add(5 * time.Hour), Failed: true},
					{Timestamp: base, Failed: false},
					{Timestamp: base.Add(3 * time.Hour), Failed: true},
					{Timestamp: base.Add(time.Hour), Failed: false},
					{Timestamp: base.Add(4 * time.Hour), Failed: true},
					{Timestamp: base.Add(2 * time.Hour), Failed: false},
				},
			},
			expected: flakiness.PatternRegression,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, tc.record.ClassifyPattern())
		})
	}
}

func TestClassifyPatternWith_MinRuns(t *testing.T) {
	t.Parallel()

	r := &flakiness.TestRecord{
		TotalRuns:  2,
		FailedRuns: 1,
		PassedRuns: 1,
		History: []flakiness.RunOutcome{
			{Failed: false},
			{Failed: true},
		},
	}

	opts := flakiness.ClassifyOptions{MinRuns: 5}

	assert.Equal(t, flakiness.PatternHealthy, r.ClassifyPatternWith(opts),
		"tests below MinRuns should be classified as healthy")

	assert.Equal(t, flakiness.PatternFlaky, r.ClassifyPattern(),
		"same record with default MinRuns should be classified")
}

func TestClassifyPatternWith_MinRunsOverridesRegression(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	r := &flakiness.TestRecord{
		TotalRuns:  4,
		FailedRuns: 3,
		PassedRuns: 1,
		History: []flakiness.RunOutcome{
			{Timestamp: base, Failed: false},
			{Timestamp: base.Add(time.Hour), Failed: true},
			{Timestamp: base.Add(2 * time.Hour), Failed: true},
			{Timestamp: base.Add(3 * time.Hour), Failed: true},
		},
	}

	assert.Equal(t, flakiness.PatternRegression, r.ClassifyPattern(),
		"with default MinRuns the pattern should be regression")

	opts := flakiness.ClassifyOptions{MinRuns: 5}
	assert.Equal(t, flakiness.PatternHealthy, r.ClassifyPatternWith(opts),
		"regression pattern must become healthy when below MinRuns")
	assert.Empty(t, r.TransitionCommitWith(opts),
		"no transition commit when below MinRuns")
}

func TestShouldQuarantine(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	opts := flakiness.DefaultClassifyOptions()

	t.Run("flaky above threshold is quarantined", func(t *testing.T) {
		t.Parallel()

		r := highFlakyRecord(base)
		assert.True(t, r.ShouldQuarantine(0.2, opts))
	})

	t.Run("flaky below threshold is not quarantined", func(t *testing.T) {
		t.Parallel()

		r := lowFlakyRecord(base)
		assert.False(t, r.ShouldQuarantine(0.2, opts))
	})

	t.Run("regression is never quarantined", func(t *testing.T) {
		t.Parallel()

		r := regressionRecord(base)
		assert.Equal(t, flakiness.PatternRegression, r.ClassifyPattern())
		assert.False(t, r.ShouldQuarantine(0.1, opts),
			"regressions must not be quarantined")
	})

	t.Run("persistent is never quarantined", func(t *testing.T) {
		t.Parallel()

		r := persistentRecord(base)
		assert.False(t, r.ShouldQuarantine(0.1, opts),
			"persistent failures must not be quarantined")
	})

	t.Run("healthy is never quarantined", func(t *testing.T) {
		t.Parallel()

		r := &flakiness.TestRecord{
			TotalRuns:  5,
			FailedRuns: 0,
			PassedRuns: 5,
		}

		assert.False(t, r.ShouldQuarantine(0.0, opts))
	})
}

func TestTransitionCommit(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	t.Run("returns commit SHA when available", func(t *testing.T) {
		t.Parallel()

		r := &flakiness.TestRecord{
			TotalRuns:  6,
			FailedRuns: 3,
			PassedRuns: 3,
			History: []flakiness.RunOutcome{
				{Timestamp: base, Failed: false},
				{Timestamp: base.Add(time.Hour), Failed: false},
				{Timestamp: base.Add(2 * time.Hour), Failed: false},
				{Timestamp: base.Add(3 * time.Hour), Failed: true, CommitSHA: "abc123", BuildID: "100"},
				{Timestamp: base.Add(4 * time.Hour), Failed: true, CommitSHA: "def456", BuildID: "101"},
				{Timestamp: base.Add(5 * time.Hour), Failed: true, CommitSHA: "ghi789", BuildID: "102"},
			},
		}

		assert.Equal(t, "abc123", r.TransitionCommit())
	})

	t.Run("falls back to build ID when no commit SHA", func(t *testing.T) {
		t.Parallel()

		r := &flakiness.TestRecord{
			TotalRuns:  6,
			FailedRuns: 3,
			PassedRuns: 3,
			History: []flakiness.RunOutcome{
				{Timestamp: base, Failed: false, BuildID: "97"},
				{Timestamp: base.Add(time.Hour), Failed: false, BuildID: "98"},
				{Timestamp: base.Add(2 * time.Hour), Failed: false, BuildID: "99"},
				{Timestamp: base.Add(3 * time.Hour), Failed: true, BuildID: "100"},
				{Timestamp: base.Add(4 * time.Hour), Failed: true, BuildID: "101"},
				{Timestamp: base.Add(5 * time.Hour), Failed: true, BuildID: "102"},
			},
		}

		assert.Equal(t, "100", r.TransitionCommit())
	})

	t.Run("returns empty for non-regression", func(t *testing.T) {
		t.Parallel()

		r := &flakiness.TestRecord{
			TotalRuns:  3,
			FailedRuns: 0,
			PassedRuns: 3,
		}

		assert.Empty(t, r.TransitionCommit())
	})
}

func TestTransitionPR(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	t.Run("returns PR number for regression", func(t *testing.T) {
		t.Parallel()

		r := &flakiness.TestRecord{
			TotalRuns:  6,
			FailedRuns: 3,
			PassedRuns: 3,
			History: []flakiness.RunOutcome{
				{Timestamp: base, Failed: false},
				{Timestamp: base.Add(time.Hour), Failed: false},
				{Timestamp: base.Add(2 * time.Hour), Failed: false},
				{Timestamp: base.Add(3 * time.Hour), Failed: true, PRNumber: "42"},
				{Timestamp: base.Add(4 * time.Hour), Failed: true, PRNumber: "42"},
				{Timestamp: base.Add(5 * time.Hour), Failed: true, PRNumber: "43"},
			},
		}

		assert.Equal(t, "42", r.TransitionPR())
	})

	t.Run("returns empty for non-regression", func(t *testing.T) {
		t.Parallel()

		r := &flakiness.TestRecord{
			TotalRuns:  3,
			FailedRuns: 0,
			PassedRuns: 3,
		}

		assert.Empty(t, r.TransitionPR())
	})
}
