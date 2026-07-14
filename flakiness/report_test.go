package flakiness_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

func TestReport_ExceedingThreshold(t *testing.T) {
	t.Parallel()

	report := &flakiness.Report{
		Tests: map[string]*flakiness.TestRecord{
			"TestA": {Name: "TestA", TotalRuns: 10, FailedRuns: 8, PassedRuns: 2},
			"TestB": {Name: "TestB", TotalRuns: 10, FailedRuns: 3, PassedRuns: 7},
			"TestC": {Name: "TestC", TotalRuns: 10, FailedRuns: 1, PassedRuns: 9},
		},
		Options: flakiness.DefaultClassifyOptions(),
	}

	results := report.ExceedingThreshold(0.25)

	require.Len(t, results, 2)
	assert.Equal(t, "TestA", results[0].Name)
	assert.Equal(t, "TestB", results[1].Name)
}

func TestReport_Regressions(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	report := &flakiness.Report{
		Tests: map[string]*flakiness.TestRecord{
			"TestRegression": {
				Name:       "TestRegression",
				TotalRuns:  6,
				FailedRuns: 3,
				PassedRuns: 3,
				LastFailed: base.Add(5 * time.Hour),
				History: []flakiness.RunOutcome{
					{Timestamp: base, Failed: false},
					{Timestamp: base.Add(time.Hour), Failed: false},
					{Timestamp: base.Add(2 * time.Hour), Failed: false},
					{Timestamp: base.Add(3 * time.Hour), Failed: true},
					{Timestamp: base.Add(4 * time.Hour), Failed: true},
					{Timestamp: base.Add(5 * time.Hour), Failed: true},
				},
			},
			"TestHealthy": {
				Name:       "TestHealthy",
				TotalRuns:  5,
				FailedRuns: 0,
				PassedRuns: 5,
			},
		},
		Options: flakiness.DefaultClassifyOptions(),
	}

	results := report.Regressions()

	require.Len(t, results, 1)
	assert.Equal(t, "TestRegression", results[0].Name)
}

func TestReport_FlakyTests(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	flakyRec := highFlakyRecord(base)
	flakyRec.Name = "TestFlaky"
	barelyRec := lowFlakyRecord(base)
	barelyRec.Name = "TestBarely"

	report := &flakiness.Report{
		Tests: map[string]*flakiness.TestRecord{
			"TestFlaky":  &flakyRec,
			"TestBarely": &barelyRec,
		},
		Options: flakiness.DefaultClassifyOptions(),
	}

	results := report.FlakyTests(0.2)

	require.Len(t, results, 1)
	assert.Equal(t, "TestFlaky", results[0].Name)
}

func TestReport_Classify(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	report := &flakiness.Report{
		Tests: map[string]*flakiness.TestRecord{
			"TestHealthy": {
				Name: "TestHealthy", TotalRuns: 5, PassedRuns: 5,
			},
			"TestRegression": {
				Name:       "TestRegression",
				TotalRuns:  6,
				FailedRuns: 3,
				PassedRuns: 3,
				LastFailed: base.Add(5 * time.Hour),
				History: []flakiness.RunOutcome{
					{Timestamp: base, Failed: false},
					{Timestamp: base.Add(time.Hour), Failed: false},
					{Timestamp: base.Add(2 * time.Hour), Failed: false},
					{Timestamp: base.Add(3 * time.Hour), Failed: true, CommitSHA: "abc123", PRNumber: "42"},
					{Timestamp: base.Add(4 * time.Hour), Failed: true, CommitSHA: "def456"},
					{Timestamp: base.Add(5 * time.Hour), Failed: true, CommitSHA: "ghi789"},
				},
			},
		},
		Options: flakiness.DefaultClassifyOptions(),
	}

	classifications := report.Classify()
	require.Len(t, classifications, 2)

	byName := make(map[string]flakiness.TestClassification)
	for _, c := range classifications {
		byName[c.Name] = c
	}

	healthy := byName["TestHealthy"]
	assert.Equal(t, flakiness.PatternHealthy, healthy.Classification)
	assert.Empty(t, healthy.RegressionSHA)

	regression := byName["TestRegression"]
	assert.Equal(t, flakiness.PatternRegression, regression.Classification)
	assert.Equal(t, "abc123", regression.RegressionSHA)
	assert.Equal(t, "42", regression.RegressionPR)
	assert.InDelta(t, 0.5, regression.FlakeRate, 0.001)
}

func TestReport_Classify_WithQuarantineThreshold(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	flakyRec := highFlakyRecord(base)
	flakyRec.Name = "TestFlaky"
	regRec := regressionRecord(base)
	regRec.Name = "TestRegression"

	report := &flakiness.Report{
		Tests: map[string]*flakiness.TestRecord{
			"TestFlaky":      &flakyRec,
			"TestRegression": &regRec,
			"TestHealthy":    {Name: "TestHealthy", TotalRuns: 5, PassedRuns: 5},
		},
		Options: flakiness.DefaultClassifyOptions(),
	}

	classifications := report.ClassifyWithQuarantine(0.2)

	byName := make(map[string]flakiness.TestClassification)
	for _, c := range classifications {
		byName[c.Name] = c
	}

	assert.True(t, byName["TestFlaky"].Quarantine,
		"flaky test above threshold should be quarantined")
	assert.False(t, byName["TestRegression"].Quarantine,
		"regression must not be quarantined")
	assert.False(t, byName["TestHealthy"].Quarantine,
		"healthy test must not be quarantined")
}

func TestReport_Classify_WithoutThreshold(t *testing.T) {
	t.Parallel()

	report := &flakiness.Report{
		Tests: map[string]*flakiness.TestRecord{
			"TestFlaky": {
				Name: "TestFlaky", TotalRuns: 10, FailedRuns: 5, PassedRuns: 5,
				History: []flakiness.RunOutcome{
					{Failed: true}, {Failed: false}, {Failed: true},
					{Failed: false}, {Failed: true}, {Failed: false},
					{Failed: true}, {Failed: false}, {Failed: true},
					{Failed: false},
				},
			},
		},
		Options: flakiness.DefaultClassifyOptions(),
	}

	classifications := report.Classify()
	assert.False(t, classifications[0].Quarantine,
		"quarantine should be false when no threshold is provided")
}

func TestReport_QuarantineList(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	flakyRec := highFlakyRecord(base)
	flakyRec.Name = "TestFlaky"
	flakyRec.LastSeen = base.Add(9 * time.Hour)
	flakyRec.LastFailed = base.Add(8 * time.Hour)

	regRec := regressionRecord(base)
	regRec.Name = "TestRegression"
	regRec.LastFailed = base.Add(5 * time.Hour)
	regRec.History[3].CommitSHA = "abc"
	regRec.History[3].PRNumber = "99"

	persRec := persistentRecord(base)
	persRec.Name = "TestPersistent"
	persRec.LastFailed = base.Add(4 * time.Hour)

	report := &flakiness.Report{
		Tests: map[string]*flakiness.TestRecord{
			"TestFlaky":      &flakyRec,
			"TestRegression": &regRec,
			"TestPersistent": &persRec,
			"TestHealthy":    {Name: "TestHealthy", TotalRuns: 5, PassedRuns: 5},
		},
		Options: flakiness.DefaultClassifyOptions(),
	}

	entries := report.QuarantineList(0.2)

	require.Len(t, entries, 3, "healthy tests should be excluded")

	byName := make(map[string]flakiness.QuarantineEntry)
	for _, e := range entries {
		byName[e.Name] = e
	}

	flaky := byName["TestFlaky"]
	assert.True(t, flaky.Quarantined)
	assert.Equal(t, flakiness.PatternFlaky, flaky.Classification)
	assert.InDelta(t, 0.4, flaky.FlakeRate, 0.001)

	regression := byName["TestRegression"]
	assert.False(t, regression.Quarantined, "regressions must not be quarantined")
	assert.Equal(t, flakiness.PatternRegression, regression.Classification)
	assert.Equal(t, "abc", regression.RegressionSHA)
	assert.Equal(t, "99", regression.RegressionPR)

	persistent := byName["TestPersistent"]
	assert.False(t, persistent.Quarantined, "persistent failures must not be quarantined")
	assert.Equal(t, flakiness.PatternPersistent, persistent.Classification)
}

func TestReport_QuarantineList_JSONCompatible(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	regRec := regressionRecord(base)
	regRec.Name = "TestBroken"
	regRec.LastFailed = base.Add(5 * time.Hour)
	regRec.History[3].CommitSHA = "deadbeef"
	regRec.History[3].PRNumber = "42"

	report := &flakiness.Report{
		Tests: map[string]*flakiness.TestRecord{
			"TestBroken": &regRec,
		},
		Options: flakiness.DefaultClassifyOptions(),
	}

	entries := report.QuarantineList(0.2)
	require.Len(t, entries, 1)

	entry := entries[0]
	assert.Equal(t, "TestBroken", entry.Name)
	assert.False(t, entry.Quarantined)
	assert.Equal(t, "deadbeef", entry.RegressionSHA)
	assert.Equal(t, "42", entry.RegressionPR)
}

func TestAnalyze_IntegrationWithStore(t *testing.T) {
	t.Parallel()

	store, err := flakiness.NewStore()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	ctx := context.Background()
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	appender := store.Appender(ctx)

	stableResults := []flakiness.TestResult{
		{Name: "TestStable", Suite: "e2e", Job: "nightly", BuildID: "1", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base},
		{Name: "TestStable", Suite: "e2e", Job: "nightly", BuildID: "2", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base.Add(time.Hour)},
		{Name: "TestStable", Suite: "e2e", Job: "nightly", BuildID: "3", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base.Add(2 * time.Hour)},
	}

	flakyResults := []flakiness.TestResult{
		{Name: "TestFlaky", Suite: "e2e", Job: "nightly", BuildID: "1", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base},
		{Name: "TestFlaky", Suite: "e2e", Job: "nightly", BuildID: "2", Result: flakiness.OutcomeFail, Duration: 2 * time.Second, Timestamp: base.Add(time.Hour)},
		{Name: "TestFlaky", Suite: "e2e", Job: "nightly", BuildID: "3", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base.Add(2 * time.Hour)},
		{Name: "TestFlaky", Suite: "e2e", Job: "nightly", BuildID: "4", Result: flakiness.OutcomeFail, Duration: 2 * time.Second, Timestamp: base.Add(3 * time.Hour)},
		{Name: "TestFlaky", Suite: "e2e", Job: "nightly", BuildID: "5", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base.Add(4 * time.Hour)},
	}

	regressionResults := []flakiness.TestResult{
		{Name: "TestBroken", Suite: "e2e", Job: "nightly", BuildID: "1", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base, CommitSHA: "aaa"},
		{Name: "TestBroken", Suite: "e2e", Job: "nightly", BuildID: "2", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base.Add(time.Hour), CommitSHA: "bbb"},
		{Name: "TestBroken", Suite: "e2e", Job: "nightly", BuildID: "3", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base.Add(2 * time.Hour), CommitSHA: "ccc"},
		{Name: "TestBroken", Suite: "e2e", Job: "nightly", BuildID: "4", Result: flakiness.OutcomeFail, Duration: 3 * time.Second, Timestamp: base.Add(3 * time.Hour), CommitSHA: "ddd", PRNumber: "99"},
		{Name: "TestBroken", Suite: "e2e", Job: "nightly", BuildID: "5", Result: flakiness.OutcomeFail, Duration: 3 * time.Second, Timestamp: base.Add(4 * time.Hour), CommitSHA: "eee"},
		{Name: "TestBroken", Suite: "e2e", Job: "nightly", BuildID: "6", Result: flakiness.OutcomeFail, Duration: 3 * time.Second, Timestamp: base.Add(5 * time.Hour), CommitSHA: "fff"},
	}

	allResults := append(append(stableResults, flakyResults...), regressionResults...)
	for _, r := range allResults {
		require.NoError(t, flakiness.RecordTestResult(appender, r))
	}

	require.NoError(t, appender.Commit())

	start := base.Add(-time.Minute)
	end := base.Add(6 * time.Hour)

	report, err := flakiness.Analyze(ctx, store, start, end, flakiness.DefaultClassifyOptions())
	require.NoError(t, err)

	require.Len(t, report.Tests, 3)

	stable := report.Tests[flakiness.TestKey("TestStable", "e2e", "nightly")]
	require.NotNil(t, stable)
	assert.Equal(t, 3, stable.TotalRuns)
	assert.Equal(t, 0, stable.FailedRuns)
	assert.Equal(t, flakiness.PatternHealthy, stable.ClassifyPattern())

	flaky := report.Tests[flakiness.TestKey("TestFlaky", "e2e", "nightly")]
	require.NotNil(t, flaky)
	assert.Equal(t, 5, flaky.TotalRuns)
	assert.Equal(t, 2, flaky.FailedRuns)
	assert.Equal(t, flakiness.PatternFlaky, flaky.ClassifyPattern())

	broken := report.Tests[flakiness.TestKey("TestBroken", "e2e", "nightly")]
	require.NotNil(t, broken)
	assert.Equal(t, 6, broken.TotalRuns)
	assert.Equal(t, 3, broken.FailedRuns)
	assert.Equal(t, flakiness.PatternRegression, broken.ClassifyPattern())
	assert.Equal(t, "ddd", broken.TransitionCommit())

	regressions := report.Regressions()
	require.Len(t, regressions, 1)
	assert.Equal(t, "TestBroken", regressions[0].Name)

	flakyTests := report.FlakyTests(0.1)
	require.Len(t, flakyTests, 1)
	assert.Equal(t, "TestFlaky", flakyTests[0].Name)

	exceeding := report.ExceedingThreshold(0.3)
	require.Len(t, exceeding, 2)

	classifications := report.Classify()
	require.Len(t, classifications, 3)

	byName := make(map[string]flakiness.TestClassification)
	for _, c := range classifications {
		byName[c.Name] = c
	}

	assert.Equal(t, "ddd", byName["TestBroken"].RegressionSHA)
	assert.Equal(t, "99", byName["TestBroken"].RegressionPR)
}

func TestAnalyze_GracefulWithoutCommitSHA(t *testing.T) {
	t.Parallel()

	store, err := flakiness.NewStore()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	ctx := context.Background()
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	appender := store.Appender(ctx)

	results := []flakiness.TestResult{
		{Name: "TestNoSHA", Suite: "e2e", Job: "ci", BuildID: "1", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base},
		{Name: "TestNoSHA", Suite: "e2e", Job: "ci", BuildID: "2", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base.Add(time.Hour)},
		{Name: "TestNoSHA", Suite: "e2e", Job: "ci", BuildID: "3", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base.Add(2 * time.Hour)},
		{Name: "TestNoSHA", Suite: "e2e", Job: "ci", BuildID: "4", Result: flakiness.OutcomeFail, Duration: time.Second, Timestamp: base.Add(3 * time.Hour)},
		{Name: "TestNoSHA", Suite: "e2e", Job: "ci", BuildID: "5", Result: flakiness.OutcomeFail, Duration: time.Second, Timestamp: base.Add(4 * time.Hour)},
		{Name: "TestNoSHA", Suite: "e2e", Job: "ci", BuildID: "6", Result: flakiness.OutcomeFail, Duration: time.Second, Timestamp: base.Add(5 * time.Hour)},
	}

	for _, r := range results {
		require.NoError(t, flakiness.RecordTestResult(appender, r))
	}

	require.NoError(t, appender.Commit())

	report, err := flakiness.Analyze(ctx, store,
		base.Add(-time.Minute), base.Add(6*time.Hour),
		flakiness.DefaultClassifyOptions())
	require.NoError(t, err)

	rec := report.Tests[flakiness.TestKey("TestNoSHA", "e2e", "ci")]
	require.NotNil(t, rec)
	assert.Equal(t, flakiness.PatternRegression, rec.ClassifyPattern())
	assert.Equal(t, "4", rec.TransitionCommit(), "should fall back to BuildID")
}

func TestAnalyze_MinRunsFilter(t *testing.T) {
	t.Parallel()

	store, err := flakiness.NewStore()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	ctx := context.Background()
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	appender := store.Appender(ctx)

	results := []flakiness.TestResult{
		{Name: "TestFew", Suite: "e2e", Job: "ci", BuildID: "1", Result: flakiness.OutcomeFail, Duration: time.Second, Timestamp: base},
		{Name: "TestFew", Suite: "e2e", Job: "ci", BuildID: "2", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base.Add(time.Hour)},
	}

	for _, r := range results {
		require.NoError(t, flakiness.RecordTestResult(appender, r))
	}

	require.NoError(t, appender.Commit())

	opts := flakiness.ClassifyOptions{MinRuns: 5}

	report, err := flakiness.Analyze(ctx, store,
		base.Add(-time.Minute), base.Add(2*time.Hour), opts)
	require.NoError(t, err)

	rec := report.Tests[flakiness.TestKey("TestFew", "e2e", "ci")]
	require.NotNil(t, rec)
	assert.Equal(t, flakiness.PatternHealthy, rec.ClassifyPatternWith(report.Options),
		"test with fewer runs than MinRuns should be healthy")
}

func TestAnalyze_SkipsNonAnalyzableOutcomes(t *testing.T) {
	t.Parallel()

	store, err := flakiness.NewStore()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	ctx := context.Background()
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	appender := store.Appender(ctx)

	results := []flakiness.TestResult{
		{Name: "TestWithSkip", Suite: "e2e", Job: "ci", BuildID: "1", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base},
		{Name: "TestWithSkip", Suite: "e2e", Job: "ci", BuildID: "2", Result: flakiness.OutcomeSkip, Duration: 0, Timestamp: base.Add(time.Hour)},
		{Name: "TestWithSkip", Suite: "e2e", Job: "ci", BuildID: "3", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base.Add(2 * time.Hour)},
	}

	for _, r := range results {
		require.NoError(t, flakiness.RecordTestResult(appender, r))
	}

	require.NoError(t, appender.Commit())

	report, err := flakiness.Analyze(ctx, store,
		base.Add(-time.Minute), base.Add(3*time.Hour),
		flakiness.DefaultClassifyOptions())
	require.NoError(t, err)

	rec := report.Tests[flakiness.TestKey("TestWithSkip", "e2e", "ci")]
	require.NotNil(t, rec)
	assert.Equal(t, 2, rec.TotalRuns, "skipped runs should not be counted")
	assert.Equal(t, 0, rec.FailedRuns)
	assert.Equal(t, 2, rec.PassedRuns)
}

func TestAnalyze_CompositeKeySeparatesSuites(t *testing.T) {
	t.Parallel()

	store, err := flakiness.NewStore()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	ctx := context.Background()
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	appender := store.Appender(ctx)

	results := []flakiness.TestResult{
		{Name: "TestShared", Suite: "unit", Job: "ci", BuildID: "1", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base},
		{Name: "TestShared", Suite: "unit", Job: "ci", BuildID: "2", Result: flakiness.OutcomePass, Duration: time.Second, Timestamp: base.Add(time.Hour)},
		{Name: "TestShared", Suite: "e2e", Job: "ci", BuildID: "1", Result: flakiness.OutcomeFail, Duration: time.Second, Timestamp: base},
		{Name: "TestShared", Suite: "e2e", Job: "ci", BuildID: "2", Result: flakiness.OutcomeFail, Duration: time.Second, Timestamp: base.Add(time.Hour)},
	}

	for _, r := range results {
		require.NoError(t, flakiness.RecordTestResult(appender, r))
	}

	require.NoError(t, appender.Commit())

	report, err := flakiness.Analyze(ctx, store,
		base.Add(-time.Minute), base.Add(2*time.Hour),
		flakiness.DefaultClassifyOptions())
	require.NoError(t, err)

	require.Len(t, report.Tests, 2, "same name in different suites should be separate records")

	unitRec := report.Tests[flakiness.TestKey("TestShared", "unit", "ci")]
	require.NotNil(t, unitRec)
	assert.Equal(t, 0, unitRec.FailedRuns)

	e2eRec := report.Tests[flakiness.TestKey("TestShared", "e2e", "ci")]
	require.NotNil(t, e2eRec)
	assert.Equal(t, 2, e2eRec.FailedRuns)
}

func TestReport_DeterministicSortOrder(t *testing.T) {
	t.Parallel()

	report := &flakiness.Report{
		Tests: map[string]*flakiness.TestRecord{
			"TestC": {Name: "TestC", TotalRuns: 10, FailedRuns: 5, PassedRuns: 5,
				History: []flakiness.RunOutcome{
					{Failed: true}, {Failed: false}, {Failed: true},
					{Failed: false}, {Failed: true}, {Failed: false},
					{Failed: true}, {Failed: false}, {Failed: true},
					{Failed: false},
				}},
			"TestA": {Name: "TestA", TotalRuns: 10, FailedRuns: 5, PassedRuns: 5,
				History: []flakiness.RunOutcome{
					{Failed: true}, {Failed: false}, {Failed: true},
					{Failed: false}, {Failed: true}, {Failed: false},
					{Failed: true}, {Failed: false}, {Failed: true},
					{Failed: false},
				}},
			"TestB": {Name: "TestB", TotalRuns: 10, FailedRuns: 5, PassedRuns: 5,
				History: []flakiness.RunOutcome{
					{Failed: true}, {Failed: false}, {Failed: true},
					{Failed: false}, {Failed: true}, {Failed: false},
					{Failed: true}, {Failed: false}, {Failed: true},
					{Failed: false},
				}},
		},
		Options: flakiness.DefaultClassifyOptions(),
	}

	for i := range 10 {
		classifications := report.ClassifyWithQuarantine(0.2)
		require.Len(t, classifications, 3)
		assert.Equal(t, "TestA", classifications[0].Name,
			"iteration %d: equal flake rates should sort alphabetically", i)
		assert.Equal(t, "TestB", classifications[1].Name,
			"iteration %d: equal flake rates should sort alphabetically", i)
		assert.Equal(t, "TestC", classifications[2].Name,
			"iteration %d: equal flake rates should sort alphabetically", i)
	}

	for i := range 10 {
		entries := report.QuarantineList(0.2)
		require.Len(t, entries, 3)
		assert.Equal(t, "TestA", entries[0].Name,
			"iteration %d: quarantine list should sort deterministically", i)
		assert.Equal(t, "TestB", entries[1].Name,
			"iteration %d: quarantine list should sort deterministically", i)
		assert.Equal(t, "TestC", entries[2].Name,
			"iteration %d: quarantine list should sort deterministically", i)
	}
}

func TestTestKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "TestFoo", flakiness.TestKey("TestFoo", "", ""),
		"empty suite and job should return just the name")
	assert.Equal(t, "e2e\x00nightly\x00TestFoo", flakiness.TestKey("TestFoo", "e2e", "nightly"))
	assert.Equal(t, "\x00ci\x00TestFoo", flakiness.TestKey("TestFoo", "", "ci"),
		"partial suite/job should still use composite format")
}
