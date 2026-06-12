package errors

import (
	"fmt"
	"time"
)

type StopError struct {
	reason       error
	requeueAfter time.Duration
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

func NewStopErrorW(reason error) StopError {
	return StopError{reason: reason}
}

func NewStopError(format string, args ...any) StopError {
	return StopError{
		reason: fmt.Errorf(format, args...),
	}
}

func NewStopErrorWithRequeueAfterW(requeueAfter time.Duration, reason error) StopError {
	return StopError{reason: reason, requeueAfter: requeueAfter}
}

func NewStopErrorWithRequeueAfter(requeueAfter time.Duration, format string, args ...any) StopError {
	return StopError{
		reason:       fmt.Errorf(format, args...),
		requeueAfter: requeueAfter,
	}
}
