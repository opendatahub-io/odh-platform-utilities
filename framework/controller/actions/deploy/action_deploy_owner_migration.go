package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/opendatahub-io/odh-platform-utilities/framework/cluster/gvk"
	odhTypes "github.com/opendatahub-io/odh-platform-utilities/framework/controller/types"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/metadata"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ownerMigration carries the state of an ownership migration across the
// current owner's write: which previous owners were handed off (and must be
// released afterwards), and the handoff failures to surface once the write
// and the releases have completed.
type ownerMigration struct {
	// migrated lists the previous owners whose claims were handed off to the
	// current owner and whose release must follow the current owner's write.
	migrated []string

	// deferred holds the handoff failures, if any. A failed handoff does not
	// abort the migration (see migratePreviousFieldOwners), but the reconcile
	// must requeue so the failed owner is retried: the error is therefore
	// surfaced only after the current owner's write and the releases have
	// completed, leaving the cluster in the best state reached so far.
	deferred error

	// patchOmitsUserFields records, from before the current owner's write,
	// whether that write omits the Deployment user-tuned fields (see
	// patchWriteOmitsUserFields). The write decodes the server response
	// into old, which can flip a re-evaluation made after it — e.g. when
	// the desired body introduces the managed-by annotation — so the
	// release fragments must use this captured value.
	patchOmitsUserFields bool
}

// migratePreviousFieldOwners hands off SSA field ownership of the live object
// from any configured previous field owner to the current one, before the
// desired state is applied. It returns the migration state, whose migrated
// owners must be released after the current owner's write.
//
// Server-side apply releases an associative-list entry only when its owning
// manager applies again without it. A manager that no longer reconciles the
// object therefore keeps its entries forever: when a controller changes its
// field owner (e.g. after a CRD refactor) and the desired state also changes
// a list merge key (e.g. a container port number), the previous owner's stale
// entry is retained in the merge and the resulting object fails validation on
// every apply. Re-applying the desired body once under the previous owner
// releases those stale entries, after which the current owner converges the
// object normally.
//
// Migration is restricted to the owners configured via WithPreviousFieldOwners
// so that fields applied by users or unrelated managers are never taken over.
// It is skipped when the object does not exist yet. The handoff itself is not
// enough: because both owners then apply the same body, the previous owner
// would keep co-owning every field, and co-ownership of an associative-list
// entry wedges any later merge-key change of that entry (neither manager can
// drop the old entry alone, so the merged object carries two entries with
// the same name and is rejected by validation on every apply). The migrated
// owners must therefore be passed to releasePreviousFieldOwners once the
// current owner's write has succeeded, which completes the transfer.
//
// Two failure and recovery cases receive specific treatment:
//
//   - A previous owner that shares at least one associative-list entry or
//     set member with the current owner or another configured previous
//     owner is release-pending: an earlier reconciliation handed its claims
//     off (or another previous owner applied the same body) but its release
//     never completed. Its handoff is exactly the wedging apply described
//     above (server-side apply retains entries another of those managers
//     claims, and the merged object then carries duplicate list keys on any
//     merge-key change), so it is released first instead. Releasing is
//     safe: the shared entries stay (the other manager keeps them), and
//     the owner's remaining claims are either the Deployment user-tuned
//     fields the current owner's patch-mode write deliberately leaves to it
//     (kept by the preserving release fragment) or stale claims of a
//     defunct controller (deleted, which is the migration's purpose).
//     Sharing with any other manager — e.g. a user tuning the Deployment —
//     does not count: see releasePendingFrom. That safety depends on the
//     ownership state holding at release time, so when the pending
//     decision was based on the informer cache it is remade from a fresh
//     read of the live object: a cached view can predate the latest writes
//     and present a sharing that no longer holds. Without a dynamic client
//     there is no fresher view than the cache; the residual race between
//     the read and the release is then accepted (even with the fresh read
//     the window is only bounded to the read-to-apply gap, not eliminated —
//     no Kubernetes mechanism closes it).
//
//   - One previous owner's failed handoff does not abort the others: the
//     failure is typically caused by another previous owner's stale entries
//     (the merged object carries duplicate list keys), and the owner of
//     those entries gets its turn later in the same loop or on the next
//     reconciliation. The failure is deferred until after the current
//     owner's write so the reconcile requeues and retries.
//
//   - In patch mode, a previous owner that a preserving release has already
//     reduced to the Deployment user-tuned fields is the owner of record
//     for them and has nothing stale to migrate: it is skipped, so the
//     steady state converges instead of re-running the handoff and release
//     on every reconcile (each of which would write and expand its claims
//     back). It leaves the owner list before the release-pending
//     partition, so its shared container merge keys cannot mark another
//     owner release-pending. See dropConvergedGuardians.
//
// # Known limitations
//
// The migration is safe for the configuration the framework's current
// consumers use: a single configured previous owner, and a current owner
// whose write carries the complete desired body (SSA mode, or patch mode
// where the patch still claims every field but the user-tuned ones). There,
// any sharing with the current owner implies it co-owns every required
// field, so a pre-write release deletes nothing the object needs. Beyond
// that configuration, three scenarios are known to wedge or hollow
// transiently:
//
//   - A sharing partner whose entry claims only a fragment of the body
//     (e.g. another previous owner that only ever wrote one container's
//     resources — but not a converged guardian, which is dropped from the
//     owner list before the partition) still marks the owner
//     release-pending, and the pre-write release deletes the required
//     fields that partner does not co-own; the release fails validation
//     and retries cannot progress.
//
//   - Previous owners with disjoint stale entries (one owning container
//     A's old port, another container B's) are handed off separately, and
//     each handoff and the current owner's write retain the other owner's
//     stale entry beside its replacement, failing duplicate-key
//     validation; only that other owner's own migration removes it.
//
//   - When several release-pending owners are released before the write,
//     the last claimant's release deletes the fields it solely owns (e.g.
//     a ConfigMap's data) that only the current owner's write would
//     restore; a failed write leaves them deleted until the next
//     reconcile restores them.
//
// The general fix — release fragments that re-claim, at field granularity,
// exactly the solely-owned fields the desired state carries — requires
// aligning desired bodies with managed-fields fieldsets and is future
// work.
func (a *Action) migratePreviousFieldOwners(
	ctx context.Context,
	rr *odhTypes.ReconciliationRequest,
	obj *unstructured.Unstructured,
	old *unstructured.Unstructured,
	fieldOwner string,
) (*ownerMigration, error) {
	m := &ownerMigration{}

	if old == nil || len(a.previousFieldOwners) == 0 {
		return m, nil
	}

	// Captured before any write: the current owner's write decodes the
	// server response into old, which can flip a later re-evaluation of
	// the predicate (e.g. when the desired body introduces the managed-by
	// annotation).
	m.patchOmitsUserFields = a.patchWriteOmitsUserFields(obj, old)

	managed, fromCache, err := liveManagedFields(ctx, rr, old)
	if err != nil {
		return nil, err
	}

	previousOwners := previousFieldOwnersFrom(managed, fieldOwner, a.previousFieldOwners)

	pending, handoff := partitionPreviousFieldOwners(previousOwners, managed, fieldOwner)

	if len(pending) > 0 && fromCache {
		// The release-pending partition may come from a stale cached view
		// that predates the current owner's latest write; see the
		// release-pending bullet in the doc comment above.
		fresh, err := readLiveManagedFields(ctx, rr, old)
		if err != nil {
			return nil, err
		}

		if fresh != nil {
			managed = fresh

			// The owner list must come from the same view as the
			// partition: an owner whose entry appeared only after the
			// cached snapshot would otherwise be missed for this
			// reconcile, and one that has since vanished would get a
			// pointless handoff and release.
			previousOwners = previousFieldOwnersFrom(managed, fieldOwner, a.previousFieldOwners)

			pending, handoff = partitionPreviousFieldOwners(previousOwners, managed, fieldOwner)
		}
	}

	if m.patchOmitsUserFields {
		// The converged guardians must leave the owner list before it
		// feeds the final partition: a guardian's reduced fieldset
		// shares the container merge keys with any full-body previous
		// owner, and counting it as a sharing partner would mark that
		// owner release-pending — whose pre-write release would delete
		// the required fields the guardian does not co-own (see
		// dropConvergedGuardians).
		previousOwners = a.dropConvergedGuardians(ctx, obj, managed, previousOwners)
		pending, handoff = partitionPreviousFieldOwners(previousOwners, managed, fieldOwner)
	}

	if len(pending) > 0 {
		if err := a.releasePreviousFieldOwners(ctx, rr.Client, obj, old, m.patchOmitsUserFields, fieldOwner, pending); err != nil {
			return nil, err
		}
	}

	var failures []error

	for _, owner := range handoff {
		body, err := a.handoffBody(obj, old)
		if err != nil {
			return nil, err
		}

		if err := resources.Apply(ctx, rr.Client, body, client.ForceOwnership, client.FieldOwner(owner)); err != nil {
			logf.FromContext(ctx).Error(err, "failed to hand off SSA field ownership from previous field owner",
				"gvk", obj.GroupVersionKind(),
				"name", client.ObjectKeyFromObject(obj),
				"previousOwner", owner,
				"owner", fieldOwner,
			)

			failures = append(failures, fmt.Errorf("failed to hand off SSA field ownership from %q for %s/%s: %w",
				owner, obj.GetNamespace(), obj.GetName(), err))

			continue
		}

		m.migrated = append(m.migrated, owner)

		logf.FromContext(ctx).Info("migrated SSA field ownership from previous field owner",
			"gvk", obj.GroupVersionKind(),
			"name", client.ObjectKeyFromObject(obj),
			"previousOwner", owner,
			"owner", fieldOwner,
		)
	}

	m.deferred = errors.Join(failures...)

	return m, nil
}

// handoffBody returns the body a previous owner applies during the handoff:
// the desired state, except that the live values are restored for the fields
// the current owner's write omits (see patchWriteOmitsUserFields). The
// previous owner may be the sole owner of those fields, and a server-side
// apply of a body that omits them would delete them from the live object —
// while the current owner's subsequent write omits them as well, so the
// values would be lost. Restoring the live values makes the handoff release
// only the stale claims.
//
// The live values come from the same informer-cache view the deploy
// action's apply customizer uses to preserve user-tuned values on every
// write (CustomizerFnApplyDeployments merges them into the applied body the
// same way), so the handoff carries the same read-to-apply race that
// mechanism has always had: a user tuning the Deployment between the read
// and the apply has the stale value written back. The window is bounded to
// a single reconcile's read-to-apply gap; this is the deploy action's
// established value-preservation model, not a new exposure. In patch mode
// the handoff is the one forced write that carries these fields at all
// (the patch itself omits them), so the race touches it once per
// migration; the alternative — omitting them from the handoff body —
// deletes them from the live object when the previous owner is their sole
// owner (verified against OpenShift 4.22), which is strictly worse.
func (a *Action) handoffBody(
	obj *unstructured.Unstructured,
	old *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	// A deep copy is used because resources.Apply may write the applied
	// object back into the given object; the desired body handed to the
	// current owner's apply afterwards must remain free of server-side
	// fields such as managedFields.
	body := obj.DeepCopy()

	if !a.patchWriteOmitsUserFields(body, old) {
		return body, nil
	}

	if err := MergeDeployments(old, body); err != nil {
		return nil, fmt.Errorf("failed to merge live Deployment values into the handoff body for %s/%s: %w",
			body.GetNamespace(), body.GetName(), err)
	}

	return body, nil
}

// patchWriteOmitsUserFields reports whether the body the current owner
// writes omits the Deployment user-tuned fields (spec.replicas and container
// resources): patch mode, for a Deployment that the operator does not
// explicitly manage. The patch customizer strips those fields so user-tuned
// values survive the write, which leaves the writer owning nothing under
// them — a migrated previous owner restoring them into its handoff body
// would remain their sole owner.
func (a *Action) patchWriteOmitsUserFields(obj *unstructured.Unstructured, old *unstructured.Unstructured) bool {
	return a.deployMode == ModePatch &&
		old != nil &&
		obj.GroupVersionKind() == gvk.Deployment &&
		resources.GetAnnotation(old, a.managedByAnnotation) != "true"
}

// releasePreviousFieldOwners releases the claims of the given previous
// owners: each applies a fragment that keeps only the fields the fragment
// carries, dropping the rest of its managedFields entry, so that no later
// merge-key change can wedge on the entries it no longer specifies.
//
// The fragment is identity-only (apiVersion/kind/metadata) unless the
// current owner's write omits the Deployment user-tuned fields (see
// patchWriteOmitsUserFields): the released owner may then be the sole owner
// of the live replicas and container resources — a handed-off owner's body
// restored those values precisely because the current owner's write strips
// them — and an identity-only fragment would delete them from the live
// object (a server-side apply by the sole owner of a field deletes it when
// the body omits it; verified against OpenShift 4.22). The preserving
// fragment re-claims exactly those fields: the values survive the release
// and the owner's fieldset is reduced to them, leaving it as the owner of
// record for the user-tuned values — the same state the pre-migration
// controller was in.
//
// Handed-off owners are released after the current owner's write succeeded:
// the current owner then owns every claim they were handed, so no entry
// loses its last owner and the live object is unchanged. Release-pending
// owners (see releasePendingFrom) are released before the write instead:
// their shared entries are held by another manager, and the claims nothing
// else carries are either re-claimed by the preserving fragment or stale
// claims of a defunct controller whose deletion is the migration's purpose;
// anything else the current owner's write restores in the same
// reconciliation.
//
// The preserving fragment re-claims the user-tuned fields even when the
// released owner acquired part of those claims only during its handoff
// (e.g. a user had solely owned spec.replicas). The retained claims are
// inert — the defunct owner never applies again — and consistent with the
// framework's SSA-mode value-preservation model, where the apply customizer
// co-claims the user-tuned fields on every write. A user who solely owned
// such a field recovers sole ownership with a single forced apply; until
// then, an apply that omits the field is silently retained rather than
// removed, since the defunct owner's claim keeps the value alive.
//
// The fragment carries the uid of the object being released, which turns
// the apply into a precondition on its identity: server-side apply creates
// absent objects, so a release issued after the object was deleted would
// otherwise create an empty replacement (e.g. a ConfigMap without data or
// owner references) while the deploy reports success. With the uid, a
// vanished or replaced object fails the apply with a conflict and the next
// reconcile converges instead.
func (a *Action) releasePreviousFieldOwners(
	ctx context.Context,
	cli client.Client,
	obj *unstructured.Unstructured,
	old *unstructured.Unstructured,
	patchOmitsUserFields bool,
	fieldOwner string,
	previousOwners []string,
) error {
	for _, owner := range previousOwners {
		release, err := a.releaseBody(obj, old, patchOmitsUserFields)
		if err != nil {
			return err
		}

		if err := resources.Apply(ctx, cli, release, client.FieldOwner(owner)); err != nil {
			// A vanished object has nothing left to release. Unreachable in
			// practice when the uid precondition is set (the apply fails
			// with a conflict instead); kept for safety.
			if k8serr.IsNotFound(err) {
				continue
			}

			return fmt.Errorf("failed to release SSA field ownership from %q for %s/%s: %w",
				owner, obj.GetNamespace(), obj.GetName(), err)
		}

		logf.FromContext(ctx).Info("released SSA field ownership of previous field owner",
			"gvk", obj.GroupVersionKind(),
			"name", client.ObjectKeyFromObject(obj),
			"previousOwner", owner,
			"owner", fieldOwner,
		)
	}

	return nil
}

// releaseBody returns the fragment a previous owner applies to release its
// migrated claims: object identity (with the uid precondition described in
// releasePreviousFieldOwners) plus, when the current owner's write omits the
// Deployment user-tuned fields, the live values of those fields. The
// released owner may be their sole owner — a handed-off owner's body
// restored them because the current owner's write strips them — so an
// identity-only fragment would delete them from the live object.
// Re-claiming them keeps the values and reduces the released owner's
// fieldset to exactly the fields nobody else carries.
//
// patchOmitsUserFields is the pre-write evaluation of that condition (see
// ownerMigration.patchOmitsUserFields): it must not be re-derived here,
// because the current owner's write decodes the server response into old
// and a desired body that introduces the managed-by annotation would flip
// the answer, degrading the release to identity-only and deleting the
// values the handoff just preserved.
//
// The Deployment-specific construction of the preserving fragment lives in
// preserveDeploymentUserFields, alongside the other Deployment merge logic.
func (a *Action) releaseBody(
	obj *unstructured.Unstructured,
	old *unstructured.Unstructured,
	patchOmitsUserFields bool,
) (*unstructured.Unstructured, error) {
	release := &unstructured.Unstructured{}
	release.SetAPIVersion(obj.GetAPIVersion())
	release.SetKind(obj.GetKind())
	release.SetName(obj.GetName())
	release.SetNamespace(obj.GetNamespace())

	if old != nil {
		release.SetUID(old.GetUID())
	}

	if !patchOmitsUserFields {
		return release, nil
	}

	if err := preserveDeploymentUserFields(obj, old, release); err != nil {
		return nil, err
	}

	return release, nil
}

// liveManagedFields returns the managed fields of the live object and
// whether they came from the informer cache, preferring the cache when it
// carries them. Controllers commonly strip managedFields from cached objects
// to reduce memory usage (e.g. via a cache TransformFunc, as the
// opendatahub-operator does); a stripped cache would hide the previous owner
// and the migration would never fire. When the cached object carries no
// managed fields, the object is therefore re-read directly from the API
// server through the metadata client, bypassing the cache. When no metadata
// client is available, the ownership state is unknowable and an error is
// returned rather than treating the empty cached view as proof that no
// previous owner exists.
//
// A cached view may be stale (it can predate the current owner's latest
// write), which is why the fromCache flag exists: decisions whose safety
// depends on the ownership state holding at a later point — the
// release-pending partition — must be remade from a fresh read via
// readLiveManagedFields.
func liveManagedFields(
	ctx context.Context,
	rr *odhTypes.ReconciliationRequest,
	old *unstructured.Unstructured,
) ([]metav1.ManagedFieldsEntry, bool, error) {
	managed := old.GetManagedFields()
	if len(managed) > 0 {
		return managed, true, nil
	}

	live, err := readLiveManagedFields(ctx, rr, old)
	if err != nil {
		return nil, false, err
	}

	if live == nil {
		// The ownership state is unknowable: an empty cached view is not
		// proof that no previous owner exists. Failing loudly surfaces the
		// misconfiguration (a Controller without a metadata client combined
		// with a cache that strips managed fields) instead of silently
		// skipping the configured handoff and wedging the deployment.
		return nil, false, fmt.Errorf(
			"cannot determine whether %s/%s still carries a previous field owner: the cached object has no managed fields and no metadata client is available to read the live object",
			old.GetNamespace(), old.GetName())
	}

	return live, false, nil
}

// readLiveManagedFields reads the managed fields of the object directly
// from the API server through the metadata client, bypassing the informer
// cache and fetching only the object's metadata instead of the whole
// resource. It returns a nil slice only when no metadata client is
// available; a successful read always yields a non-nil slice, empty when
// the live object carries no managed fields (Kubernetes permits clearing
// them, and an empty live view is a valid answer — no owner holds
// anything — not an unavailable one).
func readLiveManagedFields(
	ctx context.Context,
	rr *odhTypes.ReconciliationRequest,
	old *unstructured.Unstructured,
) ([]metav1.ManagedFieldsEntry, error) {
	metaCli := rr.Controller.GetMetadataClient()
	if metaCli == nil {
		return nil, nil
	}

	gvk := old.GroupVersionKind()

	mapping, err := rr.Client.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to map %s for the managed fields of %s/%s: %w",
			gvk, old.GetNamespace(), old.GetName(), err)
	}

	var res metadata.ResourceInterface = metaCli.Resource(mapping.Resource)
	if old.GetNamespace() != "" {
		res = metaCli.Resource(mapping.Resource).Namespace(old.GetNamespace())
	}

	live, err := res.Get(ctx, old.GetName(), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to read the managed fields of %s/%s from the API server: %w",
			old.GetNamespace(), old.GetName(), err)
	}

	managed := live.ManagedFields
	if managed == nil {
		managed = []metav1.ManagedFieldsEntry{}
	}

	return managed, nil
}

// previousFieldOwnersFrom returns the distinct previous owners among the
// given managed-fields entries: Apply-operation entries of the main resource
// that are configured previous owners of the current owner. Update-operation
// managers (e.g. kube-controller-manager), subresource entries and entries
// that own no fields are never considered: none of them can strand
// associative-list entries.
func previousFieldOwnersFrom(managed []metav1.ManagedFieldsEntry, fieldOwner string, configured []string) []string {
	var previousOwners []string

	for _, entry := range managed {
		if entry.Operation != metav1.ManagedFieldsOperationApply {
			continue
		}

		if entry.Subresource != "" {
			continue
		}

		if ownsNoFields(entry) {
			continue
		}

		if entry.Manager == fieldOwner {
			continue
		}

		if slices.Contains(configured, entry.Manager) && !slices.Contains(previousOwners, entry.Manager) {
			previousOwners = append(previousOwners, entry.Manager)
		}
	}

	return previousOwners
}

// ownsNoFields reports whether the managed-fields entry claims no fields.
// An owner of nothing cannot strand associative-list entries, so it must not
// trigger a migration: some API servers retain Apply entries whose fieldset
// has been emptied instead of dropping them.
func ownsNoFields(entry metav1.ManagedFieldsEntry) bool {
	if entry.FieldsV1 == nil || len(entry.FieldsV1.Raw) == 0 {
		return true
	}

	var fields map[string]any
	if err := json.Unmarshal(entry.FieldsV1.Raw, &fields); err != nil {
		// Malformed entry: assume it owns fields rather than ignoring a
		// possible previous owner.
		return false
	}

	return len(fields) == 0
}

// partitionPreviousFieldOwners splits the given previous owners, based on
// the given managed-fields view, into release-pending owners (released
// before the current owner's write; see releasePendingFrom) and owners to
// hand off. The safety of the pending partition depends on the view's
// freshness, which is why migratePreviousFieldOwners re-partitions from a
// live read when the first view came from the informer cache.
func partitionPreviousFieldOwners(previousOwners []string, managed []metav1.ManagedFieldsEntry, fieldOwner string) (pending, handoff []string) {
	// The managers whose claim-dropping this migration orchestrates: the
	// current owner (its write follows and drops its own stale claims) and
	// the other configured previous owners (released in the same pass).
	orchestrated := make(map[string]struct{}, len(previousOwners)+1)
	orchestrated[fieldOwner] = struct{}{}

	for _, owner := range previousOwners {
		orchestrated[owner] = struct{}{}
	}

	for _, owner := range previousOwners {
		if releasePendingFrom(managed, owner, orchestrated) {
			pending = append(pending, owner)
		} else {
			handoff = append(handoff, owner)
		}
	}

	return pending, handoff
}

// dropConvergedGuardians removes the converged guardians from the given
// previous owners: owners whose fieldset a preserving release has already
// reduced to the Deployment user-tuned fields for the containers the
// desired state carries (see convergedGuardianFieldset). Such an owner is
// the owner of record for those fields — the state the pre-migration
// controller was in — and migrating it again would expand its claims back
// to the whole body and re-release them, writing on every reconcile
// without ever converging. Owners holding anything else, including claims
// for containers the desired state no longer carries (which block their
// removal), are kept for the normal migration.
//
// The guardians must be dropped before the owners feed the release-pending
// partition, not filtered from its result: a guardian's reduced fieldset
// shares the container merge keys with any full-body previous owner, and
// counting it as a sharing partner would mark that owner release-pending —
// whose pre-write release would delete the required fields the guardian
// does not co-own (e.g. spec.selector, the container image), failing
// validation on every retry. Dropping the guardian first leaves the
// full-body owner without an orchestrated sharing partner, on the handoff
// path.
func (a *Action) dropConvergedGuardians(
	ctx context.Context,
	obj *unstructured.Unstructured,
	managed []metav1.ManagedFieldsEntry,
	previousOwners []string,
) []string {
	desiredContainers := deploymentContainerNames(obj)

	kept := make([]string, 0, len(previousOwners))

	for _, owner := range previousOwners {
		if convergedGuardianFieldset(applyFieldsetOf(managed, owner), desiredContainers) {
			logf.FromContext(ctx).V(1).Info("previous field owner already holds only the preserved Deployment user fields; nothing to migrate",
				"gvk", obj.GroupVersionKind(),
				"name", client.ObjectKeyFromObject(obj),
				"previousOwner", owner,
			)

			continue
		}

		kept = append(kept, owner)
	}

	return kept
}

// convergedGuardianFieldset reports whether the fieldset holds only the
// claims a preserving release leaves behind: spec.replicas and, under the
// container entries the desired state carries, the merge key and the
// resources. Anything else — labels, images, ports, or claims for
// containers the desired state dropped — is stale and must be handed off.
// Malformed fieldsets are conservatively reported as not converged.
func convergedGuardianFieldset(fields []byte, desiredContainers map[string]struct{}) bool {
	var fieldset map[string]any
	if err := json.Unmarshal(fields, &fieldset); err != nil {
		return false
	}

	if len(fieldset) == 0 {
		return true
	}

	spec, ok := fieldset["f:spec"].(map[string]any)
	if !ok || len(fieldset) != 1 {
		return false
	}

	for key, value := range spec {
		switch key {
		case "f:replicas":
		case "f:template":
			template, ok := value.(map[string]any)
			if !ok || len(template) != 1 {
				return false
			}

			templateSpec, ok := template["f:spec"].(map[string]any)
			if !ok || len(templateSpec) != 1 {
				return false
			}

			containers, ok := templateSpec["f:containers"].(map[string]any)
			if !ok {
				return false
			}

			for entryKey, rawEntry := range containers {
				mergeKeysJSON, ok := strings.CutPrefix(entryKey, "k:")
				if !ok {
					return false
				}

				var mergeKeys map[string]any
				if err := json.Unmarshal([]byte(mergeKeysJSON), &mergeKeys); err != nil {
					return false
				}

				name, ok := mergeKeys["name"].(string)
				if !ok {
					return false
				}

				if _, desired := desiredContainers[name]; !desired {
					return false
				}

				entry, ok := rawEntry.(map[string]any)
				if !ok {
					return false
				}

				for field := range entry {
					switch field {
					case ".", "f:name", "f:resources":
					default:
						return false
					}
				}
			}
		default:
			return false
		}
	}

	return true
}

// applyFieldsetOf returns the raw fieldset of the owner's main-resource
// Apply entry, or nil when it has none.
func applyFieldsetOf(managed []metav1.ManagedFieldsEntry, owner string) []byte {
	for i := range managed {
		entry := &managed[i]

		if entry.Manager == owner && entry.Operation == metav1.ManagedFieldsOperationApply && entry.Subresource == "" {
			return entry.FieldsV1.Raw
		}
	}

	return nil
}

// deploymentContainerNames returns the names of the containers in the
// desired Deployment body. A nil map makes every container claim look
// stale, which is the conservative direction.
func deploymentContainerNames(obj *unstructured.Unstructured) map[string]struct{} {
	containers, _, err := unstructured.NestedSlice(obj.Object, deploymentContainersPath...)
	if err != nil {
		return nil
	}

	names := make(map[string]struct{}, len(containers))
	for _, raw := range containers {
		container, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if name, ok := container["name"].(string); ok {
			names[name] = struct{}{}
		}
	}

	return names
}

// releasePendingFrom reports whether the given previous owner is a
// release-pending owner: one whose fieldset shares at least one
// associative-list entry or set member with the current owner or another
// configured previous owner — the managers whose claim-dropping this
// migration orchestrates. Such an owner was handed off in an earlier
// reconciliation whose release did not complete (e.g. a transient error
// after the current owner's write), or another previous owner applied the
// same body. Its handoff could not drop the shared entries (server-side
// apply retains entries another of those managers claims) and would
// instead wedge the object on the duplicate list keys of any subsequent
// merge-key change, so it must be released rather than handed off again.
// Releasing it is safe: the shared entries stay (the other manager keeps
// them), and the owner's remaining claims are either the Deployment
// user-tuned fields the current owner's patch-mode write deliberately
// leaves to it (kept by the preserving release fragment) or stale claims
// of a defunct controller (deleted, which is the migration's purpose).
//
// Sharing with any other manager — e.g. a user tuning the Deployment —
// does not make an owner release-pending: the release cannot resolve such
// a sharing (the other manager keeps the shared entry, so the current
// owner's write would still merge it), while a pre-write release could
// delete required fields the previous owner solely owns (the fragment
// re-claims only the user-tuned fields) and wedge the object on failed
// validation on every reconcile. The handoff is the strictly better path
// there: it drops every stale entry the owner solely owns, and entries
// shared with unorchestrated managers survive either path.
func releasePendingFrom(managed []metav1.ManagedFieldsEntry, owner string, orchestrated map[string]struct{}) bool {
	ownerFields := applyFieldsetOf(managed, owner)
	if len(ownerFields) == 0 {
		// No claims to share: either the entry is absent from this view or
		// it owns nothing, and the handoff is the right (or a harmless)
		// treatment.
		return false
	}

	for i := range managed {
		entry := &managed[i]

		if entry.Manager == owner || entry.Subresource != "" || entry.FieldsV1 == nil {
			continue
		}

		if _, relevant := orchestrated[entry.Manager]; !relevant {
			continue
		}

		if hasSharedAssociativeKey(ownerFields, entry.FieldsV1.Raw) {
			return true
		}
	}

	return false
}

// hasSharedAssociativeKey reports whether the two fieldsets claim the same
// associative-list entry or set member: a "k:<merge-keys>" or "v:<value>"
// key present in both trees at the same path. Managed-fields fieldsets are
// trees keyed by "f:<name>" (regular fields), "k:<merge-keys>"
// (associative-list entries), "v:<value>" (set members) and "." (the node
// itself), so a shared entry is a shared key reached along the same path
// of "f:" keys. Malformed fieldsets are conservatively reported as sharing
// nothing: a release that is not proven safe must not run.
func hasSharedAssociativeKey(a, b []byte) bool {
	var aFields, bFields map[string]any

	if err := json.Unmarshal(a, &aFields); err != nil {
		return false
	}

	if err := json.Unmarshal(b, &bFields); err != nil {
		return false
	}

	return fieldsetsShareAssociativeKey(aFields, bFields)
}

func fieldsetsShareAssociativeKey(a, b map[string]any) bool {
	for key, aValue := range a {
		bValue, present := b[key]
		if !present {
			continue
		}

		if strings.HasPrefix(key, "k:") || strings.HasPrefix(key, "v:") {
			return true
		}

		aChildren, ok := aValue.(map[string]any)
		if !ok {
			continue
		}

		bChildren, ok := bValue.(map[string]any)
		if !ok {
			continue
		}

		if fieldsetsShareAssociativeKey(aChildren, bChildren) {
			return true
		}
	}

	return false
}
