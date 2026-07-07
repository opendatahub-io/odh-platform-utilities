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
	defaultPrefix = "logs/periodic-ci-openshift-api-master-evals-eval-periodic/2066434830635110400/"
)

func main() {
	bucket := defaultBucket
	prefix := defaultPrefix

	if len(os.Args) >= 3 { //nolint:mnd
		bucket = os.Args[1]
		prefix = os.Args[2]
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

	printResults(store, ctx, result)

	return nil
}

func printResults(store *flakiness.Store, ctx context.Context, result *flakiness.ScrapeResult) {
	w := os.Stdout

	_, _ = fmt.Fprintf(w, "artifacts: %d  recorded: %d  errors: %d\n",
		result.Artifacts, result.TestsRecorded, len(result.Errors))

	for _, e := range result.Errors {
		_, _ = fmt.Fprintf(os.Stderr, "  warning: %v\n", e)
	}

	if result.TestsRecorded == 0 {
		return
	}

	queryTime := time.Now().Add(time.Minute)

	res, err := store.InstantQuery(ctx,
		flakiness.MetricTestExecutionTotal, queryTime)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "query failed: %v\n", err)

		return
	}

	vec, err := res.Vector()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "vector conversion: %v\n", err)

		return
	}

	_, _ = fmt.Fprintf(w, "\nstored series: %d\n", len(vec))

	for _, s := range vec {
		_, _ = fmt.Fprintf(w, "  %s = %g\n", s.Metric, s.F)
	}
}
