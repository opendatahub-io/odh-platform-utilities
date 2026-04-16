package helm_test

import (
	"path/filepath"
	"testing"

	helmRenderer "github.com/k8s-manifest-kit/renderer-helm/pkg"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/xid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/render/helm"

	. "github.com/onsi/gomega"
)

func testInstance() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "test.opendatahub.io/v1alpha1",
			"kind":       "TestComponent",
			"metadata":   map[string]any{"name": "test", "uid": xid.New().String()},
		},
	}
}

//nolint:paralleltest
func TestRenderHelmStandalone(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	chartDir := filepath.Join("testdata", "test-chart")

	resources, err := helm.Render(ctx,
		[]helmRenderer.Source{{
			Chart:       chartDir,
			ReleaseName: "test-release",
			Values: helmRenderer.Values(map[string]any{
				"replicaCount": 3,
				"namespace":    ns,
			}),
		}},
		helm.WithLabel("component.opendatahub.io/name", "test-component"),
		helm.WithAnnotation("platform.opendatahub.io/release", "1.2.3"),
	)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(resources).Should(HaveLen(1))
	g.Expect(resources[0].GetNamespace()).Should(Equal(ns))
	g.Expect(resources[0].GetLabels()).Should(HaveKeyWithValue("component.opendatahub.io/name", "test-component"))
	g.Expect(resources[0].GetAnnotations()).Should(HaveKeyWithValue("platform.opendatahub.io/release", "1.2.3"))
}

//nolint:paralleltest
func TestRenderHelmChartActionWithLabelsAndAnnotations(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	chartDir := filepath.Join("testdata", "test-chart")

	action := helm.NewAction(
		[]helm.Option{
			helm.WithLabel("component.opendatahub.io/name", "test-component"),
			helm.WithLabel("platform.opendatahub.io/namespace", ns),
			helm.WithAnnotation("platform.opendatahub.io/release", "1.2.3"),
			helm.WithAnnotation("platform.opendatahub.io/type", "managed"),
		},
		helm.WithCache(false),
	)

	render.RenderedResourcesTotal.Reset()

	for i := 1; i < 3; i++ {
		rr := render.ReconciliationRequest{
			Instance: testInstance(),
			HelmCharts: []render.HelmChartInfo{{
				Source: helmRenderer.Source{
					Chart:       chartDir,
					ReleaseName: "test-release",
					Values: helmRenderer.Values(map[string]any{
						"replicaCount": 3,
						"namespace":    ns,
					}),
				},
			}},
		}

		err := action(ctx, &rr)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(rr.Generated).Should(BeTrue())
		g.Expect(rr.Resources).Should(HaveLen(1))
		g.Expect(rr.Resources[0].GetNamespace()).Should(Equal(ns))
		g.Expect(rr.Resources[0].GetLabels()).Should(HaveKeyWithValue("component.opendatahub.io/name", "test-component"))
		g.Expect(rr.Resources[0].GetLabels()).Should(HaveKeyWithValue("platform.opendatahub.io/namespace", ns))
		g.Expect(rr.Resources[0].GetAnnotations()).Should(HaveKeyWithValue("platform.opendatahub.io/release", "1.2.3"))
		g.Expect(rr.Resources[0].GetAnnotations()).Should(HaveKeyWithValue("platform.opendatahub.io/type", "managed"))

		rc := testutil.ToFloat64(render.RenderedResourcesTotal)
		g.Expect(rc).Should(BeNumerically("==", 1*i))
	}
}

//nolint:paralleltest
func TestRenderHelmChartWithCacheAction(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	chartDir := filepath.Join("testdata", "test-chart")

	action := helm.NewAction(nil)

	render.RenderedResourcesTotal.Reset()

	inst := testInstance()

	for i := range 3 {
		if i >= 1 {
			inst.SetGeneration(1)
		}

		rr := render.ReconciliationRequest{
			Instance: inst,
			HelmCharts: []render.HelmChartInfo{{
				Source: helmRenderer.Source{
					Chart:       chartDir,
					ReleaseName: "test-release",
					Values: helmRenderer.Values(map[string]any{
						"replicaCount": 2,
						"namespace":    ns,
					}),
				},
			}},
		}

		err := action(ctx, &rr)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(rr.Resources).Should(HaveLen(1))
		g.Expect(rr.Resources[0].GetNamespace()).Should(Equal(ns))

		rc := testutil.ToFloat64(render.RenderedResourcesTotal)

		switch i {
		case 0:
			g.Expect(rc).Should(BeNumerically("==", 1))
			g.Expect(rr.Generated).Should(BeTrue())
		case 1:
			g.Expect(rc).Should(BeNumerically("==", 2))
			g.Expect(rr.Generated).Should(BeTrue())
		case 2:
			g.Expect(rc).Should(BeNumerically("==", 2))
			g.Expect(rr.Generated).Should(BeFalse())
		}
	}
}

//nolint:paralleltest
func TestRenderMultipleHelmCharts(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns1 := xid.New().String()
	ns2 := xid.New().String()
	chartDir := filepath.Join("testdata", "test-chart")

	action := helm.NewAction(
		[]helm.Option{helm.WithLabel("app", "multi-chart")},
		helm.WithCache(false),
	)

	rr := render.ReconciliationRequest{
		Instance: testInstance(),
		HelmCharts: []render.HelmChartInfo{
			{
				Source: helmRenderer.Source{
					Chart:       chartDir,
					ReleaseName: "release-one",
					Values: helmRenderer.Values(map[string]any{
						"namespace": ns1,
					}),
				},
			},
			{
				Source: helmRenderer.Source{
					Chart:       chartDir,
					ReleaseName: "release-two",
					Values: helmRenderer.Values(map[string]any{
						"namespace": ns2,
					}),
				},
			},
		},
	}

	err := action(ctx, &rr)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(2))
	g.Expect(rr.Resources[0].GetNamespace()).Should(Equal(ns1))
	g.Expect(rr.Resources[1].GetNamespace()).Should(Equal(ns2))
}

//nolint:paralleltest
func TestCRDInCrdsDirIsNotTemplated(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	chartDir := filepath.Join("testdata", "with-crds-dir")

	action := helm.NewAction(nil, helm.WithCache(false))

	rr := render.ReconciliationRequest{
		Instance: testInstance(),
		HelmCharts: []render.HelmChartInfo{{
			Source: helmRenderer.Source{
				Chart:       chartDir,
				ReleaseName: "test-crds-dir",
				Values: helmRenderer.Values(map[string]any{
					"namespace": ns,
				}),
			},
		}},
	}

	err := action(ctx, &rr)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(2))

	g.Expect(rr.Resources[0].GetKind()).Should(Equal("CustomResourceDefinition"))
	g.Expect(rr.Resources[0].GetName()).Should(Equal("testresources.test.opendatahub.io"))

	g.Expect(rr.Resources[1].GetKind()).Should(Equal("TestResource"))
	g.Expect(rr.Resources[1].GetNamespace()).Should(Equal(ns))
	g.Expect(rr.Resources[1].GetName()).Should(Equal("test-crds-dir-instance"))
}

//nolint:paralleltest
func TestCRDAndCRRender(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	chartDir := filepath.Join("testdata", "with-crd")

	action := helm.NewAction(nil, helm.WithCache(false))

	rr := render.ReconciliationRequest{
		Instance: testInstance(),
		HelmCharts: []render.HelmChartInfo{{
			Source: helmRenderer.Source{
				Chart:       chartDir,
				ReleaseName: "test-crd-release",
				Values: helmRenderer.Values(map[string]any{
					"namespace": ns,
				}),
			},
		}},
	}

	err := action(ctx, &rr)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(2))

	g.Expect(rr.Resources[0].GetKind()).Should(Equal("CustomResourceDefinition"))
	g.Expect(rr.Resources[1].GetKind()).Should(Equal("TestResource"))
	g.Expect(rr.Resources[1].GetNamespace()).Should(Equal(ns))
	g.Expect(rr.Resources[1].GetName()).Should(Equal("test-crd-release-instance"))
}
