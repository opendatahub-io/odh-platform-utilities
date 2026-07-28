package deploy_test

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/deploy"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func newDeployScheme(g Gomega) *runtime.Scheme {
	s := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(s)).Should(Succeed())

	return s
}

func TestDeployCreatesNewResource(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	cm := makeObj("v1", "ConfigMap", "default", "test-cm")

	d := deploy.NewDeployer(deploy.WithMode(deploy.ModePatch))
	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Resources: []unstructured.Unstructured{cm},
	})
	g.Expect(err).ShouldNot(HaveOccurred())

	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})

	err = cli.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "test-cm"}, result)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.GetName()).Should(Equal("test-cm"))
	g.Expect(result.GetNamespace()).Should(Equal("default"))
}

func TestDeploySkipsExistingWithManagedFalse(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)

	existing := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "test-cm",
			"namespace": "default",
			"annotations": map[string]any{
				annotations.ManagedByODHOperator: "false",
			},
		},
		"data": map[string]any{"key": "original"},
	}}

	existing.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	resource := makeObj("v1", "ConfigMap", "default", "test-cm")

	d := deploy.NewDeployer(deploy.WithMode(deploy.ModePatch))
	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Resources: []unstructured.Unstructured{resource},
	})
	g.Expect(err).ShouldNot(HaveOccurred())

	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})

	err = cli.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "test-cm"}, result)
	g.Expect(err).ShouldNot(HaveOccurred())

	data, _, err := unstructured.NestedString(result.Object, "data", "key")
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(data).Should(Equal("original"))
}

func TestDeployStampsMetadata(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)

	var created *unstructured.Unstructured

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(
				ctx context.Context, c client.WithWatch,
				obj client.Object, opts ...client.CreateOption,
			) error {
				if u, ok := obj.(*unstructured.Unstructured); ok {
					created = u.DeepCopy()
				}

				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	owner := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-controller",
			Namespace:  "default",
			UID:        "test-uid-123",
			Generation: 5,
		},
	}

	cm := makeObj("v1", "ConfigMap", "default", "test-cm")

	d := deploy.NewDeployer(
		deploy.WithMode(deploy.ModePatch),
		deploy.WithLabel("env", "test"),
		deploy.WithAnnotation("note", "hello"),
		deploy.WithFieldOwner("my-controller"),
	)

	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Owner:     owner,
		Release:   deploy.ReleaseInfo{Type: "managed", Version: "2.0"},
		Resources: []unstructured.Unstructured{cm},
	})
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(created).ShouldNot(BeNil())

	g.Expect(created.GetLabels()).Should(HaveKeyWithValue("env", "test"))
	g.Expect(created.GetLabels()).Should(HaveKeyWithValue(labels.PlatformPartOf, "my-controller"))

	ann := created.GetAnnotations()
	g.Expect(ann).Should(HaveKeyWithValue("note", "hello"))
	g.Expect(ann).Should(HaveKeyWithValue(annotations.InstanceGeneration, "5"))
	g.Expect(ann).Should(HaveKeyWithValue(annotations.InstanceName, "my-controller"))
	g.Expect(ann).Should(HaveKeyWithValue(annotations.InstanceUID, "test-uid-123"))
	g.Expect(ann).Should(HaveKeyWithValue(annotations.PlatformType, "managed"))
	g.Expect(ann).Should(HaveKeyWithValue(annotations.PlatformVersion, "2.0"))
}

func TestDeployUnsupportedModeReturnsError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	cm := makeObj("v1", "ConfigMap", "default", "test-cm")

	d := deploy.NewDeployer(deploy.WithMode("invalid"))
	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Resources: []unstructured.Unstructured{cm},
	})
	g.Expect(err).Should(HaveOccurred())
	g.Expect(errors.Is(err, deploy.ErrUnsupportedMode)).Should(BeTrue())
}

func TestDeployInvokesSortFn(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	sortCalled := false
	scheme := newDeployScheme(g)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	d := deploy.NewDeployer(
		deploy.WithMode(deploy.ModePatch),
		deploy.WithSortFn(func(_ context.Context, res []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
			sortCalled = true

			return res, nil
		}),
	)

	cm := makeObj("v1", "ConfigMap", "default", "test-cm")
	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Resources: []unstructured.Unstructured{cm},
	})
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(sortCalled).Should(BeTrue())
}

func TestDeploySortErrorPropagated(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	sortErr := errors.New("sort failed") //nolint:err113

	d := deploy.NewDeployer(
		deploy.WithMode(deploy.ModePatch),
		deploy.WithSortFn(func(_ context.Context, _ []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
			return nil, sortErr
		}),
	)

	cm := makeObj("v1", "ConfigMap", "default", "test-cm")
	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Resources: []unstructured.Unstructured{cm},
	})
	g.Expect(err).Should(HaveOccurred())
	g.Expect(errors.Is(err, sortErr)).Should(BeTrue())
}

func TestDeployInvokesMergeStrategy(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)

	existing := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "test-cm", "namespace": "default"},
		"data":       map[string]any{"key": "value"},
	}}
	existing.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(
				_ context.Context, _ client.WithWatch, _ client.Object,
				_ client.Patch, _ ...client.PatchOption,
			) error {
				return nil
			},
		}).
		Build()

	mergeCalled := false
	cmGVK := schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}

	d := deploy.NewDeployer(
		deploy.WithMode(deploy.ModePatch),
		deploy.WithMergeStrategy(cmGVK, func(_, _ *unstructured.Unstructured) error {
			mergeCalled = true

			return nil
		}),
	)

	cm := makeObj("v1", "ConfigMap", "default", "test-cm")
	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Resources: []unstructured.Unstructured{cm},
	})
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(mergeCalled).Should(BeTrue())
}

func TestDeployWithCacheSkipsUnchangedResource(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)

	createCount := 0
	patchCount := 0
	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(
				ctx context.Context, c client.WithWatch,
				obj client.Object, opts ...client.CreateOption,
			) error {
				createCount++

				return c.Create(ctx, obj, opts...)
			},
			Patch: func(
				_ context.Context, _ client.WithWatch,
				_ client.Object, _ client.Patch,
				_ ...client.PatchOption,
			) error {
				patchCount++

				return nil
			},
		}).
		Build()

	cm := makeObj("v1", "ConfigMap", "default", "test-cm")

	d := deploy.NewDeployer(
		deploy.WithMode(deploy.ModePatch),
		deploy.WithCache(),
	)

	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Resources: []unstructured.Unstructured{cm},
	})
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(createCount).Should(Equal(1))

	beforeCreate := createCount
	beforePatch := patchCount

	err = d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Resources: []unstructured.Unstructured{cm},
	})
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(createCount).Should(Equal(beforeCreate))
	g.Expect(patchCount).Should(Equal(beforePatch))
}

func TestDeployMultipleResources(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	resources := []unstructured.Unstructured{
		makeObj("v1", "ConfigMap", "default", "cm-1"),
		makeObj("v1", "ConfigMap", "default", "cm-2"),
		makeObj("v1", "ConfigMap", "default", "cm-3"),
	}

	d := deploy.NewDeployer(deploy.WithMode(deploy.ModePatch))
	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Resources: resources,
	})
	g.Expect(err).ShouldNot(HaveOccurred())

	for _, name := range []string{"cm-1", "cm-2", "cm-3"} {
		result := &unstructured.Unstructured{}
		result.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})

		err = cli.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: name}, result)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(result.GetName()).Should(Equal(name))
	}
}

func TestDeployNamespaceExcludedFromOwnershipByDefault(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	owner := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner",
			Namespace: "default",
			UID:       "owner-uid",
		},
	}

	ns := makeObj("v1", "Namespace", "", "opendatahub")

	d := deploy.NewDeployer(deploy.WithMode(deploy.ModePatch))
	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Owner:     owner,
		Resources: []unstructured.Unstructured{ns},
	})
	g.Expect(err).ShouldNot(HaveOccurred())

	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"})
	err = cli.Get(context.Background(), client.ObjectKey{Name: "opendatahub"}, result)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.GetOwnerReferences()).Should(BeEmpty())
}

func TestDeployWithExcludeFromOwnershipSkipsConfiguredGVK(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	owner := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner",
			Namespace: "default",
			UID:       "owner-uid",
		},
	}

	gvk := schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	cm := makeObj("v1", "ConfigMap", "default", "test-cm")

	d := deploy.NewDeployer(
		deploy.WithMode(deploy.ModePatch),
		deploy.WithExcludeFromOwnership(gvk),
	)

	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Owner:     owner,
		Resources: []unstructured.Unstructured{cm},
	})
	g.Expect(err).ShouldNot(HaveOccurred())

	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(gvk)
	err = cli.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "test-cm"}, result)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result.GetOwnerReferences()).Should(BeEmpty())
}

func TestDeployWithExcludeFromOwnershipIsAdditive(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	owner := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner",
			Namespace: "default",
			UID:       "owner-uid",
		},
	}

	customGVK := schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	cm := makeObj("v1", "ConfigMap", "default", "excluded-cm")
	ns := makeObj("v1", "Namespace", "", "test-ns")

	d := deploy.NewDeployer(
		deploy.WithMode(deploy.ModePatch),
		deploy.WithExcludeFromOwnership(customGVK),
	)

	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Owner:     owner,
		Resources: []unstructured.Unstructured{cm, ns},
	})
	g.Expect(err).ShouldNot(HaveOccurred())

	resultCM := &unstructured.Unstructured{}
	resultCM.SetGroupVersionKind(customGVK)
	err = cli.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "excluded-cm"}, resultCM)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(resultCM.GetOwnerReferences()).Should(BeEmpty(), "custom-excluded GVK should have no owner ref")

	resultNS := &unstructured.Unstructured{}
	resultNS.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"})
	err = cli.Get(context.Background(), client.ObjectKey{Name: "test-ns"}, resultNS)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(resultNS.GetOwnerReferences()).Should(BeEmpty(), "default-excluded Namespace should still have no owner ref")
}

func TestDeployExcludedResourcePreservesExistingOwnerReferences(t *testing.T) {
	t.Parallel()

	modes := []struct {
		name string
		mode deploy.Mode
	}{
		{"Patch", deploy.ModePatch},
		{"SSA", deploy.ModeSSA},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			scheme := newDeployScheme(g)

			preExistingRef := metav1.OwnerReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "unrelated-owner",
				UID:        "unrelated-uid",
			}

			existing := makeObj("v1", "Namespace", "", "opendatahub")
			existing.SetOwnerReferences([]metav1.OwnerReference{preExistingRef})

			cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&existing).Build()

			owner := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "owner",
					Namespace: "default",
					UID:       "owner-uid",
				},
			}

			ns := makeObj("v1", "Namespace", "", "opendatahub")

			d := deploy.NewDeployer(deploy.WithMode(tc.mode))
			err := d.Deploy(context.Background(), deploy.DeployInput{
				Client:    cli,
				Owner:     owner,
				Resources: []unstructured.Unstructured{ns},
			})
			g.Expect(err).ShouldNot(HaveOccurred())

			result := &unstructured.Unstructured{}
			result.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"})
			err = cli.Get(context.Background(), client.ObjectKey{Name: "opendatahub"}, result)
			g.Expect(err).ShouldNot(HaveOccurred())

			refs := result.GetOwnerReferences()
			g.Expect(refs).Should(HaveLen(1))
			g.Expect(string(refs[0].UID)).Should(Equal("unrelated-uid"))
			g.Expect(refs[0].Name).Should(Equal("unrelated-owner"))
		})
	}
}

func TestDeployNonExcludedResourceStillGetsOwnerReference(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := newDeployScheme(g)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	owner := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner",
			Namespace: "default",
			UID:       "owner-uid",
		},
	}

	cm := makeObj("v1", "ConfigMap", "default", "owned-cm")

	d := deploy.NewDeployer(deploy.WithMode(deploy.ModePatch))
	err := d.Deploy(context.Background(), deploy.DeployInput{
		Client:    cli,
		Owner:     owner,
		Resources: []unstructured.Unstructured{cm},
	})
	g.Expect(err).ShouldNot(HaveOccurred())

	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})
	err = cli.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "owned-cm"}, result)
	g.Expect(err).ShouldNot(HaveOccurred())

	refs := result.GetOwnerReferences()
	g.Expect(refs).Should(HaveLen(1))
	g.Expect(string(refs[0].UID)).Should(Equal("owner-uid"))
	g.Expect(refs[0].Controller).ShouldNot(BeNil())
	g.Expect(*refs[0].Controller).Should(BeTrue())
}
