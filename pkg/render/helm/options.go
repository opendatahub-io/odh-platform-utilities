package helm

import (
	"maps"

	engineTypes "github.com/k8s-manifest-kit/engine/pkg/types"
)

type options struct {
	labels       map[string]string
	annotations  map[string]string
	transformers []engineTypes.Transformer
}

// Option configures Helm rendering behaviour.
type Option func(*options)

// WithLabel adds a label to all rendered resources.
func WithLabel(name, value string) Option {
	return func(o *options) {
		if o.labels == nil {
			o.labels = make(map[string]string)
		}

		o.labels[name] = value
	}
}

// WithLabels adds multiple labels to all rendered resources.
func WithLabels(values map[string]string) Option {
	return func(o *options) {
		if o.labels == nil {
			o.labels = make(map[string]string)
		}

		maps.Copy(o.labels, values)
	}
}

// WithAnnotation adds an annotation to all rendered resources.
func WithAnnotation(name, value string) Option {
	return func(o *options) {
		if o.annotations == nil {
			o.annotations = make(map[string]string)
		}

		o.annotations[name] = value
	}
}

// WithAnnotations adds multiple annotations to all rendered resources.
func WithAnnotations(values map[string]string) Option {
	return func(o *options) {
		if o.annotations == nil {
			o.annotations = make(map[string]string)
		}

		maps.Copy(o.annotations, values)
	}
}

// WithTransformer adds a post-render transformer to the Helm rendering pipeline.
func WithTransformer(transformer engineTypes.Transformer) Option {
	return func(o *options) {
		o.transformers = append(o.transformers, transformer)
	}
}

// WithTransformers adds multiple post-render transformers to the pipeline.
func WithTransformers(transformers ...engineTypes.Transformer) Option {
	return func(o *options) {
		o.transformers = append(o.transformers, transformers...)
	}
}
