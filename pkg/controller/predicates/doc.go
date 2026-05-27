// Package predicates provides optional event-filtering predicates for
// controller-runtime controllers.
//
// Available predicates:
//
//   - [GenerationChangedPredicate] — passes updates only when metadata.generation changes.
//   - [LabelSelectorPredicate] — filters events to objects matching a label selector.
//   - [AnnotationChangedPredicate] — passes updates only when a single annotation key changes.
//   - [DeletionPredicate] — passes only delete events.
package predicates
