package deploy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	"github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/deploy"
	odhTypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/metadata"
	metadatafake "k8s.io/client-go/metadata/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	legacyProbeOwner  = "legacy-controller"
	currentProbeOwner = "current-controller"
	// strayLegacyProbeOwner is a second configured previous owner that owns
	// no stale list entries (only a label), used to exercise the handoff of
	// multiple previous owners.
	strayLegacyProbeOwner = "stray-legacy-controller"

	probeDeploymentName = "owner-migration-probe"
	probeNamespace      = "owner-migration"
)

// stubController is a minimal odhTypes.Controller for the non-CRD deploy
// path: nothing is owned and dynamic ownership is disabled, so the action
// never sets owner references on the probe Deployment. The metadata client
// is only provided by tests that exercise the uncached managed-fields lookup.
type stubController struct {
	client         client.Client
	metadataClient metadata.Interface
}

func (s *stubController) Owns(schema.GroupVersionKind) bool                           { return false }
func (s *stubController) AddDynamicOwnedType(schema.GroupVersionKind)                 {}
func (s *stubController) GetClient() client.Client                                    { return s.client }
func (s *stubController) GetDiscoveryClient() discovery.DiscoveryInterface            { return nil }
func (s *stubController) GetDynamicClient() dynamic.Interface                         { return nil }
func (s *stubController) GetMetadataClient() metadata.Interface                       { return s.metadataClient }
func (s *stubController) IsDynamicOwnershipEnabled() bool                             { return false }
func (s *stubController) IsExcludedFromDynamicOwnership(schema.GroupVersionKind) bool { return false }

// probeDeployment returns an apps/v1 Deployment whose container exposes a
// metrics port and a health port. The metrics containerPort is an SSA
// associative-list merge key (containerPort+protocol), so changing it while
// keeping the port name reproduces the stale-entry scenario this test
// covers: a field-owner change combined with a merge-key change.
func probeDeployment(metricsPort int64) unstructured.Unstructured {
	dep := unstructured.Unstructured{}
	dep.SetGroupVersionKind(gvk.Deployment)
	dep.SetName(probeDeploymentName)
	dep.SetNamespace(probeNamespace)
	dep.Object = map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      probeDeploymentName,
			"namespace": probeNamespace,
		},
		"spec": map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]any{"app": probeDeploymentName},
			},
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]any{"app": probeDeploymentName},
				},
				"spec": map[string]any{
					"containers": []any{
						map[string]any{
							"name":  "manager",
							"image": "quay.io/example/manager:latest",
							"ports": []any{
								map[string]any{"containerPort": metricsPort, "name": "metrics", "protocol": "TCP"},
								map[string]any{"containerPort": int64(8081), "name": "health", "protocol": "TCP"},
							},
						},
					},
				},
			},
		},
	}

	return dep
}

// ownerMigrationEnv bundles the fake cluster used by the owner-migration
// tests: the (cached) controller client, an optional metadata client serving
// as the uncached view of the API server, and whether the cache strips
// managedFields from the objects it returns (as the opendatahub-operator's
// cache TransformFunc does). deleteAfterCurrentApply models another actor
// deleting the object between the current owner's write and the release.
// events records every intercepted apply as "<owner>/apply" or
// "<owner>/release" in order, and failAppliesFor fails the next N full-body
// applies of each owner with a duplicate-port validation error, modeling the
// API server rejecting an intermediate merged object that carries two
// container ports with the same name.
type ownerMigrationEnv struct {
	cli                     client.Client
	metadataCli             metadata.Interface
	stripCache              bool
	deleteAfterCurrentApply bool
	events                  []string
	failAppliesFor          map[string]int
	releaseFragments        []*unstructured.Unstructured
}

// newOwnerMigrationEnv builds a fake client that records the SSA field owner
// of every Apply call issued through it. Release fragments (identity-only
// or preserving-field configurations; see isReleaseApply) are handled by
// simulateReleaseApply instead of the fake client. When stripCache is set,
// cached reads return objects without managedFields.
func newOwnerMigrationEnv(t *testing.T, appliedOwners *[]string, stripCache bool) *ownerMigrationEnv {
	t.Helper()

	s := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(gvk.Deployment, meta.RESTScopeNamespace)

	env := &ownerMigrationEnv{stripCache: stripCache}

	env.cli = ctrlfake.NewClientBuilder().
		WithScheme(s).
		WithRESTMapper(mapper).
		WithReturnManagedFields().
		WithInterceptorFuncs(interceptor.Funcs{
			Apply: func(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
				owner := applyFieldOwner(opts...)
				*appliedOwners = append(*appliedOwners, owner)
				if isReleaseApply(obj) {
					env.events = append(env.events, owner+"/release")
					if data, err := json.Marshal(obj); err == nil {
						fragment := &unstructured.Unstructured{}
						if err := fragment.UnmarshalJSON(data); err == nil {
							env.releaseFragments = append(env.releaseFragments, fragment)
						}
					}
					return simulateReleaseApply(ctx, c, obj, owner)
				}
				env.events = append(env.events, owner+"/apply")
				if env.failAppliesFor[owner] > 0 {
					env.failAppliesFor[owner]--
					return duplicatePortNameError()
				}
				err := c.Apply(ctx, obj, opts...)
				if err == nil && env.deleteAfterCurrentApply && owner == currentProbeOwner {
					// Model another actor deleting the object between the
					// current owner's write and the release.
					victim := resources.GvkToUnstructured(gvk.Deployment)
					if getErr := c.Get(ctx, client.ObjectKey{Name: probeDeploymentName, Namespace: probeNamespace}, victim); getErr == nil {
						_ = c.Delete(ctx, victim)
					}
				}
				return err
			},
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if err := c.Get(ctx, key, obj, opts...); err != nil {
					return err
				}
				if env.stripCache {
					// Model a cache TransformFunc that strips managed
					// fields from cached objects to save memory.
					if u, ok := obj.(*unstructured.Unstructured); ok {
						u.SetManagedFields(nil)
					}
				}
				return nil
			},
		}).
		Build()

	return env
}

// newMetadataFake serves the given objects as the metadata-only view of the
// API server. Hand-set managed fields cannot be pushed through the
// controller fake client (its tracker recomputes them on every write), so
// tests that need a specific server-side ownership state seed it here; the
// metadata fake preserves the entries as given, and the migration reads
// only the object's metadata anyway.
func newMetadataFake(t *testing.T, objs ...*unstructured.Unstructured) metadata.Interface {
	t.Helper()

	metas := make([]runtime.Object, 0, len(objs))
	for _, obj := range objs {
		live := &metav1.PartialObjectMetadata{}
		live.SetGroupVersionKind(obj.GroupVersionKind())
		live.SetName(obj.GetName())
		live.SetNamespace(obj.GetNamespace())
		live.SetUID(obj.GetUID())
		live.ManagedFields = obj.GetManagedFields()
		metas = append(metas, live)
	}

	s := metadatafake.NewTestScheme()
	require.NoError(t, metav1.AddMetaToScheme(s))

	return metadatafake.NewSimpleMetadataClient(s, metas...)
}

// isReleaseApply reports whether the apply configuration is a release of
// the applying manager's field claims: either an identity-only fragment
// (apiVersion/kind/metadata), or one that additionally carries only the
// Deployment user-tuned fields a patch-mode release preserves (spec.replicas
// and name+resources container skeletons). Full-body applies always carry
// further spec fields (e.g. spec.selector), so they never match.
func isReleaseApply(obj runtime.ApplyConfiguration) bool {
	data, err := json.Marshal(obj)
	if err != nil {
		return false
	}

	u := unstructured.Unstructured{}
	if err := u.UnmarshalJSON(data); err != nil {
		return false
	}

	for key := range u.Object {
		switch key {
		case "apiVersion", "kind", "metadata":
		case "spec":
			if !isPreservedFieldsSpec(u.Object["spec"]) {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// isPreservedFieldsSpec reports whether the spec carries only the Deployment
// user-tuned fields a patch-mode release preserves: spec.replicas and
// container skeletons reduced to their merge key (name) and resources.
func isPreservedFieldsSpec(spec any) bool {
	specMap, ok := spec.(map[string]any)
	if !ok {
		return false
	}

	for key, value := range specMap {
		switch key {
		case "replicas":
		case "template":
			template, ok := value.(map[string]any)
			if !ok || len(template) != 1 {
				return false
			}

			templateSpec, ok := template["spec"].(map[string]any)
			if !ok || len(templateSpec) != 1 {
				return false
			}

			containers, ok := templateSpec["containers"].([]any)
			if !ok {
				return false
			}

			for _, raw := range containers {
				container, ok := raw.(map[string]any)
				if !ok {
					return false
				}

				for field := range container {
					if field != "name" && field != "resources" {
						return false
					}
				}

				if _, ok := container["name"].(string); !ok {
					return false
				}
			}
		default:
			return false
		}
	}

	return true
}

// simulateReleaseApply models the API server behavior for release applies:
// the applying manager's main-resource Apply entry is dropped from
// managedFields, and every field it owned that the fragment does not
// re-claim and no other entry claims is deleted from the live object — the
// deletion the server performs for a field whose only owner applies a body
// omitting it (verified against OpenShift 4.22: an identity-only release by
// the sole owner of a field deletes it; a release that re-claims the field
// keeps the value). When the fragment carries a uid, it acts as a
// precondition on the object's identity: a vanished or replaced object
// fails the apply with a conflict instead of the apply creating an empty
// replacement.
//
// The controller-runtime fake client cannot model this itself: it converts
// SSA bodies through the typed scheme before the field manager sees them,
// which materializes non-omitempty typed fields (e.g. Deployment
// spec.selector) as explicit nulls and wrongly claims them, conflicting with
// the current owner. A real API server computes the fieldset from the raw
// JSON, accepts the fragment, and drops the emptied entry (verified against
// OpenShift 4.22, including the uid precondition rejecting the apply on a
// deleted object and the follow-up merge-key change converging).
//
// One divergence from the server is accepted: the previous owner's entry is
// dropped entirely instead of being reduced to the fragment's claims, which
// is what the server retains for a release that re-claims fields. The fake
// client's tracker recomputes hand-set fieldsets on every write, so a
// reduced entry cannot be represented; the surviving field values, not the
// entry, are what the tests assert.
func simulateReleaseApply(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, owner string) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	fragment := &unstructured.Unstructured{}
	if err := fragment.UnmarshalJSON(data); err != nil {
		return err
	}

	live := resources.GvkToUnstructured(fragment.GroupVersionKind())
	if err := c.Get(ctx, client.ObjectKeyFromObject(fragment), live); err != nil {
		if apierrors.IsNotFound(err) && fragment.GetUID() != "" {
			return uidMismatchError(fragment)
		}

		return client.IgnoreNotFound(err)
	}

	if fragment.GetUID() != "" && fragment.GetUID() != live.GetUID() {
		return uidMismatchError(fragment)
	}

	if err := pruneReleasedFields(live, fragment, owner); err != nil {
		return err
	}

	kept := make([]metav1.ManagedFieldsEntry, 0, len(live.GetManagedFields()))
	for _, entry := range live.GetManagedFields() {
		if entry.Manager == owner && entry.Operation == metav1.ManagedFieldsOperationApply && entry.Subresource == "" {
			continue
		}

		kept = append(kept, entry)
	}

	live.SetManagedFields(kept)

	return c.Update(ctx, live)
}

// pruneReleasedFields removes from the live object every field that the
// releasing owner claims and that neither the release fragment nor another
// managed-fields entry retains: the deletion a real API server performs for
// a field whose only owner applies a body omitting it. The owner's
// main-resource Apply entry is the one being released; its other entries,
// if any, are retained and count as other owners.
func pruneReleasedFields(live *unstructured.Unstructured, fragment *unstructured.Unstructured, owner string) error {
	var ownerClaims map[string]any
	others := map[string]any{}

	for _, entry := range live.GetManagedFields() {
		if entry.Subresource != "" || entry.FieldsV1 == nil || len(entry.FieldsV1.Raw) == 0 {
			continue
		}

		var fields map[string]any
		if err := json.Unmarshal(entry.FieldsV1.Raw, &fields); err != nil {
			return err
		}

		if entry.Manager == owner && entry.Operation == metav1.ManagedFieldsOperationApply && ownerClaims == nil {
			ownerClaims = fields
			continue
		}

		mergeFieldsets(others, fields)
	}

	if ownerClaims == nil {
		return nil
	}

	pruneClaims(ownerClaims, others, fragment.Object, live.Object)

	return nil
}

// pruneClaims removes from the object node every field addressed by the
// owner's fieldset (claims) that the release fragment does not carry and no
// other entry claims (others). It reports whether anything the claims
// address remains.
//
// Fieldset keys address the node's children: "f:<name>" a map field,
// "k:<merge-keys>" an associative-list entry, "v:<value>" a set member and
// "." the node itself. An empty claim node is a leaf: it claims the
// addressed node as a whole. A node survives while any manager claims it,
// so deletion recurses to the solely-owned leaves and drops parents left
// empty.
func pruneClaims(claims, others map[string]any, fragment, obj any) bool {
	node, ok := obj.(map[string]any)
	if !ok {
		return false
	}

	fragmentMap, _ := fragment.(map[string]any)

	retained := false

	for key, rawChildClaims := range claims {
		childClaims, ok := rawChildClaims.(map[string]any)
		if !ok {
			continue
		}

		switch {
		case key == ".":
			// The node itself: its fate is decided one level up (an entry
			// survives while any manager claims it), but a co-owned node
			// keeps its identity here.
			if _, ok := others["."]; ok {
				retained = true
			}

		case strings.HasPrefix(key, "f:"):
			if pruneFieldClaim(key[len("f:"):], childClaims, others, fragmentMap, node) {
				retained = true
			}
		}
	}

	return retained
}

// pruneFieldClaim prunes the owner's claims on a map field of node. It
// reports whether the field survives.
func pruneFieldClaim(
	name string,
	claims map[string]any,
	others map[string]any,
	fragment map[string]any,
	node map[string]any,
) bool {
	child, present := node[name]
	if !present {
		return false
	}

	otherChild, otherClaims := claimsOf(others, "f:"+name)
	fragChild, fragCarried := fragment[name]

	// A leaf claim on a field that another entry or the fragment carries
	// retains the field as a whole.
	if (otherClaims && len(otherChild) == 0) || (fragCarried && len(claims) == 0) {
		return true
	}

	switch typed := child.(type) {
	case map[string]any:
		if pruneClaims(claims, otherChild, fragChild, typed) || fragCarried || otherClaims {
			return true
		}

		delete(node, name)

		return false
	case []any:
		pruned, listRetained := pruneListClaims(claims, otherChild, fragChild, typed)
		if listRetained || fragCarried || otherClaims {
			if len(pruned) != len(typed) {
				node[name] = pruned
			}

			return true
		}

		delete(node, name)

		return false
	default:
		// Scalar field: solely owned unless re-claimed or co-owned.
		if fragCarried || otherClaims {
			return true
		}

		delete(node, name)

		return false
	}
}

// pruneListClaims prunes the owner's claims on a list node: entries
// addressed by "k:<merge-keys>" keys and members by "v:<value>" keys. It
// returns the pruned list and whether anything the claims address remains.
func pruneListClaims(claims, others map[string]any, fragment any, obj []any) ([]any, bool) {
	fragmentList, _ := fragment.([]any)

	type keyedClaim struct {
		key       string
		claims    map[string]any
		mergeKeys map[string]any
	}

	var keyedClaims []keyedClaim
	setMembers := map[string]struct{}{}

	for key, rawChildClaims := range claims {
		childClaims, ok := rawChildClaims.(map[string]any)
		if !ok {
			continue
		}

		if mergeKeysJSON, ok := strings.CutPrefix(key, "k:"); ok {
			var mergeKeys map[string]any
			if err := json.Unmarshal([]byte(mergeKeysJSON), &mergeKeys); err == nil {
				keyedClaims = append(keyedClaims, keyedClaim{key: key, claims: childClaims, mergeKeys: mergeKeys})
			}
		} else if value, ok := strings.CutPrefix(key, "v:"); ok {
			setMembers[value] = struct{}{}
		}
	}

	retained := false
	result := make([]any, 0, len(obj))

	for _, raw := range obj {
		entry, isMap := raw.(map[string]any)
		if !isMap {
			// Set member: dropped when the owner claims it and neither the
			// fragment nor another entry carries it.
			data, err := json.Marshal(raw)
			if err != nil {
				// Unreachable for values decoded from an API object; keep
				// the member rather than dropping a value that cannot be
				// inspected.
				result = append(result, raw)
				continue
			}

			if _, claimed := setMembers[string(data)]; !claimed {
				result = append(result, raw)
				continue
			}

			if _, otherClaims := others["v:"+string(data)]; otherClaims || fragmentCarriesMember(fragmentList, raw) {
				result = append(result, raw)
				retained = true
			}

			continue
		}

		var claim *keyedClaim

		for i := range keyedClaims {
			if entryMatchesMergeKeys(entry, keyedClaims[i].mergeKeys) {
				claim = &keyedClaims[i]
				break
			}
		}

		if claim == nil {
			// Not claimed by the owner: keep untouched.
			result = append(result, raw)
			continue
		}

		otherChild, otherClaims := claimsOf(others, claim.key)
		fragChild, fragCarried := fragmentEntryFor(fragmentList, claim.mergeKeys)

		if pruneClaims(claim.claims, otherChild, fragChild, entry) || fragCarried || otherClaims {
			result = append(result, raw)
			retained = true
		}

		// else: the entry is solely owned and not re-claimed: dropped.
	}

	return result, retained
}

// fragmentEntryFor returns the fragment's list entry carrying the given
// merge-key values.
func fragmentEntryFor(fragment []any, mergeKeys map[string]any) (map[string]any, bool) {
	for _, raw := range fragment {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if entryMatchesMergeKeys(entry, mergeKeys) {
			return entry, true
		}
	}

	return nil, false
}

// entryMatchesMergeKeys reports whether the list entry carries the given
// merge-key values. JSON numbers decode as float64 while unstructured
// numbers are int64, so values are compared through their JSON encoding.
func entryMatchesMergeKeys(entry, mergeKeys map[string]any) bool {
	for name, want := range mergeKeys {
		got, ok := entry[name]
		if !ok || !sameJSONValue(got, want) {
			return false
		}
	}

	return true
}

// fragmentCarriesMember reports whether the fragment's list carries the set
// member.
func fragmentCarriesMember(fragment []any, member any) bool {
	for _, raw := range fragment {
		if sameJSONValue(raw, member) {
			return true
		}
	}

	return false
}

// sameJSONValue compares two values through their JSON encoding, ignoring
// the numeric type differences between JSON decoding and unstructured
// accessors.
func sameJSONValue(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)

	return errA == nil && errB == nil && bytes.Equal(ab, bb)
}

// claimsOf returns the child claims another entry holds under the given
// fieldset key, and whether it holds any.
func claimsOf(fields map[string]any, key string) (map[string]any, bool) {
	raw, ok := fields[key]
	if !ok {
		return nil, false
	}

	child, ok := raw.(map[string]any)

	return child, ok
}

// mergeFieldsets merges the claims of src into dst.
func mergeFieldsets(dst, src map[string]any) {
	for key, value := range src {
		child, ok := value.(map[string]any)
		if !ok {
			continue
		}

		existing, ok := dst[key].(map[string]any)
		if !ok {
			dst[key] = maps.Clone(child)
			continue
		}

		mergeFieldsets(existing, child)
	}
}

// uidMismatchError mirrors the API server conflict returned when an apply
// carrying a uid does not match a live object.
func uidMismatchError(fragment *unstructured.Unstructured) error {
	gvr, _ := meta.UnsafeGuessKindToResource(fragment.GroupVersionKind())

	return apierrors.NewConflict(gvr.GroupResource(), fragment.GetName(),
		fmt.Errorf("uid mismatch: the provided object specified uid %s, and no existing object was found", fragment.GetUID()))
}

// duplicatePortNameError models the API server rejecting an apply whose
// merged object would carry two container ports with the same name, as
// happens when another previous owner's stale port entry is merged next to
// the new one.
func duplicatePortNameError() error {
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "apps", Kind: "Deployment"},
		probeDeploymentName,
		field.ErrorList{field.Duplicate(field.NewPath("spec", "template", "spec", "containers").Index(0).Child("ports"), "metrics")},
	)
}

// applyFieldOwner returns the FieldOwner carried by the given apply options.
func applyFieldOwner(opts ...client.ApplyOption) string {
	for _, opt := range opts {
		if owner, ok := opt.(client.FieldOwner); ok {
			return string(owner)
		}
	}

	return ""
}

// runOwnerMigrationDeploy runs the deploy action once with the given options
// and object and returns the stored Deployment.
func runOwnerMigrationDeploy(t *testing.T, env *ownerMigrationEnv, dep unstructured.Unstructured, opts ...deploy.ActionOpts) (*unstructured.Unstructured, error) {
	t.Helper()

	instance := &stubInstance{}
	instance.SetGroupVersionKind(schema.GroupVersionKind{Group: "test", Version: "v1", Kind: "TestComponent"})
	instance.SetName("test-instance")

	rr := &odhTypes.ReconciliationRequest{
		Client:     env.cli,
		Controller: &stubController{client: env.cli, metadataClient: env.metadataCli},
		Instance:   instance,
		Resources:  []unstructured.Unstructured{dep},
	}

	if err := deploy.NewAction(opts...)(t.Context(), rr); err != nil {
		// The object may still have been written: a deferred handoff error
		// is surfaced only after the current owner's write and the releases
		// have completed.
		stored := resources.GvkToUnstructured(gvk.Deployment)
		if getErr := env.cli.Get(t.Context(), client.ObjectKey{Name: probeDeploymentName, Namespace: probeNamespace}, stored); getErr == nil {
			return stored, err
		}

		return nil, err
	}

	stored := resources.GvkToUnstructured(gvk.Deployment)
	if err := env.cli.Get(t.Context(), client.ObjectKey{Name: probeDeploymentName, Namespace: probeNamespace}, stored); err != nil {
		return nil, err
	}

	return stored, nil
}

// requireDeployed runs the deploy action and requires it to succeed.
func requireDeployed(t *testing.T, env *ownerMigrationEnv, dep unstructured.Unstructured, opts ...deploy.ActionOpts) *unstructured.Unstructured {
	t.Helper()

	stored, err := runOwnerMigrationDeploy(t, env, dep, opts...)
	require.NoError(t, err)

	return stored
}

// ownerMigrationPorts extracts "name:containerPort" pairs from the first
// container of the stored Deployment.
func ownerMigrationPorts(t *testing.T, stored *unstructured.Unstructured) []string {
	t.Helper()

	containers, found, err := unstructured.NestedSlice(stored.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, containers)

	container, ok := containers[0].(map[string]any)
	require.True(t, ok)

	ports, found, err := unstructured.NestedSlice(container, "ports")
	require.NoError(t, err)
	require.True(t, found)

	result := make([]string, 0, len(ports))
	for _, raw := range ports {
		port, ok := raw.(map[string]any)
		require.True(t, ok)

		name, _, err := unstructured.NestedString(port, "name")
		require.NoError(t, err)

		number, _, err := unstructured.NestedInt64(port, "containerPort")
		require.NoError(t, err)

		result = append(result, fmt.Sprintf("%s:%d", name, number))
	}

	return result
}

// TestDeployPreviousFieldOwnerMigration verifies that the deploy action
// migrates SSA field ownership from configured previous field owners: the
// desired body is re-applied once under the previous owner, which releases
// the stale associative-list entries it still owns, the current owner then
// converges the object, and the previous owner's remaining claims are
// released. Without the migration, a field-owner change combined with a
// merge-key change (metrics containerPort 8080 -> 8443) leaves the previous
// owner's entry merged next to the new one.
func TestDeployPreviousFieldOwnerMigration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		opts        []deploy.ActionOpts
		seedOwner   string
		wantApplied []string
		wantPorts   []string
	}{
		{
			// Handoff, current-owner apply, then release: the previous
			// owner applies an identity-only fragment that claims no
			// fields, dropping its managedFields entry entirely.
			name: "ssa migrates and releases previous owner around its apply",
			opts: []deploy.ActionOpts{
				deploy.WithMode(deploy.ModeSSA),
				deploy.WithFieldOwner(currentProbeOwner),
				deploy.WithPreviousFieldOwners(legacyProbeOwner),
			},
			seedOwner:   legacyProbeOwner,
			wantApplied: []string{legacyProbeOwner, currentProbeOwner, legacyProbeOwner},
			wantPorts:   []string{"metrics:8443", "health:8081"},
		},
		{
			// In patch mode the current owner's write goes through Patch,
			// so only the handoff and the release are recorded as applies.
			name: "patch migrates and releases previous owner around its patch",
			opts: []deploy.ActionOpts{
				deploy.WithMode(deploy.ModePatch),
				deploy.WithFieldOwner(currentProbeOwner),
				deploy.WithPreviousFieldOwners(legacyProbeOwner),
			},
			seedOwner:   legacyProbeOwner,
			wantApplied: []string{legacyProbeOwner, legacyProbeOwner},
			wantPorts:   []string{"metrics:8443", "health:8081"},
		},
		{
			name: "ssa without stale owner applies once",
			opts: []deploy.ActionOpts{
				deploy.WithMode(deploy.ModeSSA),
				deploy.WithFieldOwner(currentProbeOwner),
				deploy.WithPreviousFieldOwners(legacyProbeOwner),
			},
			seedOwner:   currentProbeOwner,
			wantApplied: []string{currentProbeOwner},
			wantPorts:   []string{"metrics:8443", "health:8081"},
		},
		{
			name: "ssa without configured previous owners keeps the corrupted merge",
			opts: []deploy.ActionOpts{
				deploy.WithMode(deploy.ModeSSA),
				deploy.WithFieldOwner(currentProbeOwner),
			},
			seedOwner:   legacyProbeOwner,
			wantApplied: []string{currentProbeOwner},
			// Pre-existing behavior, kept as documentation of the failure
			// mode: the previous owner's stale entry is merged next to the
			// new one (a real API server rejects this object with
			// 'Duplicate value: "metrics"'; the fake client performs the
			// same merge without validation).
			wantPorts: []string{"metrics:8080", "metrics:8443", "health:8081"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var applied []string
			env := newOwnerMigrationEnv(t, &applied, false)

			// Seed the live object the way the legacy controller did: an SSA
			// apply under the previous field owner.
			seed := probeDeployment(8080)
			require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(test.seedOwner)))

			applied = nil // the seeding apply is not part of the action under test

			stored := requireDeployed(t, env, probeDeployment(8443), test.opts...)

			assert.Equal(t, test.wantApplied, applied, "SSA applies issued by the action, in order")

			if test.wantPorts != nil {
				assert.ElementsMatch(t, test.wantPorts, ownerMigrationPorts(t, stored))
			}
		})
	}
}

// TestDeployPreviousFieldOwnerMigrationConverged asserts the converged state
// after a migration: once the current owner has applied, the previous
// owner's claims are released entirely, so the current owner holds the only
// Apply entry. This matters because the co-ownership a handoff-only
// migration would leave behind wedges any later merge-key change of a
// co-owned entry: neither manager can drop the old entry on its own, so the
// merged object carries two entries with the same name and is rejected by
// validation on every apply. With the release in place, a second port change
// (8443 -> 9443) converges cleanly.
func TestDeployPreviousFieldOwnerMigrationConverged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode deploy.Mode
		// wantSteady is the apply sequence recorded on reconciles that
		// need no migration: in SSA mode the current owner's apply; in
		// patch mode its write goes through Patch and records nothing.
		wantSteady []string
	}{
		{
			name:       "ssa",
			mode:       deploy.ModeSSA,
			wantSteady: []string{currentProbeOwner},
		},
		{
			name: "patch",
			mode: deploy.ModePatch,
			// nil: in patch mode the current owner's write goes through
			// Patch and records no applies at all.
			wantSteady: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			opts := []deploy.ActionOpts{
				deploy.WithMode(test.mode),
				deploy.WithFieldOwner(currentProbeOwner),
				deploy.WithPreviousFieldOwners(legacyProbeOwner),
			}

			var applied []string
			env := newOwnerMigrationEnv(t, &applied, false)

			seed := probeDeployment(8080)
			require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

			// First reconcile: handoff, current-owner write, release.
			stored := requireDeployed(t, env, probeDeployment(8443), opts...)

			managers := applyManagers(t, stored)
			assert.Contains(t, managers, currentProbeOwner+" (Apply)")
			assert.NotContains(t, managers, legacyProbeOwner+" (Apply)",
				"previous owner's claims must be released after the current owner's write")

			// Second reconcile of the same desired state: no previous
			// owner remains, so no handoff or release is issued.
			applied = nil
			stored = requireDeployed(t, env, probeDeployment(8443), opts...)

			assert.Equal(t, test.wantSteady, applied)
			assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, stored))

			// Third reconcile with another merge-key change of the same
			// entry (8443 -> 9443): the current owner alone drops the old
			// entry and the object converges.
			applied = nil
			stored = requireDeployed(t, env, probeDeployment(9443), opts...)

			assert.Equal(t, test.wantSteady, applied)
			assert.ElementsMatch(t, []string{"metrics:9443", "health:8081"}, ownerMigrationPorts(t, stored))
		})
	}
}

// TestDeployPreviousFieldOwnerMigrationStrippedCache covers the case where
// the informer cache strips managedFields from the objects it returns (e.g.
// via a cache TransformFunc, as the opendatahub-operator configures to
// reduce memory usage): the cached object cannot reveal the previous owner,
// so the migration must read the live managed fields from the API server
// through the metadata client instead of the cache.
func TestDeployPreviousFieldOwnerMigrationStrippedCache(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, true)

	// Seed the live object the way the legacy controller did: an SSA apply
	// under the previous field owner. The tracker (the "server") keeps the
	// managed fields; only cached reads strip them.
	seed := probeDeployment(8080)
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	// The uncached view of the API server carries the legacy owner's
	// Apply entry. The fieldset content is irrelevant to the owner
	// selection; it only must not be empty.
	live := probeDeployment(8080)
	live.SetGroupVersionKind(gvk.Deployment)
	live.SetManagedFields([]metav1.ManagedFieldsEntry{{
		Manager:   legacyProbeOwner,
		Operation: metav1.ManagedFieldsOperationApply,
		FieldsV1:  &metav1.FieldsV1{Raw: []byte(`{"f:spec":{"f:template":{"f:spec":{"f:containers":{}}}}}`)},
		Time:      &metav1.Time{},
	}})
	env.metadataCli = newMetadataFake(t, live.DeepCopy())

	applied = nil // the seeding apply is not part of the action under test

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModeSSA),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	stored := requireDeployed(t, env, probeDeployment(8443), opts...)

	// Despite the stripped cache, the migration fired: handoff, apply,
	// release.
	assert.Equal(t, []string{legacyProbeOwner, currentProbeOwner, legacyProbeOwner}, applied)
	assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, stored))
}

// TestDeployPreviousFieldOwnerMigrationNoMetadataClient covers the case where
// the cache strips managedFields and the Controller provides no metadata
// client: the ownership state is unknowable, so the action must fail loudly
// instead of treating the empty cached view as proof that no previous owner
// exists and silently skipping the configured handoff.
func TestDeployPreviousFieldOwnerMigrationNoMetadataClient(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, true) // stripped cache, no metadata client

	seed := probeDeployment(8080)
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	applied = nil // the seeding apply is not part of the action under test

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModeSSA),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	_, err := runOwnerMigrationDeploy(t, env, probeDeployment(8443), opts...)
	require.Error(t, err)
	require.ErrorContains(t, err, "no metadata client is available")
	assert.Empty(t, applied, "no applies must be issued when the ownership state is unknowable")
}

// TestDeployPreviousFieldOwnerMigrationEmptyFieldsetEntry covers Apply
// entries that own no fields: some API servers retain them instead of
// dropping the entry once its fieldset empties. An owner of nothing cannot
// strand associative-list entries, so it must not trigger a migration.
func TestDeployPreviousFieldOwnerMigrationEmptyFieldsetEntry(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, true) // stripped cache

	seed := probeDeployment(8080)
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	// The uncached view of the API server shows the previous owner with an
	// Apply entry that owns no fields, the way a server that retains
	// emptied entries would. (Hand-set managed fields cannot be pushed
	// through the controller fake client: its tracker recomputes them on
	// every write. The metadata fake preserves them as given.)
	live := probeDeployment(8080)
	live.SetGroupVersionKind(gvk.Deployment)
	live.SetManagedFields([]metav1.ManagedFieldsEntry{{
		Manager:   legacyProbeOwner,
		Operation: metav1.ManagedFieldsOperationApply,
		FieldsV1:  &metav1.FieldsV1{Raw: []byte(`{}`)},
	}})
	env.metadataCli = newMetadataFake(t, live.DeepCopy())

	applied = nil // the seeding apply is not part of the action under test

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModeSSA),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	requireDeployed(t, env, probeDeployment(8443), opts...)

	// No handoff: the empty-fieldset entry is not a previous owner.
	assert.Equal(t, []string{currentProbeOwner}, applied,
		"an Apply entry that owns no fields must not trigger a migration")
}

// TestDeployPreviousFieldOwnerReleaseRequiresOriginalObject covers the
// release precondition: when the object is deleted between the current
// owner's write and the release, the release must fail instead of creating
// an empty replacement. Server-side apply creates absent objects; the uid
// carried by the fragment turns the apply into a precondition that the
// original object still exists.
func TestDeployPreviousFieldOwnerReleaseRequiresOriginalObject(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, false)
	env.deleteAfterCurrentApply = true

	seed := probeDeployment(8080)
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	// Give the object a uid the way the API server would, so the release
	// fragment can carry it as a precondition.
	live := resources.GvkToUnstructured(gvk.Deployment)
	require.NoError(t, env.cli.Get(t.Context(), client.ObjectKey{Name: probeDeploymentName, Namespace: probeNamespace}, live))
	live.SetUID("probe-uid")
	require.NoError(t, env.cli.Update(t.Context(), live))

	applied = nil // the seeding apply is not part of the action under test

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModeSSA),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	_, err := runOwnerMigrationDeploy(t, env, probeDeployment(8443), opts...)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to release SSA field ownership")
	require.ErrorContains(t, err, "uid mismatch")

	// No empty replacement was created: the object is gone, not a
	// metadata-only shell.
	probe := resources.GvkToUnstructured(gvk.Deployment)
	getErr := env.cli.Get(t.Context(), client.ObjectKey{Name: probeDeploymentName, Namespace: probeNamespace}, probe)
	assert.True(t, apierrors.IsNotFound(getErr), "the object must not be recreated by the release")
}

// applyManagers returns "manager (operation)" strings for every managedFields
// entry of the stored object.
func applyManagers(t *testing.T, stored *unstructured.Unstructured) []string {
	t.Helper()

	managed := stored.GetManagedFields()
	result := make([]string, 0, len(managed))
	for _, entry := range managed {
		result = append(result, fmt.Sprintf("%s (%s)", entry.Manager, entry.Operation))
	}

	return result
}

// setProbeReplicasAndResources sets spec.replicas and the first container's
// resource requests on a probe Deployment: the fields the patch customizer
// strips from the body the current owner writes.
func setProbeReplicasAndResources(t *testing.T, dep *unstructured.Unstructured, replicas int64, cpu string) {
	t.Helper()

	require.NoError(t, unstructured.SetNestedField(dep.Object, replicas, "spec", "replicas"))

	containers, found, err := unstructured.NestedSlice(dep.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)

	container, ok := containers[0].(map[string]any)
	require.True(t, ok)
	container["resources"] = map[string]any{"requests": map[string]any{"cpu": cpu}}
	containers[0] = container

	require.NoError(t, unstructured.SetNestedSlice(dep.Object, containers, "spec", "template", "spec", "containers"))
}

// addProbeContainer appends a container to the probe Deployment's pod spec.
func addProbeContainer(t *testing.T, dep *unstructured.Unstructured, name, image string) {
	t.Helper()

	containers, found, err := unstructured.NestedSlice(dep.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)

	containers = append(containers, map[string]any{"name": name, "image": image})
	require.NoError(t, unstructured.SetNestedSlice(dep.Object, containers, "spec", "template", "spec", "containers"))
}

// applyEntryFor builds a main-resource Apply managed-fields entry for the
// given manager and fieldset, the way the API server records it.
func applyEntryFor(manager string, fields map[string]any) metav1.ManagedFieldsEntry {
	return metav1.ManagedFieldsEntry{
		Manager:   manager,
		Operation: metav1.ManagedFieldsOperationApply,
		FieldsV1:  &metav1.FieldsV1{Raw: mustMarshalJSON(fields)},
	}
}

// probeDeploymentFieldset returns the fieldset of a probe Deployment applied
// by a single manager: the pod template's single container and its ports,
// keyed by the SSA associative-list merge keys.
func probeDeploymentFieldset(metricsPort int64) map[string]any {
	containerKey := "k:" + string(mustMarshalJSON(map[string]any{"name": "manager"}))
	portKey := "k:" + string(mustMarshalJSON(map[string]any{"containerPort": metricsPort, "protocol": "TCP"}))

	return map[string]any{
		"f:spec": map[string]any{
			"f:template": map[string]any{
				"f:spec": map[string]any{
					"f:containers": map[string]any{
						containerKey: map[string]any{
							"f:ports": map[string]any{
								portKey: map[string]any{
									"f:containerPort": map[string]any{},
									"f:name":          map[string]any{},
									"f:protocol":      map[string]any{},
								},
							},
						},
					},
				},
			},
		},
	}
}

// mustMarshalJSON marshals a value that is known to be marshalable.
func mustMarshalJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return data
}

// storedProbeUnstripped returns the stored probe Deployment with its managed
// fields, bypassing the cache-stripping interceptor for the inspection: the
// interceptor models what the controller's informer cache sees, while the
// assertions verify what the API server stores.
func storedProbeUnstripped(t *testing.T, env *ownerMigrationEnv) *unstructured.Unstructured {
	t.Helper()

	strip := env.stripCache
	env.stripCache = false
	defer func() { env.stripCache = strip }()

	stored := resources.GvkToUnstructured(gvk.Deployment)
	require.NoError(t, env.cli.Get(t.Context(), client.ObjectKey{Name: probeDeploymentName, Namespace: probeNamespace}, stored))

	return stored
}

// TestDeployPreviousFieldOwnerMigrationMultipleOwners covers the handoff of
// multiple previous owners whose stale entries are spread unevenly: the
// stray owner owns no stale entry, so its handoff of the desired body would
// merge the new port next to the one still owned by the legacy owner and be
// rejected for the duplicate port name. The failed handoff must not abort
// the legacy owner's handoff, the current owner's write, or the legacy
// owner's release; the failure is surfaced afterwards so the reconcile
// requeues, and the stray owner's handoff succeeds on the retry.
func TestDeployPreviousFieldOwnerMigrationMultipleOwners(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, true) // stripped cache

	seed := probeDeployment(8080)
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	// The uncached view of the API server shows two previous owners: the
	// stray one, which owns only a label, and the legacy one, which owns the
	// stale metrics port. (Hand-set managed fields cannot be pushed through
	// the controller fake client: its tracker recomputes them on every
	// write. The metadata fake preserves them as given. The stray owner comes
	// first so its handoff runs first.)
	live := probeDeployment(8080)
	live.SetGroupVersionKind(gvk.Deployment)
	live.SetManagedFields([]metav1.ManagedFieldsEntry{
		applyEntryFor(strayLegacyProbeOwner, map[string]any{
			"f:metadata": map[string]any{
				"f:labels": map[string]any{"f:stray": map[string]any{}},
			},
		}),
		applyEntryFor(legacyProbeOwner, probeDeploymentFieldset(8080)),
	})
	env.metadataCli = newMetadataFake(t, live.DeepCopy())

	// The API server rejects the stray owner's handoff: its apply of the
	// desired body would merge metrics:8443 next to the metrics:8080 still
	// owned by the legacy owner.
	env.failAppliesFor = map[string]int{strayLegacyProbeOwner: 1}

	applied = nil // the seeding apply is not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModeSSA),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(strayLegacyProbeOwner, legacyProbeOwner),
	}

	// First reconcile: the stray owner's handoff fails, but the legacy
	// owner's handoff, the current owner's write and the legacy owner's
	// release all run. The failure is surfaced afterwards.
	_, err := runOwnerMigrationDeploy(t, env, probeDeployment(8443), opts...)
	require.Error(t, err)
	require.ErrorContains(t, err, fmt.Sprintf("failed to hand off SSA field ownership from %q", strayLegacyProbeOwner))

	inspect := storedProbeUnstripped(t, env)
	assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, inspect))
	managers := applyManagers(t, inspect)
	assert.Contains(t, managers, currentProbeOwner+" (Apply)")
	assert.NotContains(t, managers, legacyProbeOwner+" (Apply)", "the owner of the stale entries must still be handed off and released")
	assert.NotContains(t, env.events, strayLegacyProbeOwner+"/release", "a failed handoff must not be released")

	// Second reconcile: the stray owner's handoff now succeeds (the stale
	// entry is gone) and its claims are released.
	applied = nil
	env.events = nil

	requireDeployed(t, env, probeDeployment(8443), opts...)

	inspect = storedProbeUnstripped(t, env)
	assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, inspect))
	assert.Contains(t, env.events, strayLegacyProbeOwner+"/apply")
	assert.Contains(t, env.events, strayLegacyProbeOwner+"/release")

	managers = applyManagers(t, inspect)
	assert.Contains(t, managers, currentProbeOwner+" (Apply)")
	assert.NotContains(t, managers, strayLegacyProbeOwner+" (Apply)")
	assert.NotContains(t, managers, legacyProbeOwner+" (Apply)")
}

// TestDeployPreviousFieldOwnerMigrationPatchPreservesLiveValues covers the
// patch-mode preservation of the Deployment user-tuned fields across the
// whole migration: the patch customizer strips replicas and container
// resources from the body the current owner writes (to preserve user-tuned
// values), so after the handoff the previous owner is the sole owner of
// those fields. Both the handoff body and the release fragment must restore
// the live values — a server-side apply by the sole owner of a field that
// omits it deletes it (verified against OpenShift 4.22), so an identity-only
// release would delete the values the handoff just preserved, and a handoff
// without them would have deleted them earlier. The deletion is modeled by
// the release simulation (pruneReleasedFields), so this test regresses if
// either body loses the preserved fields.
func TestDeployPreviousFieldOwnerMigrationPatchPreservesLiveValues(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, false)

	// The live Deployment is owned by the previous owner and carries
	// non-default replicas and container resources alongside the stale port.
	seed := probeDeployment(8080)
	setProbeReplicasAndResources(t, &seed, 3, "500m")
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	applied = nil // the seeding apply is not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModePatch),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	// The desired body carries different values, which the patch customizer
	// strips from the body the current owner writes.
	desired := probeDeployment(8443)
	setProbeReplicasAndResources(t, &desired, 1, "100m")

	stored := requireDeployed(t, env, desired, opts...)

	// Patch mode records the handoff and the release as applies; the
	// current owner's write goes through Patch and records nothing.
	assert.Equal(t, []string{legacyProbeOwner + "/apply", legacyProbeOwner + "/release"}, env.events)

	assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, stored))
	managers := applyManagers(t, stored)
	assert.Contains(t, managers, currentProbeOwner+" (Apply)")

	// On a real API server the previous owner's entry survives the release
	// with its claims reduced to the preserved fields (verified live). The
	// fake client's tracker cannot represent a hand-reduced fieldset (it
	// recomputes entries on every write), so the simulation drops the entry
	// instead; the surviving values, not the entry, are what this test
	// verifies.
	//
	// Of the two preserved fields, spec.replicas carries the regression
	// detection: the fake's patch-path entry materializes an empty
	// resources map (ResourceRequirements is a struct with an ineffective
	// omitempty), which claims f:resources for the current owner, so the
	// container resources survive an identity-only release in the fake
	// either way. A real API server computes the patch claims from the raw
	// JSON, which omits resources, and would delete them.
	replicas, found, err := unstructured.NestedInt64(stored.Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found, "spec.replicas must not be deleted by the handoff or the release")
	assert.Equal(t, int64(3), replicas)

	containers, found, err := unstructured.NestedSlice(stored.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)

	container, ok := containers[0].(map[string]any)
	require.True(t, ok)

	cpu, found, err := unstructured.NestedString(container, "resources", "requests", "cpu")
	require.NoError(t, err)
	require.True(t, found, "container resources must not be deleted by the handoff or the release")
	assert.Equal(t, "500m", cpu)
}

// TestDeployPreviousFieldOwnerMigrationManagedByIntroduced covers a desired
// body that introduces the managed-by annotation during the migration. The
// field-omission decision is made against the pre-write object (annotation
// absent), but the current owner's write decodes the server response — now
// carrying the annotation — into old. A release that re-evaluates the
// decision against the post-write old would degrade to identity-only and
// delete the user-tuned values the handoff just preserved; the decision
// must be captured before the write.
func TestDeployPreviousFieldOwnerMigrationManagedByIntroduced(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, false)

	// The live Deployment is owned by the previous owner and carries
	// non-default replicas and container resources alongside the stale
	// port.
	seed := probeDeployment(8080)
	setProbeReplicasAndResources(t, &seed, 3, "500m")
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	applied = nil // the seeding apply is not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModePatch),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	// The desired body introduces the managed-by annotation and carries
	// different user-tuned values, which the patch customizer strips from
	// the body the current owner writes.
	desired := probeDeployment(8443)
	setProbeReplicasAndResources(t, &desired, 1, "100m")
	desired.SetAnnotations(map[string]string{deploy.DefaultManagedByAnnotation: "true"})

	stored := requireDeployed(t, env, desired, opts...)

	assert.Equal(t, []string{legacyProbeOwner + "/apply", legacyProbeOwner + "/release"}, env.events)

	// The release fragment must still be the preserving one, not the
	// identity-only fragment the post-write annotation would select.
	require.Len(t, env.releaseFragments, 1)

	replicasInFragment, found, err := unstructured.NestedInt64(env.releaseFragments[0].Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found, "the release fragment must re-claim spec.replicas despite the annotation the write introduced")
	assert.Equal(t, int64(3), replicasInFragment)

	// The write applied the annotation, and the values survived the
	// release.
	assert.Equal(t, "true", stored.GetAnnotations()[deploy.DefaultManagedByAnnotation])

	replicas, found, err := unstructured.NestedInt64(stored.Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found, "spec.replicas must not be deleted by the release")
	assert.Equal(t, int64(3), replicas)

	containers, found, err := unstructured.NestedSlice(stored.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)

	container, ok := containers[0].(map[string]any)
	require.True(t, ok)

	cpu, found, err := unstructured.NestedString(container, "resources", "requests", "cpu")
	require.NoError(t, err)
	require.True(t, found, "container resources must not be deleted by the release")
	assert.Equal(t, "500m", cpu)
}

// TestDeployPreviousFieldOwnerMigrationReleasePendingRecovery covers the
// recovery of an unfinished release: both owners applied the same body (an
// earlier handoff completed) but the previous owner's release never ran. The
// next reconcile changes the merge key again, and the previous owner's
// handoff would retain the old entry (the current owner co-owns it) and
// wedge the object on the duplicate port name. The previous owner must be
// released first instead — safely, because the current owner co-owns every
// field it claims. This test also covers the degradation of the freshness
// guard: the cached view drives the partition and no metadata client exists
// to refresh it, so the cached partition is used as the best available view.
func TestDeployPreviousFieldOwnerMigrationReleasePendingRecovery(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, false)

	seed := probeDeployment(8443)
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(currentProbeOwner)))

	applied = nil // the seeding applies are not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModeSSA),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	stored := requireDeployed(t, env, probeDeployment(9443), opts...)

	assert.Equal(t, []string{
		legacyProbeOwner + "/release",
		currentProbeOwner + "/apply",
	}, env.events, "the release-pending owner must be released before the current owner's write, without a handoff")

	assert.ElementsMatch(t, []string{"metrics:9443", "health:8081"}, ownerMigrationPorts(t, stored))
	managers := applyManagers(t, stored)
	assert.Contains(t, managers, currentProbeOwner+" (Apply)")
	assert.NotContains(t, managers, legacyProbeOwner+" (Apply)")
}

// TestDeployPreviousFieldOwnerMigrationPendingFromLiveRead covers the
// freshness of the release-pending decision: the informer cache can
// predate the latest changes to the object's managed fields, so a
// partition computed from it can present a sharing that no longer holds
// on the server. Releasing a pending owner drops its unshared claims —
// safe only while another manager keeps the shared entries and the
// preserving fragment keeps the user-tuned fields — so when the cached
// view marks an owner pending, the decision must be remade from a fresh
// read of the live object. Here the cached view shows the two owners
// sharing every associative-list entry (the release-pending state), while
// the live view shows only the previous owner's entry (e.g. the object
// was replaced and only the previous owner has applied since): the fresh
// view must win, and the owner must be handed off rather than released
// first.
func TestDeployPreviousFieldOwnerMigrationPendingFromLiveRead(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, false)

	// The cached (tracker) view: both owners applied the same body, so
	// they share every associative-list entry — the release-pending
	// state.
	seed := probeDeployment(8443)
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(currentProbeOwner)))

	// The live (API server) view: only the previous owner's entry. (Hand-
	// set managed fields cannot be pushed through the controller fake
	// client: its tracker recomputes them on every write. The metadata
	// fake preserves them as given.)
	fresh := probeDeployment(8443)
	fresh.SetGroupVersionKind(gvk.Deployment)
	fresh.SetManagedFields([]metav1.ManagedFieldsEntry{
		applyEntryFor(legacyProbeOwner, probeDeploymentFieldset(8443)),
	})
	env.metadataCli = newMetadataFake(t, fresh.DeepCopy())

	applied = nil // the seeding applies are not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModeSSA),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	stored := requireDeployed(t, env, probeDeployment(9443), opts...)

	// The fresh view drove the partition: the previous owner was handed off
	// (apply before the current owner's write, release after it) instead of
	// being released first — which is what the stale cached view would
	// have authorized.
	assert.Equal(t, []string{
		legacyProbeOwner + "/apply",
		currentProbeOwner + "/apply",
		legacyProbeOwner + "/release",
	}, env.events, "the release-pending decision must be remade from the live view, not the stale cache")

	assert.ElementsMatch(t, []string{"metrics:9443", "health:8081"}, ownerMigrationPorts(t, stored))
	managers := applyManagers(t, stored)
	assert.Contains(t, managers, currentProbeOwner+" (Apply)")
	assert.NotContains(t, managers, legacyProbeOwner+" (Apply)")
}

// TestDeployPreviousFieldOwnerMigrationOwnersFromFreshView covers the
// freshness of the owner list: the informer cache can predate an ownership
// change that both removes one previous owner's entry and adds another's,
// so after the fresh read replaces the managed-fields view the owner list
// must be recomputed from it. Otherwise the owner whose entry exists only
// live is missed for this reconcile (its stale claims persist), and the
// owner that has since vanished gets a pointless handoff and release.
func TestDeployPreviousFieldOwnerMigrationOwnersFromFreshView(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, false)

	// The cached (tracker) view: the first previous owner and the current
	// owner applied the same body, so they share every associative-list
	// entry — the release-pending state that triggers the fresh read. The
	// second previous owner is absent from this view.
	seed := probeDeployment(8443)
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(currentProbeOwner)))

	// The live (API server) view: only the second previous owner's entry —
	// the first owner's entry is gone (e.g. its release completed) and the
	// second owner applied since the cached snapshot. (Hand-set managed
	// fields cannot be pushed through the controller fake client: its
	// tracker recomputes them on every write. The metadata fake preserves
	// them as given.)
	fresh := probeDeployment(8443)
	fresh.SetGroupVersionKind(gvk.Deployment)
	fresh.SetManagedFields([]metav1.ManagedFieldsEntry{
		applyEntryFor(strayLegacyProbeOwner, probeDeploymentFieldset(8443)),
	})
	env.metadataCli = newMetadataFake(t, fresh.DeepCopy())

	applied = nil // the seeding applies are not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModeSSA),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner, strayLegacyProbeOwner),
	}

	stored := requireDeployed(t, env, probeDeployment(9443), opts...)

	// The owner list came from the fresh view: the live-only owner was
	// handed off (apply before the current owner's write, release after
	// it), and the vanished owner was neither handed off nor released.
	assert.Equal(t, []string{
		strayLegacyProbeOwner + "/apply",
		currentProbeOwner + "/apply",
		strayLegacyProbeOwner + "/release",
	}, env.events, "the owner list must be recomputed from the fresh view, not the stale cache")

	// The live-only owner's entry is gone after its release; the vanished
	// owner's stale tracker entry is untouched (the fake's cached view
	// still shows it — on a real server it is already gone, and the
	// current owner's write drops the stale entry it owns).
	managers := applyManagers(t, stored)
	assert.Contains(t, managers, currentProbeOwner+" (Apply)")
	assert.Contains(t, managers, legacyProbeOwner+" (Apply)")
	assert.NotContains(t, managers, strayLegacyProbeOwner+" (Apply)")
}

// guardianProbeFieldset builds the fieldset a preserving release leaves
// behind on a probe Deployment: spec.replicas and, under the container
// entries the desired state carries, the merge key and the resources. The
// optional extra claims make the owner a stale (non-converged) one.
func guardianProbeFieldset(extra map[string]any) map[string]any {
	fields := map[string]any{
		"f:spec": map[string]any{
			"f:replicas": map[string]any{},
			"f:template": map[string]any{
				"f:spec": map[string]any{
					"f:containers": map[string]any{
						"k:" + string(mustMarshalJSON(map[string]any{"name": "manager"})): map[string]any{
							".":           map[string]any{},
							"f:name":      map[string]any{},
							"f:resources": map[string]any{"f:requests": map[string]any{"f:cpu": map[string]any{}}},
						},
					},
				},
			},
		},
	}

	maps.Copy(fields, extra)

	return fields
}

// TestDeployPreviousFieldOwnerMigrationPatchGuardianConverged covers the
// patch-mode steady state after a preserving release: the previous owner's
// entry survives on a real API server with its claims reduced to the
// Deployment user-tuned fields (the fake cannot represent that reduced
// entry, so it is seeded through the metadata fake, which is also what the
// migration reads with a stripped cache). Such a converged guardian is the
// owner of record for those fields and must not be migrated again — the
// handoff would expand its claims back to the whole body and the release
// would reduce them again, writing on every reconcile without converging.
// An owner holding anything beyond the preserved fields is still migrated:
// it shares the container entry with the current owner (release-pending),
// so it is released before the write — with the preserving fragment, which
// keeps the user-tuned values and drops the stale claims.
func TestDeployPreviousFieldOwnerMigrationPatchGuardianConverged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		legacyExtra   map[string]any
		wantNoApplies bool
	}{
		{
			name:          "a converged guardian is not migrated again",
			legacyExtra:   nil,
			wantNoApplies: true,
		},
		{
			name: "an owner with stale claims beyond the preserved fields is released first",
			legacyExtra: map[string]any{
				"f:metadata": map[string]any{"f:labels": map[string]any{"f:stale": map[string]any{}}},
			},
			wantNoApplies: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var applied []string
			env := newOwnerMigrationEnv(t, &applied, true) // stripped cache

			// The tracker holds the steady-state object: the desired body
			// with the live user-tuned values, seeded under the previous
			// owner the way it was before its release reduced its claims.
			seed := probeDeployment(8443)
			setProbeReplicasAndResources(t, &seed, 3, "500m")
			require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

			// The live (API server) view: the previous owner's claims
			// reduced to the preserved fields (plus, in the second case, a
			// stale label), and the current owner holding the stripped
			// patch body's claims. (Hand-set managed fields cannot be
			// pushed through the controller fake client: its tracker
			// recomputes them on every write. The metadata fake preserves
			// them as given.)
			fresh := probeDeployment(8443)
			fresh.SetGroupVersionKind(gvk.Deployment)
			fresh.SetManagedFields([]metav1.ManagedFieldsEntry{
				applyEntryFor(legacyProbeOwner, guardianProbeFieldset(test.legacyExtra)),
				applyEntryFor(currentProbeOwner, probeDeploymentFieldset(8443)),
			})
			env.metadataCli = newMetadataFake(t, fresh.DeepCopy())

			applied = nil // the seeding apply is not part of the action under test
			env.events = nil

			opts := []deploy.ActionOpts{
				deploy.WithMode(deploy.ModePatch),
				deploy.WithFieldOwner(currentProbeOwner),
				deploy.WithPreviousFieldOwners(legacyProbeOwner),
			}

			// The desired body carries different user-tuned values, which
			// the patch customizer strips from the write.
			desired := probeDeployment(8443)
			setProbeReplicasAndResources(t, &desired, 1, "100m")

			requireDeployed(t, env, desired, opts...)

			inspect := storedProbeUnstripped(t, env)

			if test.wantNoApplies {
				assert.Empty(t, applied, "a converged guardian must not be handed off or released again")
				assert.Empty(t, env.events)
				assert.Contains(t, applyManagers(t, inspect), currentProbeOwner+" (Apply)",
					"the current owner's patch write must still run")
			} else {
				assert.Equal(t, []string{legacyProbeOwner + "/release"}, env.events,
					"an owner with stale claims is release-pending (it shares the container entry with the current owner) and must be released before the write")
			}

			assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, inspect))

			replicas, found, err := unstructured.NestedInt64(inspect.Object, "spec", "replicas")
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, int64(3), replicas)

			containers, found, err := unstructured.NestedSlice(inspect.Object, "spec", "template", "spec", "containers")
			require.NoError(t, err)
			require.True(t, found)
			require.Len(t, containers, 1)

			container, ok := containers[0].(map[string]any)
			require.True(t, ok)

			cpu, found, err := unstructured.NestedString(container, "resources", "requests", "cpu")
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, "500m", cpu)
		})
	}
}

// TestDeployPreviousFieldOwnerMigrationGuardianSharing covers the chained
// (second) field-owner upgrade: one previous owner holds the full body,
// another has already been reduced to a converged guardian by the first
// migration's preserving release, and the new current owner has not
// written yet. The full-body owner shares the container merge keys with
// the guardian, so partitioning before dropping the guardian would mark it
// release-pending — and its pre-write release would delete the required
// fields it solely owns (spec.selector, the container image), failing
// validation on every retry. The guardian must leave the owner list
// before the partition, leaving the full-body owner on the handoff path.
func TestDeployPreviousFieldOwnerMigrationGuardianSharing(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, true) // stripped cache

	// The tracker (the informer cache's source) holds the object under
	// the full-body previous owner, which solely owns everything
	// including the stale port and the user-tuned values.
	seed := probeDeployment(8080)
	setProbeReplicasAndResources(t, &seed, 3, "500m")
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	// The live (API server) view: the full-body previous owner's claims,
	// the converged guardian's reduced claims from the first migration,
	// and no entry for the new current owner yet. (Hand-set managed
	// fields cannot be pushed through the controller fake client: its
	// tracker recomputes them on every write. The metadata fake preserves
	// them as given.)
	live := probeDeployment(8080)
	setProbeReplicasAndResources(t, &live, 3, "500m")
	live.SetGroupVersionKind(gvk.Deployment)
	live.SetManagedFields([]metav1.ManagedFieldsEntry{
		applyEntryFor(legacyProbeOwner, probeDeploymentFieldset(8080)),
		applyEntryFor(strayLegacyProbeOwner, guardianProbeFieldset(nil)),
	})
	env.metadataCli = newMetadataFake(t, live.DeepCopy())

	applied = nil // the seeding apply is not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModePatch),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner, strayLegacyProbeOwner),
	}

	// The desired body carries different values, which the patch
	// customizer strips from the body the current owner writes.
	desired := probeDeployment(8443)
	setProbeReplicasAndResources(t, &desired, 1, "100m")

	stored := requireDeployed(t, env, desired, opts...)

	// The full-body owner was handed off and released after the write
	// (the current owner's patch write records no event); the guardian
	// produced nothing. A pre-write release of the full-body owner is
	// the wedging behavior this test forbids.
	assert.Equal(t, []string{
		legacyProbeOwner + "/apply",
		legacyProbeOwner + "/release",
	}, env.events, "the full-body owner must be handed off, not released before the write over its sharing with the guardian")

	assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, stored))
	managers := applyManagers(t, storedProbeUnstripped(t, env))
	assert.Contains(t, managers, currentProbeOwner+" (Apply)")
	assert.NotContains(t, managers, legacyProbeOwner+" (Apply)")

	// The required fields the full-body owner solely owns must survive:
	// a pre-write release would have pruned them (the release simulation
	// models the server's deletion of solely-owned unclaimed fields).
	_, found, err := unstructured.NestedMap(stored.Object, "spec", "selector")
	require.NoError(t, err)
	require.True(t, found, "spec.selector must not be deleted by the migration")

	containers, found, err := unstructured.NestedSlice(stored.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, containers)

	container, ok := containers[0].(map[string]any)
	require.True(t, ok)

	image, found, err := unstructured.NestedString(container, "image")
	require.NoError(t, err)
	require.True(t, found, "the container image must not be deleted by the migration")
	assert.Equal(t, "quay.io/example/manager:latest", image)

	// The user-tuned values survive the preserving release.
	replicas, found, err := unstructured.NestedInt64(stored.Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found, "spec.replicas must not be deleted by the release")
	assert.Equal(t, int64(3), replicas)

	cpu, found, err := unstructured.NestedString(container, "resources", "requests", "cpu")
	require.NoError(t, err)
	require.True(t, found, "container resources must not be deleted by the release")
	assert.Equal(t, "500m", cpu)
}

// TestDeployPreviousFieldOwnerMigrationPatchPendingPreserves covers the
// patch-mode recovery of a partially co-owned previous owner: an earlier
// reconciliation handed its claims off and the current owner's patch write
// succeeded (claiming everything but the user-tuned fields and the stale
// port), but the release never ran. The previous owner shares the container
// entry with the current owner, so a handoff could not drop the stale port
// entry (the current owner's write retained it) and any later merge-key
// change would wedge the object on the duplicate port name. The owner must
// be released before the current owner's write instead — with the
// preserving fragment, because the current owner's patch write omits the
// replicas and container resources the previous owner solely owns.
func TestDeployPreviousFieldOwnerMigrationPatchPendingPreserves(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, false)

	// The live Deployment is owned by the previous owner and carries
	// non-default replicas and container resources alongside the stale port.
	seed := probeDeployment(8080)
	setProbeReplicasAndResources(t, &seed, 3, "500m")
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	// The current owner's earlier patch write: the desired body, which
	// already omits the user-tuned fields (the patch customizer strips
	// them). It claims everything but the replicas, the container
	// resources and the stale port, leaving the previous owner partially
	// co-owned: sharing the container entry (release-pending) while solely
	// owning the user-tuned values and the stale entry.
	stripped, err := json.Marshal(probeDeployment(8443).Object)
	require.NoError(t, err)

	old := resources.GvkToUnstructured(gvk.Deployment)
	require.NoError(t, env.cli.Get(t.Context(), client.ObjectKey{Name: probeDeploymentName, Namespace: probeNamespace}, old))
	require.NoError(t, env.cli.Patch(t.Context(), old, client.RawPatch(types.ApplyPatchType, stripped),
		client.FieldOwner(currentProbeOwner), client.ForceOwnership))

	applied = nil // the seeding writes are not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModePatch),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	// The desired body carries different user-tuned values, which the
	// patch customizer strips from the body the current owner writes.
	desired := probeDeployment(8443)
	setProbeReplicasAndResources(t, &desired, 1, "100m")

	stored := requireDeployed(t, env, desired, opts...)

	// The previous owner was released before the write; the current
	// owner's patch write goes through Patch and records no event.
	assert.Equal(t, []string{legacyProbeOwner + "/release"}, env.events,
		"a partially co-owned owner is release-pending and must be released before the write")

	assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, stored))
	managers := applyManagers(t, stored)
	assert.Contains(t, managers, currentProbeOwner+" (Apply)")
	assert.NotContains(t, managers, legacyProbeOwner+" (Apply)")

	// The preserving fragment kept the user-tuned values: the current
	// owner's patch write omits them, so an identity-only release would
	// have deleted them (modeled by the release simulation's prune of
	// solely-owned unclaimed fields).
	replicas, found, err := unstructured.NestedInt64(stored.Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found, "spec.replicas must not be deleted by the release")
	assert.Equal(t, int64(3), replicas)

	containers, found, err := unstructured.NestedSlice(stored.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, containers, 1)

	container, ok := containers[0].(map[string]any)
	require.True(t, ok)

	cpu, found, err := unstructured.NestedString(container, "resources", "requests", "cpu")
	require.NoError(t, err)
	require.True(t, found, "container resources must not be deleted by the release")
	assert.Equal(t, "500m", cpu)
}

// TestDeployPreviousFieldOwnerMigrationReleaseAddedContainer covers a
// desired body that adds a container the live Deployment does not carry:
// the release fragment must not claim it. A skeleton for an added
// container has no live counterpart to fill, leaving a name-only entry
// that a real API server rejects (a container without an image), failing
// the pre-write release of the pending owner on every retry. The previous
// owner never claimed the added container, so omitting it releases
// nothing; the current owner's write adds it.
func TestDeployPreviousFieldOwnerMigrationReleaseAddedContainer(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, false)

	// Release-pending setup, as in PatchPendingPreserves: the previous
	// owner owns the full live Deployment (user-tuned values, the stale
	// port); the current owner's earlier patch write left it partially
	// co-owned.
	seed := probeDeployment(8080)
	setProbeReplicasAndResources(t, &seed, 3, "500m")
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	stripped, err := json.Marshal(probeDeployment(8443).Object)
	require.NoError(t, err)

	old := resources.GvkToUnstructured(gvk.Deployment)
	require.NoError(t, env.cli.Get(t.Context(), client.ObjectKey{Name: probeDeploymentName, Namespace: probeNamespace}, old))
	require.NoError(t, env.cli.Patch(t.Context(), old, client.RawPatch(types.ApplyPatchType, stripped),
		client.FieldOwner(currentProbeOwner), client.ForceOwnership))

	applied = nil // the seeding writes are not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModePatch),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	// The desired body adds a sidecar the live Deployment does not carry.
	desired := probeDeployment(8443)
	setProbeReplicasAndResources(t, &desired, 1, "100m")
	addProbeContainer(t, &desired, "sidecar", "quay.io/example/sidecar:latest")

	stored := requireDeployed(t, env, desired, opts...)

	assert.Equal(t, []string{legacyProbeOwner + "/release"}, env.events,
		"a partially co-owned owner is release-pending and must be released before the write")

	// The fragment claims only the live container: a name-only skeleton
	// for the added sidecar is what a real API server would reject.
	require.Len(t, env.releaseFragments, 1)

	skeletons, found, err := unstructured.NestedSlice(env.releaseFragments[0].Object,
		"spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)

	skeletonNames := make([]string, 0, len(skeletons))
	for _, raw := range skeletons {
		container, ok := raw.(map[string]any)
		require.True(t, ok)

		name, _, err := unstructured.NestedString(container, "name")
		require.NoError(t, err)
		skeletonNames = append(skeletonNames, name)
	}

	assert.Equal(t, []string{"manager"}, skeletonNames,
		"the release fragment must not claim a container the live Deployment does not carry")

	// The migration completed: the stale port is gone, the preserved
	// values survive, and the current owner's write added the sidecar.
	assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, stored))
	managers := applyManagers(t, stored)
	assert.Contains(t, managers, currentProbeOwner+" (Apply)")
	assert.NotContains(t, managers, legacyProbeOwner+" (Apply)")

	replicas, found, err := unstructured.NestedInt64(stored.Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found, "spec.replicas must not be deleted by the release")
	assert.Equal(t, int64(3), replicas)

	containers, found, err := unstructured.NestedSlice(stored.Object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, containers, 2, "the current owner's write must add the sidecar")
}

// TestDeployPreviousFieldOwnerMigrationSharedStaleEntry covers stale list
// entries co-owned by two configured previous owners: both applied the same
// body carrying the stale port, and the current owner has no claims. Each
// owner's handoff would retain the stale entry the other owner claims (and
// so would the current owner's write), so every apply would fail
// duplicate-name validation and retries could not progress. Both owners
// must be released before the current owner's write instead.
//
// The second release transiently hollows the object (the first release left
// the second owner as the only claimant, and its identity-only fragment
// re-claims nothing). A real API server would reject that apply as an
// invalid Deployment, the error would requeue the reconcile, and the next
// reconcile would hand the owner off (it no longer shares anything); the
// fake does not validate the intermediate object, so it converges in one
// reconcile. The final state is the same.
func TestDeployPreviousFieldOwnerMigrationSharedStaleEntry(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, false)

	// Two previous owners applied the same body, so they co-own every
	// entry, including the stale metrics port.
	seed := probeDeployment(8080)
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(strayLegacyProbeOwner)))

	applied = nil // the seeding applies are not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModeSSA),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner, strayLegacyProbeOwner),
	}

	stored := requireDeployed(t, env, probeDeployment(8443), opts...)

	// Both previous owners were released before the write: neither handoff
	// could have dropped the entry the other claims.
	assert.Equal(t, []string{
		legacyProbeOwner + "/release",
		strayLegacyProbeOwner + "/release",
		currentProbeOwner + "/apply",
	}, env.events, "co-owning previous owners must be released before the current owner's write")

	assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, stored))
	managers := applyManagers(t, stored)
	assert.Contains(t, managers, currentProbeOwner+" (Apply)")
	assert.NotContains(t, managers, legacyProbeOwner+" (Apply)")
	assert.NotContains(t, managers, strayLegacyProbeOwner+" (Apply)")
}

// TestDeployPreviousFieldOwnerMigrationUnrelatedManager covers a sharing
// with a manager this migration does not orchestrate: a user (or any other
// controller) owns part of the container entry — enough to share its
// associative-list key with the previous owner — while the previous owner
// solely owns the required fields and the stale port. Such a sharing
// cannot be resolved by releasing (the other manager keeps the shared
// entry, so the current owner's write would still merge it), and a
// pre-write release would delete the required fields the fragment does not
// re-claim, failing validation on every retry. The previous owner must
// stay on the handoff path, which drops the stale entry it solely owns.
func TestDeployPreviousFieldOwnerMigrationUnrelatedManager(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, true) // stripped cache

	// The tracker (the informer cache's source) holds the object under the
	// previous owner, which solely owns everything including the stale
	// port.
	seed := probeDeployment(8080)
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	// The live (API server) view: the previous owner's full claims, plus a
	// user manager that owns part of the container entry (its resources),
	// sharing the container's associative-list key. (Hand-set managed
	// fields cannot be pushed through the controller fake client: its
	// tracker recomputes them on every write. The metadata fake preserves
	// them as given.)
	live := probeDeployment(8080)
	live.SetGroupVersionKind(gvk.Deployment)
	live.SetManagedFields([]metav1.ManagedFieldsEntry{
		applyEntryFor(legacyProbeOwner, probeDeploymentFieldset(8080)),
		applyEntryFor("user-controller", map[string]any{
			"f:spec": map[string]any{
				"f:template": map[string]any{
					"f:spec": map[string]any{
						"f:containers": map[string]any{
							"k:" + string(mustMarshalJSON(map[string]any{"name": "manager"})): map[string]any{
								"f:resources": map[string]any{"f:requests": map[string]any{"f:cpu": map[string]any{}}},
							},
						},
					},
				},
			},
		}),
	})
	env.metadataCli = newMetadataFake(t, live.DeepCopy())

	applied = nil // the seeding apply is not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModeSSA),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	stored := requireDeployed(t, env, probeDeployment(8443), opts...)

	// The previous owner was handed off, not released first: the sharing
	// with the user manager must not authorize a pre-write release that
	// would delete the required fields the previous owner solely owns.
	assert.Equal(t, []string{
		legacyProbeOwner + "/apply",
		currentProbeOwner + "/apply",
		legacyProbeOwner + "/release",
	}, env.events, "a sharing with an unorchestrated manager must keep the owner on the handoff path")

	assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, stored))
	managers := applyManagers(t, storedProbeUnstripped(t, env))
	assert.Contains(t, managers, currentProbeOwner+" (Apply)")
	assert.NotContains(t, managers, legacyProbeOwner+" (Apply)")
}

// TestDeployPreviousFieldOwnerMigrationEmptyLiveRead covers a live object
// whose managed fields were cleared entirely (Kubernetes permits it): a
// successful metadata read with no entries is a valid empty view — no
// manager owns anything, so there is nothing to migrate — and must not be
// mistaken for an unavailable metadata client, which would fail the deploy
// on every reconcile. The deployment proceeds to the current owner's
// write.
func TestDeployPreviousFieldOwnerMigrationEmptyLiveRead(t *testing.T) {
	t.Parallel()

	var applied []string
	env := newOwnerMigrationEnv(t, &applied, true) // stripped cache

	// The tracker (the informer cache's source) holds the object under the
	// previous owner; the live (API server) view carries no managed fields
	// at all. (Hand-set managed fields cannot be pushed through the
	// controller fake client: its tracker recomputes them on every write.
	// The metadata fake preserves them as given — here, their absence.)
	seed := probeDeployment(8443)
	require.NoError(t, resources.Apply(t.Context(), env.cli, &seed, client.ForceOwnership, client.FieldOwner(legacyProbeOwner)))

	live := probeDeployment(8443)
	live.SetGroupVersionKind(gvk.Deployment)
	env.metadataCli = newMetadataFake(t, live.DeepCopy())

	applied = nil // the seeding apply is not part of the action under test
	env.events = nil

	opts := []deploy.ActionOpts{
		deploy.WithMode(deploy.ModeSSA),
		deploy.WithFieldOwner(currentProbeOwner),
		deploy.WithPreviousFieldOwners(legacyProbeOwner),
	}

	stored := requireDeployed(t, env, probeDeployment(8443), opts...)

	// No migration ran (no previous owner exists in the live view) and the
	// current owner's write proceeded.
	assert.Equal(t, []string{currentProbeOwner + "/apply"}, env.events,
		"no previous owner exists in the live view; the deploy must proceed to the current owner's write")
	assert.ElementsMatch(t, []string{"metrics:8443", "health:8081"}, ownerMigrationPorts(t, stored))
	managers := applyManagers(t, storedProbeUnstripped(t, env))
	assert.Contains(t, managers, currentProbeOwner+" (Apply)", "the current owner's write must run")
}
