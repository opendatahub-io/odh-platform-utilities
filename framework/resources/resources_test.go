package resources_test

import (
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"

	. "github.com/onsi/gomega"
)

func TestHasAnnotationAndLabels(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]string
		key      string
		values   []string
		expected bool
	}{
		{"nil object", nil, "key1", []string{"val1"}, false},
		{"no metadata", map[string]string{}, "key1", []string{"val1"}, false},
		{"metadata exists and value matches", map[string]string{"key1": "val1"}, "key1", []string{"val1"}, true},
		{"metadata exists and value doesn't match", map[string]string{"key1": "val2"}, "key1", []string{"val1"}, false},
		{"metadata exists and value in list", map[string]string{"key1": "val2"}, "key1", []string{"val1", "val2"}, true},
		{"metadata exists and key doesn't match", map[string]string{"key2": "val1"}, "key1", []string{"val1"}, false},
		{"multiple values and no match", map[string]string{"key1": "val3"}, "key1", []string{"val1", "val2"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("annotations_"+tt.name, func(t *testing.T) {
				g := NewWithT(t)

				obj := unstructured.Unstructured{}
				if len(tt.data) != 0 {
					obj.SetAnnotations(tt.data)
				}

				result := resources.HasAnnotation(&obj, tt.key, tt.values...)

				g.Expect(result).To(Equal(tt.expected))
			})

			t.Run("labels_"+tt.name, func(t *testing.T) {
				g := NewWithT(t)

				obj := unstructured.Unstructured{}
				if len(tt.data) != 0 {
					obj.SetLabels(tt.data)
				}

				result := resources.HasLabel(&obj, tt.key, tt.values...)

				g.Expect(result).To(Equal(tt.expected))
			})
		})
	}
}

func TestGetGroupVersionKindForObject(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())

	t.Run("ObjectWithGVK", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(gvk.Deployment)

		gotGVK, err := resources.GetGroupVersionKindForObject(scheme, obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(gotGVK).To(Equal(gvk.Deployment))
	})

	t.Run("ObjectWithoutGVK_SuccessfulLookup", func(t *testing.T) {
		obj := &appsv1.Deployment{}

		gotGVK, err := resources.GetGroupVersionKindForObject(scheme, obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(gotGVK).To(Equal(gvk.Deployment))
	})

	t.Run("ObjectWithoutGVK_ErrorInLookup", func(t *testing.T) {
		obj := &unstructured.Unstructured{}

		_, err := resources.GetGroupVersionKindForObject(scheme, obj)
		g.Expect(err).To(WithTransform(
			errors.Unwrap,
			MatchError(runtime.IsMissingKind, "IsMissingKind"),
		))
	})

	t.Run("NilObject", func(t *testing.T) {
		_, err := resources.GetGroupVersionKindForObject(scheme, nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("nil object"))
	})
}

func TestEnsureGroupVersionKind(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())

	t.Run("ForObject", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(gvk.Deployment.GroupVersion().String())
		obj.SetKind(gvk.Deployment.Kind)

		err := resources.EnsureGroupVersionKind(scheme, obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj.GetObjectKind().GroupVersionKind()).To(Equal(gvk.Deployment))
	})

	t.Run("ErrorOnNilObject", func(t *testing.T) {
		err := resources.EnsureGroupVersionKind(scheme, nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("nil object"))
	})

	t.Run("ErrorOnInvalidObject", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetKind("UnknownKind")

		err := resources.EnsureGroupVersionKind(scheme, obj)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to get GVK"))
	})
}

func TestObjectToUnstructured(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())

	t.Run("ForObject", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(gvk.Deployment.GroupVersion().String())
		obj.SetKind(gvk.Deployment.Kind)

		u, err := resources.ObjectToUnstructured(scheme, obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(u.GetObjectKind().GroupVersionKind()).To(Equal(gvk.Deployment))
	})

	t.Run("ErrorOnNilObject", func(t *testing.T) {
		_, err := resources.ObjectToUnstructured(scheme, nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("nil object"))
	})

	t.Run("ErrorOnInvalidObject", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetKind("UnknownKind")

		_, err := resources.ObjectToUnstructured(scheme, obj)
		g.Expect(err).To(HaveOccurred())

		g.Expect(err.Error()).To(ContainSubstring("failed to get GVK"))
	})
}

func TestObjectFromUnstructured(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())

	t.Run("ForObject", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(gvk.Deployment.GroupVersion().String())
		obj.SetKind(gvk.Deployment.Kind)
		d := &appsv1.Deployment{}

		err := resources.ObjectFromUnstructured(scheme, obj, d)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(d.GetObjectKind().GroupVersionKind()).To(Equal(gvk.Deployment))
	})

	t.Run("ErrorOnNilObject", func(t *testing.T) {
		d := &appsv1.Deployment{}
		err := resources.ObjectFromUnstructured(scheme, nil, d)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("nil object"))
	})

	t.Run("ErrorOnInvalidObject", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(gvk.Deployment.GroupVersion().String())
		obj.SetKind("UnknownKind")
		d := &appsv1.Deployment{}

		err := resources.ObjectFromUnstructured(scheme, obj, d)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("unable to create object for GVK"))
	})
}

func TestFormatObjectReference(t *testing.T) {
	cases := []struct {
		name      string
		gvk       schema.GroupVersionKind
		namespace string
		objName   string
		expected  string
	}{
		{name: "namespaced", gvk: gvk.Deployment, namespace: "myns", objName: "mydeploy", expected: "apps/v1, Kind=Deployment myns/mydeploy"},
		{name: "cluster-scoped", gvk: gvk.ClusterRole, namespace: "", objName: "cluster-admin", expected: "rbac.authorization.k8s.io/v1, Kind=ClusterRole cluster-admin"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(tc.gvk)
			if tc.namespace != "" {
				u.SetNamespace(tc.namespace)
			}
			u.SetName(tc.objName)

			actual := resources.FormatObjectReference(u)
			if actual != tc.expected {
				t.Fatalf("unexpected reference: got %q, want %q", actual, tc.expected)
			}
		})
	}
}
