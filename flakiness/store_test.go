package flakiness_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

func TestNewStore_TempDir(t *testing.T) {
	t.Parallel()

	store, err := flakiness.NewStore()
	require.NoError(t, err)
	require.NoError(t, store.Close())
}

func TestNewStore_PersistentDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	store, err := flakiness.NewStore(
		flakiness.WithStorageDir(dir),
	)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	_, err = os.Stat(dir)
	assert.NoError(t, err,
		"persistent directory should not be removed on close",
	)
}

func TestNewStore_TempDirRemovedOnClose(t *testing.T) {
	t.Parallel()

	store, err := flakiness.NewStore()
	require.NoError(t, err)

	app := store.Appender(context.Background())
	require.NoError(t, app.Rollback())
	require.NoError(t, store.Close())
}

func TestNewStore_InvalidDir(t *testing.T) {
	t.Parallel()

	_, err := flakiness.NewStore(
		flakiness.WithStorageDir("/dev/null/invalid"),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening tsdb")
}

func TestNewStore_InvalidRetention(t *testing.T) {
	t.Parallel()

	_, err := flakiness.NewStore(flakiness.WithRetention(0))
	require.Error(t, err)
	require.ErrorIs(t, err, flakiness.ErrInvalidRetention)

	_, err = flakiness.NewStore(flakiness.WithRetention(-time.Hour))
	require.Error(t, err)
	require.ErrorIs(t, err, flakiness.ErrInvalidRetention)
}

func TestNewStore_InvalidMaxSamples(t *testing.T) {
	t.Parallel()

	_, err := flakiness.NewStore(flakiness.WithMaxSamples(0))
	require.Error(t, err)
	require.ErrorIs(t, err, flakiness.ErrInvalidMaxSamples)

	_, err = flakiness.NewStore(flakiness.WithMaxSamples(-1))
	require.Error(t, err)
	require.ErrorIs(t, err, flakiness.ErrInvalidMaxSamples)
}

func TestNewStore_InvalidQueryTimeout(t *testing.T) {
	t.Parallel()

	_, err := flakiness.NewStore(flakiness.WithQueryTimeout(0))
	require.Error(t, err)
	require.ErrorIs(t, err, flakiness.ErrInvalidQueryTimeout)

	_, err = flakiness.NewStore(flakiness.WithQueryTimeout(-time.Second))
	require.Error(t, err)
	require.ErrorIs(t, err, flakiness.ErrInvalidQueryTimeout)
}

func TestStore_IngestQueryRoundTrip(t *testing.T) { //nolint:funlen // Subtests with shared ingest setup.
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	ingestTestData(t, store, ctx, ts)

	t.Run("counts all executions", func(t *testing.T) {
		t.Parallel()

		queryTime := ts.Add(2 * time.Minute)

		res, err := store.InstantQuery(
			ctx,
			`count(`+flakiness.MetricTestExecutionTotal+`)`,
			queryTime,
		)
		require.NoError(t, err)

		vec, err := res.Vector()
		require.NoError(t, err)
		require.Len(t, vec, 1)
		assert.InDelta(t, 3.0, float64(vec[0].F), 0.001)
	})

	t.Run("filters by test name", func(t *testing.T) {
		t.Parallel()

		queryTime := ts.Add(2 * time.Minute)
		q := flakiness.MetricTestExecutionTotal +
			`{` + flakiness.LabelTestName +
			`="TestModelServing/basic_inference"}`

		res, err := store.InstantQuery(
			ctx, `count(`+q+`)`, queryTime,
		)
		require.NoError(t, err)

		vec, err := res.Vector()
		require.NoError(t, err)
		require.Len(t, vec, 1)
		assert.InDelta(t, 2.0, float64(vec[0].F), 0.001)
	})

	t.Run("filters by result outcome", func(t *testing.T) {
		t.Parallel()

		queryTime := ts.Add(2 * time.Minute)
		q := flakiness.MetricTestExecutionTotal +
			`{` + flakiness.LabelResult + `="fail"}`

		res, err := store.InstantQuery(
			ctx, `count(`+q+`)`, queryTime,
		)
		require.NoError(t, err)

		vec, err := res.Vector()
		require.NoError(t, err)
		require.Len(t, vec, 1)
		assert.InDelta(t, 1.0, float64(vec[0].F), 0.001)
	})

	t.Run("duration metric round-trips", func(t *testing.T) {
		t.Parallel()

		queryTime := ts.Add(2 * time.Minute)
		q := flakiness.MetricTestDurationSeconds +
			`{` + flakiness.LabelTestName +
			`="TestDashboard/login"}`

		res, err := store.InstantQuery(ctx, q, queryTime)
		require.NoError(t, err)

		vec, err := res.Vector()
		require.NoError(t, err)
		require.Len(t, vec, 1)
		assert.InDelta(t, 2.0, float64(vec[0].F), 0.001)
	})

	t.Run("range query returns series", func(t *testing.T) {
		t.Parallel()

		q := flakiness.MetricTestExecutionTotal +
			`{` + flakiness.LabelTestName +
			`="TestModelServing/basic_inference"}`

		res, err := store.RangeQuery(
			ctx, q,
			ts.Add(-time.Minute), ts.Add(2*time.Minute),
			time.Minute,
		)
		require.NoError(t, err)

		mat, err := res.Matrix()
		require.NoError(t, err)
		assert.NotEmpty(t, mat)
	})
}

func TestStore_CustomRetention(t *testing.T) {
	t.Parallel()

	store, err := flakiness.NewStore(
		flakiness.WithRetention(24*time.Hour),
		flakiness.WithMaxSamples(1000),
		flakiness.WithQueryTimeout(30*time.Second),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	ctx := context.Background()
	ts := time.Now()

	app := store.Appender(ctx)
	require.NoError(t, flakiness.RecordTestResult(app, flakiness.TestResult{
		Name:      "TestRetention",
		Suite:     "e2e",
		Job:       "ci",
		BuildID:   "b1",
		Result:    flakiness.OutcomePass,
		Duration:  time.Second,
		Timestamp: ts,
	}))

	require.NoError(t, app.Commit())

	res, err := store.InstantQuery(
		ctx, flakiness.MetricTestExecutionTotal, ts.Add(time.Minute),
	)
	require.NoError(t, err)

	vec, err := res.Vector()
	require.NoError(t, err)
	assert.Len(t, vec, 1)
}

func TestStore_Querier(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	ts := time.Now()

	app := store.Appender(ctx)
	require.NoError(t, flakiness.RecordTestResult(app, flakiness.TestResult{
		Name:      "TestQuerier",
		Suite:     "unit",
		Job:       "ci",
		BuildID:   "b1",
		Result:    flakiness.OutcomePass,
		Duration:  time.Second,
		Timestamp: ts,
	}))

	require.NoError(t, app.Commit())

	q, err := store.Querier(
		ts.Add(-time.Minute).UnixMilli(),
		ts.Add(time.Minute).UnixMilli(),
	)
	require.NoError(t, err)
	require.NoError(t, q.Close())
}

func TestStore_InstantQuery_InvalidPromQL(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	_, err := store.InstantQuery(
		context.Background(), `{invalid[`, time.Now(),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating instant query")
}

func TestStore_RangeQuery_InvalidPromQL(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	now := time.Now()

	_, err := store.RangeQuery(
		context.Background(), `{invalid[`,
		now.Add(-time.Hour), now, time.Minute,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating range query")
}

func TestStore_InstantQuery_EmptyResult(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	res, err := store.InstantQuery(
		context.Background(), `nonexistent_metric`, time.Now(),
	)
	require.NoError(t, err)

	vec, err := res.Vector()
	require.NoError(t, err)
	assert.Empty(t, vec)
}

func newTestStore(t *testing.T) *flakiness.Store {
	t.Helper()

	store, err := flakiness.NewStore()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	return store
}

func ingestTestData(
	t *testing.T,
	store *flakiness.Store,
	ctx context.Context,
	ts time.Time,
) {
	t.Helper()

	results := []flakiness.TestResult{
		{
			Name:      "TestModelServing/basic_inference",
			Suite:     "e2e",
			Job:       "periodic-ci-main",
			BuildID:   "build-100",
			Result:    flakiness.OutcomePass,
			Duration:  5*time.Second + 200*time.Millisecond,
			Timestamp: ts,
		},
		{
			Name:      "TestModelServing/basic_inference",
			Suite:     "e2e",
			Job:       "periodic-ci-main",
			BuildID:   "build-101",
			Result:    flakiness.OutcomeFail,
			Duration:  10 * time.Second,
			Timestamp: ts.Add(time.Minute),
		},
		{
			Name:      "TestDashboard/login",
			Suite:     "e2e",
			Job:       "periodic-ci-main",
			BuildID:   "build-100",
			Result:    flakiness.OutcomePass,
			Duration:  2 * time.Second,
			Timestamp: ts,
		},
	}

	app := store.Appender(ctx)

	for _, r := range results {
		require.NoError(t, flakiness.RecordTestResult(app, r))
	}

	require.NoError(t, app.Commit())
}
