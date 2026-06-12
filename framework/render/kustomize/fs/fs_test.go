package fs_test

import (
	"path/filepath"
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"

	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
)

func TestNewFsOnDisk(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.NewFsOnDisk()
	g.Expect(fsys).NotTo(BeNil())

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	err := fsys.WriteFile(testFile, []byte("hello world"))
	g.Expect(err).To(Succeed())

	data, err := fsys.ReadFile(testFile)
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("hello world"))

	g.Expect(fsys.Exists(testFile)).To(BeTrue())
}

func TestNewMemoryFs(t *testing.T) {
	g := NewWithT(t)

	fsys := kfs.NewMemoryFs()
	g.Expect(fsys).NotTo(BeNil())

	err := fsys.WriteFile("/test.txt", []byte("in memory"))
	g.Expect(err).To(Succeed())

	data, err := fsys.ReadFile("/test.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("in memory"))
}

func TestNewReadOnlyFs(t *testing.T) {
	g := NewWithT(t)

	base := kfs.NewMemoryFs()
	err := base.WriteFile("/readonly.txt", []byte("readonly content"))
	g.Expect(err).To(Succeed())

	readOnly := kfs.NewReadOnlyFs(base)
	g.Expect(readOnly).NotTo(BeNil())

	data, err := readOnly.ReadFile("/readonly.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("readonly content"))

	err = readOnly.WriteFile("/newfile.txt", []byte("should fail"))
	g.Expect(err).To(HaveOccurred())

	err = readOnly.Mkdir("/newdir")
	g.Expect(err).To(HaveOccurred())
}

func TestNewFromIOFS(t *testing.T) {
	g := NewWithT(t)

	testFS := fstest.MapFS{
		"subdir/test.yaml": &fstest.MapFile{
			Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n"),
		},
	}

	fsys, err := kfs.NewFromIOFS(testFS, "subdir")
	g.Expect(err).To(Succeed())
	g.Expect(fsys).NotTo(BeNil())

	data, err := fsys.ReadFile("test.yaml")
	g.Expect(err).To(Succeed())
	g.Expect(data).NotTo(BeEmpty())

	err = fsys.WriteFile("new.txt", []byte("should fail"))
	g.Expect(err).To(HaveOccurred())
}

func TestNewFromIOFS_EmptyRoot(t *testing.T) {
	g := NewWithT(t)

	testFS := fstest.MapFS{
		"dir/file.yaml": &fstest.MapFile{
			Data: []byte("test: data\n"),
		},
	}

	fsys, err := kfs.NewFromIOFS(testFS, "")
	g.Expect(err).To(Succeed())
	g.Expect(fsys).NotTo(BeNil())

	data, err := fsys.ReadFile("dir/file.yaml")
	g.Expect(err).To(Succeed())
	g.Expect(data).NotTo(BeEmpty())
}

func TestNewBasePathFs(t *testing.T) {
	g := NewWithT(t)

	base := kfs.NewMemoryFs()
	err := base.MkdirAll("/root/subdir")
	g.Expect(err).To(Succeed())
	err = base.WriteFile("/root/subdir/file.txt", []byte("content"))
	g.Expect(err).To(Succeed())

	scoped, err := kfs.NewBasePathFs(base, "/root")
	g.Expect(err).To(Succeed())

	data, err := scoped.ReadFile("/subdir/file.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("content"))
}
