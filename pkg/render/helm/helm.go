package helm

import (
	"context"
	"slices"

	"github.com/k8s-manifest-kit/engine/pkg/postrenderer"
	"github.com/k8s-manifest-kit/engine/pkg/transformer/meta/annotations"
	"github.com/k8s-manifest-kit/engine/pkg/transformer/meta/labels"
	engineTypes "github.com/k8s-manifest-kit/engine/pkg/types"
	helmRenderer "github.com/k8s-manifest-kit/renderer-helm/pkg"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Render takes a set of Helm chart sources and renders them into unstructured
// Kubernetes resources. This is the standalone entry point that does not require
// the action pipeline or ReconciliationRequest.
func Render(
	ctx context.Context, charts []helmRenderer.Source, opts ...Option,
) ([]unstructured.Unstructured, error) {
	o := options{}

	for _, opt := range opts {
		opt(&o)
	}

	helmOptions := helmRenderer.RendererOptions{
		Strict:       true,
		Transformers: slices.Clone(o.transformers),
		PostRenderers: []engineTypes.PostRenderer{
			postrenderer.ApplyOrder(),
		},
	}

	if o.annotations != nil {
		helmOptions.Transformers = append(helmOptions.Transformers, annotations.Set(o.annotations))
	}

	if o.labels != nil {
		helmOptions.Transformers = append(helmOptions.Transformers, labels.Set(o.labels))
	}

	renderer, err := helmRenderer.New(charts, helmOptions)
	if err != nil {
		return nil, err
	}

	return renderer.Process(ctx, map[string]any{})
}
