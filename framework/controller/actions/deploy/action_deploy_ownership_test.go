package deploy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	odhTypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
)

// ownsEverythingController overrides the shared stub so every GVK reports
// as statically owned. This models the ARCH-004 hazard: a module that
// registered Owns()/OwnsGVK() on the Namespace GVK.
type ownsEverythingController struct {
	stubController
}

func (c *ownsEverythingController) Owns(schema.GroupVersionKind) bool { return true }

// TestDeployNeverOwnsNamespace pins the ARCH-004 guard: even when static
// ownership (Owns/OwnsGVK) covers the Namespace GVK — the path documented
// to take precedence over the dynamic-ownership exclusion — the deploy
// action must not set a controller ownerReference on a Namespace, because
// deleting the owning CR cascade-deletes the entire namespace. The
// Namespace must still be deployed, and owned non-Namespace objects in the
// same reconcile must still get their controller reference.
func TestDeployNeverOwnsNamespace(t *testing.T) {
	t.Parallel()

	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))

	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(gvk.Namespace, meta.RESTScopeRoot)
	mapper.Add(gvk.ConfigMap, meta.RESTScopeNamespace)

	cli := ctrlfake.NewClientBuilder().
		WithScheme(s).
		WithRESTMapper(mapper).
		Build()

	// Cluster-scoped instance: the documented module shape, and the case
	// where a Namespace ownerReference is valid and the cascade completes.
	instance := &stubInstance{}
	instance.SetGroupVersionKind(schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "TestComponent"})
	instance.SetName("test-instance")

	ns := unstructured.Unstructured{}
	ns.SetGroupVersionKind(gvk.Namespace)
	ns.SetName("probe-ns")
	ns.SetLabels(map[string]string{"test.opendatahub.io/probe": "ownership"})

	cm := unstructured.Unstructured{}
	cm.SetGroupVersionKind(gvk.ConfigMap)
	cm.SetNamespace("probe-ns")
	cm.SetName("probe-cm")

	rr := &odhTypes.ReconciliationRequest{
		Client:     cli,
		Controller: &ownsEverythingController{stubController{client: cli}},
		Instance:   instance,
		Resources:  []unstructured.Unstructured{ns, cm},
	}

	require.NoError(t, deploy.NewAction()(t.Context(), rr))

	storedNS := resources.GvkToUnstructured(gvk.Namespace)
	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: "probe-ns"}, storedNS))
	assert.Empty(t, storedNS.GetOwnerReferences(),
		"a Namespace must never carry a controller ownerReference (whole-namespace cascade deletion)")

	storedCM := resources.GvkToUnstructured(gvk.ConfigMap)
	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Namespace: "probe-ns", Name: "probe-cm"}, storedCM))
	ownerRefs := storedCM.GetOwnerReferences()
	require.Len(t, ownerRefs, 1)
	assert.Equal(t, "TestComponent", ownerRefs[0].Kind)
	assert.True(t, *ownerRefs[0].Controller)
}

// dynamicOwnershipExcludingController simulates a controller built with
// dynamic ownership enabled and a GVK excluded from it, as produced by
// ReconcilerBuilder.WithUnownableGVKs however dynamic ownership is enabled.
type dynamicOwnershipExcludingController struct {
	stubController

	excluded map[schema.GroupVersionKind]struct{}
}

func (c *dynamicOwnershipExcludingController) IsDynamicOwnershipEnabled() bool { return true }

func (c *dynamicOwnershipExcludingController) IsExcludedFromDynamicOwnership(g schema.GroupVersionKind) bool {
	_, ok := c.excluded[g]
	return ok
}

// TestDeployExcludedGVKGetsNoOwnerReferenceUnderDynamicOwnership pins the
// dynamic-ownership branch of shouldOwn end to end: with dynamic ownership
// enabled, a GVK excluded from it must not receive a controller
// ownerReference, while an allowed GVK in the same reconcile still gets
// exactly one. This is the runtime decision the builder feeds through
// withOwnershipExclusions for GVKs declared via WithUnownableGVKs.
func TestDeployExcludedGVKGetsNoOwnerReferenceUnderDynamicOwnership(t *testing.T) {
	t.Parallel()

	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))

	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(gvk.Secret, meta.RESTScopeNamespace)
	mapper.Add(gvk.ConfigMap, meta.RESTScopeNamespace)

	cli := ctrlfake.NewClientBuilder().
		WithScheme(s).
		WithRESTMapper(mapper).
		Build()

	instance := &stubInstance{}
	instance.SetGroupVersionKind(schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "TestComponent"})
	instance.SetName("test-instance")

	secret := unstructured.Unstructured{}
	secret.SetGroupVersionKind(gvk.Secret)
	secret.SetNamespace("probe-ns")
	secret.SetName("probe-secret")

	cm := unstructured.Unstructured{}
	cm.SetGroupVersionKind(gvk.ConfigMap)
	cm.SetNamespace("probe-ns")
	cm.SetName("probe-cm")

	rr := &odhTypes.ReconciliationRequest{
		Client: cli,
		Controller: &dynamicOwnershipExcludingController{
			stubController: stubController{client: cli},
			excluded:       map[schema.GroupVersionKind]struct{}{gvk.Secret: {}},
		},
		Instance:  instance,
		Resources: []unstructured.Unstructured{secret, cm},
	}

	require.NoError(t, deploy.NewAction()(t.Context(), rr))

	storedSecret := resources.GvkToUnstructured(gvk.Secret)
	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Namespace: "probe-ns", Name: "probe-secret"}, storedSecret))
	assert.Empty(t, storedSecret.GetOwnerReferences(),
		"a GVK excluded from dynamic ownership must not receive a controller ownerReference")

	storedCM := resources.GvkToUnstructured(gvk.ConfigMap)
	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Namespace: "probe-ns", Name: "probe-cm"}, storedCM))
	ownerRefs := storedCM.GetOwnerReferences()
	require.Len(t, ownerRefs, 1)
	assert.Equal(t, "TestComponent", ownerRefs[0].Kind)
	assert.True(t, *ownerRefs[0].Controller)
}
