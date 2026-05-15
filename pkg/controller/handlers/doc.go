// Package handlers provides optional event-handler utilities for
// controller-runtime controllers.
//
// Every handler in this package returns a standard
// [sigs.k8s.io/controller-runtime/pkg/handler.EventHandler] and can be used
// with any controller-runtime Watch or Builder — no other import from this
// module is required.
//
// Available handlers:
//
//   - [EnqueueOwner] — enqueues reconcile requests for the owner of a watched
//     object, determined via the standard Kubernetes ownerReferences mechanism.
//     Intended for dynamic watches where owned resource types are discovered
//     at runtime.
//
// Example — dynamic watch with owner enqueue:
//
//	ctrl.Watch(
//	    source.Kind(cache, &appsv1.Deployment{}),
//	    handlers.EnqueueOwner(mgr.GetScheme(), mgr.GetRESTMapper(), ownerObj),
//	)
package handlers
