// Package cluster provides runtime helpers for working with cluster-scoped
// singleton custom resources and functional options for setting metadata
// (labels, annotations, owner references, namespace) on Kubernetes objects.
//
// The ODH Onboarding Guide mandates that all module CRDs are cluster-scoped
// singletons with enforced naming. [GetSingleton] is a generic function that
// retrieves the single instance of a given type, returning an error if zero
// or more than one instance exists.
//
// Module controllers typically call GetSingleton at the start of each
// reconciliation to obtain their own singleton CR:
//
//	var component myv1.MyComponent
//	if err := cluster.GetSingleton(ctx, client, &component); err != nil {
//	    return ctrl.Result{}, err
//	}
package cluster
