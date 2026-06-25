package flakiness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
)

var (
	_ storage.Storage = (*Store)(nil)

	// ErrInvalidRetention is returned when retention duration is not positive.
	ErrInvalidRetention = errors.New("retention duration must be positive")

	// ErrInvalidMaxSamples is returned when max samples is not positive.
	ErrInvalidMaxSamples = errors.New("max samples must be positive")

	// ErrInvalidQueryTimeout is returned when query timeout is not positive.
	ErrInvalidQueryTimeout = errors.New("query timeout must be positive")
)

// Store wraps a Prometheus TSDB and PromQL engine into a
// self-contained time-series store for test execution metrics.
type Store struct {
	db     *tsdb.DB
	engine *promql.Engine
	dir    string
	tmpDir bool
}

// StoreOption configures a [Store].
type StoreOption func(*storeConfig)

type storeConfig struct {
	dir               string
	retentionDuration time.Duration
	maxSamples        int
	queryTimeout      time.Duration
}

func defaultStoreConfig() storeConfig {
	return storeConfig{
		retentionDuration: 15 * 24 * time.Hour,
		maxSamples:        50_000_000,
		queryTimeout:      2 * time.Minute,
	}
}

// WithStorageDir sets a persistent storage directory.
// When not set, a temporary directory is used and removed on Close.
func WithStorageDir(dir string) StoreOption {
	return func(c *storeConfig) {
		c.dir = dir
	}
}

// WithRetention sets the data retention duration.
func WithRetention(d time.Duration) StoreOption {
	return func(c *storeConfig) {
		c.retentionDuration = d
	}
}

// WithMaxSamples sets the maximum number of samples a query
// can process.
func WithMaxSamples(n int) StoreOption {
	return func(c *storeConfig) {
		c.maxSamples = n
	}
}

// WithQueryTimeout sets the timeout for PromQL query execution.
func WithQueryTimeout(d time.Duration) StoreOption {
	return func(c *storeConfig) {
		c.queryTimeout = d
	}
}

// NewStore creates a [Store] backed by Prometheus TSDB with an
// embedded PromQL engine. When no [WithStorageDir] option is
// provided, a temporary directory is used and removed on Close.
func NewStore(opts ...StoreOption) (*Store, error) {
	cfg := defaultStoreConfig()
	for _, o := range opts {
		o(&cfg)
	}

	if cfg.retentionDuration <= 0 {
		return nil, fmt.Errorf("%w: got %s", ErrInvalidRetention, cfg.retentionDuration)
	}

	if cfg.maxSamples <= 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidMaxSamples, cfg.maxSamples)
	}

	if cfg.queryTimeout <= 0 {
		return nil, fmt.Errorf("%w: got %s", ErrInvalidQueryTimeout, cfg.queryTimeout)
	}

	s := &Store{}

	dir := cfg.dir
	if dir == "" {
		tmpDir, mkErr := os.MkdirTemp("", "flakiness-tsdb-*")
		if mkErr != nil {
			return nil, fmt.Errorf("creating temp directory: %w", mkErr)
		}

		dir = tmpDir
		s.tmpDir = true
	}

	s.dir = dir

	tsdbOpts := tsdb.DefaultOptions()
	tsdbOpts.RetentionDuration = int64(
		cfg.retentionDuration / time.Millisecond,
	)

	db, err := tsdb.Open(dir, nil, nil, tsdbOpts, tsdb.NewDBStats())
	if err != nil {
		if s.tmpDir {
			_ = os.RemoveAll(dir)
		}

		return nil, fmt.Errorf("opening tsdb: %w", err)
	}

	s.db = db

	s.engine = promql.NewEngine(promql.EngineOpts{
		MaxSamples:           cfg.maxSamples,
		Timeout:              cfg.queryTimeout,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	})

	return s, nil
}

// Appender returns a [storage.Appender] for writing metric samples.
// The caller must call Commit or Rollback on the returned Appender.
func (s *Store) Appender(ctx context.Context) storage.Appender {
	return s.db.Appender(ctx)
}

// AppenderV2 returns a [storage.AppenderV2] for writing metric
// samples using the v2 append API.
func (s *Store) AppenderV2(
	ctx context.Context,
) storage.AppenderV2 {
	return s.db.AppenderV2(ctx)
}

// Querier returns a [storage.Querier] for the given time range
// (milliseconds).
func (s *Store) Querier(
	mint, maxt int64,
) (storage.Querier, error) {
	return s.db.Querier(mint, maxt)
}

// ChunkQuerier returns a [storage.ChunkQuerier] for the given time
// range (milliseconds).
func (s *Store) ChunkQuerier(
	mint, maxt int64,
) (storage.ChunkQuerier, error) {
	return s.db.ChunkQuerier(mint, maxt)
}

// StartTime returns the oldest timestamp stored in the TSDB.
func (s *Store) StartTime() (int64, error) {
	return s.db.StartTime()
}

// InstantQuery executes a PromQL instant query at the given
// timestamp.
func (s *Store) InstantQuery(
	ctx context.Context, query string, ts time.Time,
) (*promql.Result, error) {
	qry, err := s.engine.NewInstantQuery(
		ctx, s.db, nil, query, ts,
	)
	if err != nil {
		return nil, fmt.Errorf("creating instant query: %w", err)
	}

	defer qry.Close()

	res := qry.Exec(ctx)
	if res.Err != nil {
		return nil, fmt.Errorf(
			"executing instant query: %w", res.Err,
		)
	}

	return res, nil
}

// RangeQuery executes a PromQL range query over the given interval.
func (s *Store) RangeQuery(
	ctx context.Context,
	query string,
	start, end time.Time,
	step time.Duration,
) (*promql.Result, error) {
	qry, err := s.engine.NewRangeQuery(
		ctx, s.db, nil, query, start, end, step,
	)
	if err != nil {
		return nil, fmt.Errorf("creating range query: %w", err)
	}

	defer qry.Close()

	res := qry.Exec(ctx)
	if res.Err != nil {
		return nil, fmt.Errorf(
			"executing range query: %w", res.Err,
		)
	}

	return res, nil
}

// Close shuts down the TSDB. If a temporary directory was created,
// it is removed.
func (s *Store) Close() error {
	err := s.db.Close()

	if s.tmpDir {
		removeErr := os.RemoveAll(s.dir)
		if removeErr != nil {
			err = errors.Join(err, fmt.Errorf(
				"removing temp directory: %w", removeErr,
			))
		}
	}

	return err
}
