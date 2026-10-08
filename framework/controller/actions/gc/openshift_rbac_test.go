package gc

import (
	"context"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	odhTypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
)

type stubDiscovery struct {
	discovery.DiscoveryInterface

	resources []*metav1.APIResourceList
}

func (s *stubDiscovery) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	return s.resources, nil
}

type stubController struct {
	discovery discovery.DiscoveryInterface
}

func (c *stubController) Owns(_ schema.GroupVersionKind) bool              { return false }
func (c *stubController) AddDynamicOwnedType(_ schema.GroupVersionKind)    {}
func (c *stubController) GetClient() client.Client                         { return nil }
func (c *stubController) GetDiscoveryClient() discovery.DiscoveryInterface { return c.discovery }
func (c *stubController) GetDynamicClient() dynamic.Interface              { return nil }
func (c *stubController) IsDynamicOwnershipEnabled() bool                  { return false }
func (c *stubController) IsExcludedFromDynamicOwnership(_ schema.GroupVersionKind) bool {
	return false
}

func TestPreferKubernetesRBACResources_DropsOpenShiftAliasesWhenKubernetesListableAndDeletable(t *testing.T) {
	t.Parallel()

	deletable := []resources.Resource{
		frameworkResource("apps", "deployments", "Deployment", true),
		frameworkResource(kubernetesRBACGroup, "clusterrolebindings", kindClusterRoleBinding, false),
		frameworkResource(openShiftAuthorizationGroup, "clusterrolebindings", kindClusterRoleBinding, false),
		frameworkResource(kubernetesRBACGroup, "roles", kindRole, true),
		frameworkResource(openShiftAuthorizationGroup, "roles", kindRole, true),
		frameworkResource(openShiftAuthorizationGroup, "rolebindingrestrictions", "RoleBindingRestriction", true),
	}
	listable := deletable

	got := preferKubernetesRBACResources(deletable, listable)
	if len(got) != 4 {
		t.Fatalf("expected 4 resources after filter, got %d", len(got))
	}

	found := map[string]bool{}

	for i := range got {
		gvk := got[i].GroupVersionKind()
		found[gvk.Group+"/"+gvk.Kind] = true

		if isOpenShiftRBACAlias(gvk) {
			t.Errorf("OpenShift RBAC alias %s should have been filtered", gvk)
		}
	}

	for _, key := range []string{
		"apps/Deployment",
		kubernetesRBACGroup + "/" + kindClusterRoleBinding,
		kubernetesRBACGroup + "/" + kindRole,
		openShiftAuthorizationGroup + "/RoleBindingRestriction",
	} {
		if !found[key] {
			t.Errorf("expected resource %s in filtered result", key)
		}
	}
}

func TestPreferKubernetesRBACResources_KeepsOpenShiftWhenKubernetesMissing(t *testing.T) {
	t.Parallel()

	deletable := []resources.Resource{
		frameworkResource(openShiftAuthorizationGroup, "clusterrolebindings", kindClusterRoleBinding, false),
		frameworkResource(openShiftAuthorizationGroup, "roles", kindRole, true),
		frameworkResource("apps", "deployments", "Deployment", true),
	}

	got := preferKubernetesRBACResources(deletable, deletable)
	if len(got) != 3 {
		t.Errorf("expected OpenShift RBAC types to be kept when Kubernetes twins are absent, got %d", len(got))
	}

	foundOpenShiftClusterRoleBinding := false
	foundOpenShiftRole := false

	for i := range got {
		gvk := got[i].GroupVersionKind()
		if gvk.Group == openShiftAuthorizationGroup && gvk.Kind == kindClusterRoleBinding {
			foundOpenShiftClusterRoleBinding = true
		}

		if gvk.Group == openShiftAuthorizationGroup && gvk.Kind == kindRole {
			foundOpenShiftRole = true
		}
	}

	if !foundOpenShiftClusterRoleBinding {
		t.Error("expected authorization.openshift.io/ClusterRoleBinding to be kept")
	}

	if !foundOpenShiftRole {
		t.Error("expected authorization.openshift.io/Role to be kept")
	}
}

func TestPreferKubernetesRBACResources_KeepsOpenShiftWhenKubernetesNotListable(t *testing.T) {
	t.Parallel()

	deletable := []resources.Resource{
		frameworkResource(kubernetesRBACGroup, "clusterrolebindings", kindClusterRoleBinding, false),
		frameworkResource(openShiftAuthorizationGroup, "clusterrolebindings", kindClusterRoleBinding, false),
		frameworkResource("apps", "deployments", "Deployment", true),
	}
	listable := []resources.Resource{
		frameworkResource(openShiftAuthorizationGroup, "clusterrolebindings", kindClusterRoleBinding, false),
		frameworkResource("apps", "deployments", "Deployment", true),
	}

	got := preferKubernetesRBACResources(deletable, listable)
	if len(got) != 2 {
		t.Fatalf("expected OpenShift kept and Kubernetes dropped when not listable, got %d", len(got))
	}

	foundOpenShift := false
	foundKubernetes := false

	for i := range got {
		gvk := got[i].GroupVersionKind()
		if gvk.Group == openShiftAuthorizationGroup && gvk.Kind == kindClusterRoleBinding {
			foundOpenShift = true
		}

		if gvk.Group == kubernetesRBACGroup && gvk.Kind == kindClusterRoleBinding {
			foundKubernetes = true
		}
	}

	if !foundOpenShift {
		t.Error("expected authorization.openshift.io/ClusterRoleBinding to be kept")
	}

	if foundKubernetes {
		t.Error("expected rbac.authorization.k8s.io/ClusterRoleBinding to be dropped to avoid Forbidden list")
	}
}

func TestPreferKubernetesRBACResources_KeepsBothWhenNeitherListable(t *testing.T) {
	t.Parallel()

	deletable := []resources.Resource{
		frameworkResource(kubernetesRBACGroup, "clusterrolebindings", kindClusterRoleBinding, false),
		frameworkResource(openShiftAuthorizationGroup, "clusterrolebindings", kindClusterRoleBinding, false),
	}
	listable := []resources.Resource{}

	got := preferKubernetesRBACResources(deletable, listable)
	if len(got) != 2 {
		t.Fatalf("expected both twins kept when neither is listable, got %d", len(got))
	}
}

func TestPreferKubernetesRBACResources_KeepsKubernetesWhenOpenShiftListableButNotDeletable(t *testing.T) {
	t.Parallel()

	// Kubernetes: delete only, not listable. OpenShift: list only, not deletable.
	// Neither path is list+delete, so OpenShift is not a usable fallback and
	// Kubernetes (the only deletable twin) is kept.
	deletable := []resources.Resource{
		frameworkResource(kubernetesRBACGroup, "clusterrolebindings", kindClusterRoleBinding, false),
	}
	listable := []resources.Resource{
		frameworkResource(openShiftAuthorizationGroup, "clusterrolebindings", kindClusterRoleBinding, false),
	}

	got := preferKubernetesRBACResources(deletable, listable)
	if len(got) != 1 {
		t.Fatalf("expected Kubernetes twin kept when OpenShift is not deletable, got %d", len(got))
	}

	gvk := got[0].GroupVersionKind()
	if gvk.Group != kubernetesRBACGroup || gvk.Kind != kindClusterRoleBinding {
		t.Fatalf("expected rbac.authorization.k8s.io/ClusterRoleBinding, got %s", gvk)
	}
}

func TestPreferKubernetesRBACResources_Empty(t *testing.T) {
	t.Parallel()

	if got := preferKubernetesRBACResources(nil, nil); got != nil {
		t.Errorf("expected nil input to return nil, got %#v", got)
	}

	if got := preferKubernetesRBACResources([]resources.Resource{}, nil); len(got) != 0 {
		t.Errorf("expected empty input to stay empty, got %d", len(got))
	}
}

func TestComputeDeletableTypes_SingleSelfSubjectRulesReview(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()

	err := authorizationv1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	ssrrCreates := 0
	cli := clientfake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				s, ok := obj.(*authorizationv1.SelfSubjectRulesReview)
				if !ok {
					t.Fatalf("unexpected create: %T", obj)
				}

				ssrrCreates++
				s.Status.ResourceRules = []authorizationv1.ResourceRule{{
					APIGroups: []string{kubernetesRBACGroup, openShiftAuthorizationGroup},
					Resources: []string{"clusterrolebindings"},
					Verbs:     []string{"list", "delete"},
				}}

				return nil
			},
		}).
		Build()

	action := &Action{
		namespaceFn: func(context.Context, *odhTypes.ReconciliationRequest) (string, error) {
			return "test-ns", nil
		},
	}
	rr := &odhTypes.ReconciliationRequest{
		Client: cli,
		Controller: &stubController{
			discovery: &stubDiscovery{resources: rbacAliasAPIResources()},
		},
	}

	got, err := action.computeDeletableTypes(t.Context(), rr)
	if err != nil {
		t.Fatalf("computeDeletableTypes: %v", err)
	}

	if ssrrCreates != 1 {
		t.Fatalf("expected exactly one SelfSubjectRulesReview, got %d", ssrrCreates)
	}

	foundKubernetes := false
	foundOpenShift := false

	for i := range got {
		gvk := got[i].GroupVersionKind()
		if gvk.Group == kubernetesRBACGroup && gvk.Kind == kindClusterRoleBinding {
			foundKubernetes = true
		}

		if gvk.Group == openShiftAuthorizationGroup && gvk.Kind == kindClusterRoleBinding {
			foundOpenShift = true
		}
	}

	if !foundKubernetes {
		t.Error("expected rbac.authorization.k8s.io/ClusterRoleBinding in result")
	}

	if foundOpenShift {
		t.Error("expected authorization.openshift.io/ClusterRoleBinding to be dropped")
	}
}

func TestSelectDeletableTypes_PrefersKubernetesWhenListAndDeleteAuthorized(t *testing.T) {
	t.Parallel()

	got, err := selectDeletableTypes(rbacAliasAPIResources(), []authorizationv1.ResourceRule{{
		APIGroups: []string{kubernetesRBACGroup, openShiftAuthorizationGroup},
		Resources: []string{"clusterrolebindings"},
		Verbs:     []string{"list", "delete"},
	}})
	if err != nil {
		t.Fatalf("selectDeletableTypes: %v", err)
	}

	foundKubernetes := false
	foundOpenShift := false

	for i := range got {
		gvk := got[i].GroupVersionKind()
		if gvk.Group == kubernetesRBACGroup && gvk.Kind == kindClusterRoleBinding {
			foundKubernetes = true
		}

		if gvk.Group == openShiftAuthorizationGroup && gvk.Kind == kindClusterRoleBinding {
			foundOpenShift = true
		}
	}

	if !foundKubernetes {
		t.Error("expected rbac.authorization.k8s.io/ClusterRoleBinding in result")
	}

	if foundOpenShift {
		t.Error("expected authorization.openshift.io/ClusterRoleBinding to be dropped")
	}
}

func TestSelectDeletableTypes_KeepsOpenShiftWhenKubernetesListIsResourceNamesScoped(t *testing.T) {
	t.Parallel()

	got, err := selectDeletableTypes(rbacAliasAPIResources(), []authorizationv1.ResourceRule{
		{
			APIGroups: []string{kubernetesRBACGroup},
			Resources: []string{"clusterrolebindings"},
			Verbs:     []string{"delete"},
		},
		{
			APIGroups:     []string{kubernetesRBACGroup},
			Resources:     []string{"clusterrolebindings"},
			Verbs:         []string{"list"},
			ResourceNames: []string{"specific-binding"},
		},
		{
			APIGroups: []string{openShiftAuthorizationGroup},
			Resources: []string{"clusterrolebindings"},
			Verbs:     []string{"list", "delete"},
		},
	})
	if err != nil {
		t.Fatalf("selectDeletableTypes: %v", err)
	}

	foundOpenShift := false
	foundKubernetes := false

	for i := range got {
		gvk := got[i].GroupVersionKind()
		if gvk.Group == openShiftAuthorizationGroup && gvk.Kind == kindClusterRoleBinding {
			foundOpenShift = true
		}

		if gvk.Group == kubernetesRBACGroup && gvk.Kind == kindClusterRoleBinding {
			foundKubernetes = true
		}
	}

	if !foundOpenShift {
		t.Error("expected OpenShift alias kept when Kubernetes list is ResourceNames-scoped")
	}

	if foundKubernetes {
		t.Error("expected Kubernetes twin dropped when not listable and OpenShift alias is available")
	}
}

func rbacAliasAPIResources() []*metav1.APIResourceList {
	verbs := []string{"list", "get", "watch", "create", "update", "patch", "delete"}

	return []*metav1.APIResourceList{
		{
			GroupVersion: kubernetesRBACGroup + "/v1",
			APIResources: []metav1.APIResource{
				{Name: "clusterrolebindings", Kind: kindClusterRoleBinding, Verbs: verbs},
			},
		},
		{
			GroupVersion: openShiftAuthorizationGroup + "/v1",
			APIResources: []metav1.APIResource{
				{Name: "clusterrolebindings", Kind: kindClusterRoleBinding, Verbs: verbs},
			},
		},
	}
}

func frameworkResource(group, resource, kind string, namespaced bool) resources.Resource {
	const version = "v1"

	scope := meta.RESTScopeRoot
	if namespaced {
		scope = meta.RESTScopeNamespace
	}

	return resources.Resource{
		RESTMapping: meta.RESTMapping{
			Resource: schema.GroupVersionResource{
				Group:    group,
				Version:  version,
				Resource: resource,
			},
			GroupVersionKind: schema.GroupVersionKind{
				Group:   group,
				Version: version,
				Kind:    kind,
			},
			Scope: scope,
		},
	}
}
