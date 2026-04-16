package cacher

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var errEmptyHash = errors.New("calculated empty hash")

// Cacher is a generic caching layer for rendering functions. It wraps a
// renderer and skips re-rendering when the cache key has not changed.
type Cacher[T any] struct {
	cachedResources T
	cachingKey      []byte
}

// Zero returns the zero value of type T.
func Zero[T any]() T {
	return *new(T)
}

// InvalidateCache clears the stored cache key, forcing the next Render call
// to re-execute the render function.
func (s *Cacher[T]) InvalidateCache() {
	s.cachingKey = nil
}

func (s *Cacher[T]) reRender(ctx context.Context, cachingKey []byte,
	r func(ctx context.Context) (T, error),
) (T, bool, error) {
	log := logf.FromContext(ctx)

	log.V(4).Info("cache is not valid, rendering resources")

	res, err := r(ctx)
	if err != nil {
		return Zero[T](), false, err
	}

	s.cachingKey = cachingKey
	s.cachedResources = res

	return res, true, nil
}

// Render returns cached resources if the cache key matches, otherwise calls
// the render function r and caches the result. If keyFn is nil, every call
// re-renders (no caching). Returns (result, rendered, error) where rendered
// is true when r was actually called.
func (s *Cacher[T]) Render(ctx context.Context,
	keyFn func() ([]byte, error),
	r func(ctx context.Context) (T, error),
) (T, bool, error) {
	log := logf.FromContext(ctx)

	if keyFn == nil {
		return s.reRender(ctx, nil, r)
	}

	cachingKey, err := keyFn()
	if err != nil {
		return Zero[T](), false, fmt.Errorf("unable to calculate caching key: %w", err)
	}

	if len(cachingKey) == 0 {
		return Zero[T](), false, errEmptyHash
	}

	if bytes.Equal(cachingKey, s.cachingKey) {
		log.V(4).Info("using cached resources")

		return s.cachedResources, false, nil
	}

	return s.reRender(ctx, cachingKey, r)
}
