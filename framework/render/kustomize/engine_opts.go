package kustomize

import (
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

type EngineOptsFn func(engine *Engine)

func WithEngineFS(value filesys.FileSystem) EngineOptsFn {
	return func(engine *Engine) {
		engine.fs = value
	}
}

func WithEngineRenderOpts(values ...RenderOptsFn) EngineOptsFn {
	return func(engine *Engine) {
		for _, fn := range values {
			fn(&engine.renderOpts)
		}
	}
}

// WithLoadRestrictions controls kustomize's file-loading security policy.
// The default (LoadRestrictionsRootOnly) prevents loading files outside the
// kustomization root. Use LoadRestrictionsNone to allow references such as
// ../shared/resource.yaml that traverse into sibling or parent directories.
func WithLoadRestrictions(r types.LoadRestrictions) EngineOptsFn {
	return func(engine *Engine) {
		engine.krustyOpts.LoadRestrictions = r
	}
}
