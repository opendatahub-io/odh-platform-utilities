package labels_test

import (
	"strings"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"

	. "github.com/onsi/gomega"
)

func TestNormalizePartOfValueLowercase(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	v, err := labels.NormalizePartOfValue("Dashboard")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(v).Should(Equal("dashboard"))
}

func TestNormalizePartOfValueTrimWhitespace(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	v, err := labels.NormalizePartOfValue("  dashboard  ")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(v).Should(Equal("dashboard"))
}

func TestNormalizePartOfValueCombined(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	v, err := labels.NormalizePartOfValue("  MyComponent  ")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(v).Should(Equal("mycomponent"))
}

func TestNormalizePartOfValueAlreadyNormalized(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	v, err := labels.NormalizePartOfValue("dashboard")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(v).Should(Equal("dashboard"))
}

func TestNormalizePartOfValueEmpty(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	v, err := labels.NormalizePartOfValue("")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(v).Should(Equal(""))
}

func TestNormalizePartOfValueWhitespaceOnly(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	v, err := labels.NormalizePartOfValue("   ")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(v).Should(Equal(""))
}

func TestNormalizePartOfValueMixedCase(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	v, err := labels.NormalizePartOfValue("DataScienceCluster")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(v).Should(Equal("datasciencecluster"))
}

func TestNormalizePartOfValueWithDots(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	v, err := labels.NormalizePartOfValue("my.component")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(v).Should(Equal("my.component"))
}

func TestNormalizePartOfValueWithDashesAndUnderscores(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	v, err := labels.NormalizePartOfValue("my-component_v2")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(v).Should(Equal("my-component_v2"))
}

func TestNormalizePartOfValueRejectsSpaces(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	_, err := labels.NormalizePartOfValue("my component")
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err).Should(MatchError(ContainSubstring("invalid characters")))
}

func TestNormalizePartOfValueRejectsTooLong(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	_, err := labels.NormalizePartOfValue(strings.Repeat("a", 64))
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err).Should(MatchError(ContainSubstring("63-character limit")))
}

func TestNormalizePartOfValueAcceptsMaxLength(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	input := strings.Repeat("a", 63)
	v, err := labels.NormalizePartOfValue(input)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(v).Should(Equal(input))
}

func TestNormalizePartOfValueRejectsStartingWithDot(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	_, err := labels.NormalizePartOfValue(".component")
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err).Should(MatchError(ContainSubstring("invalid characters")))
}

func TestNormalizePartOfValueRejectsEndingWithDash(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	_, err := labels.NormalizePartOfValue("component-")
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err).Should(MatchError(ContainSubstring("invalid characters")))
}
