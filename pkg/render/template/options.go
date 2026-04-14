package template

import (
	"context"
	"maps"
	gt "text/template"
)

const (
	// ComponentKey is the template data key for the Kubernetes instance object.
	ComponentKey = "Component"
	// AppNamespaceKey is the template data key for the target application namespace.
	AppNamespaceKey = "AppNamespace"
)

type options struct {
	funcMap     gt.FuncMap
	labels      map[string]string
	annotations map[string]string
}

// Option configures standalone template rendering behaviour.
type Option func(*options)

// WithLabel adds a label to all rendered resources.
func WithLabel(name string, value string) Option {
	return func(o *options) {
		o.labels[name] = value
	}
}

// WithLabels adds multiple labels to all rendered resources.
func WithLabels(values map[string]string) Option {
	return func(o *options) {
		maps.Copy(o.labels, values)
	}
}

// WithAnnotation adds an annotation to all rendered resources.
func WithAnnotation(name string, value string) Option {
	return func(o *options) {
		o.annotations[name] = value
	}
}

// WithAnnotations adds multiple annotations to all rendered resources.
func WithAnnotations(values map[string]string) Option {
	return func(o *options) {
		maps.Copy(o.annotations, values)
	}
}

// WithFuncMap replaces the default template function map. The default includes
// indent, nindent, and toYaml from pkg/template.TextTemplateFuncMap().
func WithFuncMap(fm gt.FuncMap) Option {
	return func(o *options) {
		o.funcMap = fm
	}
}

// actionOpts holds options specific to the action-pipeline template renderer.
type actionOpts struct {
	data          map[string]any
	nsFn          func(ctx context.Context) (string, error)
	namespace     string
	renderOpts    []Option
	dataFn        []func(context.Context) (map[string]any, error)
	cacheDisabled bool
}

// ActionOption configures the action-pipeline template renderer.
type ActionOption func(*actionOpts)

// WithCache enables or disables caching for the action adapter. When enabled
// (the default), renders are skipped if the ReconciliationRequest hash has not
// changed since the last run.
func WithCache(enabled bool) ActionOption {
	return func(o *actionOpts) {
		o.cacheDisabled = !enabled
	}
}

// WithData adds static key-value pairs to the template data map.
func WithData(data map[string]any) ActionOption {
	return func(o *actionOpts) {
		maps.Copy(o.data, data)
	}
}

// WithDataFn adds a function that dynamically computes template data at
// render time. Multiple functions are called in order and their results merged.
func WithDataFn(fns ...func(context.Context) (map[string]any, error)) ActionOption {
	return func(o *actionOpts) {
		o.dataFn = append(o.dataFn, fns...)
	}
}

// WithActionLabel adds a label to all rendered resources in the action adapter.
func WithActionLabel(name string, value string) ActionOption {
	return func(o *actionOpts) {
		o.renderOpts = append(o.renderOpts, WithLabel(name, value))
	}
}

// WithActionLabels adds multiple labels to all rendered resources.
func WithActionLabels(values map[string]string) ActionOption {
	return func(o *actionOpts) {
		o.renderOpts = append(o.renderOpts, WithLabels(values))
	}
}

// WithActionAnnotation adds an annotation to all rendered resources.
func WithActionAnnotation(name string, value string) ActionOption {
	return func(o *actionOpts) {
		o.renderOpts = append(o.renderOpts, WithAnnotation(name, value))
	}
}

// WithActionAnnotations adds multiple annotations to all rendered resources.
func WithActionAnnotations(values map[string]string) ActionOption {
	return func(o *actionOpts) {
		o.renderOpts = append(o.renderOpts, WithAnnotations(values))
	}
}

// WithNamespace sets a static namespace injected into template data as
// AppNamespaceKey. This replaces the operator's cluster.ApplicationNamespace().
func WithNamespace(ns string) ActionOption {
	return func(o *actionOpts) {
		o.namespace = ns
	}
}

// WithNamespaceFn sets a dynamic namespace resolution function. Called on
// every render to determine the target namespace.
func WithNamespaceFn(fn func(ctx context.Context) (string, error)) ActionOption {
	return func(o *actionOpts) {
		o.nsFn = fn
	}
}
