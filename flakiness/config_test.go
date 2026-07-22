package flakiness_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

const validConfigYAML = `component: kserve

gcs:
  bucket: origin-ci-test
  job_prefixes:
    - pr-logs/pull/opendatahub-io_kserve/pull-ci-kserve-main-e2e
    - logs/periodic-ci-opendatahub-io-kserve-main-e2e

analysis:
  threshold: 0.3
  window_days: 14
  min_runs: 10

quarantine:
  config_path: hack/quarantine.json
  auto_quarantine: true
  exclude_patterns:
    - "TestSmoke/.*"
    - "TestCritical/.*"

jira:
  project: RHOAIENG
  component: KServe
  labels: [flaky-test, kserve]
  token_env: QUARANTINE_JIRA_API_TOKEN
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, ".flakiness.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, validConfigYAML)

	cfg, err := flakiness.LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "kserve", cfg.Component)
	assert.Equal(t, "origin-ci-test", cfg.GCS.Bucket)
	assert.Equal(t, []string{
		"pr-logs/pull/opendatahub-io_kserve/pull-ci-kserve-main-e2e",
		"logs/periodic-ci-opendatahub-io-kserve-main-e2e",
	}, cfg.GCS.JobPrefixes)
	assert.InDelta(t, 0.3, cfg.Analysis.Threshold, 0.001)
	assert.Equal(t, 14, cfg.Analysis.WindowDays)
	assert.Equal(t, 10, cfg.Analysis.MinRuns)
	assert.Equal(t, "hack/quarantine.json", cfg.Quarantine.ConfigPath)
	assert.True(t, cfg.Quarantine.AutoQuarantine)
	assert.Equal(t, []string{"TestSmoke/.*", "TestCritical/.*"}, cfg.Quarantine.ExcludePatterns)
	assert.Equal(t, "RHOAIENG", cfg.Jira.Project)
	assert.Equal(t, "KServe", cfg.Jira.Component)
	assert.Equal(t, []string{"flaky-test", "kserve"}, cfg.Jira.Labels)
	assert.Equal(t, "QUARANTINE_JIRA_API_TOKEN", cfg.Jira.TokenEnv)
}

func TestLoadConfig_Defaults(t *testing.T) {
	t.Parallel()

	yaml := `component: model-mesh
gcs:
  bucket: origin-ci-test
  job_prefixes:
    - logs/periodic-ci-model-mesh
`
	path := writeConfig(t, yaml)

	cfg, err := flakiness.LoadConfig(path)
	require.NoError(t, err)

	assert.InDelta(t, flakiness.DefaultThreshold, cfg.Analysis.Threshold, 0.001)
	assert.Equal(t, flakiness.DefaultWindowDays, cfg.Analysis.WindowDays)
	assert.Equal(t, flakiness.DefaultMinRunsConfig, cfg.Analysis.MinRuns)
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := flakiness.LoadConfig("/nonexistent/.flakiness.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, "{{not yaml}}")

	_, err := flakiness.LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config file")
}

func TestLoadConfig_UnknownFieldRejected(t *testing.T) {
	t.Parallel()

	yaml := `component: kserve
gcs:
  bucket: origin-ci-test
  job_prefixes:
    - logs/periodic-ci-kserve
unknownfield: 0.3
`
	path := writeConfig(t, yaml)

	_, err := flakiness.LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config file")
	assert.Contains(t, err.Error(), "unknownfield")
}

func TestLoadConfig_ValidationFailure(t *testing.T) {
	t.Parallel()

	yaml := `component: ""
gcs:
  bucket: ""
  job_prefixes: []
`
	path := writeConfig(t, yaml)

	_, err := flakiness.LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validating config")
	assert.Contains(t, err.Error(), "component is required")
	assert.Contains(t, err.Error(), "gcs.bucket is required")
	assert.Contains(t, err.Error(), "gcs.job_prefixes must contain at least one entry")
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      flakiness.Config
		errParts []string
	}{
		{
			name: "valid config",
			cfg: flakiness.Config{
				Component: "kserve",
				GCS:       flakiness.GCSConfig{Bucket: "bucket", JobPrefixes: []string{"prefix/"}},
				Analysis:  flakiness.AnalysisConfig{Threshold: 0.2, WindowDays: 30, MinRuns: 5},
			},
		},
		{
			name: "missing component",
			cfg: flakiness.Config{
				GCS:      flakiness.GCSConfig{Bucket: "bucket", JobPrefixes: []string{"p/"}},
				Analysis: flakiness.AnalysisConfig{Threshold: 0.2, WindowDays: 30, MinRuns: 5},
			},
			errParts: []string{"component is required"},
		},
		{
			name: "missing bucket",
			cfg: flakiness.Config{
				Component: "kserve",
				GCS:       flakiness.GCSConfig{JobPrefixes: []string{"p/"}},
				Analysis:  flakiness.AnalysisConfig{Threshold: 0.2, WindowDays: 30, MinRuns: 5},
			},
			errParts: []string{"gcs.bucket is required"},
		},
		{
			name: "empty job prefixes",
			cfg: flakiness.Config{
				Component: "kserve",
				GCS:       flakiness.GCSConfig{Bucket: "bucket"},
				Analysis:  flakiness.AnalysisConfig{Threshold: 0.2, WindowDays: 30, MinRuns: 5},
			},
			errParts: []string{"gcs.job_prefixes must contain at least one entry"},
		},
		{
			name: "threshold too high",
			cfg: flakiness.Config{
				Component: "kserve",
				GCS:       flakiness.GCSConfig{Bucket: "bucket", JobPrefixes: []string{"p/"}},
				Analysis:  flakiness.AnalysisConfig{Threshold: 1.5, WindowDays: 30, MinRuns: 5},
			},
			errParts: []string{"analysis.threshold must be in (0, 1]"},
		},
		{
			name: "threshold zero",
			cfg: flakiness.Config{
				Component: "kserve",
				GCS:       flakiness.GCSConfig{Bucket: "bucket", JobPrefixes: []string{"p/"}},
				Analysis:  flakiness.AnalysisConfig{Threshold: 0, WindowDays: 30, MinRuns: 5},
			},
			errParts: []string{"analysis.threshold must be in (0, 1]"},
		},
		{
			name: "negative window days",
			cfg: flakiness.Config{
				Component: "kserve",
				GCS:       flakiness.GCSConfig{Bucket: "bucket", JobPrefixes: []string{"p/"}},
				Analysis:  flakiness.AnalysisConfig{Threshold: 0.2, WindowDays: -1, MinRuns: 5},
			},
			errParts: []string{"analysis.window_days must be positive"},
		},
		{
			name: "negative min runs",
			cfg: flakiness.Config{
				Component: "kserve",
				GCS:       flakiness.GCSConfig{Bucket: "bucket", JobPrefixes: []string{"p/"}},
				Analysis:  flakiness.AnalysisConfig{Threshold: 0.2, WindowDays: 30, MinRuns: -1},
			},
			errParts: []string{"analysis.min_runs must be positive"},
		},
		{
			name: "invalid exclude pattern",
			cfg: flakiness.Config{
				Component: "kserve",
				GCS:       flakiness.GCSConfig{Bucket: "bucket", JobPrefixes: []string{"p/"}},
				Analysis:  flakiness.AnalysisConfig{Threshold: 0.2, WindowDays: 30, MinRuns: 5},
				Quarantine: flakiness.QuarantineConfig{
					ExcludePatterns: []string{"[invalid"},
				},
			},
			errParts: []string{"quarantine.exclude_patterns[0]", "invalid regex"},
		},
		{
			name: "multiple errors",
			cfg: flakiness.Config{
				Analysis: flakiness.AnalysisConfig{Threshold: -1, WindowDays: 0, MinRuns: 0},
			},
			errParts: []string{
				"component is required",
				"gcs.bucket is required",
				"gcs.job_prefixes must contain at least one entry",
				"analysis.threshold must be in (0, 1]",
				"analysis.window_days must be positive",
				"analysis.min_runs must be positive",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.cfg.Validate()
			if len(tc.errParts) == 0 {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			for _, part := range tc.errParts {
				assert.Contains(t, err.Error(), part)
			}
		})
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	yaml := `component: original
gcs:
  bucket: original-bucket
  job_prefixes:
    - logs/original
analysis:
  threshold: 0.1
  window_days: 7
  min_runs: 3
quarantine:
  config_path: original/path.json
  auto_quarantine: false
jira:
  project: ORIG
  component: Original
  token_env: ORIG_TOKEN
`
	path := writeConfig(t, yaml)

	t.Setenv("FLAKINESS_COMPONENT", "overridden")
	t.Setenv("FLAKINESS_GCS_BUCKET", "overridden-bucket")
	t.Setenv("FLAKINESS_THRESHOLD", "0.5")
	t.Setenv("FLAKINESS_WINDOW_DAYS", "60")
	t.Setenv("FLAKINESS_MIN_RUNS", "20")
	t.Setenv("FLAKINESS_QUARANTINE_CONFIG_PATH", "overridden/path.json")
	t.Setenv("FLAKINESS_AUTO_QUARANTINE", "true")
	t.Setenv("FLAKINESS_JIRA_PROJECT", "OVERRIDE")
	t.Setenv("FLAKINESS_JIRA_COMPONENT", "Overridden")
	t.Setenv("FLAKINESS_JIRA_TOKEN_ENV", "OVERRIDE_TOKEN")

	cfg, err := flakiness.LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "overridden", cfg.Component)
	assert.Equal(t, "overridden-bucket", cfg.GCS.Bucket)
	assert.InDelta(t, 0.5, cfg.Analysis.Threshold, 0.001)
	assert.Equal(t, 60, cfg.Analysis.WindowDays)
	assert.Equal(t, 20, cfg.Analysis.MinRuns)
	assert.Equal(t, "overridden/path.json", cfg.Quarantine.ConfigPath)
	assert.True(t, cfg.Quarantine.AutoQuarantine)
	assert.Equal(t, "OVERRIDE", cfg.Jira.Project)
	assert.Equal(t, "Overridden", cfg.Jira.Component)
	assert.Equal(t, "OVERRIDE_TOKEN", cfg.Jira.TokenEnv)
}

func TestLoadConfig_EnvOverrides_InvalidNumericRejected(t *testing.T) {
	yaml := `component: kserve
gcs:
  bucket: bucket
  job_prefixes:
    - logs/prefix
analysis:
  threshold: 0.3
  window_days: 14
  min_runs: 10
`
	path := writeConfig(t, yaml)

	t.Setenv("FLAKINESS_THRESHOLD", "not-a-number")
	t.Setenv("FLAKINESS_WINDOW_DAYS", "abc")
	t.Setenv("FLAKINESS_MIN_RUNS", "xyz")

	_, err := flakiness.LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FLAKINESS_THRESHOLD")
	assert.Contains(t, err.Error(), "FLAKINESS_WINDOW_DAYS")
	assert.Contains(t, err.Error(), "FLAKINESS_MIN_RUNS")
}

func TestLoadConfig_EnvOverrides_AutoQuarantineFalse(t *testing.T) {
	yaml := `component: kserve
gcs:
  bucket: bucket
  job_prefixes:
    - logs/prefix
quarantine:
  auto_quarantine: true
`
	path := writeConfig(t, yaml)

	t.Setenv("FLAKINESS_AUTO_QUARANTINE", "false")

	cfg, err := flakiness.LoadConfig(path)
	require.NoError(t, err)

	assert.False(t, cfg.Quarantine.AutoQuarantine)
}

func TestScraper_ScrapeAll(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket",
		"logs/periodic-ci/1/artifacts/junit_results.xml",
		[]byte(`<testsuite name="e2e" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestA" time="1.0"/>
</testsuite>`))
	client.addObject("bucket",
		"pr-logs/pull/org_repo/42/pull-ci-e2e/100/artifacts/junit_results.xml",
		[]byte(`<testsuite name="e2e" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestB" time="2.0"/>
  <testcase name="TestC" time="3.0"/>
</testsuite>`))

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.ScrapeAll(context.Background(), appender,
		"bucket", []string{
			"logs/periodic-ci/1/",
			"pr-logs/pull/org_repo/42/",
		})
	require.NoError(t, err)

	assert.Equal(t, 2, result.Artifacts)
	assert.Equal(t, 3, result.TestsRecorded)
	assert.Empty(t, result.Errors)
}

func TestScraper_ScrapeAll_PartialFailure(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket",
		"logs/good-job/1/artifacts/junit.xml",
		[]byte(`<testsuite name="ok" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestOk" time="1.0"/>
</testsuite>`))

	scraper := flakiness.NewScraper(client,
		flakiness.WithMaxRetries(1),
		flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.ScrapeAll(context.Background(), appender,
		"bucket", []string{
			"logs/good-job/1/",
			"logs/nonexistent/",
		})
	require.NoError(t, err)

	assert.Equal(t, 1, result.Artifacts)
	assert.Equal(t, 1, result.TestsRecorded)
	assert.Empty(t, result.Errors)
}

func TestScraper_ScrapeAll_ListError(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.listErr = assert.AnError

	scraper := flakiness.NewScraper(client,
		flakiness.WithMaxRetries(1),
		flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.ScrapeAll(context.Background(), appender,
		"bucket", []string{"prefix-a/", "prefix-b/"})
	require.NoError(t, err)

	assert.Equal(t, 0, result.Artifacts)
	assert.Len(t, result.Errors, 2)
	assert.Contains(t, result.Errors[0].Error(), "prefix-a/")
	assert.Contains(t, result.Errors[1].Error(), "prefix-b/")
}

func TestScraper_ScrapeAll_IntegrationWithStore(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket",
		"logs/periodic/10/artifacts/junit.xml",
		[]byte(`<testsuite name="e2e" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestShared" time="1.0"/>
  <testcase name="TestShared" time="2.0">
    <failure message="flake"/>
  </testcase>
</testsuite>`))
	client.addObject("bucket",
		"logs/presubmit/20/artifacts/junit.xml",
		[]byte(`<testsuite name="e2e" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestShared" time="1.5"/>
</testsuite>`))

	store, err := flakiness.NewStore()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	ctx := context.Background()
	appender := store.Appender(ctx)

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))

	result, err := scraper.ScrapeAll(ctx, appender, "bucket", []string{
		"logs/periodic/10/",
		"logs/presubmit/20/",
	})
	require.NoError(t, err)
	require.NoError(t, appender.Commit())

	assert.Equal(t, 3, result.TestsRecorded)

	ts := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	queryTime := ts.Add(time.Minute)

	res, err := store.InstantQuery(ctx,
		`count(`+flakiness.MetricTestExecutionTotal+`)`, queryTime)
	require.NoError(t, err)

	vec, err := res.Vector()
	require.NoError(t, err)
	require.Len(t, vec, 1)
	assert.InDelta(t, 3.0, float64(vec[0].F), 0.001)
}

func TestConfig_FilterExcluded(t *testing.T) {
	t.Parallel()

	cfg := flakiness.Config{
		Quarantine: flakiness.QuarantineConfig{
			ExcludePatterns: []string{"TestSmoke/.*", "TestCritical/.*"},
		},
	}

	entries := []flakiness.QuarantineEntry{
		{Name: "TestSmoke/basic", Quarantined: true, FlakeRate: 0.5},
		{Name: "TestCritical/login", Quarantined: true, FlakeRate: 0.4},
		{Name: "TestFlaky/inference", Quarantined: true, FlakeRate: 0.3},
		{Name: "TestRegression/api", Quarantined: false, FlakeRate: 0.8},
	}

	filtered := cfg.FilterExcluded(entries)
	require.Len(t, filtered, 4)

	assert.False(t, filtered[0].Quarantined, "TestSmoke should be excluded from quarantine")
	assert.False(t, filtered[1].Quarantined, "TestCritical should be excluded from quarantine")
	assert.True(t, filtered[2].Quarantined, "TestFlaky should remain quarantined")
	assert.False(t, filtered[3].Quarantined, "TestRegression was already not quarantined")
}

func TestConfig_FilterExcluded_NoPatterns(t *testing.T) {
	t.Parallel()

	cfg := flakiness.Config{}

	entries := []flakiness.QuarantineEntry{
		{Name: "TestA", Quarantined: true},
	}

	filtered := cfg.FilterExcluded(entries)
	require.Len(t, filtered, 1)
	assert.True(t, filtered[0].Quarantined)
}

func TestConfig_IsolatedQuarantineState(t *testing.T) {
	t.Parallel()

	kserveYAML := `component: kserve
gcs:
  bucket: origin-ci-test
  job_prefixes:
    - logs/periodic-ci-kserve
analysis:
  threshold: 0.3
  window_days: 14
  min_runs: 3
quarantine:
  config_path: hack/kserve-quarantine.json
  auto_quarantine: true
jira:
  project: RHOAIENG
  component: KServe
  labels: [flaky-test, kserve]
`
	modelmeshYAML := `component: model-mesh
gcs:
  bucket: origin-ci-test
  job_prefixes:
    - logs/periodic-ci-model-mesh
analysis:
  threshold: 0.15
  window_days: 30
  min_runs: 5
quarantine:
  config_path: hack/modelmesh-quarantine.json
  auto_quarantine: true
jira:
  project: RHOAIENG
  component: ModelMesh
  labels: [flaky-test, model-mesh]
`
	kservePath := writeConfig(t, kserveYAML)
	modelmeshPath := writeConfig(t, modelmeshYAML)

	kserveCfg, err := flakiness.LoadConfig(kservePath)
	require.NoError(t, err)

	modelmeshCfg, err := flakiness.LoadConfig(modelmeshPath)
	require.NoError(t, err)

	assert.Equal(t, "kserve", kserveCfg.Component)
	assert.Equal(t, "model-mesh", modelmeshCfg.Component)

	assert.NotEqual(t, kserveCfg.Quarantine.ConfigPath, modelmeshCfg.Quarantine.ConfigPath,
		"each component must have an independent quarantine config path")
	assert.NotEqual(t, kserveCfg.Jira.Component, modelmeshCfg.Jira.Component,
		"each component must have its own Jira component")
	assert.NotEqual(t, kserveCfg.GCS.JobPrefixes, modelmeshCfg.GCS.JobPrefixes,
		"each component must scrape its own CI job prefixes")

	assert.InDelta(t, 0.3, kserveCfg.Analysis.Threshold, 0.001)
	assert.InDelta(t, 0.15, modelmeshCfg.Analysis.Threshold, 0.001)
}
