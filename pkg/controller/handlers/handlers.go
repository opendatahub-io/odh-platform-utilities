package handlers

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// EnqueueOwner returns an EventHandler that enqueues reconcile requests for
// the owner of watched objects. Ownership is determined via the standard
// Kubernetes ownerReferences on the watched object; the owner's type is
// inferred from ownerType.
//
// This is the standard pattern for dynamic watches where a controller
// discovers owned resource types at runtime and needs events on those
// resources to trigger reconciliation of the owner.
//
// By default both controller and non-controller owner references are matched.
// Pass [handler.OnlyControllerOwner] to restrict to controller owners only.
func EnqueueOwner(
	scheme *runtime.Scheme,
	mapper meta.RESTMapper,
	ownerType client.Object,
	opts ...handler.OwnerOption,
) handler.EventHandler {
	return handler.EnqueueRequestForOwner(scheme, mapper, ownerType, opts...)
}
