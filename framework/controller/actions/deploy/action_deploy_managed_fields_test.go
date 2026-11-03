package deploy_test

import (
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/framework/api"
	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	odhTypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	customOwner = "custom-crd-manager"

	crdName = "test-crds.test.opendatahub.io"
)

// stubInstance is the minimal api.PlatformObject that ReconciliationRequest
// requires. The deploy action reads its kind (standard field owner fallback)
// and its group version kind (owner-reference predicates).
type stubInstance struct {
	unstructured.Unstructured

	status api.Status
}

func (o *stubInstance) GetStatus() *api.Status          { return &o.status }
func (o *stubInstance) GetConditions() []api.Condition  { return o.status.Conditions }
func (o *stubInstance) SetConditions(c []api.Condition) { o.status.Conditions = c }

// runCRDDeploy deploys a single CRD with the given options and returns the
// stored object read back from the fake client. The request has no
// controller: shouldOwn, the only consumer of ReconciliationRequest
// .Controller, runs exclusively on the non-CRD deploy path.
func runCRDDeploy(t *testing.T, seed bool, opts ...deploy.ActionOpts) *unstructured.Unstructured {
	t.Helper()

	crd := unstructured.Unstructured{}
	crd.SetGroupVersionKind(gvk.CustomResourceDefinition)
	crd.SetName(crdName)

	instance := &stubInstance{}
	instance.SetGroupVersionKind(schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "TestComponent"})
	instance.SetName("test-instance")

	s := runtime.NewScheme()
	require.NoError(t, apiextensionsv1.AddToScheme(s))

	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(gvk.CustomResourceDefinition, meta.RESTScopeRoot)

	var seedObjects []client.Object
	if seed {
		seedObjects = append(seedObjects, &crd)
	}

	cli := fake.NewClientBuilder().
		WithScheme(s).
		WithRESTMapper(mapper).
		WithObjects(seedObjects...).
		WithReturnManagedFields().
		Build()

	rr := &odhTypes.ReconciliationRequest{
		Client:    cli,
		Instance:  instance,
		Resources: []unstructured.Unstructured{crd},
	}

	require.NoError(t, deploy.NewAction(opts...)(t.Context(), rr))

	stored := resources.GvkToUnstructured(gvk.CustomResourceDefinition)
	require.NoError(t, cli.Get(t.Context(), client.ObjectKey{Name: crdName}, stored))

	return stored
}

// TestDeployCRDFieldOwnerManagedFields verifies the CRD field owner end to
// end: the action deploys a CRD against a plain fake client and the recorded
// field manager is read back from metadata.managedFields.
func TestDeployCRDFieldOwnerManagedFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		opts        []deploy.ActionOpts
		seed        bool
		wantManager string
	}{
		{
			name:        "ssa omitted uses platform owner",
			opts:        []deploy.ActionOpts{deploy.WithMode(deploy.ModeSSA)},
			wantManager: resources.PlatformFieldOwner,
		},
		{
			name:        "ssa custom owner",
			opts:        []deploy.ActionOpts{deploy.WithMode(deploy.ModeSSA), deploy.WithCRDFieldOwner(customOwner)},
			wantManager: customOwner,
		},
		{
			name:        "ssa explicit empty uses platform owner",
			opts:        []deploy.ActionOpts{deploy.WithMode(deploy.ModeSSA), deploy.WithCRDFieldOwner("")},
			wantManager: resources.PlatformFieldOwner,
		},
		{
			name:        "ssa last value wins custom then empty",
			opts:        []deploy.ActionOpts{deploy.WithMode(deploy.ModeSSA), deploy.WithCRDFieldOwner(customOwner), deploy.WithCRDFieldOwner("")},
			wantManager: resources.PlatformFieldOwner,
		},
		{
			name:        "ssa last value wins empty then custom",
			opts:        []deploy.ActionOpts{deploy.WithMode(deploy.ModeSSA), deploy.WithCRDFieldOwner(""), deploy.WithCRDFieldOwner(customOwner)},
			wantManager: customOwner,
		},
		{
			name:        "patch omitted uses platform owner",
			opts:        []deploy.ActionOpts{deploy.WithMode(deploy.ModePatch)},
			seed:        true,
			wantManager: resources.PlatformFieldOwner,
		},
		{
			name:        "patch custom owner",
			opts:        []deploy.ActionOpts{deploy.WithMode(deploy.ModePatch), deploy.WithCRDFieldOwner(customOwner)},
			seed:        true,
			wantManager: customOwner,
		},
		{
			name:        "patch explicit empty uses platform owner",
			opts:        []deploy.ActionOpts{deploy.WithMode(deploy.ModePatch), deploy.WithCRDFieldOwner("")},
			seed:        true,
			wantManager: resources.PlatformFieldOwner,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stored := runCRDDeploy(t, test.seed, test.opts...)

			managed := stored.GetManagedFields()
			require.Len(t, managed, 1)
			assert.Equal(t, test.wantManager, managed[0].Manager)
			assert.Equal(t, metav1.ManagedFieldsOperationApply, managed[0].Operation)
		})
	}
}

// TestDeployCRDFieldOwnerPatchModeCreateHasNoActionManager verifies that a
// missing resource in patch mode is created without any of the action's CRD
// field managers: the stored entry is an Update whose manager is neither the
// platform owner nor a configured CRD owner.
func TestDeployCRDFieldOwnerPatchModeCreateHasNoActionManager(t *testing.T) {
	t.Parallel()

	stored := runCRDDeploy(t, false,
		deploy.WithMode(deploy.ModePatch),
		deploy.WithCRDFieldOwner(customOwner),
	)

	managed := stored.GetManagedFields()
	require.Len(t, managed, 1)
	assert.Equal(t, metav1.ManagedFieldsOperationUpdate, managed[0].Operation)

	actionManagers := []string{
		resources.PlatformFieldOwner,
		customOwner,
	}
	assert.NotContains(t, actionManagers, managed[0].Manager)
}
