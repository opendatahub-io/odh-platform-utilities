// Package predicates provides optional event-filtering utilities for
// controller-runtime controllers.
//
// Every predicate in this package implements the standard
// [sigs.k8s.io/controller-runtime/pkg/predicate.Predicate] interface and can
// be used with any controller-runtime Watch or Builder — no other import from
// this module is required.
//
// Available predicates:
//
//   - [GenerationChangedPredicate] — passes update events only when
//     metadata.generation changes. Resources with generation 0 always pass.
//   - [LabelSelectorPredicate] — filters all events to objects matching a
//     [k8s.io/apimachinery/pkg/labels.Selector].
//   - [AnnotationChangedPredicate] — passes create events through and triggers
//     updates only when a named annotation's value changes.
//   - [DeletionPredicate] — passes only delete events.
//
// Example — controller builder with predicates:
//
//	ctrl.NewControllerManagedBy(mgr).
//	    For(&v1.MyType{}, builder.WithPredicates(
//	        predicates.GenerationChangedPredicate{},
//	    )).
//	    Watches(&corev1.ConfigMap{}, myHandler, builder.WithPredicates(
//	        predicates.AnnotationChangedPredicate{Key: "config-hash"},
//	    ))
package predicates
