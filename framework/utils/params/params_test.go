package params

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestApplyNoOpWhenFileDoesNotExist(t *testing.T) {
	g := NewWithT(t)

	err := Apply(t.TempDir(), "params.env")
	g.Expect(err).ShouldNot(HaveOccurred())
}

func TestApplyNoOpWhenNoChanges(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	writeParamsFile(t, dir)

	err := Apply(dir, "params.env")
	g.Expect(err).ShouldNot(HaveOccurred())

	content, err := os.ReadFile(filepath.Join(dir, "params.env")) //nolint:gosec // test temp dir
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(content)).Should(Equal(paramsContent))
}

func TestApplyWithReplacementFromEnv(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	writeParamsFile(t, dir)

	t.Setenv("RELATED_IMAGE_CONTROLLER", "registry.example.com/controller:v2")

	imageMap := map[string]string{
		"controller_image": "RELATED_IMAGE_CONTROLLER",
	}

	err := Apply(dir, "params.env", Replacement(FromEnv(imageMap)))
	g.Expect(err).ShouldNot(HaveOccurred())

	result := readParamsFile(t, dir)
	g.Expect(result).Should(HaveKeyWithValue("controller_image", "registry.example.com/controller:v2"))
	g.Expect(result).Should(HaveKeyWithValue("namespace", "default"))
}

func TestApplyReplacementOnlyUpdatesExistingKeys(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	writeParamsFile(t, dir)

	t.Setenv("RELATED_IMAGE_NEW", "registry.example.com/new:v1")

	imageMap := map[string]string{
		"new_image": "RELATED_IMAGE_NEW",
	}

	err := Apply(dir, "params.env", Replacement(FromEnv(imageMap)))
	g.Expect(err).ShouldNot(HaveOccurred())

	content, err := os.ReadFile(filepath.Join(dir, "params.env")) //nolint:gosec // test temp dir
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(content)).Should(Equal(paramsContent))
}

func TestApplyReplacementSkipsUnsetEnvVars(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	writeParamsFile(t, dir)

	imageMap := map[string]string{
		"controller_image": "RELATED_IMAGE_DOES_NOT_EXIST",
	}

	err := Apply(dir, "params.env", Replacement(FromEnv(imageMap)))
	g.Expect(err).ShouldNot(HaveOccurred())

	content, err := os.ReadFile(filepath.Join(dir, "params.env")) //nolint:gosec // test temp dir
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(content)).Should(Equal(paramsContent))
}

func TestApplyWithValues(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	writeParamsFile(t, dir)

	err := Apply(dir, "params.env", Values(map[string]string{
		"namespace": "custom-ns",
	}))
	g.Expect(err).ShouldNot(HaveOccurred())

	result := readParamsFile(t, dir)
	g.Expect(result).Should(HaveKeyWithValue("namespace", "custom-ns"))
	g.Expect(result).Should(HaveKeyWithValue("controller_image", "quay.io/org/controller:v1"))
}

func TestApplyValuesCanAddNewKeys(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	writeParamsFile(t, dir)

	err := Apply(dir, "params.env", Values(map[string]string{
		"new_key": "new_value",
	}))
	g.Expect(err).ShouldNot(HaveOccurred())

	result := readParamsFile(t, dir)
	g.Expect(result).Should(HaveKeyWithValue("new_key", "new_value"))
}

func TestApplyWithReplacementAndValues(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	writeParamsFile(t, dir)

	t.Setenv("RELATED_IMAGE_CONTROLLER", "registry.example.com/controller:v2")

	imageMap := map[string]string{
		"controller_image": "RELATED_IMAGE_CONTROLLER",
	}
	extra := map[string]string{
		"namespace": "custom-ns",
	}

	err := Apply(dir, "params.env", Replacement(FromEnv(imageMap)), Values(extra))
	g.Expect(err).ShouldNot(HaveOccurred())

	result := readParamsFile(t, dir)
	g.Expect(result).Should(HaveKeyWithValue("controller_image", "registry.example.com/controller:v2"))
	g.Expect(result).Should(HaveKeyWithValue("namespace", "custom-ns"))
}

func TestApplyMultipleValues(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	writeParamsFile(t, dir)

	err := Apply(dir, "params.env",
		Values(map[string]string{"namespace": "ns-from-first"}),
		Values(map[string]string{"namespace": "ns-from-second"}),
	)
	g.Expect(err).ShouldNot(HaveOccurred())

	result := readParamsFile(t, dir)
	g.Expect(result).Should(HaveKeyWithValue("namespace", "ns-from-second"))
}

func TestParseParamsRoundTrip(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	writeParamsFile(t, dir)

	parsed, err := parseParams(filepath.Join(dir, "params.env"))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(parsed).Should(HaveLen(2))
	g.Expect(parsed).Should(HaveKeyWithValue("controller_image", "quay.io/org/controller:v1"))
	g.Expect(parsed).Should(HaveKeyWithValue("namespace", "default"))

	tmpFile, err := writeParamsToTmp(parsed, dir)
	g.Expect(err).ShouldNot(HaveOccurred())

	roundTripped, err := parseParams(tmpFile)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(roundTripped).Should(Equal(parsed))
}

const paramsContent = `controller_image=quay.io/org/controller:v1
namespace=default
`

func writeParamsFile(t *testing.T, dir string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(dir, "params.env"), []byte(paramsContent), 0o600)
	NewWithT(t).Expect(err).ShouldNot(HaveOccurred())
}

func readParamsFile(t *testing.T, dir string) map[string]string {
	t.Helper()

	result, err := parseParams(filepath.Join(dir, "params.env"))
	NewWithT(t).Expect(err).ShouldNot(HaveOccurred())

	return result
}
