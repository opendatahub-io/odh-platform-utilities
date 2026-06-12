package kustomize_test

import (
	"context"
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"

	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/kustomize"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	mk "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize"
	kfs "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/fs"
	kparams "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize/params"
)

// defaultParamsEnv is the base params.env content used across the params action tests.
// The leading blank line exercises the parser's blank-line skipping.
const defaultParamsEnv = `
image=registry.example.com/app:latest
replicas=1
`

// TestActionRenderWithParamsApplyOnUnionFS verifies the full stack: a read-only
// base FS (simulating embed.FS) combined with a union overlay, where params.Apply
// injects runtime overrides before the action renders, leaving the base unchanged.
func TestActionRenderWithParamsApplyOnUnionFS(t *testing.T) {
	t.Parallel()

	baseFs, err := kfs.NewFromIOFS(fstest.MapFS{
		"kustomization.yaml": &fstest.MapFile{Data: []byte(testKustomizationParamsEnv)},
		"params.env":         &fstest.MapFile{Data: []byte(defaultParamsEnv)},
	}, "")
	NewWithT(t).Expect(err).ShouldNot(HaveOccurred())

	t.Run("before and after params.Apply, each render reflects its params", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		// Hold a reference to the overlay to assert the write landed there.
		overlayFs := kfs.NewMemoryFs()
		unionFs, err := kfs.NewUnionFs(baseFs, kfs.WithOverlayFs(overlayFs))
		g.Expect(err).ShouldNot(HaveOccurred())

		action := kustomize.NewAction(
			kustomize.WithCache(false),
			kustomize.WithNamespace("test-ns"),
			kustomize.WithManifestsOptions(mk.WithEngineFS(unionFs)),
		)

		// First render: base defaults come through the union.
		rr := &types.ReconciliationRequest{
			Instance:  minimalInstance(),
			Manifests: []types.ManifestInfo{{Path: "."}},
		}

		err = action(context.Background(), rr)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(rr.Resources).Should(HaveLen(1))
		g.Expect(rr.Resources[0].Object["data"]).Should(SatisfyAll(
			HaveKeyWithValue("image", "registry.example.com/app:latest"),
			HaveKeyWithValue("replicas", "1"),
		))

		// Inject overrides into the overlay — base is never touched.
		err = kparams.Apply(unionFs, "params.env", kparams.Values(map[string]string{
			"image":    "registry.example.com/app:v2",
			"replicas": "3",
		}))
		g.Expect(err).ShouldNot(HaveOccurred())

		// Second render: union picks up the overridden params.env from the overlay.
		rr = &types.ReconciliationRequest{
			Instance:  minimalInstance(),
			Manifests: []types.ManifestInfo{{Path: "."}},
		}

		err = action(context.Background(), rr)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(rr.Resources).Should(HaveLen(1))
		g.Expect(rr.Resources[0].Object["data"]).Should(SatisfyAll(
			HaveKeyWithValue("image", "registry.example.com/app:v2"),
			HaveKeyWithValue("replicas", "3"),
		))

		// Overlay received the write — this is the real isolation proof.
		overlayContent, err := overlayFs.ReadFile("params.env")
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(string(overlayContent)).Should(ContainSubstring("image=registry.example.com/app:v2"))
	})

	t.Run("without overrides, renders base defaults", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		// Fresh union so the previous subtest's overlay writes don't bleed in.
		unionFs, err := kfs.NewUnionFs(baseFs)
		g.Expect(err).ShouldNot(HaveOccurred())

		action := kustomize.NewAction(
			kustomize.WithCache(false),
			kustomize.WithNamespace("test-ns"),
			kustomize.WithManifestsOptions(mk.WithEngineFS(unionFs)),
		)

		rr := &types.ReconciliationRequest{
			Instance:  minimalInstance(),
			Manifests: []types.ManifestInfo{{Path: "."}},
		}

		err = action(context.Background(), rr)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(rr.Resources).Should(HaveLen(1))
		g.Expect(rr.Resources[0].Object["data"]).Should(SatisfyAll(
			HaveKeyWithValue("image", "registry.example.com/app:latest"),
			HaveKeyWithValue("replicas", "1"),
		))
	})
}

// TestActionRenderWithFromEnv verifies that FromEnv resolves environment variables
// at call time and that overrides survive the full action render pipeline.
func TestActionRenderWithFromEnv(t *testing.T) {
	g := NewWithT(t)

	baseFs, err := kfs.NewFromIOFS(fstest.MapFS{
		"kustomization.yaml": &fstest.MapFile{Data: []byte(testKustomizationParamsEnv)},
		"params.env":         &fstest.MapFile{Data: []byte(defaultParamsEnv)},
	}, "")
	g.Expect(err).ShouldNot(HaveOccurred())

	overlayFs := kfs.NewMemoryFs()
	unionFs, err := kfs.NewUnionFs(baseFs, kfs.WithOverlayFs(overlayFs))
	g.Expect(err).ShouldNot(HaveOccurred())

	t.Setenv("TEST_IMAGE_OVERRIDE", "registry.example.com/app:env-injected")
	t.Setenv("TEST_NEW_KEY", "should-not-appear")

	// Replacement only updates keys already present in the file —
	// "new_key" is not in defaultParamsEnv so it must be silently skipped.
	err = kparams.Apply(unionFs, "params.env",
		kparams.Replacement(kparams.FromEnv(map[string]string{
			"image":   "TEST_IMAGE_OVERRIDE",
			"new_key": "TEST_NEW_KEY",
		})),
	)
	g.Expect(err).ShouldNot(HaveOccurred())

	action := kustomize.NewAction(
		kustomize.WithCache(false),
		kustomize.WithNamespace("test-ns"),
		kustomize.WithManifestsOptions(mk.WithEngineFS(unionFs)),
	)

	rr := &types.ReconciliationRequest{
		Instance:  minimalInstance(),
		Manifests: []types.ManifestInfo{{Path: "."}},
	}

	err = action(context.Background(), rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(1))
	g.Expect(rr.Resources[0].Object["data"]).Should(SatisfyAll(
		HaveKeyWithValue("image", "registry.example.com/app:env-injected"),
		HaveKeyWithValue("replicas", "1"),
		Not(HaveKey("new_key")),
	))

	// Overlay received the write — this is the real isolation proof.
	overlayContent, err := overlayFs.ReadFile("params.env")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(overlayContent)).Should(ContainSubstring("image=registry.example.com/app:env-injected"))
}
