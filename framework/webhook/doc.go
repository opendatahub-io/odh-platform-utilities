// Package webhook registers admission handlers with a controller-runtime manager.
// Each builder owns one endpoint; call Build before starting the manager.
//
// Place a +kubebuilder:webhook marker beside the handler in the consuming
// module, run controller-gen webhook there, and use the same path in For.
// Each endpoint accepts one GVK, checked before invoking its handler.
// The generated MutatingWebhookConfiguration and ValidatingWebhookConfiguration
// still need a Service and certificate/CA setup in the module deployment.
// Conversion webhooks use controller-runtime directly.
//
// CREATE-only handlers can be functions. The builder supplies an uncached reader:
//
//	err := webhook.For(mgr, "/validate-example", moduleGVK).
//	    OnCreate(func(ctx context.Context, req admission.Request, reader client.Reader) admission.Response {
//	        return validate(ctx, reader, req)
//	    }).
//	    Build()
//
// OnUpdate, OnDelete, and OnConnect can be chained for the same endpoint.
// Repeated operation callbacks run in registration order.
// WithHandler also supplies a decoder for arbitrary admission logic. Typed
// callbacks use Validating or Defaulting. Build derives the GVK from the callback
// type in the manager's scheme. WithWebhook copies the public configuration of
// a prebuilt *admission.Webhook when a handler needs controller-runtime behavior
// not covered by callbacks; WithAdmission customizes it before registration.
package webhook
