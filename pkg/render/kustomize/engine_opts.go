package kustomize

import (
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// EngineOptsFn configures the Kustomize Engine at construction time.
type EngineOptsFn func(engine *Engine)

// WithEngineFS overrides the default on-disk filesystem with a custom one
// (e.g. an in-memory filesystem for testing).
func WithEngineFS(value filesys.FileSystem) EngineOptsFn {
	return func(engine *Engine) {
		engine.fs = value
	}
}

// WithEngineRenderOpts bakes per-render options into the engine defaults so
// they apply to every Render call.
func WithEngineRenderOpts(values ...RenderOptsFn) EngineOptsFn {
	return func(engine *Engine) {
		for _, fn := range values {
			fn(&engine.renderOpts)
		}
	}
}
