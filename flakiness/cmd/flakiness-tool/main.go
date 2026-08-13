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
	"github.com/opendatahub-io/odh-platform-utilities/flakiness/jira"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "run" {
		log.Fatalf("Usage: %s run --config <path> [--push-back]", os.Args[0]) //nolint:gosec // usage text only echoes the current argv[0]
	}

	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := runCmd.String("config", "", "path to .flakiness.yaml config file")
	pushBackFlag := runCmd.Bool("push-back", false, "commit and push updated quarantine file back to the repo (requires GITHUB_TOKEN)")

	if err := runCmd.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}

	if *configPath == "" {
		runCmd.Usage()
		os.Exit(1)
	}

	if err := run(*configPath, *pushBackFlag); err != nil {
		log.Fatal(err)
	}
}

func run(configPath string, pushBackEnabled bool) error {
	cfg, err := flakiness.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	var opts []flakiness.RunOption

	if cfg.Quarantine.ConfigPath != "" {
		prior, loadErr := flakiness.LoadPriorEntries(cfg.Quarantine.ConfigPath)
		if loadErr != nil {
			return fmt.Errorf("loading prior quarantine entries: %w", loadErr)
		}

		if prior != nil {
			opts = append(opts, flakiness.WithPriorEntries(prior))
		}
	}

	if cfg.Jira.APIURL != "" && cfg.Jira.TokenEnv != "" {
		jiraCfg, jiraErr := jira.FromFlakinessConfig(cfg.Jira)
		if jiraErr != nil {
			return fmt.Errorf("configuring Jira: %w", jiraErr)
		}

		if jiraCfg.TokenExpirySoon(time.Now()) {
			_, _ = fmt.Fprintf(os.Stderr, "WARNING: Jira API token expires at %s (within %d day warning window)\n",
				jiraCfg.TokenExpiresAt.Format(time.RFC3339), jiraCfg.TokenExpiryWarningDays)
		}

		jiraClient, jiraErr := jira.NewClient(jiraCfg, nil)
		if jiraErr != nil {
			return fmt.Errorf("creating Jira client: %w", jiraErr)
		}

		opts = append(opts, flakiness.WithJiraClient(jiraClient))
	}

	result, err := flakiness.Run(context.Background(), cfg, opts...)
	if err != nil {
		return err
	}

	printSummary(result)

	if result.Scrape.TestsRecorded == 0 {
		return nil
	}

	if err := writeQuarantineList(cfg, result.AllEntries); err != nil {
		return err
	}

	if pushBackEnabled && cfg.Quarantine.ConfigPath != "" {
		pushCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		if err := pushBack(pushCtx, cfg.Quarantine.ConfigPath); err != nil {
			return fmt.Errorf("push-back: %w", err)
		}
	}

	return nil
}

func printSummary(result *flakiness.Result) {
	_, _ = fmt.Fprintf(os.Stdout, "component: %s\n", result.Component)
	_, _ = fmt.Fprintf(os.Stdout, "scraped: %d artifacts, %d tests, %d errors\n",
		result.Scrape.Artifacts, result.Scrape.TestsRecorded, len(result.Scrape.Errors))

	for _, e := range result.Scrape.Errors {
		_, _ = fmt.Fprintf(os.Stderr, "  warning: %v\n", e)
	}

	for _, e := range result.JiraErrors {
		_, _ = fmt.Fprintf(os.Stderr, "  jira warning: %v\n", e)
	}

	_, _ = fmt.Fprintf(os.Stdout, "quarantined: %d total, %d new, %d removed\n",
		len(result.QuarantinedTests), len(result.NewlyQuarantined), len(result.Unquarantined))

	for _, e := range result.NewlyQuarantined {
		_, _ = fmt.Fprintf(os.Stdout, "  + %s (%.1f%% flake rate, %s)\n",
			e.Name, e.FlakeRate*100, e.JiraKey)
	}

	for _, e := range result.Unquarantined {
		_, _ = fmt.Fprintf(os.Stdout, "  - %s (Jira %s resolved)\n",
			e.Name, e.JiraKey)
	}

	printBudgetReport(result.Budget)
}

func printBudgetReport(report *flakiness.BudgetReport) {
	if report == nil {
		return
	}

	_, _ = fmt.Fprintf(os.Stdout, "\ntimeout budget analysis:\n")

	if report.Pipeline != nil {
		_, _ = fmt.Fprintf(os.Stdout, "  pipeline: %s sum-of-tests / %s timeout (%.1fx)\n",
			report.Pipeline.ActualDuration.Truncate(time.Second),
			report.Pipeline.ConfiguredTimeout.Truncate(time.Second),
			report.Pipeline.Utilisation)
	}

	for _, s := range report.Suites {
		_, _ = fmt.Fprintf(os.Stdout, "  suite %s: %s sum-of-tests / %s timeout (%.1fx)\n",
			s.Name,
			s.ActualDuration.Truncate(time.Second),
			s.ConfiguredTimeout.Truncate(time.Second),
			s.Utilisation)
	}

	if len(report.NearTimeout) > 0 {
		_, _ = fmt.Fprintf(os.Stdout, "  near-timeout tests:\n")

		for _, t := range report.NearTimeout {
			_, _ = fmt.Fprintf(os.Stdout, "    %s: P95=%s, timeout=%s (%.0f%%)\n",
				t.Name, t.P95.Truncate(time.Millisecond), t.Timeout, t.Utilisation*100)
		}
	}

	if len(report.Recommendations) > 0 {
		_, _ = fmt.Fprintf(os.Stdout, "  recommendations:\n")

		for _, r := range report.Recommendations {
			_, _ = fmt.Fprintf(os.Stdout, "    %s %s: %s -> %s (%s)\n",
				r.Action, r.Name,
				r.CurrentTimeout.Truncate(time.Millisecond),
				r.SuggestedTimeout.Truncate(time.Millisecond),
				r.Reason)
		}
	}
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
