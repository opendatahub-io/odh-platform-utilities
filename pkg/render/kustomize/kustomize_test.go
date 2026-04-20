package kustomize_test

import (
	"context"
	"path"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/xid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/render"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/render/kustomize"

	. "github.com/onsi/gomega"
)

const testEngineKustomization = `
apiVersion: kustomize.config.k8s.io/v1beta1
resources:
- test-engine-cm.yaml
`

const testEngineConfigMap = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-engine-cm
data:
  foo: bar
`

//nolint:paralleltest
func TestEngine(t *testing.T) {
	g := NewWithT(t)
	id := xid.New().String()
	ns := xid.New().String()
	fs := filesys.MakeFsInMemory()

	e := kustomize.NewEngine(
		kustomize.WithEngineFS(fs),
	)

	_ = fs.MkdirAll(path.Join(id, kustomize.DefaultKustomizationFilePath))
	_ = fs.WriteFile(path.Join(id, kustomize.DefaultKustomizationFileName), []byte(testEngineKustomization))
	_ = fs.WriteFile(path.Join(id, "test-engine-cm.yaml"), []byte(testEngineConfigMap))

	r, err := e.Render(
		id,
		kustomize.WithNamespace(ns),
		kustomize.WithLabel("component.opendatahub.io/name", "foo"),
		kustomize.WithLabel("platform.opendatahub.io/namespace", ns),
		kustomize.WithAnnotations(map[string]string{
			"platform.opendatahub.io/release": "1.2.3",
			"platform.opendatahub.io/type":    "managed",
		}),
	)

	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(r).Should(HaveLen(1))
	g.Expect(r[0].GetNamespace()).Should(Equal(ns))
	g.Expect(r[0].GetLabels()).Should(HaveKeyWithValue("component.opendatahub.io/name", "foo"))
	g.Expect(r[0].GetLabels()).Should(HaveKeyWithValue("platform.opendatahub.io/namespace", ns))
	g.Expect(r[0].GetAnnotations()).Should(HaveKeyWithValue("platform.opendatahub.io/release", "1.2.3"))
	g.Expect(r[0].GetAnnotations()).Should(HaveKeyWithValue("platform.opendatahub.io/type", "managed"))
}

const testEngineKustomizationOrderLegacy = `
apiVersion: kustomize.config.k8s.io/v1beta1
sortOptions:
  order: legacy
resources:
- test-engine-cm.yaml
- test-engine-deployment.yaml
- test-engine-secrets.yaml
`

const testEngineKustomizationOrderLegacyCustom = `
apiVersion: kustomize.config.k8s.io/v1beta1
sortOptions:
  order: legacy
  legacySortOptions:
    orderFirst:
    - Secret
    - Deployment
    orderLast:
    - ConfigMap
resources:
- test-engine-cm.yaml
- test-engine-deployment.yaml
- test-engine-secrets.yaml
`

const testEngineKustomizationOrderFifo = `
apiVersion: kustomize.config.k8s.io/v1beta1
sortOptions:
  order: fifo
resources:
- test-engine-cm.yaml
- test-engine-deployment.yaml
- test-engine-secrets.yaml
`

const testEngineOrderConfigMap = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  foo: bar
`

//nolint:gosec
const testEngineOrderSecret = `
apiVersion: v1
kind: Secret
metadata:
  name: test-secrets
stringData:
  bar: baz
`

const testEngineOrderDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        volumeMounts:
        - name: config-volume
          mountPath: /etc/config
        - name: secrets-volume
          mountPath: /etc/secrets
      volumes:
        - name: config-volume
          configMap:
            name: test-cm
        - name: secrets-volume
          secret:
            name: test-secrets
`

//nolint:paralleltest
func TestEngineOrder(t *testing.T) {
	root := xid.New().String()

	fs := filesys.MakeFsInMemory()

	kustomizations := map[string]string{
		"legacy":  testEngineKustomizationOrderLegacy,
		"ordered": testEngineKustomizationOrderLegacyCustom,
		"fifo":    testEngineKustomizationOrderFifo,
	}

	for k, v := range kustomizations {
		t.Run(k, func(t *testing.T) { //nolint:paralleltest
			g := NewWithT(t)

			e := kustomize.NewEngine(
				kustomize.WithEngineFS(fs),
			)

			_ = fs.MkdirAll(path.Join(root, kustomize.DefaultKustomizationFilePath))
			_ = fs.WriteFile(path.Join(root, kustomize.DefaultKustomizationFileName), []byte(v))
			_ = fs.WriteFile(path.Join(root, "test-engine-cm.yaml"), []byte(testEngineOrderConfigMap))
			_ = fs.WriteFile(path.Join(root, "test-engine-secrets.yaml"), []byte(testEngineOrderSecret))
			_ = fs.WriteFile(path.Join(root, "test-engine-deployment.yaml"), []byte(testEngineOrderDeployment))

			r, err := e.Render(root)

			g.Expect(err).NotTo(HaveOccurred())

			switch k {
			case "legacy":
				g.Expect(r).Should(HaveLen(3))
				g.Expect(r[0].GetKind()).Should(Equal("ConfigMap"))
				g.Expect(r[1].GetKind()).Should(Equal("Secret"))
				g.Expect(r[2].GetKind()).Should(Equal("Deployment"))
			case "ordered":
				g.Expect(r).Should(HaveLen(3))
				g.Expect(r[0].GetKind()).Should(Equal("Secret"))
				g.Expect(r[1].GetKind()).Should(Equal("Deployment"))
				g.Expect(r[2].GetKind()).Should(Equal("ConfigMap"))
			case "fifo":
				g.Expect(r).Should(HaveLen(3))
				g.Expect(r[0].GetKind()).Should(Equal("ConfigMap"))
				g.Expect(r[1].GetKind()).Should(Equal("Deployment"))
				g.Expect(r[2].GetKind()).Should(Equal("Secret"))
			}
		})
	}
}

//nolint:paralleltest
func TestStandaloneRender(t *testing.T) {
	g := NewWithT(t)
	id := xid.New().String()
	ns := xid.New().String()
	fs := filesys.MakeFsInMemory()

	_ = fs.MkdirAll(path.Join(id, kustomize.DefaultKustomizationFilePath))
	_ = fs.WriteFile(path.Join(id, kustomize.DefaultKustomizationFileName), []byte(testEngineKustomization))
	_ = fs.WriteFile(path.Join(id, "test-engine-cm.yaml"), []byte(testEngineConfigMap))

	r, err := kustomize.Render(id,
		[]kustomize.EngineOptsFn{kustomize.WithEngineFS(fs)},
		kustomize.WithNamespace(ns),
		kustomize.WithLabel("app", "test"),
	)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(r).Should(HaveLen(1))
	g.Expect(r[0].GetNamespace()).Should(Equal(ns))
	g.Expect(r[0].GetLabels()).Should(HaveKeyWithValue("app", "test"))
}

const testActionKustomization = `
apiVersion: kustomize.config.k8s.io/v1beta1
resources:
- test-resources-cm.yaml
- test-resources-deployment-managed.yaml
- test-resources-deployment-unmanaged.yaml
- test-resources-deployment-forced.yaml
`

const testActionConfigMap = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  foo: bar
`

const testActionManaged = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment-managed
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
`

const testActionUnmanaged = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment-unmanaged
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
`

const testActionForced = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment-forced
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
`

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
func TestRenderResourcesAction(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	id := xid.New().String()
	fs := filesys.MakeFsInMemory()

	_ = fs.MkdirAll(path.Join(id, kustomize.DefaultKustomizationFilePath))
	_ = fs.WriteFile(path.Join(id, kustomize.DefaultKustomizationFileName), []byte(testActionKustomization))
	_ = fs.WriteFile(path.Join(id, "test-resources-cm.yaml"), []byte(testActionConfigMap))
	_ = fs.WriteFile(path.Join(id, "test-resources-deployment-managed.yaml"), []byte(testActionManaged))
	_ = fs.WriteFile(path.Join(id, "test-resources-deployment-unmanaged.yaml"), []byte(testActionUnmanaged))
	_ = fs.WriteFile(path.Join(id, "test-resources-deployment-forced.yaml"), []byte(testActionForced))

	action := kustomize.NewAction(
		[]kustomize.EngineOptsFn{
			kustomize.WithEngineFS(fs),
			kustomize.WithEngineRenderOpts(
				kustomize.WithLabel("component.opendatahub.io/name", "foo"),
				kustomize.WithLabel("platform.opendatahub.io/namespace", ns),
				kustomize.WithAnnotation("platform.opendatahub.io/release", "1.2.3"),
				kustomize.WithAnnotation("platform.opendatahub.io/type", "managed"),
			),
		},
		kustomize.WithCache(false),
		kustomize.WithActionNamespace(ns),
	)

	render.RenderedResourcesTotal.Reset()

	for i := 1; i < 3; i++ {
		rr := render.ReconciliationRequest{
			Instance:  testInstance(),
			Manifests: []render.ManifestInfo{{Path: id}},
		}

		err := action(ctx, &rr)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(rr.Generated).Should(BeTrue())
		g.Expect(rr.Resources).Should(HaveLen(4))

		for _, res := range rr.Resources {
			g.Expect(res.GetNamespace()).Should(Equal(ns))
			g.Expect(res.GetLabels()).Should(HaveKeyWithValue("component.opendatahub.io/name", "foo"))
			g.Expect(res.GetLabels()).Should(HaveKeyWithValue("platform.opendatahub.io/namespace", ns))
			g.Expect(res.GetAnnotations()).Should(HaveKeyWithValue("platform.opendatahub.io/release", "1.2.3"))
			g.Expect(res.GetAnnotations()).Should(HaveKeyWithValue("platform.opendatahub.io/type", "managed"))
		}

		rc := testutil.ToFloat64(render.RenderedResourcesTotal)
		g.Expect(rc).Should(BeNumerically("==", 4*i))
	}
}

const testCacheKustomization = `
apiVersion: kustomize.config.k8s.io/v1beta1
resources:
- test-resources-deployment.yaml
`

const testCacheDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment-managed
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
`

//nolint:paralleltest
func TestRenderResourcesWithCacheAction(t *testing.T) { //nolint:funlen
	g := NewWithT(t)

	ctx := t.Context()
	ns := xid.New().String()
	id := xid.New().String()
	fs := filesys.MakeFsInMemory()

	_ = fs.MkdirAll(path.Join(id, kustomize.DefaultKustomizationFilePath))
	_ = fs.WriteFile(path.Join(id, kustomize.DefaultKustomizationFileName), []byte(testCacheKustomization))
	_ = fs.WriteFile(path.Join(id, "test-resources-deployment.yaml"), []byte(testCacheDeployment))

	action := kustomize.NewAction(
		[]kustomize.EngineOptsFn{
			kustomize.WithEngineFS(fs),
			kustomize.WithEngineRenderOpts(
				kustomize.WithLabel("app.kubernetes.io/part-of", "foo"),
				kustomize.WithLabel("platform.opendatahub.io/namespace", ns),
				kustomize.WithAnnotation("platform.opendatahub.io/release", "1.2.3"),
				kustomize.WithAnnotation("platform.opendatahub.io/type", "managed"),
			),
		},
		kustomize.WithActionNamespace(ns),
	)

	render.RenderedResourcesTotal.Reset()

	inst := testInstance()

	for i := range 3 {
		if i >= 1 {
			inst.SetGeneration(1)
		}

		rr := render.ReconciliationRequest{
			Instance:  inst,
			Manifests: []render.ManifestInfo{{Path: id}},
		}

		err := action(ctx, &rr)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(rr.Resources).Should(HaveLen(1))
		g.Expect(rr.Resources[0].GetNamespace()).Should(Equal(ns))
		g.Expect(rr.Resources[0].GetLabels()).Should(HaveKeyWithValue("app.kubernetes.io/part-of", "foo"))
		g.Expect(rr.Resources[0].GetLabels()).Should(HaveKeyWithValue("platform.opendatahub.io/namespace", ns))
		g.Expect(rr.Resources[0].GetAnnotations()).Should(HaveKeyWithValue("platform.opendatahub.io/release", "1.2.3"))
		g.Expect(rr.Resources[0].GetAnnotations()).Should(HaveKeyWithValue("platform.opendatahub.io/type", "managed"))

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

// TestNamespaceFnChangeInvalidatesCache asserts the Kustomize action cache key
// includes the resolved application namespace. When WithActionNamespaceFn returns
// a different namespace on a later reconcile (same instance + manifests), the
// cacher must miss and re-render; a third reconcile with a stable namespace hits.
//
//nolint:paralleltest
func TestNamespaceFnChangeInvalidatesCache(t *testing.T) {
	g := NewWithT(t)

	ctx := t.Context()
	id := xid.New().String()
	ns1 := xid.New().String()
	ns2 := xid.New().String()
	fs := filesys.MakeFsInMemory()

	_ = fs.MkdirAll(path.Join(id, kustomize.DefaultKustomizationFilePath))
	_ = fs.WriteFile(path.Join(id, kustomize.DefaultKustomizationFileName), []byte(testCacheKustomization))
	_ = fs.WriteFile(path.Join(id, "test-resources-deployment.yaml"), []byte(testCacheDeployment))

	currentNS := ns1

	action := kustomize.NewAction(
		[]kustomize.EngineOptsFn{kustomize.WithEngineFS(fs)},
		kustomize.WithActionNamespaceFn(func(context.Context) (string, error) {
			return currentNS, nil
		}),
	)

	render.RenderedResourcesTotal.Reset()

	inst := testInstance()

	rr := render.ReconciliationRequest{
		Instance:  inst,
		Manifests: []render.ManifestInfo{{Path: id}},
	}

	err := action(ctx, &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Generated).Should(BeTrue())
	g.Expect(rr.Resources).Should(HaveLen(1))
	g.Expect(rr.Resources[0].GetNamespace()).Should(Equal(ns1))
	g.Expect(testutil.ToFloat64(render.RenderedResourcesTotal)).Should(BeNumerically("==", 1))

	currentNS = ns2

	rr2 := render.ReconciliationRequest{
		Instance:  inst,
		Manifests: []render.ManifestInfo{{Path: id}},
	}

	err = action(ctx, &rr2)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr2.Generated).Should(BeTrue())
	g.Expect(rr2.Resources[0].GetNamespace()).Should(Equal(ns2))
	g.Expect(testutil.ToFloat64(render.RenderedResourcesTotal)).Should(BeNumerically("==", 2))

	rr3 := render.ReconciliationRequest{
		Instance:  inst,
		Manifests: []render.ManifestInfo{{Path: id}},
	}

	err = action(ctx, &rr3)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr3.Generated).Should(BeFalse())
	g.Expect(rr3.Resources[0].GetNamespace()).Should(Equal(ns2))
	g.Expect(testutil.ToFloat64(render.RenderedResourcesTotal)).Should(BeNumerically("==", 2))
}
