package template

import (
	"context"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/render/cacher"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

const rendererEngine = "template"

// action wraps the standalone Render function with caching and the
// ReconciliationRequest lifecycle.
type action struct {
	templateData map[string]any
	cacher       cacher.ResourceCacher
	opts         actionOpts
}

func (a *action) run(ctx context.Context, rr *render.ReconciliationRequest) error {
	data, err := buildData(ctx, &a.opts, rr.Instance)
	if err != nil {
		return err
	}

	a.templateData = data

	defer func() {
		a.templateData = nil
	}()

	return a.cacher.Render(ctx, rr, a.render)
}

func (a *action) render(ctx context.Context, rr *render.ReconciliationRequest) (resources.UnstructuredList, error) {
	if len(rr.Templates) == 0 {
		return nil, nil
	}

	data := a.templateData
	if data == nil {
		var err error

		data, err = buildData(ctx, &a.opts, rr.Instance)
		if err != nil {
			return nil, err
		}
	}

	sources := make([]TemplateSource, len(rr.Templates))
	for i, t := range rr.Templates {
		sources[i] = TemplateSource{
			FS:          t.FS,
			Path:        t.Path,
			Labels:      t.Labels,
			Annotations: t.Annotations,
		}
	}

	return Render(ctx, buildScheme(rr.Client), sources, data, a.opts.renderOpts...)
}

// NewAction creates an action-pipeline Go template rendering function. It
// reads Templates from the ReconciliationRequest, renders them with the
// configured data, and writes results back to rr.Resources.
func NewAction(opts ...ActionOption) render.Fn {
	o := actionOpts{
		data: make(map[string]any),
	}

	for _, opt := range opts {
		opt(&o)
	}

	a := action{
		cacher: cacher.NewResourceCacher(rendererEngine),
		opts:   o,
	}

	if !o.cacheDisabled {
		a.cacher.SetKeyFn(a.cacheKey)
	}

	return a.run
}
