package flakiness_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

const sampleJUnitXML = `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="e2e" tests="3" failures="1" errors="0" time="17.2" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestModelServing/inference" classname="e2e" time="5.2"/>
  <testcase name="TestDashboard/login" classname="e2e" time="10.0">
    <failure message="expected 200 got 500" type="AssertionError"/>
    <properties>
      <property name="failure.category" value="product"/>
      <property name="failure.subcategory" value="api_error"/>
      <property name="failure.confidence" value="0.95"/>
    </properties>
  </testcase>
  <testcase name="TestPipelines/gpu" classname="e2e" time="0">
    <skipped message="GPU not available"/>
  </testcase>
</testsuite>`

func TestScraper_Scrape(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("ci-bucket",
		"logs/periodic-ci-main/build-100/artifacts/e2e/junit_results.xml",
		[]byte(sampleJUnitXML))

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.Scrape(context.Background(), appender,
		"ci-bucket", "logs/periodic-ci-main/build-100/")
	require.NoError(t, err)

	assert.Equal(t, 1, result.Artifacts)
	assert.Equal(t, 3, result.TestsRecorded)
	assert.Empty(t, result.Errors)

	// 3 tests × 2 metrics each = 6 append calls
	assert.Len(t, appender.calls, 6)
}

func TestScraper_Scrape_ExtractsJobAndBuild(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket",
		"logs/nightly-gpu/42/artifacts/junit_e2e.xml",
		[]byte(`<testsuite name="gpu" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestGPU" time="1.0"/>
</testsuite>`))

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.Scrape(context.Background(), appender,
		"bucket", "logs/nightly-gpu/42/")
	require.NoError(t, err)
	assert.Equal(t, 1, result.TestsRecorded)

	execCall := appender.calls[0]
	assert.Equal(t, "nightly-gpu", execCall.Labels.Get(flakiness.LabelJob))
	assert.Equal(t, "42", execCall.Labels.Get(flakiness.LabelBuildID))
}

func TestScraper_Scrape_FailureClassificationLabels(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket",
		"logs/ci/1/artifacts/junit.xml",
		[]byte(`<testsuite name="e2e" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestFlaky" time="5.0">
    <failure message="timeout"/>
    <properties>
      <property name="failure.category" value="infrastructure"/>
      <property name="failure.subcategory" value="cluster_timeout"/>
      <property name="failure.confidence" value="0.92"/>
    </properties>
  </testcase>
</testsuite>`))

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.Scrape(context.Background(), appender,
		"bucket", "logs/ci/1/")
	require.NoError(t, err)
	assert.Equal(t, 1, result.TestsRecorded)

	execCall := appender.calls[0]
	assert.Equal(t, "infrastructure",
		execCall.Labels.Get(flakiness.LabelFailureCategory))
	assert.Equal(t, "cluster_timeout",
		execCall.Labels.Get(flakiness.LabelFailureSubcategory))
	assert.Equal(t, "high",
		execCall.Labels.Get(flakiness.LabelFailureConfidence))
}

func TestScraper_Scrape_MultipleArtifacts(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket",
		"logs/ci/1/artifacts/junit_unit.xml",
		[]byte(`<testsuite name="unit" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestA" time="1.0"/>
</testsuite>`))
	client.addObject("bucket",
		"logs/ci/1/artifacts/junit_e2e.xml",
		[]byte(`<testsuite name="e2e" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestB" time="2.0"/>
</testsuite>`))

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.Scrape(context.Background(), appender,
		"bucket", "logs/ci/1/")
	require.NoError(t, err)

	assert.Equal(t, 2, result.Artifacts)
	assert.Equal(t, 2, result.TestsRecorded)
	assert.Empty(t, result.Errors)
}

func TestScraper_Scrape_MalformedXML(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket",
		"logs/ci/1/artifacts/junit_good.xml",
		[]byte(`<testsuite name="ok" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestOk" time="1.0"/>
</testsuite>`))
	client.addObject("bucket",
		"logs/ci/1/artifacts/junit_bad.xml",
		[]byte(`this is not valid xml`))

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.Scrape(context.Background(), appender,
		"bucket", "logs/ci/1/")
	require.NoError(t, err)

	assert.Equal(t, 2, result.Artifacts)
	assert.Equal(t, 1, result.TestsRecorded)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "junit_bad.xml")
}

func TestScraper_Scrape_NoArtifacts(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.Scrape(context.Background(), appender,
		"bucket", "logs/empty/")
	require.NoError(t, err)

	assert.Equal(t, 0, result.Artifacts)
	assert.Equal(t, 0, result.TestsRecorded)
	assert.Empty(t, result.Errors)
}

func TestScraper_Scrape_FiltersNonJUnit(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket",
		"logs/ci/1/artifacts/junit_results.xml",
		[]byte(`<testsuite name="e2e" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestA" time="1.0"/>
</testsuite>`))
	client.addObject("bucket",
		"logs/ci/1/artifacts/build-log.txt",
		[]byte("not junit"))
	client.addObject("bucket",
		"logs/ci/1/artifacts/config.xml",
		[]byte("<config/>"))

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.Scrape(context.Background(), appender,
		"bucket", "logs/ci/1/")
	require.NoError(t, err)

	assert.Equal(t, 1, result.Artifacts)
	assert.Equal(t, 1, result.TestsRecorded)
}

func TestScraper_Scrape_ListError(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.listErr = assert.AnError

	scraper := flakiness.NewScraper(client,
		flakiness.WithMaxRetries(2),
		flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	_, err := scraper.Scrape(context.Background(), appender,
		"bucket", "prefix/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discovering artifacts")
}

func TestScraper_Scrape_ReadError(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket", "logs/ci/1/artifacts/junit.xml", nil)
	client.readErr = assert.AnError

	scraper := flakiness.NewScraper(client,
		flakiness.WithMaxRetries(1),
		flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.Scrape(context.Background(), appender,
		"bucket", "logs/ci/1/")
	require.NoError(t, err)

	assert.Equal(t, 1, result.Artifacts)
	assert.Equal(t, 0, result.TestsRecorded)
	assert.Len(t, result.Errors, 1)
}

func TestScraper_Scrape_ContextCancelled(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.listErr = assert.AnError

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scraper := flakiness.NewScraper(client,
		flakiness.WithMaxRetries(5),
		flakiness.WithRetryDelay(time.Second))
	appender := &fakeAppender{}

	_, err := scraper.Scrape(ctx, appender, "bucket", "prefix/")
	require.Error(t, err)
}

func TestScraper_Scrape_JUnitInSubdirectory(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket",
		"logs/ci/1/artifacts/e2e/openshift-e2e-test/artifacts/junit/junit_e2e_20260624.xml",
		[]byte(`<testsuite name="e2e" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestDeep" time="3.0"/>
</testsuite>`))

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.Scrape(context.Background(), appender,
		"bucket", "logs/ci/1/")
	require.NoError(t, err)

	assert.Equal(t, 1, result.Artifacts)
	assert.Equal(t, 1, result.TestsRecorded)
}

func TestScraper_Scrape_PRLogsPath(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket",
		"pr-logs/pull/openshift_release/9999/pull-ci-openshift-release-master-e2e/1234567890/artifacts/junit_results.xml",
		[]byte(`<testsuite name="e2e" timestamp="2026-06-24T12:00:00Z">
  <testcase name="TestPR" time="3.0"/>
</testsuite>`))

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))
	appender := &fakeAppender{}

	result, err := scraper.Scrape(context.Background(), appender,
		"bucket", "pr-logs/pull/openshift_release/9999/")
	require.NoError(t, err)
	assert.Equal(t, 1, result.TestsRecorded)

	execCall := appender.calls[0]
	assert.Equal(t, "pull-ci-openshift-release-master-e2e",
		execCall.Labels.Get(flakiness.LabelJob))
	assert.Equal(t, "1234567890",
		execCall.Labels.Get(flakiness.LabelBuildID))
}

func TestScraper_Scrape_IntegrationWithStore(t *testing.T) {
	t.Parallel()

	client := newFakeBucketClient()
	client.addObject("bucket",
		"logs/periodic-ci/500/artifacts/junit_results.xml",
		[]byte(sampleJUnitXML))

	store, err := flakiness.NewStore()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	ctx := context.Background()
	appender := store.Appender(ctx)

	scraper := flakiness.NewScraper(client, flakiness.WithRetryDelay(0))

	result, err := scraper.Scrape(ctx, appender, "bucket", "logs/periodic-ci/500/")
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
