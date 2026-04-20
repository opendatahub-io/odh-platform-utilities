package template

import (
	"bytes"
	"testing"

	. "github.com/onsi/gomega"
)

//nolint:paralleltest
func TestDigestTemplateRenderOptsDiffersByActionLabel(t *testing.T) {
	g := NewWithT(t)

	a, err := digestTemplateRenderOpts([]Option{WithLabel("k", "a")})
	g.Expect(err).NotTo(HaveOccurred())

	b, err := digestTemplateRenderOpts([]Option{WithLabel("k", "b")})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(a).NotTo(Equal(b))
}

//nolint:paralleltest
func TestDigestTemplateRenderOptsDiffersByActionAnnotation(t *testing.T) {
	g := NewWithT(t)

	a, err := digestTemplateRenderOpts([]Option{WithAnnotation("k", "a")})
	g.Expect(err).NotTo(HaveOccurred())

	b, err := digestTemplateRenderOpts([]Option{WithAnnotation("k", "b")})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(a).NotTo(Equal(b))
}

//nolint:paralleltest
func TestDigestTemplateRenderOptsStableForSameOpts(t *testing.T) {
	g := NewWithT(t)

	opts := []Option{
		WithLabel("app", "x"),
		WithAnnotation("a", "b"),
	}

	first, err := digestTemplateRenderOpts(opts)
	g.Expect(err).NotTo(HaveOccurred())

	second, err := digestTemplateRenderOpts(opts)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(bytes.Equal(first, second)).Should(BeTrue())
}
