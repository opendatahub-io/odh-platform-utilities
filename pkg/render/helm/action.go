package helm

import (
	"context"

	helmRenderer "github.com/k8s-manifest-kit/renderer-helm/pkg"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/render/cacher"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

const rendererEngine = "helm"

// action wraps the standalone Render function with caching and the
// ReconciliationRequest lifecycle for use in the action pipeline.
type action struct {
	cacher cacher.ResourceCacher
	opts   []Option
}

// ActionOption configures the action-pipeline Helm renderer.
type ActionOption func(*action)

// WithCache enables or disables caching for the action adapter. When enabled
// (the default), renders are skipped if the ReconciliationRequest hash has not
// changed since the last run.
func WithCache(enabled bool) ActionOption {
	return func(a *action) {
		if enabled {
			a.cacher.SetKeyFn(render.Hash)
		} else {
			a.cacher.SetKeyFn(nil)
		}
	}
}

func (a *action) run(ctx context.Context, rr *render.ReconciliationRequest) error {
	return a.cacher.Render(ctx, rr, a.render)
}

func (a *action) render(ctx context.Context, rr *render.ReconciliationRequest) (resources.UnstructuredList, error) {
	charts := make([]helmRenderer.Source, 0, len(rr.HelmCharts))
	for _, chart := range rr.HelmCharts {
		charts = append(charts, chart.Source)
	}

	return Render(ctx, charts, a.opts...)
}

// NewAction creates an action-pipeline Helm rendering function. It reads
// HelmCharts from the ReconciliationRequest, renders them, and writes the
// results back to rr.Resources. The renderOpts configure label/annotation
// injection and transformers; the actionOpts configure caching behavior.
func NewAction(renderOpts []Option, actionOpts ...ActionOption) render.Fn {
	a := action{
		cacher: cacher.NewResourceCacher(rendererEngine),
		opts:   renderOpts,
	}

	a.cacher.SetKeyFn(render.Hash)

	for _, opt := range actionOpts {
		opt(&a)
	}

	return a.run
}
