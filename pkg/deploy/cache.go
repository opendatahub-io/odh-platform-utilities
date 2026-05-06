package deploy

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

// DefaultCacheTTL is the time-to-live for deploy cache entries.
const DefaultCacheTTL = 10 * time.Minute

var (
	errCacheCast    = errors.New("failed to cast object to string")
	errCacheNilArgs = errors.New("cache: original and modified must be non-nil")
)

// Cache stores recently deployed resource fingerprints to avoid redundant
// server-side apply calls when neither the desired manifest nor the live
// resource has changed. Inspired by cluster-api's SSA cache.
type Cache struct {
	s   cache.Store
	ttl time.Duration
}

// CacheOpt configures a Cache.
type CacheOpt func(*Cache)

// WithTTL overrides the default cache entry time-to-live.
func WithTTL(ttl time.Duration) CacheOpt {
	return func(c *Cache) {
		c.ttl = ttl
	}
}

// NewCache creates a TTL-based deploy cache.
func NewCache(opts ...CacheOpt) *Cache {
	c := Cache{
		ttl: DefaultCacheTTL,
	}

	for _, opt := range opts {
		opt(&c)
	}

	c.s = cache.NewTTLStore(
		func(obj any) (string, error) {
			s, ok := obj.(string)
			if !ok {
				return "", errCacheCast
			}

			return s, nil
		},
		c.ttl,
	)

	return &c
}

// Add records a deploy fingerprint. original is the live object returned by the
// server (provides resourceVersion); modified is the desired manifest that was
// applied (provides the content hash).
func (r *Cache) Add(original *unstructured.Unstructured, modified *unstructured.Unstructured) error {
	if original == nil || modified == nil {
		return errCacheNilArgs
	}

	key, err := r.computeCacheKey(original, modified)
	if err != nil {
		return fmt.Errorf("failed to compute cacheKey: %w", err)
	}

	if key == "" {
		return nil
	}

	err = r.s.Add(key)
	if err != nil {
		return fmt.Errorf("failed to add entry to cache: %w", err)
	}

	return nil
}

// Has returns true if the cache already contains a matching fingerprint for
// this (current-live, desired-manifest) pair, meaning deployment can be
// skipped.
func (r *Cache) Has(original *unstructured.Unstructured, modified *unstructured.Unstructured) (bool, error) {
	if original == nil || modified == nil {
		return false, nil
	}

	key, err := r.computeCacheKey(original, modified)
	if err != nil {
		return false, fmt.Errorf("failed to compute cacheKey: %w", err)
	}

	if key == "" {
		return false, nil
	}

	_, exists, err := r.s.GetByKey(key)
	if err != nil {
		return false, fmt.Errorf("failed to lookup cache entry: %w", err)
	}

	return exists, nil
}

// Delete removes the cache entry for the given pair. This is used to
// invalidate stale entries, e.g. when a resource is being deleted.
func (r *Cache) Delete(original *unstructured.Unstructured, modified *unstructured.Unstructured) error {
	if original == nil || modified == nil {
		return nil
	}

	key, err := r.computeCacheKey(original, modified)
	if err != nil {
		return fmt.Errorf("failed to compute cacheKey for deletion: %w", err)
	}

	if key == "" {
		return nil
	}

	return r.s.Delete(key)
}

// Sync triggers a pass over the TTL store to expire old entries.
func (r *Cache) Sync() {
	r.s.List()
}

func (r *Cache) computeCacheKey(
	original *unstructured.Unstructured,
	modified *unstructured.Unstructured,
) (string, error) {
	modifiedObjectHash, err := resources.Hash(modified)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s.%s.%s.%s.%s",
		original.GroupVersionKind().GroupVersion(),
		original.GroupVersionKind().Kind,
		klog.KObj(original),
		original.GetResourceVersion(),
		base64.RawURLEncoding.EncodeToString(modifiedObjectHash),
	), nil
}
