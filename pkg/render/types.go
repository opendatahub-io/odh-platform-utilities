package render

import (
	"context"
	"io/fs"

	helm "github.com/k8s-manifest-kit/renderer-helm/pkg"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Fn is the action-pipeline function signature. A renderer's NewAction()
// returns an Fn that reads inputs from ReconciliationRequest and writes
// rendered resources back to it.
type Fn func(ctx context.Context, rr *ReconciliationRequest) error

// ManifestInfo identifies a set of Kustomize manifests to render.
type ManifestInfo struct {
	Path       string
	ContextDir string
	SourcePath string
}

// String returns the fully-qualified path for this manifest info, joining
// Path, ContextDir, and SourcePath.
func (mi ManifestInfo) String() string {
	result := mi.Path

	if mi.ContextDir != "" {
		result = result + "/" + mi.ContextDir
	}

	if mi.SourcePath != "" {
		result = result + "/" + mi.SourcePath
	}

	return result
}

// TemplateInfo identifies a Go template source to render.
type TemplateInfo struct {
	Labels      map[string]string
	Annotations map[string]string
	FS          fs.FS
	Path        string
}

// HookFn is the signature for pre/post apply hooks on Helm charts.
type HookFn func(ctx context.Context, rr *ReconciliationRequest) error

// HelmChartInfo describes a Helm chart to render.
type HelmChartInfo struct {
	helm.Source

	PreApply  []HookFn
	PostApply []HookFn
}

// ReconciliationRequest carries the inputs and outputs for an action-pipeline
// rendering step. Module teams using the action pipeline pass this between
// actions. Teams using standalone Render() functions do not need this type.
type ReconciliationRequest struct {
	Client     client.Client
	Instance   client.Object
	Resources  []unstructured.Unstructured
	Manifests  []ManifestInfo
	Templates  []TemplateInfo
	HelmCharts []HelmChartInfo

	// Generated is set to true when new resources have been rendered
	// (as opposed to served from cache). Useful for downstream GC actions.
	Generated bool
}
