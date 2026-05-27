package template_test

import (
	"context"
	"embed"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/xid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"
	tmpl "github.com/opendatahub-io/odh-platform-utilities/pkg/render/template"

	. "github.com/onsi/gomega"
)

//go:embed resources
var testFS embed.FS

var errComputeData = errors.New("compute-data-error")

func testInstance(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "test.opendatahub.io/v1alpha1",
			"kind":       "TestComponent",
			"metadata":   map[string]any{"name": name, "uid": xid.New().String()},
		},
	}
}

//nolint:paralleltest
func TestRenderTemplateStandalone(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	name := xid.New().String()

	inst := testInstance(name)
	sources := []tmpl.TemplateSource{{FS: testFS, Path: "resources/smm.tmpl.yaml"}}

	data := map[string]any{
		tmpl.AppNamespaceKey: ns,
		tmpl.ComponentKey: tmpl.ComponentData{
			Name:   inst.GetName(),
			Object: inst,
		},
	}

	r, err := tmpl.Render(ctx, nil, sources, data)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(r).Should(HaveLen(1))
	g.Expect(r[0].GetNamespace()).Should(Equal(ns))
	g.Expect(r[0].GetAnnotations()).Should(HaveKeyWithValue("instance-name", name))
}

//nolint:paralleltest
func TestRenderTemplate(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	name := xid.New().String()

	action := tmpl.NewAction(
		tmpl.WithCache(false),
		tmpl.WithData(map[string]any{
			tmpl.AppNamespaceKey: ns,
		}),
	)

	render.RenderedResourcesTotal.Reset()

	for i := 1; i < 3; i++ {
		rr := render.ReconciliationRequest{
			Instance:  testInstance(name),
			Templates: []render.TemplateInfo{{FS: testFS, Path: "resources/smm.tmpl.yaml"}},
		}

		err := action(ctx, &rr)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(rr.Generated).Should(BeTrue())
		g.Expect(rr.Resources).Should(HaveLen(1))
		g.Expect(rr.Resources[0].GetNamespace()).Should(Equal(ns))
		g.Expect(rr.Resources[0].GetAnnotations()).Should(HaveKeyWithValue("instance-name", name))

		rc := testutil.ToFloat64(render.RenderedResourcesTotal)
		g.Expect(rc).Should(BeNumerically("==", i))
	}
}

//nolint:paralleltest
func TestRenderTemplateWithData(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	id := xid.New().String()
	name := xid.New().String()
	uid := xid.New().String()

	inst := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "test.opendatahub.io/v1alpha1",
			"kind":       "TestComponent",
			"metadata":   map[string]any{"name": ns, "uid": uid},
		},
	}

	action := tmpl.NewAction(
		tmpl.WithCache(false),
		tmpl.WithData(map[string]any{
			"ID": id,
			"SMM": map[string]any{
				"Name": name,
			},
			"Foo": "bar",
		}),
		tmpl.WithDataFn(func(_ context.Context) (map[string]any, error) {
			return map[string]any{
				"Foo": "bar",
				"UID": uid,
			}, nil
		}),
		tmpl.WithData(map[string]any{
			tmpl.AppNamespaceKey: ns,
		}),
	)

	rr := render.ReconciliationRequest{
		Instance:  inst,
		Templates: []render.TemplateInfo{{FS: testFS, Path: "resources/smm-data.tmpl.yaml"}},
	}

	err := action(ctx, &rr)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(1))
	g.Expect(rr.Resources[0].GetName()).Should(Equal(name))
	g.Expect(rr.Resources[0].GetNamespace()).Should(Equal(ns))
	g.Expect(rr.Resources[0].GetAnnotations()).Should(HaveKeyWithValue("instance-name", ns))
	g.Expect(rr.Resources[0].GetAnnotations()).Should(HaveKeyWithValue("instance-id", id))
	g.Expect(rr.Resources[0].GetAnnotations()).Should(HaveKeyWithValue("instance-uid", uid))
	g.Expect(rr.Resources[0].GetAnnotations()).Should(HaveKeyWithValue("instance-foo", "bar"))
}

//nolint:paralleltest
func TestRenderTemplateWithDataErr(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()

	action := tmpl.NewAction(
		tmpl.WithCache(false),
		tmpl.WithDataFn(func(_ context.Context) (map[string]any, error) {
			return map[string]any{}, errComputeData
		}),
	)

	rr := render.ReconciliationRequest{
		Instance:  testInstance(ns),
		Templates: []render.TemplateInfo{{FS: testFS, Path: "resources/smm-data.tmpl.yaml"}},
	}

	err := action(ctx, &rr)

	g.Expect(err).Should(HaveOccurred())
}

//nolint:paralleltest
func TestRenderTemplateWithNamespaceFnEmptyDoesNotOverrideStatic(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	name := xid.New().String()

	action := tmpl.NewAction(
		tmpl.WithCache(false),
		tmpl.WithNamespace(ns),
		tmpl.WithNamespaceFn(func(_ context.Context) (string, error) {
			return "", nil
		}),
	)

	rr := render.ReconciliationRequest{
		Instance:  testInstance(name),
		Templates: []render.TemplateInfo{{FS: testFS, Path: "resources/smm.tmpl.yaml"}},
	}

	err := action(ctx, &rr)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(1))
	g.Expect(rr.Resources[0].GetNamespace()).Should(Equal(ns))
}

//nolint:paralleltest
func TestRenderTemplateWithNamespaceFnNonEmptyOverridesStatic(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	staticNs := xid.New().String()
	dynamicNs := xid.New().String()
	name := xid.New().String()

	action := tmpl.NewAction(
		tmpl.WithCache(false),
		tmpl.WithNamespace(staticNs),
		tmpl.WithNamespaceFn(func(_ context.Context) (string, error) {
			return dynamicNs, nil
		}),
	)

	rr := render.ReconciliationRequest{
		Instance:  testInstance(name),
		Templates: []render.TemplateInfo{{FS: testFS, Path: "resources/smm.tmpl.yaml"}},
	}

	err := action(ctx, &rr)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(1))
	g.Expect(rr.Resources[0].GetNamespace()).Should(Equal(dynamicNs))
}

//nolint:paralleltest
func TestRenderTemplateWithNamespaceFnError(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()

	action := tmpl.NewAction(
		tmpl.WithCache(false),
		tmpl.WithNamespace("fallback"),
		tmpl.WithNamespaceFn(func(_ context.Context) (string, error) {
			return "", errComputeData
		}),
	)

	rr := render.ReconciliationRequest{
		Instance:  testInstance("test"),
		Templates: []render.TemplateInfo{{FS: testFS, Path: "resources/smm.tmpl.yaml"}},
	}

	err := action(ctx, &rr)

	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("unable to compute template namespace"))
}

//nolint:paralleltest
func TestRenderTemplateWithCache(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()

	action := tmpl.NewAction(
		tmpl.WithData(map[string]any{
			tmpl.AppNamespaceKey: ns,
		}),
	)

	render.RenderedResourcesTotal.Reset()

	inst := testInstance(ns)

	for i := range 3 {
		if i >= 1 {
			inst.SetGeneration(1)
		}

		rr := render.ReconciliationRequest{
			Instance:  inst,
			Templates: []render.TemplateInfo{{FS: testFS, Path: "resources/smm.tmpl.yaml"}},
		}

		err := action(ctx, &rr)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(rr.Resources).Should(HaveLen(1))
		g.Expect(rr.Resources[0].GetNamespace()).Should(Equal(ns))
		g.Expect(rr.Resources[0].GetAnnotations()).Should(HaveKeyWithValue("instance-name", ns))

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
func TestRenderTemplateWithGlob(t *testing.T) {
	ctx := t.Context()
	ns := xid.New().String()
	id := xid.New().String()

	action := tmpl.NewAction(
		tmpl.WithCache(false),
		tmpl.WithData(map[string]any{
			tmpl.AppNamespaceKey: ns,
		}),
	)

	t.Run("wildcard", func(t *testing.T) { //nolint:paralleltest
		gt := NewWithT(t)

		rr := render.ReconciliationRequest{
			Instance:  testInstance(id),
			Templates: []render.TemplateInfo{{FS: testFS, Path: "resources/g/*.yaml"}},
		}

		err := action(ctx, &rr)

		gt.Expect(err).ShouldNot(HaveOccurred())
		gt.Expect(rr.Resources).Should(HaveLen(2))

		for _, res := range rr.Resources {
			gt.Expect(res.GetNamespace()).Should(Equal(ns))

			data, _, _ := unstructured.NestedString(res.Object, "data", "app-namespace")
			gt.Expect(data).Should(Equal(ns))

			cname, _, _ := unstructured.NestedString(res.Object, "data", "component-name")
			gt.Expect(cname).Should(Equal(id))
		}
	})

	t.Run("named", func(t *testing.T) { //nolint:paralleltest
		gt := NewWithT(t)

		rr := render.ReconciliationRequest{
			Instance:  testInstance(id),
			Templates: []render.TemplateInfo{{FS: testFS, Path: "resources/g/sm-01.yaml"}},
		}

		err := action(ctx, &rr)

		gt.Expect(err).ShouldNot(HaveOccurred())
		gt.Expect(rr.Resources).Should(HaveLen(1))
		gt.Expect(rr.Resources[0].GetNamespace()).Should(Equal(ns))
	})
}

//nolint:paralleltest
func TestRenderTemplateWithCustomInfo(t *testing.T) { //nolint:funlen
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	id := xid.New().String()

	action := tmpl.NewAction(
		tmpl.WithCache(false),
		tmpl.WithActionLabel("label-foo", "foo-label"),
		tmpl.WithActionLabels(map[string]string{"labels-foo": "foo-labels"}),
		tmpl.WithActionLabel("label-override", "foo-override"),
		tmpl.WithActionAnnotation("annotation-foo", "foo-annotation"),
		tmpl.WithActionAnnotations(map[string]string{"annotations-foo": "foo-annotations"}),
		tmpl.WithActionAnnotation("annotation-override", "foo-override"),
		tmpl.WithData(map[string]any{
			tmpl.AppNamespaceKey: ns,
		}),
	)

	rr := render.ReconciliationRequest{
		Instance: testInstance(id),
		Templates: []render.TemplateInfo{
			{
				FS:   testFS,
				Path: "resources/g/sm-01.yaml",
				Labels: map[string]string{
					"custom-label-foo": "label-01",
					"label-override":   "label-01",
				},
			},
			{
				FS:   testFS,
				Path: "resources/g/sm-02.yaml",
				Annotations: map[string]string{
					"custom-annotation-foo": "annotation-02",
					"annotation-override":   "annotation-02",
				},
			},
		},
	}

	err := action(ctx, &rr)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(2))

	for _, res := range rr.Resources {
		g.Expect(res.GetNamespace()).Should(Equal(ns))
		g.Expect(res.GetLabels()).Should(HaveKeyWithValue("label-foo", "foo-label"))
		g.Expect(res.GetLabels()).Should(HaveKeyWithValue("labels-foo", "foo-labels"))
		g.Expect(res.GetLabels()).Should(HaveKey("label-override"))
		g.Expect(res.GetAnnotations()).Should(HaveKeyWithValue("annotation-foo", "foo-annotation"))
		g.Expect(res.GetAnnotations()).Should(HaveKeyWithValue("annotations-foo", "foo-annotations"))
		g.Expect(res.GetAnnotations()).Should(HaveKey("annotation-override"))
	}

	g.Expect(rr.Resources[0].GetLabels()).Should(HaveKeyWithValue("custom-label-foo", "label-01"))
	g.Expect(rr.Resources[0].GetLabels()).Should(HaveKeyWithValue("label-override", "label-01"))

	g.Expect(rr.Resources[1].GetAnnotations()).Should(HaveKeyWithValue("custom-annotation-foo", "annotation-02"))
	g.Expect(rr.Resources[1].GetAnnotations()).Should(HaveKeyWithValue("annotation-override", "annotation-02"))
}
