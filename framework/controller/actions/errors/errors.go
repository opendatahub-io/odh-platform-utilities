package errors

import (
	"fmt"
	"time"
)

type StopError struct {
	reason       error
	requeueAfter time.Duration
}

func NewStopErrorW(reason error) StopError {
	return StopError{reason: reason}
}

func NewStopError(format string, args ...any) StopError {
	return StopError{
		reason: fmt.Errorf(format, args...),
	}
}

func (e StopError) Error() string {
	return e.reason.Error()
}

func (e StopError) Unwrap() error {
	return e.reason
}

func (e StopError) RequeueAfter() time.Duration {
	return e.requeueAfter
}

// WithRequeueAfter returns a copy of StopError configured with a delayed requeue.
func (e StopError) WithRequeueAfter(value time.Duration) StopError {
	e.requeueAfter = value
	return e
}
