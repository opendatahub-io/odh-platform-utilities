package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

const (
	defaultBucket = "test-platform-results"
	defaultPrefix = "logs/periodic-ci-opendatahub-io-ai-edge-main-test-ai-edge-periodic/2012359524924526592/"
)

func main() {
	bucket := defaultBucket
	prefix := defaultPrefix

	switch {
	case len(os.Args) == 1:
		// use defaults
	case len(os.Args) == 3: //nolint:mnd
		bucket = os.Args[1]
		prefix = os.Args[2]
	default:
		log.Fatalf("Usage: %s [bucket prefix]", os.Args[0])
	}

	if err := run(bucket, prefix); err != nil {
		log.Fatal(err)
	}
}

func run(bucket, prefix string) error {
	ctx := context.Background()

	gcs, err := flakiness.NewGCSClient(ctx, flakiness.WithAnonymous())
	if err != nil {
		return fmt.Errorf("creating GCS client: %w", err)
	}
	defer func() { _ = gcs.Close() }()

	store, err := flakiness.NewStore()
	if err != nil {
		return fmt.Errorf("creating store: %w", err)
	}
	defer func() { _ = store.Close() }()

	scraper := flakiness.NewScraper(gcs)
	appender := store.Appender(ctx)

	result, err := scraper.Scrape(ctx, appender, bucket, prefix)
	if err != nil {
		return fmt.Errorf("scrape: %w", err)
	}

	if err := appender.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	fmt.Printf("Ingested %d tests from %d artifacts\n\n", result.TestsRecorded, result.Artifacts)

	if result.TestsRecorded == 0 {
		return nil
	}

	analyzer := flakiness.NewRuntimeAnalyzer(store)
	now := time.Now()
	window := now.Add(-24 * time.Hour)

	printTopSlowest(ctx, analyzer, window, now)
	printSuiteRuntimes(ctx, analyzer, window, now)
	printBudgetReport(ctx, analyzer, window, now)

	return nil
}

func printTopSlowest(ctx context.Context, a *flakiness.RuntimeAnalyzer, start, end time.Time) {
	top, err := a.TopSlowest(ctx, 5, start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "top slowest: %v\n", err)

		return
	}

	fmt.Println("=== Top 5 Slowest Tests ===")

	for _, t := range top {
		fmt.Printf("  %-60s max=%s  p95=%s  p50=%s  (%d samples)\n",
			t.Name, t.Max, t.P95, t.P50, t.Samples)
	}

	fmt.Println()
}

func printSuiteRuntimes(ctx context.Context, a *flakiness.RuntimeAnalyzer, start, end time.Time) {
	suites, err := a.SuiteRuntimes(ctx, start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "suite runtimes: %v\n", err)

		return
	}

	fmt.Println("=== Suite Runtimes ===")

	for _, s := range suites {
		fmt.Printf("  %-40s total=%s  tests=%d  samples=%d\n",
			s.Suite, s.Total, s.TestCount, s.Samples)
	}

	fmt.Println()
}

func printBudgetReport(ctx context.Context, a *flakiness.RuntimeAnalyzer, start, end time.Time) {
	cfg := flakiness.TimeoutConfig{
		PipelineTimeout: time.Hour,
		SuiteTimeouts: map[string]time.Duration{
			"e2e":         30 * time.Minute,
			"integration": 15 * time.Minute,
		},
	}

	report, err := a.AnalyzeBudget(ctx, cfg, flakiness.DefaultNearTimeoutThreshold, start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "budget analysis: %v\n", err)

		return
	}

	fmt.Println("=== Budget Report ===")

	if report.Pipeline != nil {
		fmt.Printf("  Pipeline: %s / %s (%.1f%% utilisation)\n",
			report.Pipeline.ActualDuration,
			report.Pipeline.ConfiguredTimeout,
			report.Pipeline.Utilisation*100)
	}

	for _, s := range report.Suites {
		fmt.Printf("  Suite %-30s %s / %s (%.1f%%)\n",
			s.Name, s.ActualDuration, s.ConfiguredTimeout, s.Utilisation*100)
	}

	if len(report.NearTimeout) > 0 {
		fmt.Println("\n  Near-timeout tests:")

		for _, nt := range report.NearTimeout {
			fmt.Printf("    %-50s p95=%s / timeout=%s (%.0f%%)\n",
				nt.Name, nt.P95, nt.Timeout, nt.Utilisation*100)
		}
	}

	if len(report.Recommendations) > 0 {
		fmt.Println("\n  Recommendations:")

		for _, r := range report.Recommendations {
			fmt.Printf("    [%s] %-50s %s -> %s (%s)\n",
				r.Action, r.Name, r.CurrentTimeout, r.SuggestedTimeout, r.Reason)
		}
	}
}
