package kustomize

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

const (
	// DefaultKustomizationFileName is the standard kustomization file name.
	DefaultKustomizationFileName = "kustomization.yaml"
	// DefaultKustomizationFilePath is the default overlay subdirectory.
	DefaultKustomizationFilePath = "default"
)

// Engine wraps the Kustomize API to render overlays into unstructured
// Kubernetes resources.
type Engine struct {
	k          *krusty.Kustomizer
	fs         filesys.FileSystem
	renderOpts renderOpts
}

// NewEngine creates a new Kustomize rendering engine with the given options.
func NewEngine(opts ...EngineOptsFn) *Engine {
	e := Engine{
		k:  krusty.MakeKustomizer(krusty.MakeDefaultOptions()),
		fs: filesys.MakeFsOnDisk(),
		renderOpts: renderOpts{
			kustomizationFileName:    DefaultKustomizationFileName,
			kustomizationFileOverlay: DefaultKustomizationFilePath,
		},
	}

	for _, fn := range opts {
		fn(&e)
	}

	return &e
}

// Render renders the Kustomize overlay at path and returns the resulting
// unstructured resources. Additional per-render options (namespace, labels,
// annotations) can be provided to override engine defaults.
func (e *Engine) Render(path string, opts ...RenderOptsFn) ([]unstructured.Unstructured, error) {
	ro := e.renderOpts
	ro.labels = maps.Clone(e.renderOpts.labels)
	ro.annotations = maps.Clone(e.renderOpts.annotations)
	ro.plugins = slices.Clone(e.renderOpts.plugins)

	for _, fn := range opts {
		fn(&ro)
	}

	if !e.fs.Exists(filepath.Join(path, ro.kustomizationFileName)) {
		path = filepath.Join(path, ro.kustomizationFileOverlay)
	}

	resMap, err := e.k.Run(e.fs, path)
	if err != nil {
		return nil, err
	}

	err = applyTransformers(resMap, &ro)
	if err != nil {
		return nil, err
	}

	return toUnstructuredSlice(resMap)
}

func applyTransformers(resMap resmap.ResMap, ro *renderOpts) error {
	if ro.ns != "" {
		plugin := createNamespaceApplierPlugin(ro.ns)

		err := plugin.Transform(resMap)
		if err != nil {
			return fmt.Errorf("failed applying namespace plugin: %w", err)
		}
	}

	if len(ro.labels) != 0 {
		plugin := createSetLabelsPlugin(ro.labels)

		err := plugin.Transform(resMap)
		if err != nil {
			return fmt.Errorf("failed applying labels plugin: %w", err)
		}
	}

	if len(ro.annotations) != 0 {
		plugin := createAddAnnotationsPlugin(ro.annotations)

		err := plugin.Transform(resMap)
		if err != nil {
			return fmt.Errorf("failed applying annotations plugin: %w", err)
		}
	}

	for i := range ro.plugins {
		err := ro.plugins[i].Transform(resMap)
		if err != nil {
			return fmt.Errorf("failed applying plugin: %w", err)
		}
	}

	return nil
}

func toUnstructuredSlice(resMap resmap.ResMap) ([]unstructured.Unstructured, error) {
	renderedRes := resMap.Resources()
	resp := make([]unstructured.Unstructured, len(renderedRes))

	for i := range renderedRes {
		m, err := renderedRes[i].Map()
		if err != nil {
			return nil, fmt.Errorf("failed to transform Resources to Unstructured: %w", err)
		}

		u, err := resources.ToUnstructured(&m)
		if err != nil {
			return nil, fmt.Errorf("failed to transform Resources to Unstructured: %w", err)
		}

		resp[i] = *u
	}

	return resp, nil
}
