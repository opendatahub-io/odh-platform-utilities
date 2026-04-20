package cacher

import (
	"context"
	"strings"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

// CachingKeyFn computes a cache key from the ReconciliationRequest.
// The returned byte slice MUST NOT be empty by contract.
// ctx is the reconciliation context (deadlines, cancellation, values loading).
type CachingKeyFn func(ctx context.Context, rr *render.ReconciliationRequest) ([]byte, error)

// Renderer is the function signature for rendering resources from a
// ReconciliationRequest.
type Renderer func(ctx context.Context, rr *render.ReconciliationRequest) (resources.UnstructuredList, error)

// ResourceCacher wraps a generic Cacher specialized for UnstructuredList and
// integrates Prometheus metrics and the ReconciliationRequest lifecycle
// (Generated flag, Resources accumulation).
//
// A ResourceCacher instance must not be used concurrently from multiple
// goroutines (matches typical single-threaded reconcile per object).
type ResourceCacher struct { //nolint:govet
	Cacher[resources.UnstructuredList]

	keyFn CachingKeyFn
	name  string
}

// NewResourceCacher creates a new ResourceCacher for the given engine name
// (used as the "engine" label in metrics).
func NewResourceCacher(name string) ResourceCacher {
	return ResourceCacher{name: name}
}

// SetKeyFn installs the caching key function. If set to nil, caching is
// disabled and every Render call will re-execute the render function.
func (s *ResourceCacher) SetKeyFn(key CachingKeyFn) {
	s.keyFn = key
}

// Render executes the render function (possibly from cache), appends results
// to rr.Resources, updates metrics, and sets rr.Generated when resources were
// freshly rendered.
func (s *ResourceCacher) Render(ctx context.Context, rr *render.ReconciliationRequest, r Renderer) error {
	log := logf.FromContext(ctx)

	var keyFn func() ([]byte, error)

	if s.keyFn != nil {
		keyFn = func() ([]byte, error) {
			return s.keyFn(ctx, rr)
		}
	}

	res, acted, err := s.Cacher.Render(ctx, keyFn, func(ctx context.Context) (resources.UnstructuredList, error) {
		return r(ctx, rr)
	})
	if err != nil {
		return err
	}

	resLen := len(res)

	if acted {
		log.V(4).Info("accounted rendered resources", "count", resLen)

		controllerName := strings.ToLower(rr.Instance.GetObjectKind().GroupVersionKind().Kind)

		render.RenderedResourcesTotal.WithLabelValues(controllerName, s.name).Add(float64(resLen))

		rr.Generated = true
	}

	rr.Resources = append(rr.Resources, res.Clone()...)

	log.V(4).Info("added resources to the request", "count", resLen)

	return nil
}
