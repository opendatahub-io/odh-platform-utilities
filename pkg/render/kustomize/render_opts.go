package kustomize

import (
	"maps"

	"sigs.k8s.io/kustomize/api/resmap"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
)

// FilterFn is a function that transforms a set of Kustomize RNodes.
type FilterFn func(nodes []*kyaml.RNode) ([]*kyaml.RNode, error)

type renderOpts struct {
	kustomizationFileName    string
	kustomizationFileOverlay string
	ns                       string
	labels                   map[string]string
	annotations              map[string]string
	plugins                  []resmap.Transformer
}

// RenderOptsFn configures a single Engine.Render call.
type RenderOptsFn func(*renderOpts)

// WithKustomizationFileName overrides the expected kustomization file name.
func WithKustomizationFileName(value string) RenderOptsFn {
	return func(opts *renderOpts) {
		opts.kustomizationFileName = value
	}
}

// WithKustomizationOverlayPath overrides the default overlay subdirectory.
func WithKustomizationOverlayPath(value string) RenderOptsFn {
	return func(opts *renderOpts) {
		opts.kustomizationFileOverlay = value
	}
}

// WithNamespace sets the namespace to inject into all rendered resources.
func WithNamespace(value string) RenderOptsFn {
	return func(opts *renderOpts) {
		opts.ns = value
	}
}

// WithLabel adds a label to all rendered resources.
func WithLabel(name string, value string) RenderOptsFn {
	return func(opts *renderOpts) {
		if opts.labels == nil {
			opts.labels = map[string]string{}
		}

		opts.labels[name] = value
	}
}

// WithLabels adds multiple labels to all rendered resources.
func WithLabels(values map[string]string) RenderOptsFn {
	return func(opts *renderOpts) {
		if opts.labels == nil {
			opts.labels = map[string]string{}
		}

		maps.Copy(opts.labels, values)
	}
}

// WithAnnotation adds an annotation to all rendered resources.
func WithAnnotation(name string, value string) RenderOptsFn {
	return func(opts *renderOpts) {
		if opts.annotations == nil {
			opts.annotations = map[string]string{}
		}

		opts.annotations[name] = value
	}
}

// WithAnnotations adds multiple annotations to all rendered resources.
func WithAnnotations(values map[string]string) RenderOptsFn {
	return func(opts *renderOpts) {
		if opts.annotations == nil {
			opts.annotations = map[string]string{}
		}

		maps.Copy(opts.annotations, values)
	}
}

// WithPlugin adds a custom Kustomize transformer plugin.
func WithPlugin(value resmap.Transformer) RenderOptsFn {
	return func(opts *renderOpts) {
		opts.plugins = append(opts.plugins, value)
	}
}

// WithFilter adds a FilterFn as a Kustomize transformer plugin.
func WithFilter(value FilterFn) RenderOptsFn {
	return func(opts *renderOpts) {
		opts.plugins = append(opts.plugins, &filterPlugin{f: value})
	}
}

// WithFilters adds multiple FilterFns as Kustomize transformer plugins.
func WithFilters(values ...FilterFn) RenderOptsFn {
	return func(opts *renderOpts) {
		for i := range values {
			opts.plugins = append(opts.plugins, &filterPlugin{f: values[i]})
		}
	}
}
