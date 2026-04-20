package template_test

import (
	"strings"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/template"

	. "github.com/onsi/gomega"
)

func TestIndent(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	g.Expect(template.Indent(2, "a\nb")).Should(Equal("  a\n  b"))
	g.Expect(template.Indent(-1, "x")).Should(Equal("x"))
	g.Expect(template.Indent(1, "")).Should(Equal(""))
}

func TestTextTemplateFuncMapToYaml(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	fm := template.TextTemplateFuncMap()

	toYaml, ok := fm["toYaml"].(func(any) (string, error))
	g.Expect(ok).Should(BeTrue())

	out, err := toYaml(map[string]string{"a": "b"})
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(strings.TrimSpace(out)).Should(Equal(`a: b`))
}
