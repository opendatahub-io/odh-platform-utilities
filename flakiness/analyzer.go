package flakiness

import (
	"sort"
	"time"
)

// FailurePattern classifies how a test fails across multiple CI runs.
type FailurePattern string

const (
	PatternHealthy    FailurePattern = "healthy"
	PatternFlaky      FailurePattern = "flaky"
	PatternRegression FailurePattern = "regression"
	PatternPersistent FailurePattern = "persistent"
)

const (
	DefaultMinRecentFailsForRegression = 3
	DefaultMaxPreTransitionFailRate    = 0.2
	DefaultMinRuns                     = 1
)

// ClassifyOptions configures the failure classification algorithm.
type ClassifyOptions struct {
	MinRecentFailsForRegression int
	MaxPreTransitionFailRate    float64
	MinRuns                     int
}

// DefaultClassifyOptions returns the default classification parameters.
func DefaultClassifyOptions() ClassifyOptions {
	return ClassifyOptions{
		MinRecentFailsForRegression: DefaultMinRecentFailsForRegression,
		MaxPreTransitionFailRate:    DefaultMaxPreTransitionFailRate,
		MinRuns:                     DefaultMinRuns,
	}
}

func (o ClassifyOptions) withDefaults() ClassifyOptions {
	if o.MinRecentFailsForRegression <= 0 {
		o.MinRecentFailsForRegression = DefaultMinRecentFailsForRegression
	}

	if o.MaxPreTransitionFailRate <= 0 {
		o.MaxPreTransitionFailRate = DefaultMaxPreTransitionFailRate
	}

	if o.MinRuns <= 0 {
		o.MinRuns = DefaultMinRuns
	}

	return o
}

// RunOutcome records the result of a single test in one CI run.
type RunOutcome struct {
	Timestamp time.Time
	Failed    bool
	BuildID   string
	CommitSHA string
	PRNumber  string
}

// TestRecord tracks pass/fail counts for a single test across multiple
// CI runs.
type TestRecord struct {
	Name       string
	Suite      string
	Job        string
	TotalRuns  int
	FailedRuns int
	PassedRuns int
	LastSeen   time.Time
	LastFailed time.Time
	History    []RunOutcome
}

// TestKey returns a composite key that uniquely identifies the test
// across suites and jobs. When suite and job are both empty, returns
// just the name.
func TestKey(name, suite, job string) string {
	if suite == "" && job == "" {
		return name
	}

	return suite + "\x00" + job + "\x00" + name
}

// Key returns the composite identity key for this record.
func (r *TestRecord) Key() string {
	return TestKey(r.Name, r.Suite, r.Job)
}

// FlakeRate returns the failure rate as a fraction [0.0, 1.0].
func (r *TestRecord) FlakeRate() float64 {
	if r.TotalRuns == 0 {
		return 0
	}

	return float64(r.FailedRuns) / float64(r.TotalRuns)
}

// ClassifyPattern analyzes the run history using default options.
func (r *TestRecord) ClassifyPattern() FailurePattern {
	return r.ClassifyPatternWith(DefaultClassifyOptions())
}

// ClassifyPatternWith analyzes the run history to determine whether
// failures are flaky (scattered) or a regression (step-function
// transition), using the provided options.
func (r *TestRecord) ClassifyPatternWith(opts ClassifyOptions) FailurePattern {
	return r.analyzeWith(opts).Pattern
}

// TransitionCommit returns the commit SHA of the first failure in the
// trailing failure streak using default options. Falls back to BuildID
// when commit SHA is not recorded. Returns "" when the pattern is not
// a regression.
func (r *TestRecord) TransitionCommit() string {
	return r.TransitionCommitWith(DefaultClassifyOptions())
}

// TransitionCommitWith returns the commit SHA of the first failure in
// the trailing failure streak using the provided options. Falls back
// to BuildID when commit SHA is not recorded. Returns "" when the
// pattern is not a regression.
func (r *TestRecord) TransitionCommitWith(opts ClassifyOptions) string {
	a := r.analyzeWith(opts)
	if !a.HasTransition {
		return ""
	}

	if a.Transition.CommitSHA != "" {
		return a.Transition.CommitSHA
	}

	return a.Transition.BuildID
}

// TransitionPR returns the PR number associated with the regression
// transition point using default options. Returns "" when unavailable.
func (r *TestRecord) TransitionPR() string {
	return r.TransitionPRWith(DefaultClassifyOptions())
}

// TransitionPRWith returns the PR number associated with the
// regression transition point using the provided options. Returns ""
// when unavailable.
func (r *TestRecord) TransitionPRWith(opts ClassifyOptions) string {
	a := r.analyzeWith(opts)
	if !a.HasTransition {
		return ""
	}

	return a.Transition.PRNumber
}

// ShouldQuarantine returns true when a test is flaky (not a regression
// or persistent failure) and its flake rate exceeds the threshold.
func (r *TestRecord) ShouldQuarantine(threshold float64, opts ClassifyOptions) bool {
	return r.analyzeWith(opts).Pattern == PatternFlaky && r.FlakeRate() > threshold
}

// AnalysisResult holds the outcome of a single-pass classification.
type AnalysisResult struct {
	Pattern       FailurePattern
	Transition    RunOutcome
	HasTransition bool
}

func (r *TestRecord) analyzeWith(opts ClassifyOptions) AnalysisResult {
	opts = opts.withDefaults()

	if r.TotalRuns < opts.MinRuns || r.FailedRuns == 0 {
		return AnalysisResult{Pattern: PatternHealthy}
	}

	if r.PassedRuns == 0 {
		return AnalysisResult{Pattern: PatternPersistent}
	}

	sorted := r.sortedHistory()
	if len(sorted) < 2 {
		return AnalysisResult{Pattern: PatternFlaky}
	}

	trailingFails := countTrailingFailures(sorted)

	if trailingFails < opts.MinRecentFailsForRegression {
		return AnalysisResult{Pattern: PatternFlaky}
	}

	preTransitionEnd := len(sorted) - trailingFails
	if preTransitionEnd == 0 {
		return AnalysisResult{Pattern: PatternPersistent}
	}

	preFails := 0
	for _, run := range sorted[:preTransitionEnd] {
		if run.Failed {
			preFails++
		}
	}

	preFailRate := float64(preFails) / float64(preTransitionEnd)

	if preFailRate <= opts.MaxPreTransitionFailRate {
		return AnalysisResult{
			Pattern:       PatternRegression,
			Transition:    sorted[preTransitionEnd],
			HasTransition: true,
		}
	}

	return AnalysisResult{Pattern: PatternFlaky}
}

func (r *TestRecord) sortedHistory() []RunOutcome {
	sorted := make([]RunOutcome, len(r.History))
	copy(sorted, r.History)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	return sorted
}

func countTrailingFailures(sorted []RunOutcome) int {
	count := 0
	for i := len(sorted) - 1; i >= 0; i-- {
		if !sorted[i].Failed {
			break
		}

		count++
	}

	return count
}
