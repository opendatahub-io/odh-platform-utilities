package gc_test

import (
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/controller/gc"
)

func TestIsResourceMatchingRule_ExactMatch(t *testing.T) {
	t.Parallel()

	rule := authorizationv1.ResourceRule{
		APIGroups: []string{""},
		Resources: []string{"configmaps"},
		Verbs:     []string{"delete"},
	}

	apiRes := metav1.APIResource{Name: "configmaps"}

	if !gc.IsResourceMatchingRule("", apiRes, rule) {
		t.Error("expected exact match to return true")
	}
}

func TestIsResourceMatchingRule_WildcardGroup(t *testing.T) {
	t.Parallel()

	rule := authorizationv1.ResourceRule{
		APIGroups: []string{"*"},
		Resources: []string{"configmaps"},
		Verbs:     []string{"delete"},
	}

	apiRes := metav1.APIResource{Name: "configmaps"}

	if !gc.IsResourceMatchingRule("apps", apiRes, rule) {
		t.Error("expected wildcard group to match any group")
	}
}

func TestIsResourceMatchingRule_WildcardResource(t *testing.T) {
	t.Parallel()

	rule := authorizationv1.ResourceRule{
		APIGroups: []string{""},
		Resources: []string{"*"},
		Verbs:     []string{"delete"},
	}

	apiRes := metav1.APIResource{Name: "secrets"}

	if !gc.IsResourceMatchingRule("", apiRes, rule) {
		t.Error("expected wildcard resource to match any resource")
	}
}

func TestIsResourceMatchingRule_NoMatch(t *testing.T) {
	t.Parallel()

	rule := authorizationv1.ResourceRule{
		APIGroups: []string{"apps"},
		Resources: []string{"deployments"},
		Verbs:     []string{"delete"},
	}

	apiRes := metav1.APIResource{Name: "configmaps"}

	if gc.IsResourceMatchingRule("", apiRes, rule) {
		t.Error("expected non-matching resource to return false")
	}
}

func TestHasPermissions_AllVerbs(t *testing.T) {
	t.Parallel()

	rules := []authorizationv1.ResourceRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"get", "list", "delete"},
		},
	}

	apiRes := metav1.APIResource{Name: "configmaps"}

	if !gc.HasPermissions("", apiRes, rules, []string{"delete"}) {
		t.Error("expected permission to be granted")
	}
}

func TestHasPermissions_MissingVerb(t *testing.T) {
	t.Parallel()

	rules := []authorizationv1.ResourceRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"get", "list"},
		},
	}

	apiRes := metav1.APIResource{Name: "configmaps"}

	if gc.HasPermissions("", apiRes, rules, []string{"delete"}) {
		t.Error("expected permission to be denied when verb is missing")
	}
}

func TestHasPermissions_WildcardVerb(t *testing.T) {
	t.Parallel()

	rules := []authorizationv1.ResourceRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"*"},
		},
	}

	apiRes := metav1.APIResource{Name: "configmaps"}

	if !gc.HasPermissions("", apiRes, rules, []string{"delete"}) {
		t.Error("expected wildcard verb to grant permission")
	}
}

func TestHasPermissions_EmptyVerbs(t *testing.T) {
	t.Parallel()

	rules := []authorizationv1.ResourceRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"delete"},
		},
	}

	apiRes := metav1.APIResource{Name: "configmaps"}

	if gc.HasPermissions("", apiRes, rules, nil) {
		t.Error("expected empty required verbs to return false")
	}
}

func TestHasPermissions_MultipleRequiredVerbs(t *testing.T) {
	t.Parallel()

	rules := []authorizationv1.ResourceRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"get", "delete"},
		},
	}

	apiRes := metav1.APIResource{Name: "configmaps"}

	if !gc.HasPermissions("", apiRes, rules, []string{"get", "delete"}) {
		t.Error("expected all required verbs to be satisfied")
	}

	if gc.HasPermissions("", apiRes, rules, []string{"get", "delete", "create"}) {
		t.Error("expected missing create verb to deny permission")
	}
}

func TestComputeAuthorizedResources_Basic(t *testing.T) {
	t.Parallel()

	resourceLists := []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "configmaps", Kind: "ConfigMap", Namespaced: true},
				{Name: "secrets", Kind: "Secret", Namespaced: true},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Namespaced: true},
			},
		},
	}

	rules := []authorizationv1.ResourceRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"delete"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments"},
			Verbs:     []string{"delete"},
		},
	}

	result, err := gc.ComputeAuthorizedResources(resourceLists, rules, []string{"delete"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 authorized resources, got %d", len(result))
	}
}

func TestComputeAuthorizedResources_ClusterScoped(t *testing.T) {
	t.Parallel()

	resourceLists := []*metav1.APIResourceList{
		{
			GroupVersion: "rbac.authorization.k8s.io/v1",
			APIResources: []metav1.APIResource{
				{Name: "clusterroles", Kind: "ClusterRole", Namespaced: false},
			},
		},
	}

	rules := []authorizationv1.ResourceRule{
		{
			APIGroups: []string{"rbac.authorization.k8s.io"},
			Resources: []string{"clusterroles"},
			Verbs:     []string{"delete"},
		},
	}

	result, err := gc.ComputeAuthorizedResources(resourceLists, rules, []string{"delete"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 authorized resource, got %d", len(result))
	}

	if result[0].IsNamespaced() {
		t.Error("expected ClusterRole to be cluster-scoped")
	}
}

func TestComputeAuthorizedResources_InvalidGroupVersion(t *testing.T) {
	t.Parallel()

	resourceLists := []*metav1.APIResourceList{
		{
			GroupVersion: "not/a/valid/gv",
			APIResources: []metav1.APIResource{
				{Name: "foos", Kind: "Foo"},
			},
		},
	}

	rules := []authorizationv1.ResourceRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
	}

	_, err := gc.ComputeAuthorizedResources(resourceLists, rules, []string{"delete"})
	if err == nil {
		t.Error("expected error for invalid group version")
	}
}

func TestComputeAuthorizedResources_NoPermissions(t *testing.T) {
	t.Parallel()

	resourceLists := []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "configmaps", Kind: "ConfigMap", Namespaced: true},
			},
		},
	}

	result, err := gc.ComputeAuthorizedResources(resourceLists, nil, []string{"delete"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Fatalf("expected 0 authorized resources, got %d", len(result))
	}
}
