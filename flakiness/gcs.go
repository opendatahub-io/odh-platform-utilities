package flakiness

import (
	"context"
	"errors"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// BucketClient abstracts GCS bucket operations for testability.
type BucketClient interface {
	ListObjects(ctx context.Context, bucket, prefix string) ([]string, error)
	ReadObject(ctx context.Context, bucket, name string) ([]byte, error)
}

// GCSClient implements [BucketClient] via Google Cloud Storage.
type GCSClient struct {
	client *storage.Client
}

// GCSOption configures a [GCSClient].
type GCSOption func(*gcsConfig)

type gcsConfig struct {
	clientOpts []option.ClientOption
}

// WithAnonymous enables unauthenticated access for public buckets.
func WithAnonymous() GCSOption {
	return func(c *gcsConfig) {
		c.clientOpts = append(c.clientOpts, option.WithoutAuthentication())
	}
}

// WithGCSClientOption passes a raw [option.ClientOption] through.
func WithGCSClientOption(opt option.ClientOption) GCSOption {
	return func(c *gcsConfig) {
		c.clientOpts = append(c.clientOpts, opt)
	}
}

// NewGCSClient creates a [GCSClient]. Uses Application Default Credentials
// unless [WithAnonymous] is provided.
func NewGCSClient(ctx context.Context, opts ...GCSOption) (*GCSClient, error) {
	cfg := &gcsConfig{}
	for _, o := range opts {
		o(cfg)
	}

	client, err := storage.NewClient(ctx, cfg.clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating GCS client: %w", err)
	}

	return &GCSClient{client: client}, nil
}

// ListObjects implements [BucketClient].
func (g *GCSClient) ListObjects(ctx context.Context, bucket, prefix string) ([]string, error) {
	it := g.client.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: prefix})

	var names []string

	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}

		if err != nil {
			return names, fmt.Errorf("listing objects in gs://%s/%s: %w", bucket, prefix, err)
		}

		names = append(names, attrs.Name)
	}

	return names, nil
}

const maxObjectSize = 64 << 20 // 64 MiB

// ReadObject implements [BucketClient].
func (g *GCSClient) ReadObject(ctx context.Context, bucket, name string) ([]byte, error) {
	reader, err := g.client.Bucket(bucket).Object(name).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening gs://%s/%s: %w", bucket, name, err)
	}

	defer func() {
		_ = reader.Close()
	}()

	limited := io.LimitReader(reader, maxObjectSize+1)

	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading gs://%s/%s: %w", bucket, name, err)
	}

	if len(data) > maxObjectSize {
		return nil, fmt.Errorf("object gs://%s/%s exceeds max size %d bytes", bucket, name, maxObjectSize)
	}

	return data, nil
}

// Close shuts down the underlying GCS client.
func (g *GCSClient) Close() error {
	if err := g.client.Close(); err != nil {
		return fmt.Errorf("closing GCS client: %w", err)
	}

	return nil
}
