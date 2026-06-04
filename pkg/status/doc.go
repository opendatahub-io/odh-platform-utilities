// Package status provides safe status subresource updates with automatic
// conflict retry for Kubernetes controllers.
//
// When multiple actors (e.g., a module controller and the platform
// orchestrator) write to the same resource's status subresource,
// optimistic concurrency conflicts are common. [Update] handles these
// conflicts by re-reading the resource, re-applying the caller's
// mutation, and retrying the write.
//
// # Example
//
//	err := status.Update(ctx, c, myObj, func(obj *v1alpha1.MyComponent) {
//	    obj.Status.Phase = common.PhaseReady
//	    obj.Status.ObservedGeneration = obj.Generation
//	})
package status
