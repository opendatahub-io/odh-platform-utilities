package predicates

import (
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// --- GenerationChangedPredicate ---

var _ predicate.Predicate = GenerationChangedPredicate{}

// GenerationChangedPredicate is a predicate that passes update events only
// when the object's metadata.generation field has changed. Create, delete, and
// generic events pass through unchanged.
//
// Resources whose generation is 0 (e.g. ConfigMaps, Secrets — types for which
// the API server does not increment generation) always pass the update filter,
// since there is no generation signal to compare.
type GenerationChangedPredicate struct {
	predicate.Funcs
}

// Update returns true when the object's generation changed, or when either
// object reports generation 0 (meaning generation is not tracked).
func (GenerationChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	if e.ObjectNew.GetGeneration() == 0 || e.ObjectOld.GetGeneration() == 0 {
		return true
	}

	return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
}

// --- LabelSelectorPredicate ---

var _ predicate.Predicate = LabelSelectorPredicate{}

// LabelSelectorPredicate is a predicate that filters events to objects whose
// labels match a [labels.Selector]. All event types are filtered.
//
// For update events the new object's labels are tested. An empty selector
// matches everything.
type LabelSelectorPredicate struct {
	predicate.Funcs

	// Selector is the label selector used to filter events. A nil or empty
	// selector matches all objects.
	Selector labels.Selector
}

// Create returns true when the object's labels match the selector.
func (p LabelSelectorPredicate) Create(e event.CreateEvent) bool {
	if e.Object == nil {
		return false
	}

	return p.matches(e.Object)
}

// Update returns true when the new object's labels match the selector.
func (p LabelSelectorPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectNew == nil {
		return false
	}

	return p.matches(e.ObjectNew)
}

// Delete returns true when the object's labels match the selector.
func (p LabelSelectorPredicate) Delete(e event.DeleteEvent) bool {
	if e.Object == nil {
		return false
	}

	return p.matches(e.Object)
}

// Generic returns true when the object's labels match the selector.
func (p LabelSelectorPredicate) Generic(e event.GenericEvent) bool {
	if e.Object == nil {
		return false
	}

	return p.matches(e.Object)
}

func (p LabelSelectorPredicate) matches(obj client.Object) bool {
	if p.Selector == nil || p.Selector.Empty() {
		return true
	}

	return p.Selector.Matches(labels.Set(obj.GetLabels()))
}

// --- AnnotationChangedPredicate ---

var _ predicate.Predicate = AnnotationChangedPredicate{}

// AnnotationChangedPredicate is a predicate that passes create events through
// and triggers updates only when the value of a specific annotation changes.
// Delete and generic events are rejected.
//
// Unlike controller-runtime's AnnotationChangedPredicate (which reacts to any
// annotation change), this predicate watches a single Key. This is useful when
// only changes to a particular annotation (e.g. a config-hash or version
// stamp) should trigger reconciliation.
type AnnotationChangedPredicate struct {
	predicate.Funcs

	// Key is the annotation key to watch for changes.
	Key string
}

// Create returns true so newly created objects are always reconciled.
func (AnnotationChangedPredicate) Create(event.CreateEvent) bool {
	return true
}

// Update returns true when the watched annotation's value differs between
// the old and new object.
func (p AnnotationChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	return p.annotation(e.ObjectOld) != p.annotation(e.ObjectNew)
}

// Delete returns false — deletions are not filtered by annotation changes.
func (AnnotationChangedPredicate) Delete(event.DeleteEvent) bool {
	return false
}

// Generic returns false.
func (AnnotationChangedPredicate) Generic(event.GenericEvent) bool {
	return false
}

func (p AnnotationChangedPredicate) annotation(obj client.Object) string {
	a := obj.GetAnnotations()
	if a == nil {
		return ""
	}

	return a[p.Key]
}

// --- DeletionPredicate ---

var _ predicate.Predicate = DeletionPredicate{}

// DeletionPredicate is a predicate that passes only delete events.
// Create, update, and generic events are filtered out.
type DeletionPredicate struct {
	predicate.Funcs
}

// Create returns false.
func (DeletionPredicate) Create(event.CreateEvent) bool {
	return false
}

// Update returns false.
func (DeletionPredicate) Update(event.UpdateEvent) bool {
	return false
}

// Delete returns true.
func (DeletionPredicate) Delete(event.DeleteEvent) bool {
	return true
}

// Generic returns false.
func (DeletionPredicate) Generic(event.GenericEvent) bool {
	return false
}
