package flakiness

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// RuntimeAnalyzer queries stored [MetricTestDurationSeconds] to surface
// runtime trends, identify slow tests, and compute per-suite totals.
type RuntimeAnalyzer struct {
	store *Store
}

// NewRuntimeAnalyzer creates a [RuntimeAnalyzer] backed by the given store.
func NewRuntimeAnalyzer(store *Store) *RuntimeAnalyzer {
	return &RuntimeAnalyzer{store: store}
}

// TestRuntime holds percentile statistics for a single test.
type TestRuntime struct {
	Name    string
	Suite   string
	P50     time.Duration
	P95     time.Duration
	P99     time.Duration
	Max     time.Duration
	Samples int
}

// SuiteRuntime holds aggregate runtime for a test suite.
type SuiteRuntime struct {
	Suite     string
	Total     time.Duration
	TestCount int
	Samples   int
}

// RuntimeTrendPoint is a single data point in a runtime trend series.
type RuntimeTrendPoint struct {
	Timestamp time.Time
	Duration  time.Duration
}

// TopSlowest returns the top N tests by max duration within [start, end].
func (a *RuntimeAnalyzer) TopSlowest(
	ctx context.Context,
	n int,
	start, end time.Time,
) ([]TestRuntime, error) {
	if n <= 0 {
		return nil, nil
	}

	series, err := a.queryDurationSeries(ctx, start, end)
	if err != nil {
		return nil, err
	}

	runtimes := aggregateRuntimes(series)

	sort.Slice(runtimes, func(i, j int) bool {
		return runtimes[i].Max > runtimes[j].Max
	})

	if len(runtimes) > n {
		runtimes = runtimes[:n]
	}

	return runtimes, nil
}

// SuiteRuntimes returns aggregate runtime per suite within [start, end].
func (a *RuntimeAnalyzer) SuiteRuntimes(
	ctx context.Context,
	start, end time.Time,
) ([]SuiteRuntime, error) {
	series, err := a.queryDurationSeries(ctx, start, end)
	if err != nil {
		return nil, err
	}

	return aggregateSuiteRuntimes(series), nil
}

// RuntimeTrend returns chronological runtime points for a test within
// [start, end].
func (a *RuntimeAnalyzer) RuntimeTrend(
	ctx context.Context,
	testName string,
	start, end time.Time,
) ([]RuntimeTrendPoint, error) {
	series, err := a.queryDurationSeriesForTest(ctx, testName, start, end)
	if err != nil {
		return nil, err
	}

	var points []RuntimeTrendPoint
	for _, s := range series {
		for i, v := range s.values {
			points = append(points, RuntimeTrendPoint{
				Timestamp: time.UnixMilli(s.timestamps[i]),
				Duration:  time.Duration(v * float64(time.Second)),
			})
		}
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp.Before(points[j].Timestamp)
	})

	return points, nil
}

type durationSeries struct {
	labels     labels.Labels
	values     []float64
	timestamps []int64
}

func (a *RuntimeAnalyzer) queryDurationSeriesForTest(
	ctx context.Context,
	testName string,
	start, end time.Time,
) ([]durationSeries, error) {
	nameMatcher, err := labels.NewMatcher(
		labels.MatchEqual, "__name__", MetricTestDurationSeconds,
	)
	if err != nil {
		return nil, fmt.Errorf("creating name matcher: %w", err)
	}

	testMatcher, err := labels.NewMatcher(
		labels.MatchEqual, LabelTestName, testName,
	)
	if err != nil {
		return nil, fmt.Errorf("creating test name matcher: %w", err)
	}

	return a.selectDurationSeries(ctx, start, end, nameMatcher, testMatcher)
}

func (a *RuntimeAnalyzer) queryDurationSeries(
	ctx context.Context,
	start, end time.Time,
) ([]durationSeries, error) {
	nameMatcher, err := labels.NewMatcher(
		labels.MatchEqual, "__name__", MetricTestDurationSeconds,
	)
	if err != nil {
		return nil, fmt.Errorf("creating name matcher: %w", err)
	}

	return a.selectDurationSeries(ctx, start, end, nameMatcher)
}

func (a *RuntimeAnalyzer) selectDurationSeries(
	ctx context.Context,
	start, end time.Time,
	matchers ...*labels.Matcher,
) ([]durationSeries, error) {
	q, err := a.store.Querier(start.UnixMilli(), end.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("creating querier: %w", err)
	}

	defer func() { _ = q.Close() }()

	ss := q.Select(ctx, false, nil, matchers...)

	var result []durationSeries

	for ss.Next() {
		s := ss.At()
		ds := durationSeries{labels: s.Labels()}
		it := s.Iterator(nil)

		for it.Next() != chunkenc.ValNone {
			t, v := it.At()
			ds.timestamps = append(ds.timestamps, t)
			ds.values = append(ds.values, v)
		}

		if it.Err() != nil {
			return nil, fmt.Errorf("iterating series: %w", it.Err())
		}

		if len(ds.values) > 0 {
			result = append(result, ds)
		}
	}

	if ss.Err() != nil {
		return nil, fmt.Errorf("selecting series: %w", ss.Err())
	}

	return result, nil
}

func aggregateRuntimes(series []durationSeries) []TestRuntime {
	type key struct{ name, suite string }
	type acc struct {
		values []float64
	}

	grouped := make(map[key]*acc)

	for _, s := range series {
		k := key{
			name:  s.labels.Get(LabelTestName),
			suite: s.labels.Get(LabelSuite),
		}

		a, ok := grouped[k]
		if !ok {
			a = &acc{}
			grouped[k] = a
		}

		a.values = append(a.values, s.values...)
	}

	results := make([]TestRuntime, 0, len(grouped))

	for k, a := range grouped {
		if len(a.values) == 0 {
			continue
		}

		sort.Float64s(a.values)

		results = append(results, TestRuntime{
			Name:    k.name,
			Suite:   k.suite,
			P50:     time.Duration(percentile(a.values, 0.50) * float64(time.Second)),
			P95:     time.Duration(percentile(a.values, 0.95) * float64(time.Second)),
			P99:     time.Duration(percentile(a.values, 0.99) * float64(time.Second)),
			Max:     time.Duration(a.values[len(a.values)-1] * float64(time.Second)),
			Samples: len(a.values),
		})
	}

	return results
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if len(sorted) == 1 {
		return sorted[0]
	}

	rank := p * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))

	if lower == upper {
		return sorted[lower]
	}

	frac := rank - float64(lower)

	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

func aggregateSuiteRuntimes(series []durationSeries) []SuiteRuntime {
	type suiteAcc struct {
		total   float64
		tests   map[string]struct{}
		samples int
	}

	suites := make(map[string]*suiteAcc)

	for _, s := range series {
		suiteName := s.labels.Get(LabelSuite)
		testName := s.labels.Get(LabelTestName)

		acc, ok := suites[suiteName]
		if !ok {
			acc = &suiteAcc{tests: make(map[string]struct{})}
			suites[suiteName] = acc
		}

		acc.tests[testName] = struct{}{}

		for _, v := range s.values {
			acc.total += v
			acc.samples++
		}
	}

	results := make([]SuiteRuntime, 0, len(suites))
	for name, acc := range suites {
		results = append(results, SuiteRuntime{
			Suite:     name,
			Total:     time.Duration(acc.total * float64(time.Second)),
			TestCount: len(acc.tests),
			Samples:   acc.samples,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Total > results[j].Total
	})

	return results
}
