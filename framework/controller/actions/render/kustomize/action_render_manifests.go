package kustomize

import (
	"context"
	"path"

	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/resourcecacher"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
)

const rendererEngine = "kustomize"

type Action struct {
	cacher      resourcecacher.ResourceCacher
	cache       bool
	namespaceFn actions.Getter[string]

	keOpts []kustomize.EngineOptsFn
	ke     *kustomize.Engine
}

type ActionOpts func(*Action)

func WithEngineFS(value filesys.FileSystem) ActionOpts {
	return func(a *Action) {
		a.keOpts = append(a.keOpts, kustomize.WithEngineFS(value))
	}
}

func WithLabel(name string, value string) ActionOpts {
	return func(a *Action) {
		a.keOpts = append(a.keOpts, kustomize.WithEngineRenderOpts(kustomize.WithLabel(name, value)))
	}
}

func WithLabels(values map[string]string) ActionOpts {
	return func(a *Action) {
		a.keOpts = append(a.keOpts, kustomize.WithEngineRenderOpts(kustomize.WithLabels(values)))
	}
}

func WithAnnotation(name string, value string) ActionOpts {
	return func(a *Action) {
		a.keOpts = append(a.keOpts, kustomize.WithEngineRenderOpts(kustomize.WithAnnotation(name, value)))
	}
}

func WithAnnotations(values map[string]string) ActionOpts {
	return func(a *Action) {
		a.keOpts = append(a.keOpts, kustomize.WithEngineRenderOpts(kustomize.WithAnnotations(values)))
	}
}

func WithManifestsOptions(values ...kustomize.EngineOptsFn) ActionOpts {
	return func(a *Action) {
		a.keOpts = append(a.keOpts, values...)
	}
}

func WithNamespaceFn(fn actions.Getter[string]) ActionOpts {
	return func(a *Action) {
		if fn != nil {
			a.namespaceFn = fn
		}
	}
}

func WithCache(enabled bool) ActionOpts {
	return func(a *Action) {
		a.cache = enabled
	}
}

func (a *Action) run(ctx context.Context, rr *types.ReconciliationRequest) error {
	return a.cacher.Render(ctx, rr, a.render)
}

func (a *Action) render(ctx context.Context, rr *types.ReconciliationRequest) (resources.UnstructuredList, error) {
	result := make(resources.UnstructuredList, 0)

	var ns string
	if a.namespaceFn != nil {
		var err error

		ns, err = a.namespaceFn(ctx, rr)
		if err != nil {
			return nil, err
		}
	}

	for i := range rr.Manifests {
		renderNS := ns
		if rr.Manifests[i].Namespace != "" {
			renderNS = rr.Manifests[i].Namespace
		}

		manifestPath := path.Join(rr.Manifests[i].Path, rr.Manifests[i].ContextDir, rr.Manifests[i].SourcePath)

		renderedResources, err := a.ke.Render(
			manifestPath,
			kustomize.WithNamespace(renderNS),
		)

		if err != nil {
			return nil, err
		}

		result = append(result, renderedResources...)
	}

	return result, nil
}

func NewAction(opts ...ActionOpts) actions.Fn {
	action := Action{
		cacher: resourcecacher.NewResourceCacher(rendererEngine),
		cache:  true,
	}

	for _, opt := range opts {
		opt(&action)
	}

	if action.cache {
		action.cacher.SetKeyFn(types.Hash)
	}

	action.ke = kustomize.NewEngine(action.keOpts...)

	return action.run
}
