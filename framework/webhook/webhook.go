package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/opendatahub-io/odh-platform-utilities/framework/resources"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Builder registers one admission endpoint with a controller-runtime manager.
type Builder struct {
	mgr        ctrl.Manager
	path       string
	name       string
	gvk        schema.GroupVersionKind
	object     runtime.Object
	factory    func(*runtime.Scheme, client.Reader) *admission.Webhook
	operations map[admissionv1.Operation][]func(context.Context, admission.Request, client.Reader) admission.Response
	handlers   int
	configure  []func(*admission.Webhook)
	err        error
}

// For starts configuring the admission endpoint at path for the given GVK.
// Requests with a different Kind are rejected before the handler runs.
// The path is also the default name in admission logs and must match the corresponding
// +kubebuilder:webhook marker in the consuming module.
func For(mgr ctrl.Manager, path string, gvk schema.GroupVersionKind) *Builder {
	return &Builder{mgr: mgr, path: path, name: path, gvk: gvk}
}

// WithHandler serves a raw admission callback. The reader is uncached.
func (b *Builder) WithHandler(handler func(context.Context, admission.Request, client.Reader, admission.Decoder) admission.Response) *Builder {
	b.handlers++
	if handler == nil {
		b.err = errors.Join(b.err, errors.New("nil admission handler"))
		return b
	}
	b.factory = func(scheme *runtime.Scheme, reader client.Reader) *admission.Webhook {
		decoder := admission.NewDecoder(scheme)
		return &admission.Webhook{Handler: admission.HandlerFunc(func(ctx context.Context, req admission.Request) admission.Response {
			return handler(ctx, req, reader, decoder)
		})}
	}
	return b
}

// OnCreate serves a CREATE-only callback. The reader is uncached.
func (b *Builder) OnCreate(handler func(context.Context, admission.Request, client.Reader) admission.Response) *Builder {
	return b.onOperation(admissionv1.Create, handler)
}

// OnUpdate serves an UPDATE-only callback. The reader is uncached.
func (b *Builder) OnUpdate(handler func(context.Context, admission.Request, client.Reader) admission.Response) *Builder {
	return b.onOperation(admissionv1.Update, handler)
}

// OnDelete serves a DELETE-only callback. The reader is uncached.
func (b *Builder) OnDelete(handler func(context.Context, admission.Request, client.Reader) admission.Response) *Builder {
	return b.onOperation(admissionv1.Delete, handler)
}

// OnConnect serves a CONNECT-only callback. The reader is uncached.
func (b *Builder) OnConnect(handler func(context.Context, admission.Request, client.Reader) admission.Response) *Builder {
	return b.onOperation(admissionv1.Connect, handler)
}

func (b *Builder) onOperation(operation admissionv1.Operation, handler func(context.Context, admission.Request, client.Reader) admission.Response) *Builder {
	if handler == nil {
		b.err = errors.Join(b.err, fmt.Errorf("nil %s admission handler", operation))
		return b
	}
	if b.operations == nil {
		b.operations = make(map[admissionv1.Operation][]func(context.Context, admission.Request, client.Reader) admission.Response)
		b.handlers++
		b.factory = func(_ *runtime.Scheme, reader client.Reader) *admission.Webhook {
			handlers := make(map[admissionv1.Operation]admission.Handler, len(b.operations))
			for operation, callbacks := range b.operations {
				parts := make([]admission.Handler, 0, len(callbacks))
				for _, callback := range callbacks {
					parts = append(parts, admission.HandlerFunc(func(ctx context.Context, req admission.Request) admission.Response {
						return callback(ctx, req, reader)
					}))
				}
				if len(parts) == 1 {
					handlers[operation] = parts[0]
				} else {
					multi := admission.MultiMutatingHandler(parts...)
					handlers[operation] = admission.HandlerFunc(func(ctx context.Context, req admission.Request) admission.Response {
						response := multi.Handle(ctx, req)
						// MultiMutatingHandler emits [] when nothing changed. Omit it for
						// validating webhooks; mutating webhooks may also omit empty patches.
						if string(response.Patch) == "[]" {
							response.Patch = nil
							response.PatchType = nil
						}
						return response
					})
				}
			}
			return &admission.Webhook{Handler: admission.HandlerFunc(func(ctx context.Context, req admission.Request) admission.Response {
				if handle, ok := handlers[req.Operation]; ok {
					return handle.Handle(ctx, req)
				}
				return admission.Allowed("")
			})}
		}
	}
	b.operations[operation] = append(b.operations[operation], handler)
	return b
}

// WithWebhook serves a caller-built admission webhook. Build copies its public
// configuration so the same webhook can be used for multiple endpoints.
func (b *Builder) WithWebhook(hook *admission.Webhook) *Builder {
	b.handlers++
	b.factory = func(*runtime.Scheme, client.Reader) *admission.Webhook {
		if hook == nil {
			return nil
		}
		// Share the handler, but not Webhook's sync.Once and logger state.
		registered := &admission.Webhook{
			Handler:         hook.Handler,
			WithContextFunc: hook.WithContextFunc,
			LogConstructor:  hook.LogConstructor,
		}
		if hook.RecoverPanic != nil {
			recoverPanic := *hook.RecoverPanic
			registered.RecoverPanic = &recoverPanic
		}
		return registered
	}
	return b
}

// WithName sets the webhook name used in admission logs.
func (b *Builder) WithName(name string) *Builder {
	b.name = name
	return b
}

// WithAdmission customizes the constructed admission webhook. It runs after
// the default log constructor is set, so it can replace that default.
func (b *Builder) WithAdmission(configure func(*admission.Webhook)) *Builder {
	if configure == nil {
		b.err = errors.Join(b.err, errors.New("nil admission configuration"))
	} else {
		b.configure = append(b.configure, configure)
	}
	return b
}

// Build constructs and registers the webhook before the manager starts.
func (b *Builder) Build() error {
	if b.err != nil {
		return b.err
	}
	if isNil(b.mgr) {
		return errors.New("webhook manager is nil")
	}
	if !strings.HasPrefix(b.path, "/") {
		return fmt.Errorf("webhook path %q must start with /", b.path)
	}
	if b.handlers != 1 {
		return fmt.Errorf("webhook path %q requires exactly one handler, got %d", b.path, b.handlers)
	}
	server := b.mgr.GetWebhookServer()
	if isNil(server) {
		return errors.New("webhook server is nil")
	}
	scheme := b.mgr.GetScheme()
	if scheme == nil {
		return errors.New("webhook scheme is nil")
	}
	gvk := b.gvk
	if b.object != nil {
		var err error
		gvk, err = resources.GetGroupVersionKindForObject(scheme, b.object)
		if err != nil {
			return fmt.Errorf("webhook path %q: %w", b.path, err)
		}
	}
	if gvk.Version == "" || gvk.Kind == "" {
		return fmt.Errorf("webhook path %q requires a GVK with version and kind", b.path)
	}
	b.gvk = gvk

	hook := b.factory(scheme, b.mgr.GetAPIReader())
	if hook == nil || isNil(hook.Handler) {
		return fmt.Errorf("webhook path %q has a nil admission handler", b.path)
	}
	if hook.LogConstructor == nil {
		hook.LogConstructor = func(base logr.Logger, _ *admission.Request) logr.Logger {
			return base.WithValues("webhook", b.name)
		}
	}
	for _, configure := range b.configure {
		configure(hook)
	}
	if isNil(hook.Handler) {
		return fmt.Errorf("webhook path %q has a nil admission handler", b.path)
	}
	handler := hook.Handler
	hook.Handler = admission.HandlerFunc(func(ctx context.Context, req admission.Request) admission.Response {
		actual := schema.GroupVersionKind(req.Kind)
		if actual != gvk {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("expected GVK %s, got %s", gvk, actual))
		}
		var objects []runtime.RawExtension
		switch req.Operation {
		case admissionv1.Create:
			objects = []runtime.RawExtension{req.Object}
		case admissionv1.Update:
			objects = []runtime.RawExtension{req.Object, req.OldObject}
		case admissionv1.Delete:
			objects = []runtime.RawExtension{req.OldObject}
		default:
			// CONNECT payloads contain options, not the admitted resource.
		}
		for _, object := range objects {
			if len(object.Raw) == 0 {
				continue
			}
			var meta metav1.TypeMeta
			if err := json.Unmarshal(object.Raw, &meta); err != nil {
				return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode object type metadata: %w", err))
			}
			if (meta.APIVersion != "" && meta.APIVersion != gvk.GroupVersion().String()) ||
				(meta.Kind != "" && meta.Kind != gvk.Kind) {
				return admission.Errored(http.StatusBadRequest, fmt.Errorf("expected object GVK %s, got %s", gvk, meta.GroupVersionKind()))
			}
		}
		return handler.Handle(ctx, req)
	})
	server.Register(b.path, hook)
	return nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
