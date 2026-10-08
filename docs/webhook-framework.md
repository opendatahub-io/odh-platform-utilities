# Webhook framework

`framework/webhook` registers admission endpoints on a controller-runtime manager.
Call `Build()` for each endpoint before starting the manager. The builder supplies
the manager's uncached API reader and registers the handler on its webhook server.

Each endpoint accepts one GVK. `For(mgr, path, gvk)` requires it explicitly;
`Validating[*MyModule](mgr, path)` and `Defaulting[*MyModule](mgr, path)` derive
it from the Go type in the manager's scheme during `Build()`. Typed constructors
allocate their object with Go's `new`, without reflective object construction or
caller-supplied prototypes. Missing or ambiguous type registrations fail before
registration; dynamic objects should use `For` with an explicit GVK.
The expected GVK must include version and kind; an empty group is valid for core
resources. Requests whose `Kind` differs are rejected with HTTP 400 before
decoding or invoking any handler, including handlers replaced by `WithAdmission`.
`RequestKind` is not compared because it can identify the original version before
API server conversion. This also applies to operations without callbacks.
For CREATE and UPDATE, declared `apiVersion` and `kind` in the object must also
match; UPDATE and DELETE check the old object as well. Missing type metadata
is left to the decoder. CONNECT payloads contain options and are not checked
as resource objects.

## Create-only validation

This example rejects a second instance of a singleton CR. The module imports
the root-module singleton helper; the framework module does not depend on it.

```go
import (
    "context"

    webhookutil "github.com/opendatahub-io/odh-platform-utilities/pkg/webhook"
    frameworkwebhook "github.com/opendatahub-io/odh-platform-utilities/framework/webhook"
    "k8s.io/apimachinery/pkg/runtime/schema"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var moduleGVK = schema.GroupVersionKind{Group: "example.opendatahub.io", Version: "v1", Kind: "MyModule"}

// +kubebuilder:webhook:path=/validate-mymodule,mutating=false,failurePolicy=fail,sideEffects=None,groups=example.opendatahub.io,resources=mymodules,verbs=create,versions=v1,name=mymodule-validator.opendatahub.io,admissionReviewVersions=v1
func SetupWebhook(mgr ctrl.Manager) error {
    return frameworkwebhook.For(mgr, "/validate-mymodule", moduleGVK).
        OnCreate(func(ctx context.Context, req admission.Request, reader client.Reader) admission.Response {
            return webhookutil.ValidateSingletonCreation(ctx, reader, &req, moduleGVK)
        }).Build()
}
```

`OnCreate`, `OnUpdate`, `OnDelete`, and `OnConnect` can be chained on one builder.
Repeated callbacks for an operation run in registration order and stop at the
first denial. An operation without a callback is allowed. Multiple callbacks
use controller-runtime's `MultiMutatingHandler`: it combines `Response.Patches`
and warnings on success, but each callback sees the original request, so patches
must be disjoint. It does not combine direct `Response.Patch`, audit annotations,
or custom success status; a denial discards warnings from earlier callbacks.
Use `WithHandler` if those response fields need custom composition.
Return `admission.Allowed`, `admission.Denied`, or
`admission.Errored` as appropriate. Add a warning with
`response.WithWarnings("message")`.

The `+kubebuilder:webhook` marker controls which operations the API server
sends to the endpoint. Its path must match the path passed to `For`. Run
`controller-gen webhook` in the module and deploy the generated webhook
configuration with its Service, serving certificate, and CA bundle. The
builder registers the Go handler; it does not generate or install those
Kubernetes resources.

## Typed validation and defaulting

Use `Validating[T]` when a callback should receive decoded objects. It accepts
CREATE, UPDATE (old and new objects), and DELETE callbacks. Each validation
callback returns `(admission.Warnings, error)`: a non-nil error rejects the
request. Repeated callbacks for an operation run in registration order, stop at
the first error, and retain warnings from callbacks already run. Omitted
operations are allowed.

```go
err := frameworkwebhook.Validating[*MyModule](mgr, "/validate-mymodule").
    OnCreate(func(ctx context.Context, obj *MyModule, reader client.Reader) (admission.Warnings, error) {
        return validateModule(ctx, obj, reader)
    }).
    OnCreate(func(ctx context.Context, obj *MyModule, reader client.Reader) (admission.Warnings, error) {
        return validateSpec(ctx, obj, reader)
    }).
    OnUpdate(func(ctx context.Context, oldObj, newObj *MyModule, reader client.Reader) (admission.Warnings, error) {
        return validateChange(ctx, oldObj, newObj, reader)
    }).Build()
```

Use `Defaulting[T]` for a mutating endpoint. Modify the decoded object in the
callback; controller-runtime creates the admission patch. Return an error to
reject the request. Repeated `WithDefault` callbacks run in order on the same
object and stop at the first error. Use a separate mutating webhook marker and
path, with the operations on which defaulting should run.

```go
err := frameworkwebhook.Defaulting[*MyModule](mgr, "/mutate-mymodule").
    WithDefault(func(ctx context.Context, obj *MyModule, reader client.Reader) error {
        return applyDefaults(ctx, obj, reader)
    }).Build()
```

## Custom handlers

`WithHandler` accepts a raw callback and also supplies an `admission.Decoder`.
Use it when operation callbacks need to decode objects themselves or handle
admission requests differently. `WithWebhook` copies a caller-built
`*admission.Webhook`'s public configuration, so it can be reused for another
endpoint without changing the first one. Choose operation callbacks,
group, `WithHandler`, or `WithWebhook` for an endpoint; `Build` rejects
conflicting handler setups.

`WithAdmission(func(*admission.Webhook))` changes the constructed webhook before
registration. `WithName` changes the name included in admission logs; the
default is the endpoint path. The default logger includes only that name;
use `WithAdmission` to supply a custom log constructor. Conversion webhooks use
controller-runtime directly.
