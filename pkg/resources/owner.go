package resources

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RemoveOwnerReferences removes all owner references from obj that match
// predicate and persists the change via cli.Update. If no references match,
// the update is skipped.
func RemoveOwnerReferences(
	ctx context.Context,
	cli client.Client,
	obj client.Object,
	predicate func(reference metav1.OwnerReference) bool,
) error {
	oldRefs := obj.GetOwnerReferences()
	if len(oldRefs) == 0 {
		return nil
	}

	newRefs := make([]metav1.OwnerReference, 0, len(oldRefs))
	for _, ref := range oldRefs {
		if !predicate(ref) {
			newRefs = append(newRefs, ref)
		}
	}

	if len(newRefs) == len(oldRefs) {
		return nil
	}

	obj.SetOwnerReferences(newRefs)

	err := cli.Update(ctx, obj)
	if err != nil {
		return fmt.Errorf(
			"failed to remove owner references from object %s/%s with gvk %s: %w",
			obj.GetNamespace(),
			obj.GetName(),
			obj.GetObjectKind().GroupVersionKind(),
			err,
		)
	}

	return nil
}

// IsOwnedByType returns true if obj has an owner reference whose GVK matches
// ownerGVK.
func IsOwnedByType(obj client.Object, ownerGVK schema.GroupVersionKind) (bool, error) {
	for _, ref := range obj.GetOwnerReferences() {
		av, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return false, err
		}

		if av.Group == ownerGVK.Group && av.Version == ownerGVK.Version && ref.Kind == ownerGVK.Kind {
			return true, nil
		}
	}

	return false, nil
}
