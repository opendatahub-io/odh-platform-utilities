package deploy_test

import (
	"context"
	"errors"
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

var errTestSort = errors.New("first failed")

func TestSortFnThen(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	first := deploy.SortFn(func(_ context.Context, in []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
		out := make([]unstructured.Unstructured, len(in))
		copy(out, in)

		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}

		return out, nil
	})

	second := deploy.SortFn(func(_ context.Context, in []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
		out := make([]unstructured.Unstructured, 0, len(in)+1)
		out = append(out, in...)
		out = append(out, makeObj("v1", "ConfigMap", "", "marker"))

		return out, nil
	})

	composed := first.Then(second)
	input := []unstructured.Unstructured{
		makeObj("v1", "Service", "", "svc"),
		makeObj("v1", "ConfigMap", "", "cm"),
	}

	result, err := composed(t.Context(), input)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(result).Should(HaveLen(3))
	g.Expect(result[0].GetName()).Should(Equal("cm"))
	g.Expect(result[1].GetName()).Should(Equal("svc"))
	g.Expect(result[2].GetName()).Should(Equal("marker"))
}

func TestSortFnThenFirstError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	failing := deploy.SortFn(func(_ context.Context, _ []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
		return nil, errTestSort
	})

	second := deploy.SortFn(func(_ context.Context, in []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
		return in, nil
	})

	composed := failing.Then(second)
	_, err := composed(t.Context(), nil)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("first failed"))
}

func TestSortFnThenSecondError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	first := deploy.SortFn(func(_ context.Context, in []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
		return in, nil
	})

	errSecond := errors.New("second failed") //nolint:err113
	failing := deploy.SortFn(func(_ context.Context, _ []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
		return nil, errSecond
	})

	composed := first.Then(failing)
	_, err := composed(t.Context(), nil)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err.Error()).Should(ContainSubstring("second failed"))
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
