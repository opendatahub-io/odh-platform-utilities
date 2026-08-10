//nolint:testpackage
package dynamicownership

import (
	"context"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/predicates"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	. "github.com/onsi/gomega"
)

type stubPlatformObject struct {
	unstructured.Unstructured

	status api.Status
}

func (o *stubPlatformObject) GetStatus() *api.Status          { return &o.status }
func (o *stubPlatformObject) GetConditions() []api.Condition  { return o.status.Conditions }
func (o *stubPlatformObject) SetConditions(c []api.Condition) { o.status.Conditions = c }

var (
	configMapGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	ownerGVK     = schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "Owner"}
)

type stubController struct {
	dynamicOwned map[schema.GroupVersionKind]struct{}
}

func (c *stubController) Owns(_ schema.GroupVersionKind) bool { return false }
func (c *stubController) AddDynamicOwnedType(gvk schema.GroupVersionKind) {
	c.dynamicOwned[gvk] = struct{}{}
}
func (c *stubController) GetClient() client.Client                                      { return nil }
func (c *stubController) GetDiscoveryClient() discovery.DiscoveryInterface              { return nil }
func (c *stubController) GetDynamicClient() dynamic.Interface                           { return nil }
func (c *stubController) IsDynamicOwnershipEnabled() bool                               { return true }
func (c *stubController) IsExcludedFromDynamicOwnership(_ schema.GroupVersionKind) bool { return false }

func newStubController() *stubController {
	return &stubController{dynamicOwned: make(map[schema.GroupVersionKind]struct{})}
}

type watchRecord struct {
	gvk        schema.GroupVersionKind
	predicates []predicate.Predicate
}

func newTestClient() client.Client {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)

	mapper := meta.NewDefaultRESTMapper(s.PreferredVersionAllGroups())
	for gvk := range s.AllKnownTypes() {
		mapper.Add(gvk, meta.RESTScopeNamespace)
	}

	return fake.NewClientBuilder().WithScheme(s).WithRESTMapper(mapper).Build()
}

func makeResource(gvk schema.GroupVersionKind, name string) unstructured.Unstructured {
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName(name)
	u.SetNamespace("default")
	return u
}

func newTestInstance() *stubPlatformObject {
	obj := &stubPlatformObject{}
	obj.SetGroupVersionKind(ownerGVK)
	obj.SetName("test-owner")
	obj.SetNamespace("default")
	return obj
}

func TestWithDefaultPredicates_SetsField(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	p := predicate.Funcs{
		CreateFunc: func(_ event.TypedCreateEvent[client.Object]) bool { return false },
	}

	a := Action{
		gvkPredicates: make(map[schema.GroupVersionKind][]predicate.Predicate),
	}

	opt := WithDefaultPredicates(p)
	opt(&a)

	g.Expect(a.defaultPredicates).To(HaveLen(1))
	g.Expect(a.defaultPredicates[0].Create(event.TypedCreateEvent[client.Object]{})).To(BeFalse())
}

func TestWithDefaultPredicates_AcceptedByNewAction(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	p := predicate.Funcs{
		CreateFunc: func(_ event.TypedCreateEvent[client.Object]) bool { return false },
	}

	fn := NewAction(
		func(_ client.Object, _ handler.EventHandler, _ ...predicate.Predicate) error {
			return nil
		},
		ownerGVK,
		WithDefaultPredicates(p),
	)

	g.Expect(fn).ToNot(BeNil())
}

func TestPredicateResolution_GVKOverrideWinsForDeployment(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	gvkPred := predicate.Funcs{
		CreateFunc: func(_ event.TypedCreateEvent[client.Object]) bool { return true },
	}
	defaultPred := predicate.Funcs{
		CreateFunc: func(_ event.TypedCreateEvent[client.Object]) bool { return false },
	}

	var records []watchRecord
	a := Action{
		ownerGVK:              ownerGVK,
		managedByFalseMatcher: func(_ *unstructured.Unstructured) bool { return false },
		defaultPredicates:     []predicate.Predicate{defaultPred},
		gvkPredicates: map[schema.GroupVersionKind][]predicate.Predicate{
			deploymentGVK: {gvkPred},
		},
		watchRegisterFn: func(obj client.Object, _ handler.EventHandler, preds ...predicate.Predicate) error {
			records = append(records, watchRecord{gvk: obj.GetObjectKind().GroupVersionKind(), predicates: preds})
			return nil
		},
	}

	cli := newTestClient()
	rr := types.ReconciliationRequest{
		Client:     cli,
		Controller: newStubController(),
		Instance:   newTestInstance(),
		Resources: []unstructured.Unstructured{
			makeResource(deploymentGVK, "dep1"),
			makeResource(configMapGVK, "cm1"),
		},
	}

	err := a.run(context.Background(), &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(records).To(HaveLen(2))

	// Deployment: GVK override wins over DefaultDeploymentPredicate
	g.Expect(records[0].gvk).To(Equal(deploymentGVK))
	g.Expect(records[0].predicates).To(HaveLen(1))
	g.Expect(records[0].predicates[0].Create(event.TypedCreateEvent[client.Object]{})).To(BeTrue())

	// ConfigMap: defaultPredicates used (no GVK override, not a Deployment)
	g.Expect(records[1].gvk).To(Equal(configMapGVK))
	g.Expect(records[1].predicates).To(HaveLen(1))
	g.Expect(records[1].predicates[0].Create(event.TypedCreateEvent[client.Object]{})).To(BeFalse())
}

func TestPredicateResolution_DeploymentUsesBuiltInWhenNoGVKOverride(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	defaultPred := predicate.Funcs{
		CreateFunc: func(_ event.TypedCreateEvent[client.Object]) bool { return false },
	}

	var records []watchRecord
	a := Action{
		ownerGVK:              ownerGVK,
		managedByFalseMatcher: func(_ *unstructured.Unstructured) bool { return false },
		defaultPredicates:     []predicate.Predicate{defaultPred},
		gvkPredicates:         make(map[schema.GroupVersionKind][]predicate.Predicate),
		watchRegisterFn: func(obj client.Object, _ handler.EventHandler, preds ...predicate.Predicate) error {
			records = append(records, watchRecord{gvk: obj.GetObjectKind().GroupVersionKind(), predicates: preds})
			return nil
		},
	}

	cli := newTestClient()
	rr := types.ReconciliationRequest{
		Client:     cli,
		Controller: newStubController(),
		Instance:   newTestInstance(),
		Resources: []unstructured.Unstructured{
			makeResource(deploymentGVK, "dep1"),
		},
	}

	err := a.run(context.Background(), &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(records).To(HaveLen(1))

	// Deployment ignores defaultPredicates, uses built-in DefaultDeploymentPredicate
	g.Expect(records[0].gvk).To(Equal(deploymentGVK))
	g.Expect(records[0].predicates).To(HaveLen(1))
	g.Expect(records[0].predicates[0]).To(Equal(predicates.DefaultDeploymentPredicate))
}

func TestPredicateResolution_NoDefaultFallsToBuiltIn(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	var records []watchRecord
	a := Action{
		ownerGVK:              ownerGVK,
		managedByFalseMatcher: func(_ *unstructured.Unstructured) bool { return false },
		gvkPredicates:         make(map[schema.GroupVersionKind][]predicate.Predicate),
		watchRegisterFn: func(obj client.Object, _ handler.EventHandler, preds ...predicate.Predicate) error {
			records = append(records, watchRecord{gvk: obj.GetObjectKind().GroupVersionKind(), predicates: preds})
			return nil
		},
	}

	cli := newTestClient()
	rr := types.ReconciliationRequest{
		Client:     cli,
		Controller: newStubController(),
		Instance:   newTestInstance(),
		Resources: []unstructured.Unstructured{
			makeResource(configMapGVK, "cm1"),
		},
	}

	err := a.run(context.Background(), &rr)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(records).To(HaveLen(1))

	// No defaultPredicates, no GVK override → falls to DefaultPredicate
	g.Expect(records[0].gvk).To(Equal(configMapGVK))
	g.Expect(records[0].predicates).To(HaveLen(1))
	g.Expect(records[0].predicates[0]).To(Equal(predicates.DefaultPredicate))
}
