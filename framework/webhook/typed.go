package webhook

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ValidatingBuilder configures typed validation callbacks for one endpoint.
type ValidatingBuilder[T runtime.Object] struct {
	*Builder

	create []func(context.Context, T, client.Reader) (admission.Warnings, error)
	update []func(context.Context, T, T, client.Reader) (admission.Warnings, error)
	delete []func(context.Context, T, client.Reader) (admission.Warnings, error)
}

// Validating starts a typed validating webhook. Build resolves the expected
// GVK from T in the manager's scheme and rejects missing or ambiguous registrations.
// Omitted operations are allowed.
func Validating[T interface {
	*V
	runtime.Object
}, V any](mgr ctrl.Manager, path string) *ValidatingBuilder[T] {
	v := &ValidatingBuilder[T]{Builder: &Builder{mgr: mgr, path: path, name: path}}
	v.object = T(new(V))
	v.handlers = 1
	v.factory = func(scheme *runtime.Scheme, reader client.Reader) *admission.Webhook {
		if len(v.create) == 0 && len(v.update) == 0 && len(v.delete) == 0 {
			return nil
		}
		return admission.WithValidator(scheme, &validationHandler[T]{
			reader: reader, create: v.create, update: v.update, delete: v.delete,
		})
	}
	return v
}

// OnCreate adds a CREATE validator. Validators run in registration order.
func (v *ValidatingBuilder[T]) OnCreate(fn func(context.Context, T, client.Reader) (admission.Warnings, error)) *ValidatingBuilder[T] {
	if fn == nil {
		v.err = errors.Join(v.err, errors.New("nil CREATE validation handler"))
		return v
	}
	v.create = append(v.create, fn)
	return v
}

// OnUpdate adds an UPDATE validator. The objects are old then new.
func (v *ValidatingBuilder[T]) OnUpdate(fn func(context.Context, T, T, client.Reader) (admission.Warnings, error)) *ValidatingBuilder[T] {
	if fn == nil {
		v.err = errors.Join(v.err, errors.New("nil UPDATE validation handler"))
		return v
	}
	v.update = append(v.update, fn)
	return v
}

// OnDelete adds a DELETE validator.
func (v *ValidatingBuilder[T]) OnDelete(fn func(context.Context, T, client.Reader) (admission.Warnings, error)) *ValidatingBuilder[T] {
	if fn == nil {
		v.err = errors.Join(v.err, errors.New("nil DELETE validation handler"))
		return v
	}
	v.delete = append(v.delete, fn)
	return v
}

// WithName sets the webhook name used in admission logs.
func (v *ValidatingBuilder[T]) WithName(name string) *ValidatingBuilder[T] {
	v.Builder.WithName(name)
	return v
}

// WithAdmission customizes the underlying admission webhook.
func (v *ValidatingBuilder[T]) WithAdmission(configure func(*admission.Webhook)) *ValidatingBuilder[T] {
	v.Builder.WithAdmission(configure)
	return v
}

type validationHandler[T runtime.Object] struct {
	reader client.Reader
	create []func(context.Context, T, client.Reader) (admission.Warnings, error)
	update []func(context.Context, T, T, client.Reader) (admission.Warnings, error)
	delete []func(context.Context, T, client.Reader) (admission.Warnings, error)
}

func (h *validationHandler[T]) ValidateCreate(ctx context.Context, obj T) (admission.Warnings, error) {
	var warnings admission.Warnings
	for _, validate := range h.create {
		more, err := validate(ctx, obj, h.reader)
		warnings = append(warnings, more...)
		if err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

func (h *validationHandler[T]) ValidateUpdate(ctx context.Context, oldObj, newObj T) (admission.Warnings, error) {
	var warnings admission.Warnings
	for _, validate := range h.update {
		more, err := validate(ctx, oldObj, newObj, h.reader)
		warnings = append(warnings, more...)
		if err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

func (h *validationHandler[T]) ValidateDelete(ctx context.Context, obj T) (admission.Warnings, error) {
	var warnings admission.Warnings
	for _, validate := range h.delete {
		more, err := validate(ctx, obj, h.reader)
		warnings = append(warnings, more...)
		if err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

// DefaultingBuilder configures a typed defaulting callback for one endpoint.
type DefaultingBuilder[T runtime.Object] struct {
	*Builder

	defaultFns []func(context.Context, T, client.Reader) error
}

// Defaulting starts a typed mutating webhook. Build resolves the expected
// GVK from T in the manager's scheme and rejects missing or ambiguous registrations.
func Defaulting[T interface {
	*V
	runtime.Object
}, V any](mgr ctrl.Manager, path string) *DefaultingBuilder[T] {
	d := &DefaultingBuilder[T]{Builder: &Builder{mgr: mgr, path: path, name: path}}
	d.object = T(new(V))
	d.handlers = 1
	d.factory = func(scheme *runtime.Scheme, reader client.Reader) *admission.Webhook {
		if len(d.defaultFns) == 0 {
			return nil
		}
		return admission.WithDefaulter(scheme, &defaultingHandler[T]{reader: reader, defaultFns: d.defaultFns})
	}
	return d
}

// WithDefault adds a defaulting callback. Callbacks run in registration order.
func (d *DefaultingBuilder[T]) WithDefault(fn func(context.Context, T, client.Reader) error) *DefaultingBuilder[T] {
	if fn == nil {
		d.err = errors.Join(d.err, errors.New("nil defaulting handler"))
		return d
	}
	d.defaultFns = append(d.defaultFns, fn)
	return d
}

// WithName sets the webhook name used in admission logs.
func (d *DefaultingBuilder[T]) WithName(name string) *DefaultingBuilder[T] {
	d.Builder.WithName(name)
	return d
}

// WithAdmission customizes the underlying admission webhook.
func (d *DefaultingBuilder[T]) WithAdmission(configure func(*admission.Webhook)) *DefaultingBuilder[T] {
	d.Builder.WithAdmission(configure)
	return d
}

type defaultingHandler[T runtime.Object] struct {
	reader     client.Reader
	defaultFns []func(context.Context, T, client.Reader) error
}

func (h *defaultingHandler[T]) Default(ctx context.Context, obj T) error {
	for _, defaultFn := range h.defaultFns {
		if err := defaultFn(ctx, obj, h.reader); err != nil {
			return err
		}
	}
	return nil
}
