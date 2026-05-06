package deploy_test

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/deploy"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"

	. "github.com/onsi/gomega"
)

func TestNewDeployerDefaults(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	d := deploy.NewDeployer()
	g.Expect(d).ShouldNot(BeNil())
}

func TestWithApplyOrderSorts(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	input := []unstructured.Unstructured{
		makeObj("apps/v1", "Deployment", "ns", "deploy"),
		makeObj("apiextensions.k8s.io/v1", "CustomResourceDefinition", "", "crd"),
		makeObj("v1", "Namespace", "", "ns"),
	}

	sorted, err := resources.SortByApplyOrder(t.Context(), input)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(sorted).Should(HaveLen(3))
	g.Expect(sorted[0].GetKind()).Should(Equal("Namespace"))
	g.Expect(sorted[1].GetKind()).Should(Equal("CustomResourceDefinition"))
	g.Expect(sorted[2].GetKind()).Should(Equal("Deployment"))
}

func TestWithMergeStrategy(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	called := false
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	d := deploy.NewDeployer(
		deploy.WithMergeStrategy(gvk, func(existing, desired *unstructured.Unstructured) error {
			called = true
			return nil
		}),
	)

	g.Expect(d).ShouldNot(BeNil())
	g.Expect(called).Should(BeFalse())
}

func TestWithFieldOwner(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	d := deploy.NewDeployer(deploy.WithFieldOwner("my-controller"))
	g.Expect(d).ShouldNot(BeNil())
}

func TestWithCache(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	d := deploy.NewDeployer(deploy.WithCache())
	g.Expect(d).ShouldNot(BeNil())
}

func TestWithMode(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	d := deploy.NewDeployer(deploy.WithMode(deploy.ModePatch))
	g.Expect(d).ShouldNot(BeNil())
}

func TestWithLabelsAndAnnotations(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	d := deploy.NewDeployer(
		deploy.WithLabel("env", "test"),
		deploy.WithLabels(map[string]string{"app": "my-app"}),
		deploy.WithAnnotation("note", "hello"),
		deploy.WithAnnotations(map[string]string{"extra": "val"}),
	)
	g.Expect(d).ShouldNot(BeNil())
}

func makeObj(apiVersion, kind, namespace, name string) unstructured.Unstructured {
	obj := unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name},
	}}

	if namespace != "" {
		obj.SetNamespace(namespace)
	}

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		panic(fmt.Sprintf("invalid apiVersion %q in test fixture: %v", apiVersion, err))
	}

	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: kind})

	return obj
}
