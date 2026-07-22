package flakiness

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ScrapeResult summarises a scrape operation.
type ScrapeResult struct {
	Artifacts     int
	TestsRecorded int
	// Errors collects non-fatal errors (malformed XML, unreadable
	// objects). Partial results may still have been recorded.
	Errors []error
}

// Scraper reads JUnit XML artifacts from GCS and writes test metrics
// to a [SampleAppender].
type Scraper struct {
	client     BucketClient
	maxRetries int
	retryDelay time.Duration
}

// ScraperOption configures a [Scraper].
type ScraperOption func(*Scraper)

// WithMaxRetries sets the retry attempt limit (default 3).
// Values below 1 are clamped to 1.
func WithMaxRetries(n int) ScraperOption {
	return func(s *Scraper) {
		if n < 1 {
			n = 1
		}

		s.maxRetries = n
	}
}

// WithRetryDelay sets the delay between retries (default 1s).
func WithRetryDelay(d time.Duration) ScraperOption {
	return func(s *Scraper) {
		s.retryDelay = d
	}
}

// NewScraper creates a [Scraper].
func NewScraper(client BucketClient, opts ...ScraperOption) *Scraper {
	s := &Scraper{
		client:     client,
		maxRetries: 3,
		retryDelay: time.Second,
	}

	for _, o := range opts {
		o(s)
	}

	return s
}

// Scrape discovers JUnit XML files under bucket/prefix, parses them,
// and writes test metrics to the appender. Individual artifact failures
// are collected in [ScrapeResult.Errors] rather than aborting.
// The caller must call Commit on the appender after Scrape returns.
func (s *Scraper) Scrape(ctx context.Context, a SampleAppender, bucket, prefix string) (*ScrapeResult, error) {
	objects, err := s.listWithRetry(ctx, bucket, prefix)
	if err != nil {
		return nil, fmt.Errorf("discovering artifacts: %w", err)
	}

	xmlObjects := filterJUnitFiles(objects)

	result := &ScrapeResult{
		Artifacts: len(xmlObjects),
	}

	for _, name := range xmlObjects {
		n, scrapeErr := s.scrapeObject(ctx, a, bucket, name)
		if scrapeErr != nil {
			result.Errors = append(result.Errors, fmt.Errorf("artifact %s: %w", name, scrapeErr))

			continue
		}

		result.TestsRecorded += n
	}

	return result, nil
}

func (s *Scraper) scrapeObject(ctx context.Context, a SampleAppender, bucket, name string) (int, error) {
	data, err := s.readWithRetry(ctx, bucket, name)
	if err != nil {
		return 0, err
	}

	suites, err := ParseJUnit(data)
	if err != nil {
		return 0, err
	}

	job, buildID := extractJobBuild(name)

	results := ConvertTestResults(suites, job, buildID)

	var errs []error

	recorded := 0

	for i := range results {
		if results[i].Timestamp.IsZero() {
			results[i].Timestamp = time.Now()
		}

		if recordErr := RecordTestResult(a, results[i]); recordErr != nil {
			errs = append(errs, recordErr)

			continue
		}

		recorded++
	}

	return recorded, errors.Join(errs...)
}

func (s *Scraper) listWithRetry(ctx context.Context, bucket, prefix string) ([]string, error) {
	var lastErr error

	for attempt := range s.maxRetries {
		objects, err := s.client.ListObjects(ctx, bucket, prefix)
		if err == nil {
			return objects, nil
		}

		lastErr = err

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if attempt < s.maxRetries-1 {
			sleepCtx(ctx, s.retryDelay)
		}
	}

	return nil, lastErr
}

func (s *Scraper) readWithRetry(ctx context.Context, bucket, name string) ([]byte, error) {
	var lastErr error

	for attempt := range s.maxRetries {
		data, err := s.client.ReadObject(ctx, bucket, name)
		if err == nil {
			return data, nil
		}

		lastErr = err

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if attempt < s.maxRetries-1 {
			sleepCtx(ctx, s.retryDelay)
		}
	}

	return nil, lastErr
}

func sleepCtx(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// extractJobBuild parses job name and build ID from an OpenShift CI
// GCS artifact path:
//
//	logs/<job-name>/<build-id>/artifacts/...
//	pr-logs/pull/<org_repo>/<pr>/<job-name>/<build-id>/artifacts/...
func extractJobBuild(path string) (job, buildID string) {
	parts := strings.Split(path, "/")

	if len(parts) >= 4 && parts[0] == "logs" {
		return parts[1], parts[2]
	}

	if len(parts) >= 7 && parts[0] == "pr-logs" && parts[1] == "pull" {
		return parts[4], parts[5]
	}

	return "", ""
}

func filterJUnitFiles(objects []string) []string {
	var result []string

	for _, name := range objects {
		if isJUnitFile(name) {
			result = append(result, name)
		}
	}

	return result
}

// ScrapeAll scrapes JUnit XML artifacts from multiple GCS job prefixes
// and aggregates the results. Each prefix is scraped independently;
// individual prefix failures are collected in [ScrapeResult.Errors]
// rather than aborting.
func (s *Scraper) ScrapeAll(ctx context.Context, a SampleAppender, bucket string, prefixes []string) (*ScrapeResult, error) {
	combined := &ScrapeResult{}

	for _, prefix := range prefixes {
		result, err := s.Scrape(ctx, a, bucket, prefix)
		if err != nil {
			combined.Errors = append(combined.Errors, fmt.Errorf("prefix %s: %w", prefix, err))

			continue
		}

		combined.Artifacts += result.Artifacts
		combined.TestsRecorded += result.TestsRecorded
		combined.Errors = append(combined.Errors, result.Errors...)
	}

	return combined, nil
}

func isJUnitFile(name string) bool {
	lower := strings.ToLower(name)

	if !strings.HasSuffix(lower, ".xml") {
		return false
	}

	base := lower
	if idx := strings.LastIndex(lower, "/"); idx >= 0 {
		base = lower[idx+1:]
	}

	return strings.HasPrefix(base, "junit") || strings.Contains(lower, "/junit/")
}
