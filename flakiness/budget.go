package flakiness

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// TimeoutConfig holds configured timeout values against which observed
// runtimes are compared.
type TimeoutConfig struct {
	PipelineTimeout time.Duration
	SuiteTimeouts   map[string]time.Duration
	TestTimeouts    map[string]time.Duration
}

// BudgetReport is the result of a timeout budget analysis.
type BudgetReport struct {
	Pipeline        *BudgetUtilisation
	Suites          []BudgetUtilisation
	NearTimeout     []NearTimeoutTest
	Recommendations []TimeoutRecommendation
}

// BudgetUtilisation represents how much of a timeout budget is consumed.
// Utilisation is ActualDuration / ConfiguredTimeout.
type BudgetUtilisation struct {
	Name              string
	ActualDuration    time.Duration
	ConfiguredTimeout time.Duration
	Utilisation       float64
}

// NearTimeoutTest flags a test whose P95 runtime is close to its timeout.
type NearTimeoutTest struct {
	Name        string
	Suite       string
	P95         time.Duration
	Timeout     time.Duration
	Utilisation float64
}

// RecommendationAction is the suggested direction for a timeout adjustment.
type RecommendationAction string

const (
	RecommendRaise RecommendationAction = "raise"
	RecommendLower RecommendationAction = "lower"
)

// TimeoutRecommendation suggests a concrete timeout adjustment for a test.
type TimeoutRecommendation struct {
	Name             string
	Suite            string
	Action           RecommendationAction
	CurrentTimeout   time.Duration
	SuggestedTimeout time.Duration
	P95              time.Duration
	P99              time.Duration
	Reason           string
}

const (
	// DefaultNearTimeoutThreshold is the utilisation ratio above which a
	// test is considered near-timeout (80%).
	DefaultNearTimeoutThreshold = 0.80

	headroomMultiplier = 1.5 // applied to P99 for suggested timeout
	lowerThreshold     = 0.30
)

// AnalyzeBudget compares observed runtimes against configured timeouts.
// Pipeline and suite utilisation are computed per-build (grouped by BuildID)
// and the worst single build is reported. Per-test near-timeout detection
// uses percentiles across all builds.
func (a *RuntimeAnalyzer) AnalyzeBudget(
	ctx context.Context,
	cfg TimeoutConfig,
	threshold float64,
	start, end time.Time,
) (*BudgetReport, error) {
	series, err := a.queryDurationSeries(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying duration series: %w", err)
	}

	runtimes := aggregateRuntimes(series)
	perBuild := groupByBuild(series)

	report := &BudgetReport{}

	if cfg.PipelineTimeout > 0 {
		report.Pipeline = computePipelineBudget(perBuild, cfg.PipelineTimeout)
	}

	report.Suites = computeSuiteBudgets(perBuild, cfg.SuiteTimeouts)
	report.NearTimeout = detectNearTimeout(runtimes, cfg.TestTimeouts, threshold)
	report.Recommendations = computeRecommendations(runtimes, cfg.TestTimeouts, threshold)

	return report, nil
}

// buildSuiteTotals holds per-suite duration totals for a single build.
type buildSuiteTotals struct {
	suites map[string]float64
}

// groupByBuild partitions duration series by BuildID, returning a map
// from BuildID to per-suite totals.
func groupByBuild(series []durationSeries) map[string]*buildSuiteTotals {
	builds := make(map[string]*buildSuiteTotals)

	for _, s := range series {
		buildID := s.labels.Get(LabelBuildID)
		if buildID == "" {
			buildID = "_unknown"
		}

		bt, ok := builds[buildID]
		if !ok {
			bt = &buildSuiteTotals{suites: make(map[string]float64)}
			builds[buildID] = bt
		}

		suite := s.labels.Get(LabelSuite)

		for _, v := range s.values {
			bt.suites[suite] += v
		}
	}

	return builds
}

func computePipelineBudget(
	perBuild map[string]*buildSuiteTotals,
	pipelineTimeout time.Duration,
) *BudgetUtilisation {
	var maxTotal float64

	for _, bt := range perBuild {
		var buildTotal float64
		for _, suiteTotal := range bt.suites {
			buildTotal += suiteTotal
		}

		if buildTotal > maxTotal {
			maxTotal = buildTotal
		}
	}

	actual := time.Duration(maxTotal * float64(time.Second))

	return &BudgetUtilisation{
		Name:              "pipeline",
		ActualDuration:    actual,
		ConfiguredTimeout: pipelineTimeout,
		Utilisation:       float64(actual) / float64(pipelineTimeout),
	}
}

func computeSuiteBudgets(
	perBuild map[string]*buildSuiteTotals,
	timeouts map[string]time.Duration,
) []BudgetUtilisation {
	if len(timeouts) == 0 {
		return nil
	}

	maxPerSuite := make(map[string]float64)

	for _, bt := range perBuild {
		for suite, total := range bt.suites {
			if total > maxPerSuite[suite] {
				maxPerSuite[suite] = total
			}
		}
	}

	results := make([]BudgetUtilisation, 0, len(timeouts))

	for suite, timeout := range timeouts {
		if timeout <= 0 {
			continue
		}

		maxDur, ok := maxPerSuite[suite]
		if !ok {
			continue
		}

		actual := time.Duration(maxDur * float64(time.Second))
		results = append(results, BudgetUtilisation{
			Name:              suite,
			ActualDuration:    actual,
			ConfiguredTimeout: timeout,
			Utilisation:       float64(actual) / float64(timeout),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Utilisation > results[j].Utilisation
	})

	return results
}

func detectNearTimeout(
	runtimes []TestRuntime,
	timeouts map[string]time.Duration,
	threshold float64,
) []NearTimeoutTest {
	if len(timeouts) == 0 {
		return nil
	}

	var results []NearTimeoutTest

	for _, rt := range runtimes {
		timeout, ok := timeouts[rt.Name]
		if !ok || timeout <= 0 {
			continue
		}

		utilisation := float64(rt.P95) / float64(timeout)
		if utilisation >= threshold {
			results = append(results, NearTimeoutTest{
				Name:        rt.Name,
				Suite:       rt.Suite,
				P95:         rt.P95,
				Timeout:     timeout,
				Utilisation: utilisation,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Utilisation > results[j].Utilisation
	})

	return results
}

func computeRecommendations(
	runtimes []TestRuntime,
	timeouts map[string]time.Duration,
	threshold float64,
) []TimeoutRecommendation {
	if len(timeouts) == 0 {
		return nil
	}

	var results []TimeoutRecommendation

	for _, rt := range runtimes {
		timeout, ok := timeouts[rt.Name]
		if !ok || timeout <= 0 {
			continue
		}

		utilisation := float64(rt.P95) / float64(timeout)

		switch {
		case utilisation >= threshold:
			suggested := time.Duration(
				float64(rt.P99) * headroomMultiplier,
			)

			results = append(results, TimeoutRecommendation{
				Name:             rt.Name,
				Suite:            rt.Suite,
				Action:           RecommendRaise,
				CurrentTimeout:   timeout,
				SuggestedTimeout: suggested,
				P95:              rt.P95,
				P99:              rt.P99,
				Reason:           fmt.Sprintf("P95 uses %.0f%% of timeout", utilisation*100),
			})

		case utilisation <= lowerThreshold && rt.Samples >= 5:
			suggested := time.Duration(
				float64(rt.P99) * headroomMultiplier,
			)

			if suggested < time.Second {
				suggested = time.Second
			}

			results = append(results, TimeoutRecommendation{
				Name:             rt.Name,
				Suite:            rt.Suite,
				Action:           RecommendLower,
				CurrentTimeout:   timeout,
				SuggestedTimeout: suggested,
				P95:              rt.P95,
				P99:              rt.P99,
				Reason:           fmt.Sprintf("P95 uses only %.0f%% of timeout (%d samples)", utilisation*100, rt.Samples),
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Action != results[j].Action {
			return results[i].Action == RecommendRaise
		}

		return results[i].Name < results[j].Name
	})

	return results
}
