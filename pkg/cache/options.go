package cache

import (
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

// Option configures the cache.Options returned by NewOptions.
type Option func(*cache.Options)

// WithStrictInformers sets ReaderFailOnMissingInformer: cached reads of a type
// without an informer return cache.ErrResourceNotCached instead of starting a
// cluster-wide informer. Recommended for envtest and development; see
// docs/module-operator-scenarios.md before enabling it in production.
func WithStrictInformers() Option {
	return func(o *cache.Options) {
		o.ReaderFailOnMissingInformer = true
	}
}

// NewOptions returns cache.Options with StripUnusedFields as DefaultTransform.
func NewOptions(opts ...Option) cache.Options {
	o := cache.Options{
		DefaultTransform: StripUnusedFields(),
	}

	for _, opt := range opts {
		opt(&o)
	}

	return o
}
