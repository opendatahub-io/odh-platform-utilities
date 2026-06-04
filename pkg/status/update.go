package status

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const defaultMaxRetries = 5

var (
	ErrRetriesExhausted = errors.New(
		"conflict retries exhausted:" +
			" the object was modified by another actor on every attempt",
	)

	ErrNilMutateFn = errors.New(
		"mutateFn must not be nil",
	)
)

type options struct {
	maxRetries int
}

// Option configures the behavior of [Update].
type Option func(*options)

// WithMaxRetries sets the maximum number of conflict retries.
// The default is 5. Retries are immediate with no backoff; for
// high-contention resources, consider keeping this value low or
// coordinating writes at a higher level.
func WithMaxRetries(n int) Option {
	return func(o *options) {
		o.maxRetries = n
	}
}

// Update applies mutateFn to obj and writes the status subresource.
// If a conflict occurs because another actor modified the resource
// between read and write, it re-reads the object, re-applies mutateFn,
// and retries up to the configured limit (default 5).
//
// Retries are immediate — there is no exponential backoff or jitter
// between attempts. Use [WithMaxRetries] to bound the number of attempts.
//
// Returns [ErrRetriesExhausted] if all attempts fail due to conflicts.
func Update[T client.Object](
	ctx context.Context,
	c client.Client,
	obj T,
	mutateFn func(T),
	opts ...Option,
) error {
	if mutateFn == nil {
		return ErrNilMutateFn
	}

	cfg := options{maxRetries: defaultMaxRetries}
	for _, opt := range opts {
		opt(&cfg)
	}

	key := client.ObjectKeyFromObject(obj)

	var lastErr error

	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		if attempt > 0 {
			err := c.Get(ctx, key, obj)
			if err != nil {
				return fmt.Errorf("re-reading object for status update retry: %w", err)
			}
		}

		mutateFn(obj)

		lastErr = c.Status().Update(ctx, obj)
		if lastErr == nil {
			return nil
		}

		if !apierrors.IsConflict(lastErr) {
			return fmt.Errorf("updating status: %w", lastErr)
		}
	}

	return fmt.Errorf("%w: %w", ErrRetriesExhausted, lastErr)
}
