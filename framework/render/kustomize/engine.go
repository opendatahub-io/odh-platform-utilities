package kustomize

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
)

const (
	DefaultKustomizationFileName = "kustomization.yaml"
	DefaultKustomizationFilePath = "default"
)

type Engine struct {
	krustyOpts *krusty.Options
	k          *krusty.Kustomizer
	fs         filesys.FileSystem
	renderOpts renderOpts
}

func NewEngine(opts ...EngineOptsFn) *Engine {
	e := Engine{
		krustyOpts: krusty.MakeDefaultOptions(),
		fs:         filesys.MakeFsOnDisk(),
		renderOpts: renderOpts{
			kustomizationFileName:    DefaultKustomizationFileName,
			kustomizationFileOverlay: DefaultKustomizationFilePath,
		},
	}

	for _, fn := range opts {
		fn(&e)
	}

	e.k = krusty.MakeKustomizer(e.krustyOpts)
	return &e
}

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

	if ro.ns != "" {
		plugin := namespaceApplierPlugin(ro.ns)
		if err := plugin.Transform(resMap); err != nil {
			return nil, fmt.Errorf("failed applying namespace plugin: %w", err)
		}
	}

	if len(ro.labels) != 0 {
		plugin := setLabelsPlugin(ro.labels)
		if err := plugin.Transform(resMap); err != nil {
			return nil, fmt.Errorf("failed applying labels plugin: %w", err)
		}
	}

	if len(ro.annotations) != 0 {
		plugin := addAnnotationsPlugin(ro.annotations)
		if err := plugin.Transform(resMap); err != nil {
			return nil, fmt.Errorf("failed applying annotations plugin: %w", err)
		}
	}

	for i := range ro.plugins {
		if err := ro.plugins[i].Transform(resMap); err != nil {
			return nil, fmt.Errorf("failed applying plugin: %w", err)
		}
	}

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
