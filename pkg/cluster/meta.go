package cluster

import (
	"errors"
	"fmt"
	"maps"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/labels"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

var (
	// ErrOwnerMissingAPIVersion is returned when an owner reference has an
	// empty APIVersion.
	ErrOwnerMissingAPIVersion = errors.New("owner has no APIVersion set")

	// ErrOwnerMissingKind is returned when an owner object has no Kind set.
	ErrOwnerMissingKind = errors.New("owner has no Kind set (call SetGroupVersionKind first)")

	// ErrOwnerMissingName is returned when an owner object has an empty Name.
	ErrOwnerMissingName = errors.New("owner has no Name set")

	// ErrOwnerMissingUID is returned when an owner object has an empty UID.
	ErrOwnerMissingUID = errors.New("owner has no UID set")

	// ErrOwnerNil is returned when a nil owner is passed to OwnerRefFrom.
	ErrOwnerNil = errors.New("owner is nil")

	// ErrOddKeyValuePairs is returned when a key-value pair slice has an odd
	// number of elements.
	ErrOddKeyValuePairs = errors.New("expected even number of key-value pairs")

	// ErrControllerAlreadySet is returned when ControlledBy or
	// WithOwnerReference is called with a controller reference on an object
	// that already has a different controller owner reference.
	ErrControllerAlreadySet = errors.New("object already has a controller owner reference")
)

// MetaOptions is a functional option that mutates metadata on a Kubernetes
// object. Use the provided With* constructors and combine them via
// ApplyMetaOptions.
type MetaOptions func(obj client.Object) error

// ApplyMetaOptions applies all given MetaOptions to obj in order. It returns
// the first error encountered, if any.
func ApplyMetaOptions(obj client.Object, opts ...MetaOptions) error {
	for _, opt := range opts {
		err := opt(obj)
		if err != nil {
			return err
		}
	}

	return nil
}

// WithLabels returns a MetaOptions that merges the key-value pairs into the
// object's labels. The pairs slice must have an even number of elements
// (alternating key, value); an odd count returns an error.
func WithLabels(pairs ...string) MetaOptions {
	return func(obj client.Object) error {
		kv, err := ExtractKeyValues(pairs)
		if err != nil {
			return fmt.Errorf("WithLabels: %w", err)
		}

		existing := obj.GetLabels()
		if existing == nil {
			existing = make(map[string]string, len(kv))
		}

		maps.Copy(existing, kv)
		obj.SetLabels(existing)

		return nil
	}
}

// WithAnnotations returns a MetaOptions that merges the key-value pairs into
// the object's annotations. The pairs slice must have an even number of
// elements (alternating key, value); an odd count returns an error.
func WithAnnotations(pairs ...string) MetaOptions {
	return func(obj client.Object) error {
		kv, err := ExtractKeyValues(pairs)
		if err != nil {
			return fmt.Errorf("WithAnnotations: %w", err)
		}

		existing := obj.GetAnnotations()
		if existing == nil {
			existing = make(map[string]string, len(kv))
		}

		maps.Copy(existing, kv)
		obj.SetAnnotations(existing)

		return nil
	}
}

// WithOwnerReference upserts a raw OwnerReference on the object. If a
// reference with the same UID already exists it is replaced; otherwise the
// new reference is appended. If the incoming reference is a controller
// reference, it validates that no *different* controller reference already
// exists on the object.
func WithOwnerReference(ref metav1.OwnerReference) MetaOptions {
	return func(obj client.Object) error {
		if ref.Controller != nil && *ref.Controller {
			for _, existing := range obj.GetOwnerReferences() {
				if existing.Controller != nil && *existing.Controller && existing.UID != ref.UID {
					return fmt.Errorf("%s %s/%s: %w",
						existing.Kind, obj.GetNamespace(), existing.Name, ErrControllerAlreadySet)
				}
			}
		}

		obj.SetOwnerReferences(upsertOwnerRef(obj.GetOwnerReferences(), ref))

		return nil
	}
}

// OwnedBy adds or updates a non-controller owner reference pointing at owner.
// Reapplying with the same owner is idempotent (deduplicates by UID).
func OwnedBy(owner client.Object) MetaOptions {
	return func(obj client.Object) error {
		ref, err := OwnerRefFrom(owner, false)
		if err != nil {
			return err
		}

		obj.SetOwnerReferences(upsertOwnerRef(obj.GetOwnerReferences(), ref))

		return nil
	}
}

// ControlledBy adds or updates a controller owner reference pointing at owner.
// Reapplying with the same owner is idempotent. It returns an error if the
// object already has a controller owner reference from a *different* owner,
// since Kubernetes allows at most one controller per object.
func ControlledBy(owner client.Object) MetaOptions {
	return func(obj client.Object) error {
		ref, err := OwnerRefFrom(owner, true)
		if err != nil {
			return err
		}

		for _, existing := range obj.GetOwnerReferences() {
			if existing.Controller != nil && *existing.Controller && existing.UID != ref.UID {
				return fmt.Errorf("%s %s/%s: %w",
					existing.Kind, obj.GetNamespace(), existing.Name, ErrControllerAlreadySet)
			}
		}

		obj.SetOwnerReferences(upsertOwnerRef(obj.GetOwnerReferences(), ref))

		return nil
	}
}

// WithDynamicOwner stamps labels and annotations on obj that map it back to
// the owning CR, enabling watch-based reconciliation without OwnerReferences.
//
// Use this instead of [ControlledBy] or [OwnedBy] when the child resource
// lives in a different namespace than the owner, since Kubernetes
// OwnerReferences require same-namespace residency. For same-namespace
// resources, prefer standard OwnerReferences via [ControlledBy].
//
// The following metadata is set:
//   - Label [labels.PlatformPartOf]: normalized owner Kind (for watch filtering and GC)
//   - Annotation [annotations.InstanceName]: owner name
//   - Annotation [annotations.InstanceNamespace]: owner namespace (empty for cluster-scoped)
//   - Annotation [annotations.InstanceUID]: owner UID (for staleness detection)
//   - Annotation [annotations.InstanceGeneration]: owner generation
//
// The owner must have Kind, Name, and UID populated. Call
// SetGroupVersionKind on the owner if the GVK is not already set (e.g.
// after a client.Get call).
//
// Use [EnqueueOwner] as the handler.MapFunc to resolve these annotations
// back to a reconcile.Request in a controller Watch.
func WithDynamicOwner(owner client.Object) MetaOptions {
	return func(obj client.Object) error {
		if owner == nil {
			return ErrOwnerNil
		}

		kind := owner.GetObjectKind().GroupVersionKind().Kind
		if kind == "" {
			return fmt.Errorf("%s/%s: %w",
				owner.GetNamespace(), owner.GetName(), ErrOwnerMissingKind)
		}

		if owner.GetName() == "" {
			return ErrOwnerMissingName
		}

		if owner.GetUID() == "" {
			return ErrOwnerMissingUID
		}

		partOf, err := labels.NormalizePartOfValue(kind)
		if err != nil {
			return fmt.Errorf("normalizing owner kind %q: %w", kind, err)
		}

		resources.SetLabel(obj, labels.PlatformPartOf, partOf)
		resources.SetAnnotation(obj, annotations.InstanceName, owner.GetName())
		resources.SetAnnotation(obj, annotations.InstanceNamespace, owner.GetNamespace())
		resources.SetAnnotation(obj, annotations.InstanceUID, string(owner.GetUID()))
		resources.SetAnnotation(obj, annotations.InstanceGeneration,
			strconv.FormatInt(owner.GetGeneration(), 10))

		return nil
	}
}

// InNamespace returns a MetaOptions that sets the object's namespace.
func InNamespace(ns string) MetaOptions {
	return func(obj client.Object) error {
		obj.SetNamespace(ns)

		return nil
	}
}

// ExtractKeyValues converts a flat alternating [key, value, key, value, …]
// slice into a map. It returns an error if the number of elements is odd.
func ExtractKeyValues(pairs []string) (map[string]string, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("%w: got %d", ErrOddKeyValuePairs, len(pairs))
	}

	result := make(map[string]string, len(pairs)/2)

	for i := 0; i < len(pairs); i += 2 {
		result[pairs[i]] = pairs[i+1]
	}

	return result, nil
}

// OwnerRefFrom builds an OwnerReference from owner. If controller is true the
// reference is marked as the managing controller. BlockOwnerDeletion is always
// set to true; callers that need it false should construct the reference
// manually and use WithOwnerReference.
//
// It returns an error if owner is nil, or if APIVersion, Kind, Name, or UID
// are empty.
func OwnerRefFrom(owner client.Object, controller bool) (metav1.OwnerReference, error) {
	if owner == nil {
		return metav1.OwnerReference{}, ErrOwnerNil
	}

	gvk := owner.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" {
		return metav1.OwnerReference{}, fmt.Errorf("%s/%s: %w",
			owner.GetNamespace(), owner.GetName(), ErrOwnerMissingKind)
	}

	apiVersion := gvk.GroupVersion().String()
	if apiVersion == "" {
		return metav1.OwnerReference{}, fmt.Errorf("%s/%s: %w",
			owner.GetNamespace(), owner.GetName(), ErrOwnerMissingAPIVersion)
	}

	if owner.GetName() == "" {
		return metav1.OwnerReference{}, ErrOwnerMissingName
	}

	if owner.GetUID() == "" {
		return metav1.OwnerReference{}, ErrOwnerMissingUID
	}

	return metav1.OwnerReference{
		APIVersion:         apiVersion,
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		Controller:         ptr.To(controller),
		BlockOwnerDeletion: ptr.To(true),
	}, nil
}

// OwnerRefRaw builds an OwnerReference from explicit fields. This is useful
// when the owner's GVK is known but the full client.Object is not available.
// BlockOwnerDeletion is always set to true; callers that need it false should
// construct the OwnerReference directly.
//
// It returns an error if apiVersion, kind, name, or uid is empty.
func OwnerRefRaw(apiVersion, kind, name string, uid types.UID, controller bool) (metav1.OwnerReference, error) {
	if apiVersion == "" {
		return metav1.OwnerReference{}, ErrOwnerMissingAPIVersion
	}

	if kind == "" {
		return metav1.OwnerReference{}, ErrOwnerMissingKind
	}

	if name == "" {
		return metav1.OwnerReference{}, ErrOwnerMissingName
	}

	if uid == "" {
		return metav1.OwnerReference{}, ErrOwnerMissingUID
	}

	return metav1.OwnerReference{
		APIVersion:         apiVersion,
		Kind:               kind,
		Name:               name,
		UID:                uid,
		Controller:         ptr.To(controller),
		BlockOwnerDeletion: ptr.To(true),
	}, nil
}

// upsertOwnerRef replaces an existing OwnerReference with the same UID, or
// appends if no match is found.
func upsertOwnerRef(refs []metav1.OwnerReference, ref metav1.OwnerReference) []metav1.OwnerReference {
	for i, existing := range refs {
		if existing.UID == ref.UID {
			refs[i] = ref

			return refs
		}
	}

	return append(refs, ref)
}
