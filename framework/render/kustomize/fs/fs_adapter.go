//nolint:wrapcheck,mnd
package fs

import (
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// Adapter wraps an afero.Fs to implement filesys.FileSystem.
type Adapter struct {
	fs afero.Fs
}

// New creates a filesys.FileSystem backed by the given afero.Fs.
func New(afs afero.Fs) filesys.FileSystem {
	return &Adapter{fs: afs}
}

func (a *Adapter) Create(path string) (filesys.File, error) {
	return a.fs.Create(path)
}

func (a *Adapter) Mkdir(path string) error {
	return a.fs.Mkdir(path, 0777|os.ModeDir)
}

func (a *Adapter) MkdirAll(path string) error {
	return a.fs.MkdirAll(path, 0777|os.ModeDir)
}

func (a *Adapter) RemoveAll(path string) error {
	return a.fs.RemoveAll(path)
}

func (a *Adapter) Open(path string) (filesys.File, error) {
	return a.fs.Open(path)
}

func (a *Adapter) Exists(path string) bool {
	_, err := a.fs.Stat(path)
	return err == nil
}

func (a *Adapter) IsDir(path string) bool {
	info, err := a.fs.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func (a *Adapter) ReadDir(path string) ([]string, error) {
	entries, err := afero.ReadDir(a.fs, path)
	if err != nil {
		return nil, err
	}

	result := make([]string, len(entries))
	for i, entry := range entries {
		result[i] = entry.Name()
	}
	return result, nil
}

func (a *Adapter) ReadFile(path string) ([]byte, error) {
	return afero.ReadFile(a.fs, path)
}

func (a *Adapter) WriteFile(path string, data []byte) error {
	return afero.WriteFile(a.fs, path, data, 0666)
}

func (a *Adapter) Glob(pattern string) ([]string, error) {
	return afero.Glob(a.fs, pattern)
}

func (a *Adapter) Walk(path string, walkFn filepath.WalkFunc) error {
	return afero.Walk(a.fs, path, walkFn)
}

func (a *Adapter) CleanedAbs(path string) (filesys.ConfirmedDir, string, error) {
	if path == "" {
		path = "."
	}

	var resolvedPath string
	if _, ok := a.fs.(*afero.OsFs); ok {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", "", fmt.Errorf("abs path error on %q: %w", path, err)
		}
		deLinked, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			return "", "", fmt.Errorf("evalsymlink failure on %q: %w", path, err)
		}
		resolvedPath = deLinked
	} else {
		// For virtual filesystems (in-memory, io.FS-backed, union) keep the path
		// as-is. Converting relative paths to absolute via filepath.Abs breaks
		// filesystems that use relative key spaces (e.g. io/fs requires no leading
		// slash), while absolute paths are preserved correctly either way.
		resolvedPath = path
	}

	info, err := a.fs.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", &iofs.PathError{Op: "stat", Path: path, Err: iofs.ErrNotExist}
		}
		return "", "", fmt.Errorf("stat error on %q: %w", path, err)
	}

	if info.IsDir() {
		return filesys.ConfirmedDir(resolvedPath), "", nil
	}

	dir := filepath.Dir(resolvedPath)
	file := filepath.Base(resolvedPath)
	return filesys.ConfirmedDir(dir), file, nil
}

// Unwrap returns the underlying afero.Fs.
func (a *Adapter) Unwrap() afero.Fs {
	return a.fs
}

var _ filesys.File = (afero.File)(nil)
var _ filesys.FileSystem = (*Adapter)(nil)
