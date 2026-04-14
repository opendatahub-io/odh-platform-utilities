package kustomize

import (
	"context"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/render/cacher"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

const rendererEngine = "kustomize"

// action wraps the Kustomize engine with caching and the
// ReconciliationRequest lifecycle for use in the action pipeline.
type action struct {
	nsFn      func(ctx context.Context) (string, error)
	ke        *Engine
	cacher    cacher.ResourceCacher
	namespace string
}

// ActionOption configures the action-pipeline Kustomize renderer.
type ActionOption func(*action)

// WithActionEngineOpts adds engine-level options (e.g. custom filesystem).
func WithActionEngineOpts(values ...EngineOptsFn) ActionOption {
	return func(a *action) {
		for _, fn := range values {
			fn(a.ke)
		}
	}
}

// WithCache enables or disables caching for the action adapter.
func WithCache(enabled bool) ActionOption {
	return func(a *action) {
		if enabled {
			a.cacher.SetKeyFn(render.Hash)
		} else {
			a.cacher.SetKeyFn(nil)
		}
	}
}

// WithActionNamespace sets a static namespace to inject into all rendered
// resources. This replaces the operator's cluster.ApplicationNamespace() call.
func WithActionNamespace(ns string) ActionOption {
	return func(a *action) {
		a.namespace = ns
	}
}

// WithActionNamespaceFn sets a dynamic namespace resolution function. It is
// called on every render to determine the target namespace.
func WithActionNamespaceFn(fn func(ctx context.Context) (string, error)) ActionOption {
	return func(a *action) {
		a.nsFn = fn
	}
}

func (a *action) run(ctx context.Context, rr *render.ReconciliationRequest) error {
	return a.cacher.Render(ctx, rr, a.render)
}

func (a *action) render(
	ctx context.Context, rr *render.ReconciliationRequest,
) (resources.UnstructuredList, error) {
	result := make(resources.UnstructuredList, 0)

	ns := a.namespace

	if a.nsFn != nil {
		var err error

		ns, err = a.nsFn(ctx)
		if err != nil {
			return nil, err
		}
	}

	for i := range rr.Manifests {
		var opts []RenderOptsFn

		if ns != "" {
			opts = append(opts, WithNamespace(ns))
		}

		renderedResources, err := a.ke.Render(rr.Manifests[i].String(), opts...)
		if err != nil {
			return nil, err
		}

		result = append(result, renderedResources...)
	}

	return result, nil
}

// NewAction creates an action-pipeline Kustomize rendering function. It reads
// Manifests from the ReconciliationRequest, renders them through the Kustomize
// engine, and writes results back to rr.Resources.
func NewAction(engineOpts []EngineOptsFn, actionOpts ...ActionOption) render.Fn {
	a := action{
		cacher: cacher.NewResourceCacher(rendererEngine),
		ke:     NewEngine(engineOpts...),
	}

	a.cacher.SetKeyFn(render.Hash)

	for _, opt := range actionOpts {
		opt(&a)
	}

	return a.run
}
