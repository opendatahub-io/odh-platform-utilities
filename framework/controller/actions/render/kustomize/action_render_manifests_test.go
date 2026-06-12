package kustomize_test

import (
	"context"
	"path"
	"testing"

	"github.com/rs/xid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	common "github.com/opendatahub-io/odh-platform-utilities/api/common"
	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/render/kustomize"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	mk "github.com/opendatahub-io/odh-platform-utilities/framework/render/kustomize"

	. "github.com/onsi/gomega"
)

const testKustomization = `
apiVersion: kustomize.config.k8s.io/v1beta1
resources:
- configmap.yaml
- deployment.yaml
`

const testConfigMap = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  foo: bar
`

const testDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
`

type fakeInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	status api.Status
}

func (f *fakeInstance) GetStatus() *api.Status {
	return &f.status
}

func (f *fakeInstance) GetConditions() []api.Condition {
	return f.status.Conditions
}

func (f *fakeInstance) SetConditions(c []api.Condition) {
	f.status.Conditions = c
}

func (f *fakeInstance) GetReleaseStatus() *common.ComponentReleaseStatus {
	return nil
}

func (f *fakeInstance) SetReleaseStatus(_ common.ComponentReleaseStatus) {}

func (f *fakeInstance) DeepCopyObject() runtime.Object {
	o := *f
	return &o
}

func minimalInstance() api.PlatformObject {
	return &fakeInstance{
		TypeMeta:   metav1.TypeMeta{APIVersion: "test/v1", Kind: "Fake"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-instance", UID: k8stypes.UID("uid-1234"), Generation: 1},
	}
}

func setupFS(t *testing.T) (filesys.FileSystem, string) {
	t.Helper()

	fs := filesys.MakeFsInMemory()
	id := xid.New().String()

	_ = fs.MkdirAll(path.Join(id, mk.DefaultKustomizationFilePath))
	_ = fs.WriteFile(path.Join(id, mk.DefaultKustomizationFileName), []byte(testKustomization))
	_ = fs.WriteFile(path.Join(id, "configmap.yaml"), []byte(testConfigMap))
	_ = fs.WriteFile(path.Join(id, "deployment.yaml"), []byte(testDeployment))

	return fs, id
}

func TestRenderWithNamespace(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()
	fs, id := setupFS(t)
	ns := "test-ns"

	action := kustomize.NewAction(
		kustomize.WithCache(false),
		kustomize.WithNamespaceFn(func(_ context.Context, _ *types.ReconciliationRequest) (string, error) {
			return ns, nil
		}),
		kustomize.WithManifestsOptions(mk.WithEngineFS(fs)),
	)

	rr := &types.ReconciliationRequest{
		Instance:  minimalInstance(),
		Manifests: []types.ManifestInfo{{Path: id}},
	}

	err := action(ctx, rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(2))

	for _, r := range rr.Resources {
		g.Expect(r.GetNamespace()).Should(Equal(ns))
	}
}

func TestRenderWithLabelsAndAnnotations(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()
	fs, id := setupFS(t)

	action := kustomize.NewAction(
		kustomize.WithCache(false),
		kustomize.WithLabel("app", "test"),
		kustomize.WithAnnotation("version", "1.0"),
		kustomize.WithManifestsOptions(mk.WithEngineFS(fs)),
	)

	rr := &types.ReconciliationRequest{
		Instance:  minimalInstance(),
		Manifests: []types.ManifestInfo{{Path: id}},
	}

	err := action(ctx, rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(2))

	for _, r := range rr.Resources {
		g.Expect(r.GetLabels()).Should(HaveKeyWithValue("app", "test"))
		g.Expect(r.GetAnnotations()).Should(HaveKeyWithValue("version", "1.0"))
	}
}

func TestRenderPerManifestNamespaceOverride(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()

	fs := filesys.MakeFsInMemory()
	idA := xid.New().String()
	idB := xid.New().String()

	_ = fs.MkdirAll(path.Join(idA, mk.DefaultKustomizationFilePath))
	_ = fs.WriteFile(path.Join(idA, "cm.yaml"), []byte(testConfigMap))
	_ = fs.WriteFile(path.Join(idA, mk.DefaultKustomizationFileName), []byte(
		"apiVersion: kustomize.config.k8s.io/v1beta1\nresources:\n- cm.yaml\n",
	))

	_ = fs.MkdirAll(path.Join(idB, mk.DefaultKustomizationFilePath))
	_ = fs.WriteFile(path.Join(idB, "cm.yaml"), []byte(testConfigMap))
	_ = fs.WriteFile(path.Join(idB, mk.DefaultKustomizationFileName), []byte(
		"apiVersion: kustomize.config.k8s.io/v1beta1\nresources:\n- cm.yaml\n",
	))

	defaultNS := "default-ns"
	overrideNS := "override-ns"

	action := kustomize.NewAction(
		kustomize.WithCache(false),
		kustomize.WithNamespaceFn(func(_ context.Context, _ *types.ReconciliationRequest) (string, error) {
			return defaultNS, nil
		}),
		kustomize.WithManifestsOptions(mk.WithEngineFS(fs)),
	)

	rr := &types.ReconciliationRequest{
		Instance: minimalInstance(),
		Manifests: []types.ManifestInfo{
			{Path: idA},
			{Path: idB, Namespace: overrideNS},
		},
	}

	err := action(ctx, rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(2))

	g.Expect(rr.Resources[0].GetNamespace()).Should(Equal(defaultNS))
	g.Expect(rr.Resources[1].GetNamespace()).Should(Equal(overrideNS))
}

func TestRenderNoNamespaceFn(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()
	fs, id := setupFS(t)

	action := kustomize.NewAction(
		kustomize.WithCache(false),
		kustomize.WithManifestsOptions(mk.WithEngineFS(fs)),
	)

	rr := &types.ReconciliationRequest{
		Instance:  minimalInstance(),
		Manifests: []types.ManifestInfo{{Path: id}},
	}

	err := action(ctx, rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(rr.Resources).Should(HaveLen(2))
}

func TestRenderWithCache(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	ctx := context.Background()
	fs, id := setupFS(t)
	ns := "test-ns"

	action := kustomize.NewAction(
		kustomize.WithLabel("app", "cached"),
		kustomize.WithNamespaceFn(func(_ context.Context, _ *types.ReconciliationRequest) (string, error) {
			return ns, nil
		}),
		kustomize.WithManifestsOptions(mk.WithEngineFS(fs)),
	)

	for i := range 3 {
		inst := &fakeInstance{
			TypeMeta: metav1.TypeMeta{APIVersion: "test/v1", Kind: "Fake"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-instance",
				UID:  k8stypes.UID("uid-1234"),
			},
		}

		if i >= 1 {
			inst.Generation = 1
		}

		rr := &types.ReconciliationRequest{
			Instance:  inst,
			Manifests: []types.ManifestInfo{{Path: id}},
		}

		err := action(ctx, rr)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(rr.Resources).Should(HaveLen(2))

		for _, r := range rr.Resources {
			g.Expect(r.GetNamespace()).Should(Equal(ns))
			g.Expect(r.GetLabels()).Should(HaveKeyWithValue("app", "cached"))
		}

		switch i {
		case 0:
			g.Expect(rr.Generated).Should(BeTrue())
		case 1:
			g.Expect(rr.Generated).Should(BeTrue())
		case 2:
			g.Expect(rr.Generated).Should(BeFalse())
		}
	}
}
