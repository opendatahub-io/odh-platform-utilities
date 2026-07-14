package fs_test

import (
	iofs "io/fs"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/afero"

	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
)

func TestCleanedAbs_Directory(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.New(afero.NewMemMapFs())
	err := fsys.MkdirAll("/test/dir")
	g.Expect(err).To(Succeed())

	dir, file, err := fsys.CleanedAbs("/test/dir")
	g.Expect(err).To(Succeed())
	g.Expect(string(dir)).To(Equal("/test/dir"))
	g.Expect(file).To(BeEmpty())
}

func TestCleanedAbs_File(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.New(afero.NewMemMapFs())
	err := fsys.MkdirAll("/test/dir")
	g.Expect(err).To(Succeed())
	err = fsys.WriteFile("/test/dir/file.txt", []byte("content"))
	g.Expect(err).To(Succeed())

	dir, file, err := fsys.CleanedAbs("/test/dir/file.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(dir)).To(Equal("/test/dir"))
	g.Expect(file).To(Equal("file.txt"))
}

func TestCleanedAbs_NonExistent(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.New(afero.NewMemMapFs())

	_, _, err := fsys.CleanedAbs("/nonexistent/path")
	g.Expect(err).To(HaveOccurred())
}

func TestReadDir(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.New(afero.NewMemMapFs())
	err := fsys.MkdirAll("/testdir")
	g.Expect(err).To(Succeed())
	err = fsys.WriteFile("/testdir/file1.txt", []byte("1"))
	g.Expect(err).To(Succeed())
	err = fsys.WriteFile("/testdir/file2.txt", []byte("2"))
	g.Expect(err).To(Succeed())

	files, err := fsys.ReadDir("/testdir")
	g.Expect(err).To(Succeed())
	g.Expect(files).To(HaveLen(2))
	g.Expect(files).To(ContainElements("file1.txt", "file2.txt"))
}

func TestGlob(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.New(afero.NewMemMapFs())
	err := fsys.MkdirAll("/test")
	g.Expect(err).To(Succeed())
	err = fsys.WriteFile("/test/file1.yaml", []byte("1"))
	g.Expect(err).To(Succeed())
	err = fsys.WriteFile("/test/file2.yaml", []byte("2"))
	g.Expect(err).To(Succeed())
	err = fsys.WriteFile("/test/file3.txt", []byte("3"))
	g.Expect(err).To(Succeed())

	matches, err := fsys.Glob("/test/*.yaml")
	g.Expect(err).To(Succeed())
	g.Expect(matches).To(HaveLen(2))
}

func TestWalk(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.New(afero.NewMemMapFs())
	err := fsys.MkdirAll("/walk/subdir")
	g.Expect(err).To(Succeed())
	err = fsys.WriteFile("/walk/file1.txt", []byte("1"))
	g.Expect(err).To(Succeed())
	err = fsys.WriteFile("/walk/subdir/file2.txt", []byte("2"))
	g.Expect(err).To(Succeed())

	var paths []string
	err = fsys.Walk("/walk", func(path string, _ iofs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})

	g.Expect(err).To(Succeed())
	g.Expect(paths).To(ContainElements("/walk", "/walk/file1.txt", "/walk/subdir", "/walk/subdir/file2.txt"))
}

func TestMkdir(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.New(afero.NewMemMapFs())
	err := fsys.Mkdir("/testdir")
	g.Expect(err).To(Succeed())
	g.Expect(fsys.IsDir("/testdir")).To(BeTrue())
}

func TestMkdirAll(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.New(afero.NewMemMapFs())
	err := fsys.MkdirAll("/test/nested/dir")
	g.Expect(err).To(Succeed())
	g.Expect(fsys.IsDir("/test/nested/dir")).To(BeTrue())
}

func TestCreate(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.New(afero.NewMemMapFs())
	file, err := fsys.Create("/created.txt")
	g.Expect(err).To(Succeed())
	g.Expect(file).NotTo(BeNil())

	_, err = file.Write([]byte("created content"))
	g.Expect(err).To(Succeed())
	err = file.Close()
	g.Expect(err).To(Succeed())

	data, err := fsys.ReadFile("/created.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("created content"))
}

func TestOpen(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.New(afero.NewMemMapFs())
	err := fsys.WriteFile("/open.txt", []byte("open content"))
	g.Expect(err).To(Succeed())

	file, err := fsys.Open("/open.txt")
	g.Expect(err).To(Succeed())
	g.Expect(file).NotTo(BeNil())

	data := make([]byte, 12)
	n, err := file.Read(data)
	g.Expect(err).To(Succeed())
	g.Expect(n).To(Equal(12))
	g.Expect(string(data)).To(Equal("open content"))
	err = file.Close()
	g.Expect(err).To(Succeed())
}

func TestRemoveAll(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.New(afero.NewMemMapFs())
	err := fsys.MkdirAll("/remove/nested")
	g.Expect(err).To(Succeed())
	err = fsys.WriteFile("/remove/nested/file.txt", []byte("content"))
	g.Expect(err).To(Succeed())

	err = fsys.RemoveAll("/remove")
	g.Expect(err).To(Succeed())
	g.Expect(fsys.Exists("/remove")).To(BeFalse())
}
