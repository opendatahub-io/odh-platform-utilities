package helm

import (
	"bytes"
	"context"
	"testing"

	engineTypes "github.com/k8s-manifest-kit/engine/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	. "github.com/onsi/gomega"
)

func noopTransform() engineTypes.Transformer {
	return func(_ context.Context, object unstructured.Unstructured) (unstructured.Unstructured, error) {
		return object, nil
	}
}

//nolint:paralleltest
func TestDigestRenderOptsDiffersByLabel(t *testing.T) {
	g := NewWithT(t)

	a, err := digestRenderOpts([]Option{WithLabel("k", "a")})
	g.Expect(err).NotTo(HaveOccurred())

	b, err := digestRenderOpts([]Option{WithLabel("k", "b")})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(a).NotTo(Equal(b))
}

//nolint:paralleltest
func TestDigestRenderOptsDiffersByAnnotation(t *testing.T) {
	g := NewWithT(t)

	a, err := digestRenderOpts([]Option{WithAnnotation("k", "a")})
	g.Expect(err).NotTo(HaveOccurred())

	b, err := digestRenderOpts([]Option{WithAnnotation("k", "b")})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(a).NotTo(Equal(b))
}

//nolint:paralleltest
func TestDigestRenderOptsDiffersByTransformerCount(t *testing.T) {
	g := NewWithT(t)

	a, err := digestRenderOpts(nil)
	g.Expect(err).NotTo(HaveOccurred())

	b, err := digestRenderOpts([]Option{WithTransformer(noopTransform())})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(a).NotTo(Equal(b))
}

//nolint:paralleltest
func TestDigestRenderOptsDiffersByTransformerFuncIdentity(t *testing.T) {
	g := NewWithT(t)

	f1 := noopTransform()
	f2 := noopTransform()

	a, err := digestRenderOpts([]Option{WithTransformer(f1)})
	g.Expect(err).NotTo(HaveOccurred())

	b, err := digestRenderOpts([]Option{WithTransformer(f2)})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(a).NotTo(Equal(b))
}

//nolint:paralleltest
func TestDigestRenderOptsStableForSameOpts(t *testing.T) {
	g := NewWithT(t)

	opts := []Option{
		WithLabel("app", "x"),
		WithAnnotation("a", "b"),
	}

	first, err := digestRenderOpts(opts)
	g.Expect(err).NotTo(HaveOccurred())

	second, err := digestRenderOpts(opts)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(bytes.Equal(first, second)).Should(BeTrue())
}
