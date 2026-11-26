package cache

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrNilObject          = errors.New("object must not be nil")
	ErrUnscoped           = errors.New("object must be scoped by namespaces, a label selector, or a field selector")
	ErrNoNamespaces       = errors.New("at least one namespace is required")
	ErrAllNamespaces      = errors.New("the empty namespace (all namespaces) is not allowed")
	ErrEmptyLabelSelector = errors.New("label selector must not be nil or empty")
	ErrEmptyFieldSelector = errors.New("field selector must not be nil or empty")

	// ErrDuplicateObject is detected by Go type (plus GVK for unstructured and
	// metadata-only objects); a typed and an unstructured entry for the same
	// GVK are not detected.
	ErrDuplicateObject = errors.New("object type added more than once")
)

// ScopeOption configures the cache.ByObject entry of a single object type.
type ScopeOption func(*scope)

type scope struct {
	byObject cache.ByObject
	errs     []error
}

// InNamespaces restricts the object's informer to the given namespaces. Use it
// only for types the operator handles in fixed namespaces; use
// WithLabelSelector for user namespaces.
func InNamespaces(namespaces ...string) ScopeOption {
	return func(s *scope) {
		if len(namespaces) == 0 {
			s.errs = append(s.errs, ErrNoNamespaces)

			return
		}

		if s.byObject.Namespaces == nil {
			s.byObject.Namespaces = make(map[string]cache.Config, len(namespaces))
		}

		for _, ns := range namespaces {
			if ns == cache.AllNamespaces {
				s.errs = append(s.errs, ErrAllNamespaces)

				continue
			}

			s.byObject.Namespaces[ns] = cache.Config{}
		}
	}
}

// WithLabelSelector restricts the object's informer to objects matching the
// selector.
func WithLabelSelector(selector labels.Selector) ScopeOption {
	return func(s *scope) {
		if selector == nil || selector.Empty() {
			s.errs = append(s.errs, ErrEmptyLabelSelector)

			return
		}

		s.byObject.Label = selector
	}
}

// WithFieldSelector restricts the object's informer to objects matching the
// field selector, e.g. metadata.name for a singleton.
func WithFieldSelector(selector fields.Selector) ScopeOption {
	return func(s *scope) {
		if selector == nil || selector.Empty() {
			s.errs = append(s.errs, ErrEmptyFieldSelector)

			return
		}

		s.byObject.Field = selector
	}
}

// WithTransform sets the object's cache transform, replacing DefaultTransform.
func WithTransform(transform toolscache.TransformFunc) ScopeOption {
	return func(s *scope) {
		s.byObject.Transform = transform
	}
}

// ByObjectBuilder builds a cache.Options.ByObject map in which every object
// type is explicitly scoped.
type ByObjectBuilder struct {
	objects []client.Object
	scopes  []scope
}

// NewByObjectBuilder returns an empty ByObjectBuilder.
func NewByObjectBuilder() *ByObjectBuilder {
	return &ByObjectBuilder{}
}

// Add registers the cache scope for an object type. At least one namespace,
// label, or field scope is required.
func (b *ByObjectBuilder) Add(obj client.Object, opts ...ScopeOption) *ByObjectBuilder {
	var s scope
	for _, opt := range opts {
		opt(&s)
	}

	b.objects = append(b.objects, obj)
	b.scopes = append(b.scopes, s)

	return b
}

// Build validates every scope and returns the ByObject map, or all validation
// errors joined.
func (b *ByObjectBuilder) Build() (map[client.Object]cache.ByObject, error) {
	result := make(map[client.Object]cache.ByObject, len(b.objects))
	seen := make(map[string]struct{}, len(b.objects))

	var errs []error

	for i, obj := range b.objects {
		if obj == nil {
			errs = append(errs, ErrNilObject)

			continue
		}

		key := objectKey(obj)
		s := b.scopes[i]

		if _, dup := seen[key]; dup {
			s.errs = append(s.errs, ErrDuplicateObject)
		}

		seen[key] = struct{}{}

		if len(s.byObject.Namespaces) == 0 && s.byObject.Label == nil && s.byObject.Field == nil {
			s.errs = append(s.errs, ErrUnscoped)
		}

		if len(s.errs) > 0 {
			errs = append(errs, fmt.Errorf("%s: %w", key, errors.Join(s.errs...)))

			continue
		}

		result[obj] = s.byObject
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return result, nil
}

// objectKey identifies an object type; unstructured and metadata-only objects
// need their GVK as well.
func objectKey(obj client.Object) string {
	switch obj.(type) {
	case *unstructured.Unstructured, *metav1.PartialObjectMetadata:
		return fmt.Sprintf("%T(%s)", obj, obj.GetObjectKind().GroupVersionKind())
	default:
		return fmt.Sprintf("%T", obj)
	}
}
