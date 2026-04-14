package kustomize

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Render is a convenience function that creates a one-shot Engine and renders
// the Kustomize overlay at path. For repeated rendering, create an Engine with
// NewEngine and call Engine.Render directly.
func Render(path string, engineOpts []EngineOptsFn, renderOpts ...RenderOptsFn) ([]unstructured.Unstructured, error) {
	e := NewEngine(engineOpts...)
	return e.Render(path, renderOpts...)
}
