package cache_test

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cachepkg "github.com/opendatahub-io/odh-platform-utilities/pkg/cache"

	. "github.com/onsi/gomega"
)

func TestByObjectBuilderBuildsScopedEntries(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	selector := labels.SelectorFromSet(labels.Set{"app.kubernetes.io/part-of": "mymodule"})

	nameSelector := fields.OneTermEqualSelector("metadata.name", "cluster")

	secret := &corev1.Secret{}
	configMap := &corev1.ConfigMap{}
	clusterRole := &rbacv1.ClusterRole{}
	namespace := &corev1.Namespace{}
	service := &corev1.Service{}

	byObject, err := cachepkg.NewByObjectBuilder().
		Add(secret, cachepkg.InNamespaces("app-ns", "monitoring-ns"), cachepkg.WithTransform(cachepkg.StripData())).
		Add(configMap, cachepkg.InNamespaces("app-ns"), cachepkg.WithLabelSelector(selector)).
		Add(clusterRole, cachepkg.WithLabelSelector(selector)).
		Add(namespace, cachepkg.WithFieldSelector(nameSelector)).
		Add(service, cachepkg.WithLabelSelector(selector), cachepkg.WithFieldSelector(nameSelector)).
		Build()
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(byObject).Should(HaveLen(5))

	// Field-only scope is valid on its own.
	g.Expect(byObject[namespace].Namespaces).Should(BeNil())
	g.Expect(byObject[namespace].Label).Should(BeNil())
	g.Expect(byObject[namespace].Field.String()).Should(Equal("metadata.name=cluster"))

	g.Expect(byObject[service].Label.String()).Should(Equal("app.kubernetes.io/part-of=mymodule"))
	g.Expect(byObject[service].Field.String()).Should(Equal("metadata.name=cluster"))
	g.Expect(byObject[configMap].Field).Should(BeNil())

	g.Expect(byObject[secret].Namespaces).Should(Equal(map[string]cache.Config{
		"app-ns":        {},
		"monitoring-ns": {},
	}))
	g.Expect(byObject[secret].Label).Should(BeNil())
	g.Expect(byObject[secret].Transform).ShouldNot(BeNil())

	g.Expect(byObject[configMap].Namespaces).Should(Equal(map[string]cache.Config{"app-ns": {}}))
	g.Expect(byObject[configMap].Label.String()).Should(Equal("app.kubernetes.io/part-of=mymodule"))
	g.Expect(byObject[configMap].Transform).Should(BeNil())

	// Cluster-scoped types must leave Namespaces nil.
	g.Expect(byObject[clusterRole].Namespaces).Should(BeNil())
	g.Expect(byObject[clusterRole].Label.String()).Should(Equal("app.kubernetes.io/part-of=mymodule"))
}

func TestByObjectBuilderEmptyBuild(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	byObject, err := cachepkg.NewByObjectBuilder().Build()
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(byObject).Should(BeEmpty())
}

type invalidScopeCase struct {
	obj     client.Object
	wantErr error
	name    string
	opts    []cachepkg.ScopeOption
}

func TestByObjectBuilderRejectsInvalidScopes(t *testing.T) {
	t.Parallel()

	for _, tt := range append(invalidScopeCases(), invalidFieldScopeCases()...) {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			byObject, err := cachepkg.NewByObjectBuilder().
				Add(&appsv1.Deployment{}, cachepkg.InNamespaces("valid")).
				Add(tt.obj, tt.opts...).
				Build()
			g.Expect(err).Should(MatchError(tt.wantErr))
			g.Expect(byObject).Should(BeNil())
		})
	}
}

func invalidScopeCases() []invalidScopeCase {
	selector := labels.SelectorFromSet(labels.Set{"app": "x"})

	return []invalidScopeCase{
		{
			name:    "nil object",
			obj:     nil,
			opts:    []cachepkg.ScopeOption{cachepkg.InNamespaces("ns")},
			wantErr: cachepkg.ErrNilObject,
		},
		{
			name:    "no scope",
			obj:     &corev1.Secret{},
			wantErr: cachepkg.ErrUnscoped,
		},
		{
			name:    "transform only",
			obj:     &corev1.Secret{},
			opts:    []cachepkg.ScopeOption{cachepkg.WithTransform(cachepkg.StripData())},
			wantErr: cachepkg.ErrUnscoped,
		},
		{
			name:    "no namespaces",
			obj:     &corev1.Secret{},
			opts:    []cachepkg.ScopeOption{cachepkg.InNamespaces()},
			wantErr: cachepkg.ErrNoNamespaces,
		},
		{
			name:    "all namespaces",
			obj:     &corev1.Secret{},
			opts:    []cachepkg.ScopeOption{cachepkg.InNamespaces("ns", cache.AllNamespaces)},
			wantErr: cachepkg.ErrAllNamespaces,
		},
		{
			name:    "nil selector",
			obj:     &corev1.Secret{},
			opts:    []cachepkg.ScopeOption{cachepkg.WithLabelSelector(nil)},
			wantErr: cachepkg.ErrEmptyLabelSelector,
		},
		{
			name:    "match-everything selector",
			obj:     &corev1.Secret{},
			opts:    []cachepkg.ScopeOption{cachepkg.WithLabelSelector(labels.Everything())},
			wantErr: cachepkg.ErrEmptyLabelSelector,
		},
		{
			name:    "valid namespace does not hide invalid selector",
			obj:     &corev1.Secret{},
			opts:    []cachepkg.ScopeOption{cachepkg.InNamespaces("ns"), cachepkg.WithLabelSelector(labels.Everything())},
			wantErr: cachepkg.ErrEmptyLabelSelector,
		},
		{
			name:    "valid selector does not hide invalid namespace",
			obj:     &corev1.Secret{},
			opts:    []cachepkg.ScopeOption{cachepkg.WithLabelSelector(selector), cachepkg.InNamespaces("")},
			wantErr: cachepkg.ErrAllNamespaces,
		},
	}
}

func invalidFieldScopeCases() []invalidScopeCase {
	selector := labels.SelectorFromSet(labels.Set{"app": "x"})

	return []invalidScopeCase{
		{
			name:    "nil field selector",
			obj:     &corev1.Secret{},
			opts:    []cachepkg.ScopeOption{cachepkg.WithFieldSelector(nil)},
			wantErr: cachepkg.ErrEmptyFieldSelector,
		},
		{
			name:    "match-everything field selector",
			obj:     &corev1.Secret{},
			opts:    []cachepkg.ScopeOption{cachepkg.WithFieldSelector(fields.Everything())},
			wantErr: cachepkg.ErrEmptyFieldSelector,
		},
		{
			name: "valid label selector does not hide invalid field selector",
			obj:  &corev1.Secret{},
			opts: []cachepkg.ScopeOption{
				cachepkg.WithLabelSelector(selector),
				cachepkg.WithFieldSelector(fields.Everything()),
			},
			wantErr: cachepkg.ErrEmptyFieldSelector,
		},
	}
}

func TestByObjectBuilderRejectsDuplicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		first  client.Object
		second client.Object
		name   string
	}{
		{
			name:   "typed",
			first:  &corev1.Secret{},
			second: &corev1.Secret{},
		},
		{
			name:   "unstructured same GVK",
			first:  newUnstructured(schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}),
			second: newUnstructured(schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}),
		},
		{
			name:   "metadata-only same GVK",
			first:  newPartial(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}),
			second: newPartial(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			_, err := cachepkg.NewByObjectBuilder().
				Add(tt.first, cachepkg.InNamespaces("ns-a")).
				Add(tt.second, cachepkg.InNamespaces("ns-b")).
				Build()
			g.Expect(err).Should(MatchError(cachepkg.ErrDuplicateObject))
		})
	}
}

func TestByObjectBuilderAllowsDistinctUnstructuredGVKs(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	byObject, err := cachepkg.NewByObjectBuilder().
		Add(newUnstructured(schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}),
			cachepkg.InNamespaces("ns")).
		Add(newUnstructured(schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Gadget"}),
			cachepkg.InNamespaces("ns")).
		Add(newPartial(schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}),
			cachepkg.InNamespaces("ns")).
		Build()
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(byObject).Should(HaveLen(3))
}

func TestByObjectBuilderOutputIsAcceptedByControllerRuntime(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())

	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Secret"), meta.RESTScopeNamespace)
	mapper.Add(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"), meta.RESTScopeRoot)
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Namespace"), meta.RESTScopeRoot)

	byObject, err := cachepkg.NewByObjectBuilder().
		Add(&corev1.Secret{}, cachepkg.InNamespaces("app-ns"), cachepkg.WithTransform(cachepkg.StripData())).
		Add(&rbacv1.ClusterRole{}, cachepkg.WithLabelSelector(labels.SelectorFromSet(labels.Set{"app": "x"}))).
		Add(&corev1.Namespace{}, cachepkg.WithFieldSelector(fields.OneTermEqualSelector("metadata.name", "app-ns"))).
		Build()
	g.Expect(err).ShouldNot(HaveOccurred())

	opts := cachepkg.NewOptions(cachepkg.WithStrictInformers())
	opts.Scheme = scheme
	opts.Mapper = mapper
	opts.ByObject = byObject

	_, err = cache.New(&rest.Config{Host: "https://127.0.0.1:1"}, opts)
	g.Expect(err).ShouldNot(HaveOccurred())

	// Negative control: namespace-scoping a cluster-scoped type must fail.
	invalid, err := cachepkg.NewByObjectBuilder().
		Add(&rbacv1.ClusterRole{}, cachepkg.InNamespaces("app-ns")).
		Build()
	g.Expect(err).ShouldNot(HaveOccurred())

	opts.ByObject = invalid
	_, err = cache.New(&rest.Config{Host: "https://127.0.0.1:1"}, opts)
	g.Expect(err).Should(MatchError(ContainSubstring("is not namespaced")))
}

func newUnstructured(gvk schema.GroupVersionKind) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)

	return u
}

func newPartial(gvk schema.GroupVersionKind) *metav1.PartialObjectMetadata {
	p := &metav1.PartialObjectMetadata{}
	p.SetGroupVersionKind(gvk)

	return p
}
