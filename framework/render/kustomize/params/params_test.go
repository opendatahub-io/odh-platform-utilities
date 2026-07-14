package params_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
	"github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
)

const testParamsContent = "controller_image=quay.io/org/controller:v1\nnamespace=default\n"

func TestApplyNoOpWhenFileAbsent(t *testing.T) {
	g := NewWithT(t)

	err := params.Apply(kfs.NewMemoryFs(), "params.env")
	g.Expect(err).ShouldNot(HaveOccurred())
}

func TestApplyNoOpWhenNoChanges(t *testing.T) {
	g := NewWithT(t)

	memFs := kfs.NewMemoryFs()
	g.Expect(memFs.WriteFile("params.env", []byte(testParamsContent))).ShouldNot(HaveOccurred())

	err := params.Apply(memFs, "params.env")
	g.Expect(err).ShouldNot(HaveOccurred())

	content, err := memFs.ReadFile("params.env")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(content)).Should(Equal(testParamsContent))
}

func TestApplyWithValues(t *testing.T) {
	g := NewWithT(t)

	memFs := kfs.NewMemoryFs()
	g.Expect(memFs.WriteFile("params.env", []byte(testParamsContent))).ShouldNot(HaveOccurred())

	err := params.Apply(memFs, "params.env", params.Values(map[string]string{
		"namespace": "custom-ns",
	}))
	g.Expect(err).ShouldNot(HaveOccurred())

	content, err := memFs.ReadFile("params.env")
	result := mustParseParams(t, content, err)
	g.Expect(result).Should(HaveKeyWithValue("namespace", "custom-ns"))
	g.Expect(result).Should(HaveKeyWithValue("controller_image", "quay.io/org/controller:v1"))
}

func TestApplyValuesCanAddNewKeys(t *testing.T) {
	g := NewWithT(t)

	memFs := kfs.NewMemoryFs()
	g.Expect(memFs.WriteFile("params.env", []byte(testParamsContent))).ShouldNot(HaveOccurred())

	err := params.Apply(memFs, "params.env", params.Values(map[string]string{
		"new_key": "new_value",
	}))
	g.Expect(err).ShouldNot(HaveOccurred())

	content, err := memFs.ReadFile("params.env")
	result := mustParseParams(t, content, err)
	g.Expect(result).Should(HaveKeyWithValue("new_key", "new_value"))
}

func TestApplyReplacementOnlyUpdatesExistingKeys(t *testing.T) {
	g := NewWithT(t)

	memFs := kfs.NewMemoryFs()
	g.Expect(memFs.WriteFile("params.env", []byte(testParamsContent))).ShouldNot(HaveOccurred())

	err := params.Apply(memFs, "params.env", params.Replacement(map[string]string{
		"new_key": "new_value",
	}))
	g.Expect(err).ShouldNot(HaveOccurred())

	content, err := memFs.ReadFile("params.env")
	result := mustParseParams(t, content, err)
	g.Expect(result).ShouldNot(HaveKey("new_key"))
	g.Expect(result).Should(HaveKeyWithValue("namespace", "default"))
}

func TestApplyWithUnionFS(t *testing.T) {
	g := NewWithT(t)

	// Base: memory FS containing params.env, wrapped as read-only.
	// Represents an embedded or on-disk manifest tree that must not be modified.
	base := kfs.NewMemoryFs()
	g.Expect(base.WriteFile("params.env", []byte(testParamsContent))).ShouldNot(HaveOccurred())

	readOnly := kfs.NewReadOnlyFs(base)

	// Union: copy-on-write overlay. Reads fall through to base; all writes go
	// to the in-memory overlay — base is never touched.
	union, err := kfs.NewUnionFs(readOnly)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = params.Apply(union, "params.env", params.Values(map[string]string{
		"namespace": "injected-ns",
	}))
	g.Expect(err).ShouldNot(HaveOccurred())

	// Union sees the updated value written to the overlay.
	unionContent, err := union.ReadFile("params.env")
	unionResult := mustParseParams(t, unionContent, err)
	g.Expect(unionResult).Should(HaveKeyWithValue("namespace", "injected-ns"))
	g.Expect(unionResult).Should(HaveKeyWithValue("controller_image", "quay.io/org/controller:v1"))

	// Base is unchanged: the write went to the overlay only.
	baseContent, err := base.ReadFile("params.env")
	baseResult := mustParseParams(t, baseContent, err)
	g.Expect(baseResult).Should(HaveKeyWithValue("namespace", "default"))
}

func TestApplyAtPath(t *testing.T) {
	g := NewWithT(t)

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "params.env"), []byte(testParamsContent), 0o600)
	g.Expect(err).ShouldNot(HaveOccurred())

	err = params.ApplyAtPath(dir, "params.env", params.Values(map[string]string{
		"namespace": "on-disk-ns",
	}))
	g.Expect(err).ShouldNot(HaveOccurred())

	content, err := os.ReadFile(filepath.Join(dir, "params.env")) //nolint:gosec
	result := mustParseParams(t, content, err)
	g.Expect(result).Should(HaveKeyWithValue("namespace", "on-disk-ns"))
	g.Expect(result).Should(HaveKeyWithValue("controller_image", "quay.io/org/controller:v1"))
}

func TestApplySkipsCommentLines(t *testing.T) {
	g := NewWithT(t)

	content := "# this is a comment\nimage=quay.io/org/ctrl:v1\n# another comment\nns=default\n"
	memFs := kfs.NewMemoryFs()
	g.Expect(memFs.WriteFile("params.env", []byte(content))).ShouldNot(HaveOccurred())

	// Pass a mapper so the file is rewritten, proving comment lines are dropped
	// from the serialized output and don't produce key entries.
	err := params.Apply(memFs, "params.env", params.Values(map[string]string{"ns": "prod"}))
	g.Expect(err).ShouldNot(HaveOccurred())

	c, err := memFs.ReadFile("params.env")
	result := mustParseParams(t, c, err)
	g.Expect(result).Should(HaveLen(2))
	g.Expect(result).Should(HaveKeyWithValue("image", "quay.io/org/ctrl:v1"))
	g.Expect(result).Should(HaveKeyWithValue("ns", "prod"))
}

func TestApplyAtPathNoOpWhenFileAbsent(t *testing.T) {
	g := NewWithT(t)

	err := params.ApplyAtPath(t.TempDir(), "params.env")
	g.Expect(err).ShouldNot(HaveOccurred())
}

// mustParseParams parses key=value content using the production Unmarshal, failing on error.
func mustParseParams(t *testing.T, content []byte, err error) map[string]string {
	t.Helper()

	g := NewWithT(t)
	g.Expect(err).ShouldNot(HaveOccurred())

	result, err := params.Unmarshal(content)
	g.Expect(err).ShouldNot(HaveOccurred())

	return result
}
