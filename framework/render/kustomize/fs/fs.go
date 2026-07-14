package fs

import (
	"errors"
	iofs "io/fs"
	"path/filepath"

	"github.com/spf13/afero"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// NewFsOnDisk creates a filesys.FileSystem backed by the OS filesystem.
func NewFsOnDisk() filesys.FileSystem {
	return New(afero.NewOsFs())
}

// NewMemoryFs creates an in-memory filesys.FileSystem.
func NewMemoryFs() filesys.FileSystem {
	return New(afero.NewMemMapFs())
}

// NewReadOnlyFs creates a read-only wrapper around the given filesys.FileSystem.
func NewReadOnlyFs(base filesys.FileSystem) filesys.FileSystem {
	if unwrapper, ok := base.(interface{ Unwrap() afero.Fs }); ok {
		return New(afero.NewReadOnlyFs(unwrapper.Unwrap()))
	}
	return &readOnlyWrapper{base: base}
}

// NewFromIOFS creates a filesys.FileSystem from an fs.FS (e.g., embed.FS).
// The root parameter specifies the root directory within the fs.FS to use.
// The resulting filesystem is read-only.
func NewFromIOFS(fsys iofs.FS, root string) (filesys.FileSystem, error) {
	baseFs := afero.FromIOFS{FS: fsys}

	var resultFs afero.Fs = baseFs
	if root != "" {
		root = filepath.Clean(root)
		resultFs = afero.NewBasePathFs(baseFs, root)
	}

	return New(afero.NewReadOnlyFs(resultFs)), nil
}

// NewBasePathFs creates a filesys.FileSystem that restricts operations to a base path.
func NewBasePathFs(base filesys.FileSystem, basePath string) (filesys.FileSystem, error) {
	if unwrapper, ok := base.(interface{ Unwrap() afero.Fs }); ok {
		return New(afero.NewBasePathFs(unwrapper.Unwrap(), basePath)), nil
	}
	return nil, errors.New("base filesystem must be created with fs package functions") //nolint:err113
}

// readOnlyWrapper blocks writes for non-Afero filesystems.
type readOnlyWrapper struct {
	base filesys.FileSystem
}

func (r *readOnlyWrapper) Create(_ string) (filesys.File, error) {
	return nil, errors.New("create not supported on read-only filesystem") //nolint:err113
}

func (r *readOnlyWrapper) Mkdir(_ string) error {
	return errors.New("mkdir not supported on read-only filesystem") //nolint:err113
}

func (r *readOnlyWrapper) MkdirAll(_ string) error {
	return errors.New("mkdirall not supported on read-only filesystem") //nolint:err113
}

func (r *readOnlyWrapper) RemoveAll(_ string) error {
	return errors.New("removeall not supported on read-only filesystem") //nolint:err113
}

func (r *readOnlyWrapper) WriteFile(_ string, _ []byte) error {
	return errors.New("writefile not supported on read-only filesystem") //nolint:err113
}

func (r *readOnlyWrapper) Open(path string) (filesys.File, error) {
	return r.base.Open(path) //nolint:wrapcheck
}

func (r *readOnlyWrapper) Exists(path string) bool {
	return r.base.Exists(path)
}

func (r *readOnlyWrapper) IsDir(path string) bool {
	return r.base.IsDir(path)
}

func (r *readOnlyWrapper) ReadDir(path string) ([]string, error) {
	return r.base.ReadDir(path) //nolint:wrapcheck
}

func (r *readOnlyWrapper) ReadFile(path string) ([]byte, error) {
	return r.base.ReadFile(path) //nolint:wrapcheck
}

func (r *readOnlyWrapper) Glob(pattern string) ([]string, error) {
	return r.base.Glob(pattern) //nolint:wrapcheck
}

func (r *readOnlyWrapper) Walk(path string, walkFn filepath.WalkFunc) error {
	return r.base.Walk(path, walkFn) //nolint:wrapcheck
}

func (r *readOnlyWrapper) CleanedAbs(path string) (filesys.ConfirmedDir, string, error) {
	return r.base.CleanedAbs(path) //nolint:wrapcheck
}

var _ filesys.FileSystem = (*readOnlyWrapper)(nil)
