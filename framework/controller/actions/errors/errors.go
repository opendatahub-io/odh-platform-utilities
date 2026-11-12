package errors

import (
	"errors"
	"fmt"
	"time"
)

// ActionErrorType classifies an action error's reconciliation behavior along two
// independent axes: pipeline control (do the remaining actions run?) and outcome
// severity (is the object Ready?).
//
//	Type         Pipeline   Outcome     Requeue behavior
//	-----------  ---------  ----------  ---------------------------------------
//	Terminal     stop       Not-Ready   requeue>0: paused (nil err); else: error
//	NonBlocking  continue   Not-Ready   requeue>0: requeue+warn; else: error
//	Advisory     continue   Ready       requeue (or default), nil err
//
// Terminal and NonBlocking differ only in whether the remaining actions run:
// with requeueAfter == 0 both fail the reconcile identically. The sole effect of
// NonBlocking is that later actions still execute before the reconcile fails.
type ActionErrorType uint8

const (
	ActionErrorTerminal ActionErrorType = iota + 1
	ActionErrorNonBlocking
	ActionErrorAdvisory
)

// ActionError carries an action error's classification and optional explicit
// requeue delay. Public constructors always create a terminal ActionError; the
// zero value is reserved for the reconciler's internal no-outcome state.
//
// The reconciler aggregates all action returns into a single ActionError. When
// a joined action return contains multiple terminal errors, it retains every
// diagnostic and selects their earliest positive requeue delay. A plain Go
// error in the same tree takes precedence and uses normal error backoff.
type ActionError struct {
	reason       error
	errorType    ActionErrorType
	requeueAfter time.Duration
}

// NewActionError creates a terminal ActionError with the supplied message.
func NewActionError(message string) ActionError {
	return NewActionErrorW(errors.New(message))
}

// NewActionErrorf creates a terminal ActionError with a formatted message.
func NewActionErrorf(format string, args ...any) ActionError {
	return NewActionErrorW(fmt.Errorf(format, args...))
}

// NewActionErrorW creates a terminal ActionError with the supplied reason.
func NewActionErrorW(reason error) ActionError {
	return ActionError{
		reason:    reason,
		errorType: ActionErrorTerminal,
	}
}

func (e ActionError) Error() string {
	if e.reason == nil {
		return ""
	}
	return e.reason.Error()
}

func (e ActionError) Unwrap() error {
	return e.reason
}

// Err returns nil when the outcome does not contain a failure or advisory
// message, otherwise it returns e.
func (e ActionError) Err() error {
	if e.reason == nil {
		return nil
	}
	return e
}

func (e ActionError) Type() ActionErrorType {
	return e.errorType
}

func (e ActionError) IsTerminal() bool {
	return e.errorType == ActionErrorTerminal
}

// Terminal returns a copy of e classified as terminal.
func (e ActionError) Terminal() ActionError {
	e.errorType = ActionErrorTerminal
	return e
}

// NonBlocking returns a copy of e classified as a non-blocking error.
func (e ActionError) NonBlocking() ActionError {
	e.errorType = ActionErrorNonBlocking
	return e
}

// Advisory returns a copy of e classified as advisory.
func (e ActionError) Advisory() ActionError {
	e.errorType = ActionErrorAdvisory
	return e
}

func (e ActionError) RequeueAfter() time.Duration {
	return e.requeueAfter
}

// WithRequeueAfter returns a copy of ActionError configured with an explicit
// delayed requeue.
func (e ActionError) WithRequeueAfter(value time.Duration) ActionError {
	e.requeueAfter = value
	return e
}

// Add incorporates one action return into e and reports whether execution must
// stop. The action name is added to reported errors.
func (e ActionError) Add(action string, err error) (ActionError, bool) {
	if err == nil {
		return e, false
	}

	var terminal error
	var terminalRequeueAfter time.Duration
	terminalExplicit := false
	plainTerminal := false
	var errorsToReport []error
	var errorType ActionErrorType
	var requestedRequeueAfter time.Duration
	setTerminal := func(reason error, requeueAfter time.Duration, explicit bool) {
		if !explicit {
			plainTerminal = true
		}

		switch {
		case terminal == nil:
			terminal = reason
			terminalRequeueAfter = requeueAfter
			terminalExplicit = explicit
		case explicit && !terminalExplicit:
			if terminal != nil {
				errorsToReport = append(errorsToReport, terminal)
			}
			terminal = reason
			terminalRequeueAfter = requeueAfter
			terminalExplicit = explicit
		case explicit:
			errorsToReport = append(errorsToReport, reason)
			terminalRequeueAfter = earliestRequeueAfter(terminalRequeueAfter, requeueAfter)
		default:
			errorsToReport = append(errorsToReport, reason)
		}
	}

	var visit func(error, error)
	visit = func(value, reported error) {
		if value == nil {
			return
		}

		if actionError, ok := asActionError(value); ok {
			switch actionError.Type() {
			case ActionErrorTerminal:
				setTerminal(fmt.Errorf("action %s: %w", action, reported), actionError.RequeueAfter(), true)
			case ActionErrorNonBlocking, ActionErrorAdvisory:
				errorsToReport = append(errorsToReport, fmt.Errorf("action %s: %w", action, reported))
				errorType = moreSevere(errorType, actionError.Type())
				requestedRequeueAfter = earliestRequeueAfter(requestedRequeueAfter, actionError.RequeueAfter())
			default:
				setTerminal(fmt.Errorf("action %s: invalid ActionError type %d", action, actionError.Type()), 0, true)
			}
			return
		}

		if stopError, ok := asStopError(value); ok {
			setTerminal(fmt.Errorf("action %s: %w", action, reported), stopError.RequeueAfter(), true)
			return
		}

		if requeueError, ok := asRequeueAfterError(value); ok {
			requestedRequeueAfter = earliestRequeueAfter(requestedRequeueAfter, requeueError.Duration)
			return
		}

		switch unwrapped := value.(type) { //nolint:errorlint // The visitor intentionally inspects one error-tree node at a time.
		case interface{ Unwrap() []error }:
			for _, cause := range unwrapped.Unwrap() {
				visit(cause, reported)
			}
		case interface{ Unwrap() error }:
			visit(unwrapped.Unwrap(), reported)
		default:
			setTerminal(fmt.Errorf("action %s: %w", action, reported), 0, false)
		}
	}

	visit(err, err)

	if terminal != nil {
		if plainTerminal {
			terminalRequeueAfter = 0
		}

		return ActionError{
			reason:       errors.Join(e.reason, errors.Join(errorsToReport...), terminal),
			errorType:    ActionErrorTerminal,
			requeueAfter: terminalRequeueAfter,
		}, true
	}

	combined := errors.Join(e.reason, errors.Join(errorsToReport...))
	next := ActionError{
		reason:    combined,
		errorType: moreSevere(e.errorType, errorType),
	}
	next.requeueAfter = earliestRequeueAfter(e.requeueAfter, requestedRequeueAfter)

	return next, false
}

func earliestRequeueAfter(current, requested time.Duration) time.Duration {
	switch {
	case requested <= 0:
		return current
	case current <= 0:
		return requested
	default:
		return min(current, requested)
	}
}

func moreSevere(current, candidate ActionErrorType) ActionErrorType {
	if severity(candidate) > severity(current) {
		return candidate
	}
	return current
}

func severity(errorType ActionErrorType) int {
	switch errorType {
	case ActionErrorTerminal:
		return 3
	case ActionErrorNonBlocking:
		return 2
	case ActionErrorAdvisory:
		return 1
	default:
		return 0
	}
}

func asActionError(err error) (ActionError, bool) {
	switch value := err.(type) { //nolint:errorlint // The visitor already unwraps the error tree node by node.
	case ActionError:
		return value, true
	case *ActionError:
		if value != nil {
			return *value, true
		}
	}
	return ActionError{}, false
}

func asStopError(err error) (StopError, bool) {
	switch value := err.(type) { //nolint:errorlint // The visitor already unwraps the error tree node by node.
	case StopError:
		return value, true
	case *StopError:
		if value != nil {
			return *value, true
		}
	}
	return StopError{}, false
}

func asRequeueAfterError(err error) (RequeueAfterError, bool) {
	switch value := err.(type) { //nolint:errorlint // The visitor already unwraps the error tree node by node.
	case RequeueAfterError:
		return value, true
	case *RequeueAfterError:
		if value != nil {
			return *value, true
		}
	}
	return RequeueAfterError{}, false
}
