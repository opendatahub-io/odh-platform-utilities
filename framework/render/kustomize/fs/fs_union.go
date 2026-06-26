package fs

import (
	"errors"
	"fmt"
	"maps"

	"github.com/spf13/afero"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// UnionOption configures a union filesystem.
type UnionOption func(*unionConfig) error

type unionConfig struct {
	overrides map[string][]byte
	overlay   filesys.FileSystem
}

// WithOverride adds a virtual file to the overlay layer.
func WithOverride(path string, content []byte) UnionOption {
	return func(cfg *unionConfig) error {
		if cfg.overrides == nil {
			cfg.overrides = make(map[string][]byte)
		}
		cfg.overrides[path] = content
		return nil
	}
}

// WithOverrides adds multiple virtual files to the overlay layer.
func WithOverrides(overrides map[string][]byte) UnionOption {
	return func(cfg *unionConfig) error {
		if cfg.overrides == nil {
			cfg.overrides = make(map[string][]byte)
		}
		maps.Copy(cfg.overrides, overrides)
		return nil
	}
}

// WithOverlayFs specifies a custom overlay filesystem.
func WithOverlayFs(overlay filesys.FileSystem) UnionOption {
	return func(cfg *unionConfig) error {
		cfg.overlay = overlay
		return nil
	}
}

// NewUnionFs creates a union filesystem that layers an overlay over a base filesystem.
// Reads check the overlay first, then fall back to the base. Writes go to the overlay.
func NewUnionFs(base filesys.FileSystem, opts ...UnionOption) (filesys.FileSystem, error) {
	cfg := &unionConfig{
		overrides: make(map[string][]byte),
	}

	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}

	overlay := cfg.overlay
	if overlay == nil {
		overlay = NewMemoryFs()
		for path, content := range cfg.overrides {
			if err := overlay.WriteFile(path, content); err != nil {
				return nil, fmt.Errorf("failed to write override %s: %w", path, err)
			}
		}
	}

	baseUnwrapper, ok := base.(interface{ Unwrap() afero.Fs })
	if !ok {
		return nil, errors.New("base filesystem must be created with fs package functions") //nolint:err113
	}

	overlayUnwrapper, ok := overlay.(interface{ Unwrap() afero.Fs })
	if !ok {
		return nil, errors.New("overlay filesystem must be created with fs package functions") //nolint:err113
	}

	unionFs := afero.NewCopyOnWriteFs(baseUnwrapper.Unwrap(), overlayUnwrapper.Unwrap())
	return New(unionFs), nil
}
