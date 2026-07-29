package flakiness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Result holds the outcome of a full flakiness analysis run.
type Result struct {
	// Component is the name from the config.
	Component string

	// Scrape summarizes the GCS scrape phase.
	Scrape ScrapeResult

	// AllEntries is the full quarantine list (flaky + regressions + persistent).
	AllEntries []QuarantineEntry

	// QuarantinedTests lists tests currently quarantined after this run.
	QuarantinedTests []QuarantineEntry

	// NewlyQuarantined lists tests quarantined for the first time in this run.
	// Jira tickets have been filed for these (when Jira is configured).
	NewlyQuarantined []QuarantineEntry

	// Unquarantined lists tests removed from quarantine because their Jira
	// ticket was resolved (status "Done", "Closed", or "Resolved").
	Unquarantined []QuarantineEntry

	// JiraErrors collects non-fatal Jira API errors. The quarantine list is
	// still produced even when individual ticket operations fail.
	JiraErrors []error

	// Budget is the timeout budget report, populated only when
	// timeout_budget is configured. Nil otherwise.
	Budget *BudgetReport

	// TokenExpiryWarning is non-empty when the Jira API token is
	// approaching expiry. Contains a human-readable warning message.
	TokenExpiryWarning string
}

// RunOption configures a [Run] invocation.
type RunOption func(*runConfig)

type runConfig struct {
	bucketClient BucketClient
	jiraClient   JiraFiler
	now          func() time.Time
	priorEntries []QuarantineEntry
}

// JiraFiler abstracts Jira ticket operations for testability.
type JiraFiler interface {
	CreateBug(ctx context.Context, entry QuarantineEntry) (string, error)
	GetStatus(ctx context.Context, key string) (string, error)
}

// WithBucketClient injects a custom [BucketClient] instead of creating a
// real GCS client. Useful for testing or non-GCS storage backends.
func WithBucketClient(c BucketClient) RunOption {
	return func(rc *runConfig) {
		rc.bucketClient = c
	}
}

// WithJiraClient injects a [JiraFiler] for ticket creation and status
// checks. When not set, Jira integration is disabled and no tickets are
// filed. The CLI tool constructs a real client from [Config.Jira]; library
// consumers must inject their own.
func WithJiraClient(c JiraFiler) RunOption {
	return func(rc *runConfig) {
		rc.jiraClient = c
	}
}

// WithClock overrides the time source (default: time.Now). Used in tests
// to control the analysis window boundaries.
func WithClock(now func() time.Time) RunOption {
	return func(rc *runConfig) {
		rc.now = now
	}
}

// WithPriorEntries supplies the quarantine list from the previous run.
// Run compares these against newly computed entries to determine which
// tests are newly quarantined (need Jira tickets) and which can be
// unquarantined (resolved Jira tickets).
func WithPriorEntries(entries []QuarantineEntry) RunOption {
	return func(rc *runConfig) {
		rc.priorEntries = entries
	}
}

// LoadPriorEntries reads a quarantine JSON file and returns the entries.
// Returns nil (no error) when the file does not exist.
func LoadPriorEntries(path string) ([]QuarantineEntry, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-provided config path
	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("reading prior quarantine file %s: %w", path, err)
	}

	var entries []QuarantineEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing prior quarantine file %s: %w", path, err)
	}

	return entries, nil
}

// Run executes the full flakiness pipeline: scrape → ingest → analyze →
// quarantine → Jira. It uses an in-memory TSDB sink by default (zero
// external infrastructure required).
func Run(ctx context.Context, cfg Config, opts ...RunOption) (*Result, error) {
	rc := &runConfig{
		now: time.Now,
	}

	for _, o := range opts {
		o(rc)
	}

	tokenWarning := checkTokenExpiry(cfg.Jira, rc.now())

	store, err := NewStore()
	if err != nil {
		return nil, fmt.Errorf("creating store: %w", err)
	}

	defer func() { _ = store.Close() }()

	scrapeResult, err := runScrape(ctx, cfg, rc, store)
	if err != nil {
		return nil, err
	}

	if scrapeResult.TestsRecorded == 0 {
		return &Result{
			Component:          cfg.Component,
			Scrape:             *scrapeResult,
			TokenExpiryWarning: tokenWarning,
		}, nil
	}

	entries, err := runAnalysis(ctx, cfg, rc, store)
	if err != nil {
		return nil, err
	}

	entries = cfg.FilterExcluded(entries)

	var budgetReport *BudgetReport
	if cfg.TimeoutBudget.IsConfigured() {
		budgetReport, err = runBudgetAnalysis(ctx, cfg, rc, store)
		if err != nil {
			return nil, err
		}
	}

	if !cfg.Quarantine.AutoQuarantine {
		for i := range entries {
			entries[i].Quarantined = false
		}

		return &Result{
			Component:          cfg.Component,
			Scrape:             *scrapeResult,
			AllEntries:         entries,
			Budget:             budgetReport,
			TokenExpiryWarning: tokenWarning,
		}, nil
	}

	newly, unquarantined, jiraErrors := reconcileQuarantine(ctx, rc, entries)

	var quarantined []QuarantineEntry
	for i := range entries {
		if entries[i].Quarantined {
			quarantined = append(quarantined, entries[i])
		}
	}

	return &Result{
		Component:          cfg.Component,
		Scrape:             *scrapeResult,
		AllEntries:         entries,
		QuarantinedTests:   quarantined,
		NewlyQuarantined:   newly,
		Unquarantined:      unquarantined,
		JiraErrors:         jiraErrors,
		Budget:             budgetReport,
		TokenExpiryWarning: tokenWarning,
	}, nil
}

func runScrape(ctx context.Context, cfg Config, rc *runConfig, store *Store) (*ScrapeResult, error) {
	client := rc.bucketClient
	if client == nil {
		gcs, err := NewGCSClient(ctx, WithAnonymous())
		if err != nil {
			return nil, fmt.Errorf("creating GCS client: %w", err)
		}

		defer func() { _ = gcs.Close() }()

		client = gcs
	}

	scraper := NewScraper(client)
	appender := store.Appender(ctx)

	result, err := scraper.ScrapeAll(ctx, appender, cfg.GCS.Bucket, cfg.GCS.JobPrefixes)
	if err != nil {
		return nil, fmt.Errorf("scraping GCS artifacts: %w", err)
	}

	if err := appender.Commit(); err != nil {
		return nil, fmt.Errorf("committing metrics: %w", err)
	}

	return result, nil
}

func runAnalysis(ctx context.Context, cfg Config, rc *runConfig, store *Store) ([]QuarantineEntry, error) {
	now := rc.now()
	start := now.AddDate(0, 0, -cfg.Analysis.WindowDays)

	opts := ClassifyOptions{
		MinRuns: cfg.Analysis.MinRuns,
	}

	report, err := Analyze(ctx, store, start, now, opts)
	if err != nil {
		return nil, fmt.Errorf("analyzing test results: %w", err)
	}

	return report.QuarantineList(cfg.Analysis.Threshold), nil
}

func reconcileQuarantine(ctx context.Context, rc *runConfig, entries []QuarantineEntry) (newly, unquarantined []QuarantineEntry, jiraErrors []error) {
	priorByKey := indexPriorEntries(rc.priorEntries)

	jiraClient := rc.jiraClient

	for i := range entries {
		if !entries[i].Quarantined {
			continue
		}

		key := TestKey(entries[i].Name, entries[i].Suite, entries[i].Job)
		prior, wasPreviouslyQuarantined := priorByKey[key]

		if wasPreviouslyQuarantined && prior.JiraKey != "" {
			entries[i].JiraKey = prior.JiraKey
		} else {
			newly = append(newly, entries[i])
		}
	}

	if jiraClient != nil {
		for i := range newly {
			ticketKey, createErr := jiraClient.CreateBug(ctx, newly[i])
			if createErr != nil {
				jiraErrors = append(jiraErrors, fmt.Errorf("filing Jira bug for %q: %w", newly[i].Name, createErr))

				continue
			}

			newly[i].JiraKey = ticketKey

			for j := range entries {
				if TestKey(entries[j].Name, entries[j].Suite, entries[j].Job) == TestKey(newly[i].Name, newly[i].Suite, newly[i].Job) {
					entries[j].JiraKey = ticketKey

					break
				}
			}
		}

		var resolveErrs []error
		unquarantined, resolveErrs = checkResolved(ctx, jiraClient, rc.priorEntries, entries)
		jiraErrors = append(jiraErrors, resolveErrs...)
	}

	return newly, unquarantined, jiraErrors
}

var resolvedStatuses = map[string]bool{
	"Done":     true,
	"Closed":   true,
	"Resolved": true,
}

func checkResolved(ctx context.Context, client JiraFiler, priorEntries, currentEntries []QuarantineEntry) ([]QuarantineEntry, []error) {
	currentKeys := make(map[string]bool, len(currentEntries))
	for _, e := range currentEntries {
		if e.Quarantined {
			currentKeys[TestKey(e.Name, e.Suite, e.Job)] = true
		}
	}

	var unquarantined []QuarantineEntry
	var errs []error

	for _, prior := range priorEntries {
		if !prior.Quarantined || prior.JiraKey == "" {
			continue
		}

		key := TestKey(prior.Name, prior.Suite, prior.Job)
		if currentKeys[key] {
			continue
		}

		status, err := client.GetStatus(ctx, prior.JiraKey)
		if err != nil {
			errs = append(errs, fmt.Errorf("checking Jira status for %s: %w", prior.JiraKey, err))

			continue
		}

		if resolvedStatuses[status] {
			entry := prior
			entry.Quarantined = false
			unquarantined = append(unquarantined, entry)
		}
	}

	return unquarantined, errs
}

func indexPriorEntries(entries []QuarantineEntry) map[string]QuarantineEntry {
	m := make(map[string]QuarantineEntry, len(entries))
	for _, e := range entries {
		if e.Quarantined {
			m[TestKey(e.Name, e.Suite, e.Job)] = e
		}
	}

	return m
}

func runBudgetAnalysis(ctx context.Context, cfg Config, rc *runConfig, store *Store) (*BudgetReport, error) {
	tc, err := cfg.TimeoutBudget.ToTimeoutConfig()
	if err != nil {
		return nil, fmt.Errorf("parsing timeout budget config: %w", err)
	}

	threshold := cfg.TimeoutBudget.WarningThreshold
	if threshold <= 0 {
		threshold = DefaultNearTimeoutThreshold
	}

	now := rc.now()
	start := now.AddDate(0, 0, -cfg.Analysis.WindowDays)

	analyzer := NewRuntimeAnalyzer(store)

	report, err := analyzer.AnalyzeBudget(ctx, tc, threshold, start, now)
	if err != nil {
		return nil, fmt.Errorf("analyzing timeout budget: %w", err)
	}

	return report, nil
}

const defaultTokenExpiryWarningDays = 14

func checkTokenExpiry(jiraCfg JiraConfig, now time.Time) string {
	if jiraCfg.TokenExpiresAt == "" {
		return ""
	}

	expiresAt, err := time.Parse(time.RFC3339, jiraCfg.TokenExpiresAt)
	if err != nil {
		return ""
	}

	warningDays := jiraCfg.TokenExpiryWarningDays
	if warningDays <= 0 {
		warningDays = defaultTokenExpiryWarningDays
	}

	deadline := expiresAt.AddDate(0, 0, -warningDays)
	if now.Before(deadline) {
		return ""
	}

	return fmt.Sprintf("Jira API token expires at %s (within %d day warning window)",
		expiresAt.Format(time.RFC3339), warningDays)
}
