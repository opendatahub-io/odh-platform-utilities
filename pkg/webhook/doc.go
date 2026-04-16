// Package webhook provides admission webhook utilities for enforcing
// singleton constraints on cluster-scoped custom resources.
//
// The ODH Onboarding Guide mandates that all module CRDs are cluster-scoped
// singletons. [ValidateSingletonCreation] is a validating admission webhook
// helper that denies creation if another instance of the same GVK already
// exists.
//
// Example usage as a raw admission.Handler (the req parameter is available
// directly from the Handle signature):
//
//	func (w *MyComponentWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
//	    return webhook.ValidateSingletonCreation(ctx, w.reader, &req, myGVK)
//	}
package webhook
