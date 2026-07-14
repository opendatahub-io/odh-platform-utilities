package fs_test

import (
	"testing"

	. "github.com/onsi/gomega"

	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
)

func TestNewUnionFs(t *testing.T) {
	g := NewWithT(t)

	base := kfs.NewMemoryFs()
	err := base.WriteFile("/base.txt", []byte("base content"))
	g.Expect(err).To(Succeed())
	err = base.WriteFile("/override.txt", []byte("original"))
	g.Expect(err).To(Succeed())

	unionFs, err := kfs.NewUnionFs(base,
		kfs.WithOverride("/overlay.txt", []byte("overlay content")),
		kfs.WithOverride("/override.txt", []byte("overridden")),
	)
	g.Expect(err).To(Succeed())

	data, err := unionFs.ReadFile("/base.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("base content"))

	data, err = unionFs.ReadFile("/overlay.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("overlay content"))

	data, err = unionFs.ReadFile("/override.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("overridden"))
}

func TestNewUnionFs_MultipleOverrides(t *testing.T) {
	g := NewWithT(t)

	base := kfs.NewMemoryFs()
	err := base.WriteFile("/base.txt", []byte("base"))
	g.Expect(err).To(Succeed())

	unionFs, err := kfs.NewUnionFs(base,
		kfs.WithOverride("/override.txt", []byte("override content")),
		kfs.WithOverride("/another.txt", []byte("another content")),
	)
	g.Expect(err).To(Succeed())

	data, err := unionFs.ReadFile("/base.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("base"))

	data, err = unionFs.ReadFile("/override.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("override content"))

	data, err = unionFs.ReadFile("/another.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("another content"))
}

func TestNewUnionFs_WithOverridesMap(t *testing.T) {
	g := NewWithT(t)

	base := kfs.NewMemoryFs()

	overrides := map[string][]byte{
		"/file1.txt": []byte("content1"),
		"/file2.txt": []byte("content2"),
	}

	unionFs, err := kfs.NewUnionFs(base, kfs.WithOverrides(overrides))
	g.Expect(err).To(Succeed())

	data, err := unionFs.ReadFile("/file1.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("content1"))

	data, err = unionFs.ReadFile("/file2.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("content2"))
}

func TestNewUnionFs_WithCustomOverlay(t *testing.T) {
	g := NewWithT(t)

	base := kfs.NewMemoryFs()
	err := base.WriteFile("/base.txt", []byte("base"))
	g.Expect(err).To(Succeed())

	overlay := kfs.NewMemoryFs()
	err = overlay.WriteFile("/overlay.txt", []byte("overlay"))
	g.Expect(err).To(Succeed())

	unionFs, err := kfs.NewUnionFs(base, kfs.WithOverlayFs(overlay))
	g.Expect(err).To(Succeed())

	data, err := unionFs.ReadFile("/base.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("base"))

	data, err = unionFs.ReadFile("/overlay.txt")
	g.Expect(err).To(Succeed())
	g.Expect(string(data)).To(Equal("overlay"))
}

func TestNewUnionFs_Empty(t *testing.T) {
	g := NewWithT(t)

	base := kfs.NewMemoryFs()
	unionFs, err := kfs.NewUnionFs(base)
	g.Expect(err).To(Succeed())
	g.Expect(unionFs).NotTo(BeNil())
}
