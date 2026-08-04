package flakiness_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

type fakeJiraClient struct {
	created  []flakiness.QuarantineEntry
	nextKey  int
	statuses map[string]string
}

func newFakeJiraClient() *fakeJiraClient {
	return &fakeJiraClient{
		statuses: make(map[string]string),
	}
}

func (f *fakeJiraClient) CreateBug(_ context.Context, entry flakiness.QuarantineEntry) (string, error) {
	f.nextKey++
	key := fmt.Sprintf("TEST-%d", f.nextKey)
	f.created = append(f.created, entry)

	return key, nil
}

func (f *fakeJiraClient) GetStatus(_ context.Context, key string) (string, error) {
	status, ok := f.statuses[key]
	if !ok {
		return "To Do", nil
	}

	return status, nil
}

func TestRun_BasicPipeline(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	for i := range 10 {
		buildID := strconv.Itoa(100 + i)
		path := fmt.Sprintf("logs/periodic-ci-main/%s/artifacts/junit_results.xml", buildID)

		var xml []byte
		if i < 7 {
			xml = fmt.Appendf(nil, `<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestStable" time="5.0"/>
  <testcase name="TestFlaky" time="3.0">
    <failure message="intermittent"/>
  </testcase>
</testsuite>`, i+1)
		} else {
			xml = fmt.Appendf(nil, `<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestStable" time="5.0"/>
  <testcase name="TestFlaky" time="3.0"/>
</testsuite>`, i+1)
		}

		client.addObject("ci-bucket", path, xml)
	}

	cfg := flakiness.Config{
		Component: "test-component",
		GCS: flakiness.GCSConfig{
			Bucket:      "ci-bucket",
			JobPrefixes: []string{"logs/periodic-ci-main/"},
		},
		Analysis: flakiness.AnalysisConfig{
			Threshold:  0.3,
			WindowDays: 30,
			MinRuns:    5,
		},
		Quarantine: flakiness.QuarantineConfig{
			AutoQuarantine: true,
		},
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	result, err := flakiness.Run(context.Background(), cfg,
		flakiness.WithBucketClient(client),
		flakiness.WithClock(func() time.Time { return now }),
	)
	require.NoError(t, err)

	assert.Equal(t, "test-component", result.Component)
	assert.Equal(t, 10, result.Scrape.Artifacts)
	assert.Positive(t, result.Scrape.TestsRecorded)
	assert.NotEmpty(t, result.AllEntries)

	hasFlaky := false
	for _, e := range result.QuarantinedTests {
		if e.Name == "TestFlaky" {
			hasFlaky = true
			assert.True(t, e.Quarantined)
			assert.Greater(t, e.FlakeRate, 0.3)
		}
	}

	assert.True(t, hasFlaky, "TestFlaky should be quarantined")
}

func TestRun_JiraIntegration(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	for i := range 10 {
		buildID := strconv.Itoa(200 + i)
		path := fmt.Sprintf("logs/job/%s/artifacts/junit.xml", buildID)

		var xml []byte
		if i%3 == 0 {
			xml = fmt.Appendf(nil, `<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestFlakyJira" time="2.0">
    <failure message="oops"/>
  </testcase>
</testsuite>`, i+1)
		} else {
			xml = fmt.Appendf(nil, `<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestFlakyJira" time="2.0"/>
</testsuite>`, i+1)
		}

		client.addObject("bucket", path, xml)
	}

	jiraFiler := newFakeJiraClient()

	cfg := flakiness.Config{
		Component: "jira-test",
		GCS: flakiness.GCSConfig{
			Bucket:      "bucket",
			JobPrefixes: []string{"logs/job/"},
		},
		Analysis: flakiness.AnalysisConfig{
			Threshold:  0.2,
			WindowDays: 30,
			MinRuns:    3,
		},
		Quarantine: flakiness.QuarantineConfig{
			AutoQuarantine: true,
		},
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	result, err := flakiness.Run(context.Background(), cfg,
		flakiness.WithBucketClient(client),
		flakiness.WithJiraClient(jiraFiler),
		flakiness.WithClock(func() time.Time { return now }),
	)
	require.NoError(t, err)

	assert.NotEmpty(t, result.NewlyQuarantined)

	for _, entry := range result.NewlyQuarantined {
		assert.NotEmpty(t, entry.JiraKey, "newly quarantined entries should have Jira key")
	}

	assert.NotEmpty(t, jiraFiler.created, "Jira bugs should have been filed")
}

func TestRun_ComponentIsolation(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()

	for i := range 10 {
		buildID := strconv.Itoa(300 + i)

		pathA := fmt.Sprintf("logs/comp-a/%s/artifacts/junit.xml", buildID)

		var xmlA []byte
		if i%2 == 0 {
			xmlA = fmt.Appendf(nil,
				`<testsuite name="a" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestOnlyInA" time="1.0">
    <failure message="flaky"/>
  </testcase>
</testsuite>`, i+1)
		} else {
			xmlA = fmt.Appendf(nil,
				`<testsuite name="a" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestOnlyInA" time="1.0"/>
</testsuite>`, i+1)
		}

		client.addObject("bucket", pathA, xmlA)

		pathB := fmt.Sprintf("logs/comp-b/%s/artifacts/junit.xml", buildID)
		client.addObject("bucket", pathB, fmt.Appendf(nil,
			`<testsuite name="b" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestOnlyInB" time="1.0"/>
</testsuite>`, i+1))
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	cfgA := flakiness.Config{
		Component: "component-a",
		GCS: flakiness.GCSConfig{
			Bucket:      "bucket",
			JobPrefixes: []string{"logs/comp-a/"},
		},
		Analysis: flakiness.AnalysisConfig{
			Threshold:  0.2,
			WindowDays: 30,
			MinRuns:    3,
		},
		Quarantine: flakiness.QuarantineConfig{
			AutoQuarantine: true,
		},
	}

	cfgB := flakiness.Config{
		Component: "component-b",
		GCS: flakiness.GCSConfig{
			Bucket:      "bucket",
			JobPrefixes: []string{"logs/comp-b/"},
		},
		Analysis: flakiness.AnalysisConfig{
			Threshold:  0.2,
			WindowDays: 30,
			MinRuns:    3,
		},
		Quarantine: flakiness.QuarantineConfig{
			AutoQuarantine: true,
		},
	}

	resultA, err := flakiness.Run(context.Background(), cfgA,
		flakiness.WithBucketClient(client),
		flakiness.WithClock(func() time.Time { return now }),
	)
	require.NoError(t, err)

	resultB, err := flakiness.Run(context.Background(), cfgB,
		flakiness.WithBucketClient(client),
		flakiness.WithClock(func() time.Time { return now }),
	)
	require.NoError(t, err)

	assert.Equal(t, "component-a", resultA.Component)
	assert.Equal(t, "component-b", resultB.Component)

	assert.NotEmpty(t, resultA.QuarantinedTests, "component-a should have quarantined tests")
	assert.Empty(t, resultB.QuarantinedTests, "component-b should have no quarantined tests")

	for _, e := range resultA.AllEntries {
		assert.Equal(t, "TestOnlyInA", e.Name)
	}
}

func TestRun_Unquarantine(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	for i := range 10 {
		buildID := strconv.Itoa(400 + i)
		path := fmt.Sprintf("logs/job/%s/artifacts/junit.xml", buildID)
		client.addObject("bucket", path, fmt.Appendf(nil,
			`<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestNowFixed" time="1.0"/>
</testsuite>`, i+1))
	}

	priorEntries := []flakiness.QuarantineEntry{
		{
			Name:        "TestNowFixed",
			Suite:       "e2e",
			Job:         "job",
			Quarantined: true,
			JiraKey:     "PROJ-999",
			FlakeRate:   0.5,
			TotalRuns:   10,
			FailedRuns:  5,
		},
	}

	jiraFiler := newFakeJiraClient()
	jiraFiler.statuses["PROJ-999"] = "Done"

	cfg := flakiness.Config{
		Component: "unquarantine-test",
		GCS: flakiness.GCSConfig{
			Bucket:      "bucket",
			JobPrefixes: []string{"logs/job/"},
		},
		Analysis: flakiness.AnalysisConfig{
			Threshold:  0.2,
			WindowDays: 30,
			MinRuns:    3,
		},
		Quarantine: flakiness.QuarantineConfig{
			AutoQuarantine: true,
		},
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	result, err := flakiness.Run(context.Background(), cfg,
		flakiness.WithBucketClient(client),
		flakiness.WithJiraClient(jiraFiler),
		flakiness.WithPriorEntries(priorEntries),
		flakiness.WithClock(func() time.Time { return now }),
	)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Unquarantined)
	assert.Equal(t, "TestNowFixed", result.Unquarantined[0].Name)
	assert.False(t, result.Unquarantined[0].Quarantined)
}

func TestRun_NoTestResults(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()

	cfg := flakiness.Config{
		Component: "empty",
		GCS: flakiness.GCSConfig{
			Bucket:      "bucket",
			JobPrefixes: []string{"logs/nonexistent/"},
		},
		Analysis: flakiness.AnalysisConfig{
			Threshold:  0.2,
			WindowDays: 30,
			MinRuns:    3,
		},
	}

	result, err := flakiness.Run(context.Background(), cfg,
		flakiness.WithBucketClient(client),
	)
	require.NoError(t, err)

	assert.Equal(t, "empty", result.Component)
	assert.Equal(t, 0, result.Scrape.TestsRecorded)
	assert.Empty(t, result.AllEntries)
	assert.Empty(t, result.QuarantinedTests)
}

func TestRun_ExcludePatterns(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	for i := range 10 {
		buildID := strconv.Itoa(500 + i)
		path := fmt.Sprintf("logs/job/%s/artifacts/junit.xml", buildID)
		client.addObject("bucket", path, fmt.Appendf(nil,
			`<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestKnownFlaky" time="1.0">
    <failure message="known"/>
  </testcase>
</testsuite>`, i+1))
	}

	cfg := flakiness.Config{
		Component: "exclude-test",
		GCS: flakiness.GCSConfig{
			Bucket:      "bucket",
			JobPrefixes: []string{"logs/job/"},
		},
		Analysis: flakiness.AnalysisConfig{
			Threshold:  0.2,
			WindowDays: 30,
			MinRuns:    3,
		},
		Quarantine: flakiness.QuarantineConfig{
			AutoQuarantine:  true,
			ExcludePatterns: []string{`TestKnownFlaky`},
		},
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	result, err := flakiness.Run(context.Background(), cfg,
		flakiness.WithBucketClient(client),
		flakiness.WithClock(func() time.Time { return now }),
	)
	require.NoError(t, err)

	assert.Empty(t, result.QuarantinedTests, "excluded test should not be quarantined")

	for _, e := range result.AllEntries {
		if e.Name == "TestKnownFlaky" {
			assert.False(t, e.Quarantined)
		}
	}
}

func TestRun_PriorEntriesPreserveJiraKey(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	for i := range 10 {
		buildID := strconv.Itoa(600 + i)
		path := fmt.Sprintf("logs/job/%s/artifacts/junit.xml", buildID)

		var xml []byte
		if i%2 == 0 {
			xml = fmt.Appendf(nil, `<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestStillFlaky" time="1.0">
    <failure message="still flaky"/>
  </testcase>
</testsuite>`, i+1)
		} else {
			xml = fmt.Appendf(nil, `<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestStillFlaky" time="1.0"/>
</testsuite>`, i+1)
		}

		client.addObject("bucket", path, xml)
	}

	priorEntries := []flakiness.QuarantineEntry{
		{
			Name:        "TestStillFlaky",
			Suite:       "e2e",
			Job:         "job",
			Quarantined: true,
			JiraKey:     "EXISTING-42",
			FlakeRate:   0.5,
			TotalRuns:   10,
			FailedRuns:  5,
		},
	}

	jiraFiler := newFakeJiraClient()

	cfg := flakiness.Config{
		Component: "preserve-key-test",
		GCS: flakiness.GCSConfig{
			Bucket:      "bucket",
			JobPrefixes: []string{"logs/job/"},
		},
		Analysis: flakiness.AnalysisConfig{
			Threshold:  0.2,
			WindowDays: 30,
			MinRuns:    3,
		},
		Quarantine: flakiness.QuarantineConfig{
			AutoQuarantine: true,
		},
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	result, err := flakiness.Run(context.Background(), cfg,
		flakiness.WithBucketClient(client),
		flakiness.WithJiraClient(jiraFiler),
		flakiness.WithPriorEntries(priorEntries),
		flakiness.WithClock(func() time.Time { return now }),
	)
	require.NoError(t, err)

	assert.Empty(t, result.NewlyQuarantined, "already-known flaky should not be newly quarantined")
	assert.Empty(t, jiraFiler.created, "no new Jira bugs should be filed for existing entries")

	for _, e := range result.QuarantinedTests {
		if e.Name == "TestStillFlaky" {
			assert.Equal(t, "EXISTING-42", e.JiraKey)
		}
	}
}

func TestLoadPriorEntries(t *testing.T) {
	t.Parallel()

	t.Run("file exists", func(t *testing.T) {
		t.Parallel()

		entries := []flakiness.QuarantineEntry{
			{Name: "TestA", Quarantined: true, JiraKey: "KEY-1"},
			{Name: "TestB", Quarantined: false},
		}

		data, err := json.MarshalIndent(entries, "", "  ")
		require.NoError(t, err)

		path := filepath.Join(t.TempDir(), "quarantine.json")
		require.NoError(t, os.WriteFile(path, data, 0o600))

		loaded, err := flakiness.LoadPriorEntries(path)
		require.NoError(t, err)
		assert.Len(t, loaded, 2)
		assert.Equal(t, "TestA", loaded[0].Name)
		assert.Equal(t, "KEY-1", loaded[0].JiraKey)
	})

	t.Run("file does not exist", func(t *testing.T) {
		t.Parallel()

		loaded, err := flakiness.LoadPriorEntries("/nonexistent/path/quarantine.json")
		require.NoError(t, err)
		assert.Nil(t, loaded)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "bad.json")
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

		_, err := flakiness.LoadPriorEntries(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parsing prior quarantine file")
	})
}

func TestRun_AutoQuarantineDisabled(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	for i := range 10 {
		buildID := strconv.Itoa(700 + i)
		path := fmt.Sprintf("logs/job/%s/artifacts/junit.xml", buildID)

		var xml []byte
		if i%2 == 0 {
			xml = fmt.Appendf(nil, `<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestFlakyButNoAction" time="1.0">
    <failure message="flaky"/>
  </testcase>
</testsuite>`, i+1)
		} else {
			xml = fmt.Appendf(nil, `<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestFlakyButNoAction" time="1.0"/>
</testsuite>`, i+1)
		}

		client.addObject("bucket", path, xml)
	}

	jiraFiler := newFakeJiraClient()

	cfg := flakiness.Config{
		Component: "no-auto-quarantine",
		GCS: flakiness.GCSConfig{
			Bucket:      "bucket",
			JobPrefixes: []string{"logs/job/"},
		},
		Analysis: flakiness.AnalysisConfig{
			Threshold:  0.2,
			WindowDays: 30,
			MinRuns:    3,
		},
		Quarantine: flakiness.QuarantineConfig{
			AutoQuarantine: false,
		},
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	result, err := flakiness.Run(context.Background(), cfg,
		flakiness.WithBucketClient(client),
		flakiness.WithJiraClient(jiraFiler),
		flakiness.WithClock(func() time.Time { return now }),
	)
	require.NoError(t, err)

	assert.NotEmpty(t, result.AllEntries, "analysis should still produce entries")
	assert.Empty(t, result.QuarantinedTests, "no tests should be quarantined when auto_quarantine is false")
	assert.Empty(t, result.NewlyQuarantined, "no new quarantine when disabled")
	assert.Empty(t, jiraFiler.created, "no Jira tickets filed when auto_quarantine is false")
}

func TestRun_TimeoutBudgetAnalysis(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	for i := range 10 {
		buildID := strconv.Itoa(800 + i)
		path := fmt.Sprintf("logs/budget-job/%s/artifacts/junit.xml", buildID)
		xml := fmt.Appendf(nil, `<testsuite name="e2e-predictor" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestSlowPredictor" time="250.0"/>
  <testcase name="TestFastPredictor" time="10.0"/>
</testsuite>`, i+1)
		client.addObject("bucket", path, xml)
	}

	cfg := flakiness.Config{
		Component: "budget-test",
		GCS: flakiness.GCSConfig{
			Bucket:      "bucket",
			JobPrefixes: []string{"logs/budget-job/"},
		},
		Analysis: flakiness.AnalysisConfig{
			Threshold:  0.2,
			WindowDays: 30,
			MinRuns:    3,
		},
		Quarantine: flakiness.QuarantineConfig{
			AutoQuarantine: true,
		},
		TimeoutBudget: flakiness.TimeoutBudgetConfig{
			PipelineTimeout:  30 * time.Minute,
			SuiteTimeouts:    map[string]string{"e2e-predictor": "10m"},
			TestTimeouts:     map[string]string{"TestSlowPredictor": "5m"},
			WarningThreshold: 0.8,
		},
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	result, err := flakiness.Run(context.Background(), cfg,
		flakiness.WithBucketClient(client),
		flakiness.WithClock(func() time.Time { return now }),
	)
	require.NoError(t, err)

	require.NotNil(t, result.Budget, "budget report should be populated when timeout_budget is configured")
	assert.NotNil(t, result.Budget.Pipeline, "pipeline budget should be computed")
	assert.Positive(t, result.Budget.Pipeline.Utilisation)
	assert.NotEmpty(t, result.Budget.NearTimeout, "TestSlowPredictor (250s) exceeds 80%% of 5m timeout")
}

func TestRun_NoBudgetWhenUnconfigured(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	for i := range 5 {
		buildID := strconv.Itoa(900 + i)
		path := fmt.Sprintf("logs/no-budget/%s/artifacts/junit.xml", buildID)
		xml := fmt.Appendf(nil, `<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestNormal" time="1.0"/>
</testsuite>`, i+1)
		client.addObject("bucket", path, xml)
	}

	cfg := flakiness.Config{
		Component: "no-budget",
		GCS: flakiness.GCSConfig{
			Bucket:      "bucket",
			JobPrefixes: []string{"logs/no-budget/"},
		},
		Analysis: flakiness.AnalysisConfig{
			Threshold:  0.2,
			WindowDays: 30,
			MinRuns:    3,
		},
		Quarantine: flakiness.QuarantineConfig{
			AutoQuarantine: true,
		},
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	result, err := flakiness.Run(context.Background(), cfg,
		flakiness.WithBucketClient(client),
		flakiness.WithClock(func() time.Time { return now }),
	)
	require.NoError(t, err)

	assert.Nil(t, result.Budget, "budget should be nil when timeout_budget is not configured")
}

func TestRun_TokenExpiryWarning(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	for i := range 5 {
		buildID := strconv.Itoa(1000 + i)
		path := fmt.Sprintf("logs/token-test/%s/artifacts/junit.xml", buildID)
		xml := fmt.Appendf(nil, `<testsuite name="e2e" timestamp="2026-06-%02dT12:00:00Z">
  <testcase name="TestOK" time="1.0"/>
</testsuite>`, i+1)
		client.addObject("bucket", path, xml)
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	t.Run("warning when token expires soon", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.Config{
			Component: "token-warn",
			GCS: flakiness.GCSConfig{
				Bucket:      "bucket",
				JobPrefixes: []string{"logs/token-test/"},
			},
			Analysis: flakiness.AnalysisConfig{
				Threshold:  0.2,
				WindowDays: 30,
				MinRuns:    3,
			},
			Quarantine: flakiness.QuarantineConfig{AutoQuarantine: true},
			Jira: flakiness.JiraConfig{
				TokenExpiresAt:         "2026-06-20T00:00:00Z",
				TokenExpiryWarningDays: 14,
			},
		}

		result, err := flakiness.Run(context.Background(), cfg,
			flakiness.WithBucketClient(client),
			flakiness.WithClock(func() time.Time { return now }),
		)
		require.NoError(t, err)
		assert.NotEmpty(t, result.TokenExpiryWarning, "should warn when token expires within warning window")
		assert.Contains(t, result.TokenExpiryWarning, "2026-06-20")
	})

	t.Run("no warning when token is not expiring", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.Config{
			Component: "token-ok",
			GCS: flakiness.GCSConfig{
				Bucket:      "bucket",
				JobPrefixes: []string{"logs/token-test/"},
			},
			Analysis: flakiness.AnalysisConfig{
				Threshold:  0.2,
				WindowDays: 30,
				MinRuns:    3,
			},
			Quarantine: flakiness.QuarantineConfig{AutoQuarantine: true},
			Jira: flakiness.JiraConfig{
				TokenExpiresAt:         "2027-12-31T00:00:00Z",
				TokenExpiryWarningDays: 14,
			},
		}

		result, err := flakiness.Run(context.Background(), cfg,
			flakiness.WithBucketClient(client),
			flakiness.WithClock(func() time.Time { return now }),
		)
		require.NoError(t, err)
		assert.Empty(t, result.TokenExpiryWarning, "should not warn when token is far from expiry")
	})

	t.Run("no warning when token_expires_at is empty", func(t *testing.T) {
		t.Parallel()

		cfg := flakiness.Config{
			Component: "token-none",
			GCS: flakiness.GCSConfig{
				Bucket:      "bucket",
				JobPrefixes: []string{"logs/token-test/"},
			},
			Analysis: flakiness.AnalysisConfig{
				Threshold:  0.2,
				WindowDays: 30,
				MinRuns:    3,
			},
			Quarantine: flakiness.QuarantineConfig{AutoQuarantine: true},
		}

		result, err := flakiness.Run(context.Background(), cfg,
			flakiness.WithBucketClient(client),
			flakiness.WithClock(func() time.Time { return now }),
		)
		require.NoError(t, err)
		assert.Empty(t, result.TokenExpiryWarning, "should not warn when token_expires_at is not set")
	})
}
