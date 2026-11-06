package errors

import (
	"fmt"
	"time"
)

// StopError stops the current action pipeline.
//
// Deprecated: use NewActionError(...).Terminal() instead.
type StopError struct {
	reason       error
	requeueAfter time.Duration
}

// Deprecated: use NewActionErrorW(reason).Terminal() instead.
func NewStopErrorW(reason error) StopError {
	return StopError{reason: reason}
}

// Deprecated: use NewActionErrorf(format, args...).Terminal() instead.
func NewStopError(format string, args ...any) StopError {
	return StopError{reason: fmt.Errorf(format, args...)}
}

func (e StopError) Error() string {
	if e.reason == nil {
		return ""
	}
	return e.reason.Error()
}

func (e StopError) Unwrap() error {
	return e.reason
}

func (e StopError) RequeueAfter() time.Duration {
	return e.requeueAfter
}

// WithRequeueAfter returns a copy of StopError configured with a delayed requeue.
//
// Deprecated: use ActionError.WithRequeueAfter instead.
func (e StopError) WithRequeueAfter(value time.Duration) StopError {
	e.requeueAfter = value
	return e
}

// RequeueAfterError is a non-failing action marker that requests a delayed
// reconciliation.
//
// Deprecated: use an advisory ActionError with ActionError.WithRequeueAfter.
// Unlike this legacy marker, an advisory ActionError adds context to the
// provisioning condition.
type RequeueAfterError struct {
	// Duration is the requested delayed requeue interval.
	Duration time.Duration
}

func (e RequeueAfterError) Error() string {
	return fmt.Sprintf("requeue after %s", e.Duration)
}

// NewRequeueAfterError creates a non-failing delayed-requeue marker.
//
// Deprecated: use NewActionError(...).Advisory().WithRequeueAfter instead.
// Unlike this legacy marker, an advisory ActionError adds context to the
// provisioning condition.
func NewRequeueAfterError(d time.Duration) RequeueAfterError {
	return RequeueAfterError{Duration: d}
}
