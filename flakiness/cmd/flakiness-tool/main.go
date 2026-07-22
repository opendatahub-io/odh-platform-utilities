package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "run" {
		log.Fatalf("Usage: %s run --config <path>", os.Args[0])
	}

	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := runCmd.String("config", "", "path to .flakiness.yaml config file")

	if err := runCmd.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}

	if *configPath == "" {
		runCmd.Usage()
		os.Exit(1)
	}

	if err := run(*configPath); err != nil {
		log.Fatal(err)
	}
}

func run(configPath string) error {
	cfg, err := flakiness.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

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

	result, err := scrape(ctx, cfg, gcs, store)
	if err != nil {
		return err
	}

	if result.TestsRecorded == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "no test results found")

		return nil
	}

	entries, err := analyze(ctx, cfg, store)
	if err != nil {
		return err
	}

	entries = cfg.FilterExcluded(entries)

	return writeQuarantineList(cfg, entries)
}

func scrape(ctx context.Context, cfg flakiness.Config, gcs *flakiness.GCSClient, store *flakiness.Store) (*flakiness.ScrapeResult, error) {
	scraper := flakiness.NewScraper(gcs)
	appender := store.Appender(ctx)

	result, err := scraper.ScrapeAll(ctx, appender, cfg.GCS.Bucket, cfg.GCS.JobPrefixes)
	if err != nil {
		return nil, fmt.Errorf("scraping: %w", err)
	}

	if err := appender.Commit(); err != nil {
		return nil, fmt.Errorf("committing metrics: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "component: %s\n", cfg.Component)
	_, _ = fmt.Fprintf(os.Stdout, "scraped: %d artifacts, %d tests, %d errors\n",
		result.Artifacts, result.TestsRecorded, len(result.Errors))

	for _, e := range result.Errors {
		_, _ = fmt.Fprintf(os.Stderr, "  warning: %v\n", e)
	}

	return result, nil
}

func analyze(ctx context.Context, cfg flakiness.Config, store *flakiness.Store) ([]flakiness.QuarantineEntry, error) {
	end := time.Now()
	start := end.AddDate(0, 0, -cfg.Analysis.WindowDays)

	opts := flakiness.ClassifyOptions{
		MinRuns: cfg.Analysis.MinRuns,
	}

	report, err := flakiness.Analyze(ctx, store, start, end, opts)
	if err != nil {
		return nil, fmt.Errorf("analyzing: %w", err)
	}

	entries := report.QuarantineList(cfg.Analysis.Threshold)

	quarantined := 0
	for _, e := range entries {
		if e.Quarantined {
			quarantined++
		}
	}

	_, _ = fmt.Fprintf(os.Stdout, "analysis: %d unhealthy tests, %d quarantined (threshold=%.2f, window=%dd)\n",
		len(entries), quarantined, cfg.Analysis.Threshold, cfg.Analysis.WindowDays)

	return entries, nil
}

func writeQuarantineList(cfg flakiness.Config, entries []flakiness.QuarantineEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling quarantine list: %w", err)
	}

	if cfg.Quarantine.ConfigPath != "" {
		if err := os.WriteFile(cfg.Quarantine.ConfigPath, data, 0o644); err != nil { //nolint:gosec,mnd // config output file
			return fmt.Errorf("writing quarantine file %s: %w", cfg.Quarantine.ConfigPath, err)
		}

		_, _ = fmt.Fprintf(os.Stdout, "wrote quarantine list to %s\n", cfg.Quarantine.ConfigPath)
	} else {
		_, _ = fmt.Fprintln(os.Stdout, string(data))
	}

	return nil
}
