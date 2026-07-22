package flakiness

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// TestClassification is the JSON-compatible output for a single test's
// classification result.
type TestClassification struct {
	Name           string         `json:"name"`
	Suite          string         `json:"suite,omitempty"`
	Job            string         `json:"job,omitempty"`
	Classification FailurePattern `json:"classification"`
	Quarantine     bool           `json:"quarantine"`
	FlakeRate      float64        `json:"flakeRate"`
	TotalRuns      int            `json:"totalRuns"`
	FailedRuns     int            `json:"failedRuns"`
	RegressionSHA  string         `json:"regressionSha,omitempty"`
	RegressionPR   string         `json:"regressionPr,omitempty"`
	LastSeen       time.Time      `json:"lastSeen"`
	LastFailed     time.Time      `json:"lastFailed,omitempty"`
}

// QuarantineEntry is a single entry in the quarantine JSON output.
type QuarantineEntry struct {
	Name           string         `json:"name"`
	Suite          string         `json:"suite,omitempty"`
	Job            string         `json:"job,omitempty"`
	Quarantined    bool           `json:"quarantined"`
	Classification FailurePattern `json:"classification"`
	FlakeRate      float64        `json:"flakeRate"`
	TotalRuns      int            `json:"totalRuns"`
	FailedRuns     int            `json:"failedRuns"`
	RegressionSHA  string         `json:"regressionSha,omitempty"`
	RegressionPR   string         `json:"regressionPr,omitempty"`
	LastSeen       time.Time      `json:"lastSeen"`
	LastFailed     time.Time      `json:"lastFailed,omitempty"`
	JiraKey        string         `json:"jiraKey,omitempty"`
}

// Report holds analysis results for test executions in a time window.
type Report struct {
	Tests   map[string]*TestRecord
	Options ClassifyOptions
}

// Classify returns a TestClassification for each test in the report
// without quarantine decisions.
func (r *Report) Classify() []TestClassification {
	return r.classifyWith(false, 0)
}

// ClassifyWithQuarantine returns a TestClassification for each test,
// marking flaky tests above the threshold for quarantine.
func (r *Report) ClassifyWithQuarantine(threshold float64) []TestClassification {
	return r.classifyWith(true, threshold)
}

func (r *Report) classifyWith(quarantine bool, threshold float64) []TestClassification {
	results := make([]TestClassification, 0, len(r.Tests))

	for _, rec := range r.Tests {
		a := rec.analyzeWith(r.Options)

		tc := TestClassification{
			Name:           rec.Name,
			Suite:          rec.Suite,
			Job:            rec.Job,
			Classification: a.Pattern,
			Quarantine:     quarantine && a.Pattern == PatternFlaky && rec.FlakeRate() > threshold,
			FlakeRate:      rec.FlakeRate(),
			TotalRuns:      rec.TotalRuns,
			FailedRuns:     rec.FailedRuns,
			LastSeen:       rec.LastSeen,
			LastFailed:     rec.LastFailed,
		}

		if a.HasTransition {
			if a.Transition.CommitSHA != "" {
				tc.RegressionSHA = a.Transition.CommitSHA
			} else {
				tc.RegressionSHA = a.Transition.BuildID
			}

			tc.RegressionPR = a.Transition.PRNumber
		}

		results = append(results, tc)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].FlakeRate != results[j].FlakeRate {
			return results[i].FlakeRate > results[j].FlakeRate
		}

		return TestKey(results[i].Name, results[i].Suite, results[i].Job) <
			TestKey(results[j].Name, results[j].Suite, results[j].Job)
	})

	return results
}

// QuarantineList returns quarantine entries for all tests in the
// report. Flaky tests exceeding the threshold are marked
// quarantined; regressions and persistent failures are included
// but NOT quarantined, with regression metadata preserved.
func (r *Report) QuarantineList(threshold float64) []QuarantineEntry {
	entries := make([]QuarantineEntry, 0, len(r.Tests))

	for _, rec := range r.Tests {
		a := rec.analyzeWith(r.Options)
		if a.Pattern == PatternHealthy {
			continue
		}

		entry := QuarantineEntry{
			Name:           rec.Name,
			Suite:          rec.Suite,
			Job:            rec.Job,
			Quarantined:    a.Pattern == PatternFlaky && rec.FlakeRate() > threshold,
			Classification: a.Pattern,
			FlakeRate:      rec.FlakeRate(),
			TotalRuns:      rec.TotalRuns,
			FailedRuns:     rec.FailedRuns,
			LastSeen:       rec.LastSeen,
			LastFailed:     rec.LastFailed,
		}

		if a.HasTransition {
			if a.Transition.CommitSHA != "" {
				entry.RegressionSHA = a.Transition.CommitSHA
			} else {
				entry.RegressionSHA = a.Transition.BuildID
			}

			entry.RegressionPR = a.Transition.PRNumber
		}

		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].FlakeRate != entries[j].FlakeRate {
			return entries[i].FlakeRate > entries[j].FlakeRate
		}

		return TestKey(entries[i].Name, entries[i].Suite, entries[i].Job) <
			TestKey(entries[j].Name, entries[j].Suite, entries[j].Job)
	})

	return entries
}

// ExceedingThreshold returns test records whose flake rate exceeds the
// given threshold, sorted by flake rate descending.
func (r *Report) ExceedingThreshold(threshold float64) []*TestRecord {
	var results []*TestRecord

	for _, record := range r.Tests {
		if record.FlakeRate() > threshold {
			results = append(results, record)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		ri, rj := results[i].FlakeRate(), results[j].FlakeRate()
		if ri != rj {
			return ri > rj
		}

		return results[i].Key() < results[j].Key()
	})

	return results
}

// Regressions returns test records classified as regressions, sorted
// by last-failed time descending (most recent first).
func (r *Report) Regressions() []*TestRecord {
	var results []*TestRecord

	for _, record := range r.Tests {
		if record.ClassifyPatternWith(r.Options) == PatternRegression {
			results = append(results, record)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if !results[i].LastFailed.Equal(results[j].LastFailed) {
			return results[i].LastFailed.After(results[j].LastFailed)
		}

		return results[i].Key() < results[j].Key()
	})

	return results
}

// FlakyTests returns test records classified as flaky that exceed the
// given threshold, sorted by flake rate descending.
func (r *Report) FlakyTests(threshold float64) []*TestRecord {
	var results []*TestRecord

	for _, record := range r.Tests {
		if record.ClassifyPatternWith(r.Options) == PatternFlaky && record.FlakeRate() > threshold {
			results = append(results, record)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		ri, rj := results[i].FlakeRate(), results[j].FlakeRate()
		if ri != rj {
			return ri > rj
		}

		return results[i].Key() < results[j].Key()
	})

	return results
}

// Analyze queries a Queryable for test execution metrics in the given
// time range and returns a Report with per-test records.
func Analyze(ctx context.Context, q storage.Queryable, start, end time.Time, opts ClassifyOptions) (*Report, error) {
	opts = opts.withDefaults()

	querier, err := q.Querier(start.UnixMilli(), end.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("creating querier: %w", err)
	}

	defer func() {
		_ = querier.Close()
	}()

	matcher, err := labels.NewMatcher(
		labels.MatchEqual, labels.MetricName, MetricTestExecutionTotal,
	)
	if err != nil {
		return nil, fmt.Errorf("creating matcher: %w", err)
	}

	seriesSet := querier.Select(ctx, false, &storage.SelectHints{
		Start: start.UnixMilli(),
		End:   end.UnixMilli(),
	}, matcher)

	report := &Report{
		Tests:   make(map[string]*TestRecord),
		Options: opts,
	}

	for seriesSet.Next() {
		series := seriesSet.At()
		lbls := series.Labels()

		testName := lbls.Get(LabelTestName)
		result := lbls.Get(LabelResult)

		if testName == "" || !isAnalyzableOutcome(result) {
			continue
		}

		suite := lbls.Get(LabelSuite)
		job := lbls.Get(LabelJob)
		buildID := lbls.Get(LabelBuildID)
		commitSHA := lbls.Get(LabelCommitSHA)
		prNumber := lbls.Get(LabelPRNumber)

		key := TestKey(testName, suite, job)

		it := series.Iterator(nil)
		for it.Next() == chunkenc.ValFloat {
			ts, _ := it.At()
			timestamp := time.UnixMilli(ts)

			failed := result == string(OutcomeFail) || result == string(OutcomeError)

			record, ok := report.Tests[key]
			if !ok {
				record = &TestRecord{Name: testName, Suite: suite, Job: job}
				report.Tests[key] = record
			}

			record.TotalRuns++

			if failed {
				record.FailedRuns++

				if timestamp.After(record.LastFailed) {
					record.LastFailed = timestamp
				}
			} else if result == string(OutcomePass) {
				record.PassedRuns++
			}

			if timestamp.After(record.LastSeen) {
				record.LastSeen = timestamp
			}

			record.History = append(record.History, RunOutcome{
				Timestamp: timestamp,
				Failed:    failed,
				BuildID:   buildID,
				CommitSHA: commitSHA,
				PRNumber:  prNumber,
			})
		}

		if err := it.Err(); err != nil {
			return nil, fmt.Errorf("iterating series for %s: %w", key, err)
		}
	}

	if err := seriesSet.Err(); err != nil {
		return nil, fmt.Errorf("selecting series: %w", err)
	}

	return report, nil
}

func isAnalyzableOutcome(result string) bool {
	switch result {
	case string(OutcomePass), string(OutcomeFail), string(OutcomeError):
		return true
	default:
		return false
	}
}
