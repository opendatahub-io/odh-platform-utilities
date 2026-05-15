package cluster

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/metadata/annotations"
	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

// EnqueueOwner returns a handler.MapFunc that resolves the owner CR from the
// dynamic ownership annotations stamped by [WithDynamicOwner]. The returned
// function reads [annotations.InstanceName] and [annotations.InstanceNamespace]
// to construct a reconcile.Request, allowing a controller Watch to trigger
// reconciliation of the owner when a child resource changes.
//
// If the resource does not carry an InstanceName annotation, the function
// returns nil (no reconcile request), so it is safe to use on mixed workloads
// where only some resources carry ownership annotations.
//
// Usage with raw controller-runtime Watches:
//
//	ctrl.NewControllerManagedBy(mgr).
//	    For(&myv1.MyModule{}).
//	    Watches(&corev1.ConfigMap{},
//	        handler.EnqueueRequestsFromMapFunc(cluster.EnqueueOwner())).
//	    Complete(r)
func EnqueueOwner() handler.MapFunc {
	return func(_ context.Context, obj client.Object) []reconcile.Request {
		name := resources.GetAnnotation(obj, annotations.InstanceName)
		if name == "" {
			return nil
		}

		ns := resources.GetAnnotation(obj, annotations.InstanceNamespace)

		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{Name: name, Namespace: ns}},
		}
	}
}
