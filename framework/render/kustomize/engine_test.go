// Package kustomize_test exercises the Engine with in-memory and union filesystems,
// with no disk I/O. The union FS pattern mirrors real operator usage: a read-only
// base (embed.FS) layered under a writable overlay for runtime config injection.
package kustomize_test

import (
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"

	"github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize"
	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
	"github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
)

// kustomization lists two plain manifest files.
const kustomization = `
apiVersion: kustomize.config.k8s.io/v1beta1
resources:
- configmap.yaml
- deployment.yaml
`

// kustomizationWithParams generates a ConfigMap from params.env,
// so rendered output reflects whatever params.Apply injects.
const kustomizationWithParams = `
apiVersion: kustomize.config.k8s.io/v1beta1
configMapGenerator:
- name: params
  envs:
  - params.env
`

const configMap = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  foo: bar
`

const deployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: app
        image: registry.example.com/app:latest
`

// originalParamsEnv is the base FS default. The leading blank line is intentional,
// exercising the parser's blank-line skipping. Tests assert this content is
// byte-for-byte unchanged in the base after params.Apply writes to the overlay.
const originalParamsEnv = `
image=registry.example.com/app:latest
replicas=1
`

// TestEngineRenderWithMemoryFS verifies rendering from a fully in-memory filesystem.
func TestEngineRenderWithMemoryFS(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	memFs := kfs.NewMemoryFs()
	g.Expect(memFs.WriteFile("kustomization.yaml", []byte(kustomization))).To(Succeed())
	g.Expect(memFs.WriteFile("configmap.yaml", []byte(configMap))).To(Succeed())
	g.Expect(memFs.WriteFile("deployment.yaml", []byte(deployment))).To(Succeed())

	engine := kustomize.NewEngine(kustomize.WithEngineFS(memFs))

	t.Run("when rendering, then all resources are returned", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		resources, err := engine.Render(".")
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(resources).Should(HaveLen(2))
	})
}

// TestEngineRenderWithUnionFS verifies that a read-only base combined with an overlay
// injecting kustomization.yaml renders without touching the base.
func TestEngineRenderWithUnionFS(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	baseFs, err := kfs.NewFromIOFS(fstest.MapFS{
		"configmap.yaml":  &fstest.MapFile{Data: []byte(configMap)},
		"deployment.yaml": &fstest.MapFile{Data: []byte(deployment)},
	}, "")
	g.Expect(err).ShouldNot(HaveOccurred())

	overlayFs := kfs.NewMemoryFs()
	g.Expect(overlayFs.WriteFile("kustomization.yaml", []byte(kustomization))).To(Succeed())

	unionFs, err := kfs.NewUnionFs(baseFs, kfs.WithOverlayFs(overlayFs))
	g.Expect(err).ShouldNot(HaveOccurred())

	engine := kustomize.NewEngine(kustomize.WithEngineFS(unionFs))

	t.Run("when rendering, then all resources from the base are returned", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		resources, err := engine.Render(".")
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(resources).Should(HaveLen(2))
	})
}

// TestEngineRenderWithUnionFSAndParamsApply verifies the full pipeline: base holds
// a kustomization + default params.env; params.Apply injects overrides into the
// overlay only, leaving the base byte-for-byte unchanged.
func TestEngineRenderWithUnionFSAndParamsApply(t *testing.T) {
	t.Parallel()

	baseMapFS := fstest.MapFS{
		"kustomization.yaml": &fstest.MapFile{Data: []byte(kustomizationWithParams)},
		"params.env":         &fstest.MapFile{Data: []byte(originalParamsEnv)},
	}

	baseFs, err := kfs.NewFromIOFS(baseMapFS, "")
	NewWithT(t).Expect(err).ShouldNot(HaveOccurred())

	t.Run("when rendering before and after params.Apply then each render reflects its params", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		overlayFs := kfs.NewMemoryFs()
		unionFs, err := kfs.NewUnionFs(baseFs, kfs.WithOverlayFs(overlayFs))
		g.Expect(err).ShouldNot(HaveOccurred())

		engine := kustomize.NewEngine(kustomize.WithEngineFS(unionFs))

		// Before: base defaults come through.
		before, err := engine.Render(".")
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(before[0].Object["data"]).Should(SatisfyAll(
			HaveKeyWithValue("image", "registry.example.com/app:latest"),
			HaveKeyWithValue("replicas", "1"),
		))

		// Inject overrides into the overlay — base is never touched.
		err = params.Apply(unionFs, "params.env", params.Values(map[string]string{
			"image":    "registry.example.com/app:v2",
			"replicas": "3",
		}))
		g.Expect(err).ShouldNot(HaveOccurred())

		// After: overlay values win.
		after, err := engine.Render(".")
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(after[0].Object["data"]).Should(SatisfyAll(
			HaveKeyWithValue("image", "registry.example.com/app:v2"),
			HaveKeyWithValue("replicas", "3"),
		))

		// Overlay received the write — reading through the overlay FS confirms the new value.
		overlayContent, err := overlayFs.ReadFile("params.env")
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(string(overlayContent)).Should(ContainSubstring("image=registry.example.com/app:v2"))

		// Base is unchanged — confirmed both via the FS API and the backing slice.
		baseContent, err := baseFs.ReadFile("params.env")
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(string(baseContent)).Should(Equal(originalParamsEnv))
		g.Expect(string(baseMapFS["params.env"].Data)).Should(Equal(originalParamsEnv))
	})

	t.Run("when rendering without overrides then the ConfigMap reflects the base defaults", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		unionFs, err := kfs.NewUnionFs(baseFs)
		g.Expect(err).ShouldNot(HaveOccurred())

		resources, err := kustomize.NewEngine(kustomize.WithEngineFS(unionFs)).Render(".")
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(resources[0].Object["data"]).Should(SatisfyAll(
			HaveKeyWithValue("image", "registry.example.com/app:latest"),
			HaveKeyWithValue("replicas", "1"),
		))
	})
}
