package predicates

import (
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = GenerationChangedPredicate{}

// GenerationChangedPredicate passes update events only when the object's
// metadata.generation has changed. Resources whose generation is 0
// (e.g. ConfigMaps, Secrets) always pass, since there is no generation
// signal to compare. Create, delete, and generic events pass through.
type GenerationChangedPredicate struct {
	predicate.Funcs
}

func (GenerationChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	if e.ObjectNew.GetGeneration() == 0 || e.ObjectOld.GetGeneration() == 0 {
		return true
	}

	return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
}

var _ predicate.Predicate = LabelSelectorPredicate{}

// LabelSelectorPredicate filters all event types to objects whose labels
// match a [labels.Selector]. For update events the new object's labels are
// tested. A nil or empty selector matches everything.
type LabelSelectorPredicate struct {
	predicate.Funcs

	Selector labels.Selector
}

func (p LabelSelectorPredicate) Create(e event.CreateEvent) bool {
	if e.Object == nil {
		return false
	}

	return p.matches(e.Object)
}

func (p LabelSelectorPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectNew == nil {
		return false
	}

	return p.matches(e.ObjectNew)
}

func (p LabelSelectorPredicate) Delete(e event.DeleteEvent) bool {
	if e.Object == nil {
		return false
	}

	return p.matches(e.Object)
}

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

var _ predicate.Predicate = AnnotationChangedPredicate{}

// AnnotationChangedPredicate passes create events and triggers updates only
// when the value of a specific annotation key changes. Unlike
// controller-runtime's AnnotationChangedPredicate, this watches a single Key
// rather than the entire annotation map. Delete and generic events are rejected.
type AnnotationChangedPredicate struct {
	predicate.Funcs

	Key string
}

func (AnnotationChangedPredicate) Create(event.CreateEvent) bool {
	return true
}

func (p AnnotationChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	return p.annotation(e.ObjectOld) != p.annotation(e.ObjectNew)
}

func (AnnotationChangedPredicate) Delete(event.DeleteEvent) bool {
	return false
}

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

var _ predicate.Predicate = DeletionPredicate{}

// DeletionPredicate passes only delete events.
type DeletionPredicate struct {
	predicate.Funcs
}

func (DeletionPredicate) Create(event.CreateEvent) bool {
	return false
}

func (DeletionPredicate) Update(event.UpdateEvent) bool {
	return false
}

func (DeletionPredicate) Delete(event.DeleteEvent) bool {
	return true
}

func (DeletionPredicate) Generic(event.GenericEvent) bool {
	return false
}
