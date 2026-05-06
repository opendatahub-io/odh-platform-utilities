package deploy_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/deploy"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func driftGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   appsv1.SchemeGroupVersion.Group,
		Version: appsv1.SchemeGroupVersion.Version,
		Kind:    "Deployment",
	}
}

func TestRevertDeploymentDriftRejectsNilClient(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := makeDriftDeployment("default", "nil-cli", nil, nil)
	old := makeDriftDeployment("default", "nil-cli", nil, nil)

	err := deploy.RevertDeploymentDrift(context.Background(), nil, obj, old)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(errors.Is(err, deploy.ErrNilArgument)).Should(BeTrue())
}

func TestRevertDeploymentDriftRejectsNilObj(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	patchCalled := false
	cli := newDriftClient(g, &patchCalled, nil)

	old := makeDriftDeployment("default", "nil-obj", nil, nil)

	err := deploy.RevertDeploymentDrift(context.Background(), cli, nil, old)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(errors.Is(err, deploy.ErrNilArgument)).Should(BeTrue())
}

func TestRevertDeploymentDriftRejectsNilOld(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	patchCalled := false
	cli := newDriftClient(g, &patchCalled, nil)

	obj := makeDriftDeployment("default", "nil-old", nil, nil)

	err := deploy.RevertDeploymentDrift(context.Background(), cli, obj, nil)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(errors.Is(err, deploy.ErrNilArgument)).Should(BeTrue())
}

func TestRevertDeploymentDriftRejectsNonDeploymentObj(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	patchCalled := false
	cli := newDriftClient(g, &patchCalled, nil)

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})
	obj.SetName("test")
	obj.SetNamespace("default")

	old := makeDriftDeployment("ns-a", "test", nil, nil)

	err := deploy.RevertDeploymentDrift(context.Background(), cli, obj, old)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(errors.Is(err, deploy.ErrNotDeployment)).Should(BeTrue())
}

func TestRevertDeploymentDriftRejectsNonDeploymentOld(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	patchCalled := false
	cli := newDriftClient(g, &patchCalled, nil)

	obj := makeDriftDeployment("ns-b", "deploy-b", nil, nil)

	old := &unstructured.Unstructured{}
	old.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})
	old.SetName("deploy-b")
	old.SetNamespace("ns-b")

	err := deploy.RevertDeploymentDrift(context.Background(), cli, obj, old)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(errors.Is(err, deploy.ErrNotDeployment)).Should(BeTrue())
}

func TestRevertDeploymentDriftNoOpWhenNoDrift(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	containers := []any{
		map[string]any{
			"name": "app",
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "500m"},
			},
		},
	}

	obj := makeDriftDeployment("default", "no-drift", containers, nil)
	old := makeDriftDeployment("default", "no-drift", containers, nil)

	patchCalled := false
	cli := newDriftClient(g, &patchCalled, nil)

	err := deploy.RevertDeploymentDrift(context.Background(), cli, obj, old)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(patchCalled).Should(BeFalse())
}

func TestRevertDeploymentDriftNoOpWithoutContainers(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	obj := makeDriftDeployment("default", "empty", nil, nil)
	old := makeDriftDeployment("default", "empty", nil, nil)

	patchCalled := false
	cli := newDriftClient(g, &patchCalled, nil)

	err := deploy.RevertDeploymentDrift(context.Background(), cli, obj, old)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(patchCalled).Should(BeFalse())
}

func TestRevertDeploymentDriftClearsResources(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	objContainers := []any{
		map[string]any{"name": "app"},
	}
	oldContainers := []any{
		map[string]any{
			"name": "app",
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "500m"},
			},
		},
	}

	obj := makeDriftDeployment("default", "clear-res", objContainers, nil)
	old := makeDriftDeployment("default", "clear-res", oldContainers, nil)

	patchCalled := false

	var capturedPatch map[string]any

	cli := newDriftClient(g, &patchCalled, &capturedPatch)

	err := deploy.RevertDeploymentDrift(context.Background(), cli, obj, old)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(patchCalled).Should(BeTrue())

	spec, ok := capturedPatch["spec"].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	template, ok := spec["template"].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	podSpec, ok := template["spec"].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	containers, ok := podSpec["containers"].([]any)
	g.Expect(ok).Should(BeTrue())
	g.Expect(containers).Should(HaveLen(1))

	container, ok := containers[0].(map[string]any)
	g.Expect(ok).Should(BeTrue())
	g.Expect(container["name"]).Should(Equal("app"))
	g.Expect(container["resources"]).Should(BeNil())
}

func TestRevertDeploymentDriftClearsReplicas(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	containers := []any{
		map[string]any{"name": "app"},
	}

	replicas := int64(3)
	obj := makeDriftDeployment("default", "clear-replicas", containers, nil)
	old := makeDriftDeployment("default", "clear-replicas", containers, &replicas)

	patchCalled := false

	var capturedPatch map[string]any

	cli := newDriftClient(g, &patchCalled, &capturedPatch)

	err := deploy.RevertDeploymentDrift(context.Background(), cli, obj, old)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(patchCalled).Should(BeTrue())

	spec, ok := capturedPatch["spec"].(map[string]any)
	g.Expect(ok).Should(BeTrue())
	g.Expect(spec["replicas"]).Should(BeNil())
}

func TestRevertDeploymentDriftClearsResourcesAndReplicas(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	objContainers := []any{
		map[string]any{"name": "app"},
	}
	oldContainers := []any{
		map[string]any{
			"name": "app",
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "500m"},
			},
		},
	}

	replicas := int64(3)
	obj := makeDriftDeployment("default", "both", objContainers, nil)
	old := makeDriftDeployment("default", "both", oldContainers, &replicas)

	patchCalled := false

	var capturedPatch map[string]any

	cli := newDriftClient(g, &patchCalled, &capturedPatch)

	err := deploy.RevertDeploymentDrift(context.Background(), cli, obj, old)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(patchCalled).Should(BeTrue())

	spec, ok := capturedPatch["spec"].(map[string]any)
	g.Expect(ok).Should(BeTrue())
	g.Expect(spec["replicas"]).Should(BeNil())
	g.Expect(spec).Should(HaveKey("template"))
}

func TestRevertDeploymentDriftClearsExtraResourceKeys(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	objContainers := []any{
		map[string]any{
			"name": "app",
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "500m"},
			},
		},
	}
	oldContainers := []any{
		map[string]any{
			"name": "app",
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "500m", "memory": "1Gi"},
			},
		},
	}

	obj := makeDriftDeployment("default", "extra-keys", objContainers, nil)
	old := makeDriftDeployment("default", "extra-keys", oldContainers, nil)

	patchCalled := false

	var capturedPatch map[string]any

	cli := newDriftClient(g, &patchCalled, &capturedPatch)

	err := deploy.RevertDeploymentDrift(context.Background(), cli, obj, old)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(patchCalled).Should(BeTrue())

	patchData := extractDriftPatchContainerResources(g, capturedPatch, 0)
	requests, ok := patchData["requests"].(map[string]any)
	g.Expect(ok).Should(BeTrue())
	g.Expect(requests["cpu"]).Should(Equal("500m"))
	g.Expect(requests["memory"]).Should(BeNil())
}

func TestRevertDeploymentDriftPropagatesPatchError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	objContainers := []any{
		map[string]any{"name": "app"},
	}
	oldContainers := []any{
		map[string]any{
			"name": "app",
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "500m"},
			},
		},
	}

	obj := makeDriftDeployment("default", "patch-err", objContainers, nil)
	old := makeDriftDeployment("default", "patch-err", oldContainers, nil)

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).Should(Succeed())

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(
				_ context.Context, _ client.WithWatch, _ client.Object,
				_ client.Patch, _ ...client.PatchOption,
			) error {
				return errPatchFailed
			},
		}).
		Build()

	err := deploy.RevertDeploymentDrift(context.Background(), cli, obj, old)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(errors.Is(err, errPatchFailed)).Should(BeTrue())
}

func TestRevertDeploymentDriftEmptyResourceMap(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	objContainers := []any{
		map[string]any{
			"name":      "app",
			"resources": map[string]any{},
		},
	}
	oldContainers := []any{
		map[string]any{
			"name": "app",
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "500m"},
			},
		},
	}

	obj := makeDriftDeployment("default", "empty-res", objContainers, nil)
	old := makeDriftDeployment("default", "empty-res", oldContainers, nil)

	patchCalled := false

	var capturedPatch map[string]any

	cli := newDriftClient(g, &patchCalled, &capturedPatch)

	err := deploy.RevertDeploymentDrift(context.Background(), cli, obj, old)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(patchCalled).Should(BeTrue())

	container := extractDriftPatchContainer(g, capturedPatch, 0)
	g.Expect(container["resources"]).Should(BeNil())
}

// --- Drift test helpers ---

var errPatchFailed = errors.New("patch failed")

func makeDriftDeployment(
	ns, name string,
	containers []any,
	replicas *int64,
) *unstructured.Unstructured {
	spec := map[string]any{}

	if containers != nil {
		spec["template"] = map[string]any{
			"spec": map[string]any{
				"containers": containers,
			},
		}
	}

	if replicas != nil {
		spec["replicas"] = *replicas
	}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": name, "namespace": ns},
		"spec":       spec,
	}}

	obj.SetGroupVersionKind(driftGVK())

	return obj
}

func newDriftClient(
	g Gomega,
	patchCalled *bool,
	capturedPatch *map[string]any,
) client.Client {
	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).Should(Succeed())

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(
				_ context.Context, _ client.WithWatch,
				obj client.Object, patch client.Patch,
				_ ...client.PatchOption,
			) error {
				*patchCalled = true

				if capturedPatch != nil {
					data, dataErr := patch.Data(obj)
					if dataErr != nil {
						return dataErr
					}

					var p map[string]any

					unmarshalErr := json.Unmarshal(data, &p)
					if unmarshalErr != nil {
						return unmarshalErr
					}

					*capturedPatch = p
				}

				return nil
			},
		}).
		Build()
}

func extractDriftPatchContainer(
	g Gomega,
	capturedPatch map[string]any,
	idx int,
) map[string]any {
	spec, ok := capturedPatch["spec"].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	template, ok := spec["template"].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	podSpec, ok := template["spec"].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	containers, ok := podSpec["containers"].([]any)
	g.Expect(ok).Should(BeTrue())
	g.Expect(containers).Should(HaveLen(idx + 1))

	container, ok := containers[idx].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	return container
}

func extractDriftPatchContainerResources(
	g Gomega,
	capturedPatch map[string]any,
	idx int,
) map[string]any {
	container := extractDriftPatchContainer(g, capturedPatch, idx)

	resources, ok := container["resources"].(map[string]any)
	g.Expect(ok).Should(BeTrue())

	return resources
}
