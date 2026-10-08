# Action Error Semantics

This document defines the action-error contract for the framework reconciler.
It is the authoritative reference for changes to the action pipeline and its
status, event, and requeue behavior.

## Scope

An action returns an `error`, but errors have two roles:

- reporting a failed reconciliation; and
- controlling the action pipeline or its next scheduling time.

The central rule is: return a plain Go error for an unexpected failure; return
an `ActionError` when the action intentionally controls whether the pipeline
continues or when it retries. Plain errors stop the pipeline, mark provisioning
as failed, and use normal controller-runtime error backoff.

Create an action error with `NewActionError`, `NewActionErrorf`, or
`NewActionErrorW`; each creates a terminal outcome by default. Use
`NonBlocking` or `Advisory` to opt into continuing after an outcome.

## Classifiers

`ActionError` has three classifiers. `RequeueAfter` is independent metadata
that any classifier can carry.

| Returned value | Pipeline | Provisioning status | Reconcile result |
| --- | --- | --- | --- |
| `nil` | Continues. | Successful. | No explicit retry. |
| Plain Go error | Stops immediately. | `ProvisioningSucceeded=False`. | Returns an error for normal rate-limited retry. |
| `Terminal` | Stops immediately. | `ProvisioningSucceeded=False`. | Returns an error unless it carries a positive delay. |
| `NonBlocking` | Continues. | `ProvisioningSucceeded=False`. | Returns an error unless an explicit action-error delay is selected. |
| `Advisory` | Continues. | Remains successful; its reason and message provide context. | Succeeds, optionally with an explicit delayed requeue. |

Terminal action errors and plain Go errors are the terminal outcomes: the
pipeline stops as soon as the action returns either one. Plain errors mark
provisioning failed and use normal controller-runtime error backoff. Terminal
action errors do the same unless they carry a positive delay. Actions normally
return one terminal error. If one action returns joined terminal errors, every
diagnostic is retained and the earliest positive terminal delay is selected. A
plain Go error anywhere in the same joined tree takes precedence over those
delays and uses normal controller-runtime error backoff.

## Examples

The snippets use `errors` for the Go standard library and `odherrors` for the
framework's `controller/actions/errors` package.

Return a plain error for an unexpected failure. Wrapping preserves both context
and the original cause:

```go
return fmt.Errorf("reading Secret: %w", err)
```

Return a terminal action error for an expected blocking state with a known
retry interval. Terminal is the default classifier:

```go
return odherrors.NewActionError("dependency unavailable").
    WithRequeueAfter(time.Minute)
```

Use a non-blocking action error when provisioning has failed but later actions
must still run:

```go
return odherrors.NewActionError("optional resource failed").
    NonBlocking()
```

Use an advisory when the outcome provides useful context but does not make
provisioning fail:

```go
return odherrors.NewActionError("rollout still progressing").
    Advisory().
    WithRequeueAfter(30 * time.Second)
```

A plain error in a joined return always selects normal error backoff, even when
another member requests a delay:

```go
return errors.Join(
    fmt.Errorf("reading Secret: %w", secretErr),
    odherrors.NewActionError("dependency unavailable").
        WithRequeueAfter(time.Minute),
)
```

To deliberately apply a fixed retry interval to the complete joined failure,
classify the join itself:

```go
return odherrors.NewActionErrorW(errors.Join(secretErr, dependencyErr)).
    WithRequeueAfter(time.Minute)
```

## Aggregation and precedence

The reconciler evaluates all errors returned by an action, including wrapped
and joined errors, and then aggregates outcomes across the pipeline.

1. A terminal outcome stops the pipeline. Earlier diagnostics are retained in
   the reported error and condition message, but their classifiers and delays
   are discarded. A plain Go error in a joined action return always uses normal
   error backoff. Otherwise, positive `RequeueAfter` values attached to
   terminal errors are considered and their earliest delay is honored. To
   explicitly schedule a composite failure, wrap the entire joined error with
   `NewActionErrorW(...).WithRequeueAfter(...)`.
2. Otherwise, the earliest positive `RequeueAfter` requested by semantic
   action errors is selected.
3. Non-blocking errors are joined and outrank advisories. Advisory messages
   are appended as detail when a non-blocking error is present; advisory-only
   outcomes are successful provisioning conditions.

## Requeue behavior

Controller-runtime ignores `ctrl.Result.RequeueAfter` whenever `Reconcile`
also returns a non-nil error. Therefore, when a terminal or semantic failure
selects an explicit delay, the reconciler must first persist the failed status,
then return:

```go
ctrl.Result{RequeueAfter: delay}, nil
```

The default requeue interval applies only to successful apply reconciliations
without an explicit delay. It never applies during deletion.

## Apply and deletion

During apply, the reconciler writes the provisioning condition before handling
the aggregate action outcome:

- terminal, plain-error, and non-blocking outcomes mark it false;
- advisory-only outcomes mark it true with advisory context; and
- legacy requeue markers do not cause a failed condition or add advisory
  context.

When `PreApplyFn` returns true, it skips the action pipeline and creates a
terminal pre-apply outcome. The provisioning condition is marked false with
the configured pre-apply reason. By default the reconciler emits a
`ProvisioningPaused` event and requeues after `DefaultPreApplyRequeueAfter`
(30 seconds). `WithPreApplyRequeueAfter` overrides that interval; a
non-positive value uses normal controller-runtime error backoff instead.

During deletion there is no provisioning-status update. The same terminal,
non-blocking, advisory, and delay rules control finalizer actions instead:

- a non-blocking error retains the finalizer and returns an error, unless it has a
  selected explicit delay;
- a selected delay retains the finalizer and schedules the next reconcile; and
- a successful, non-requeued outcome removes the finalizer.

## Deprecated marker compatibility

`StopError` and `RequeueAfterError` remain supported only as deprecated
adapters while consumers migrate to `ActionError`.

- `StopError` is adapted to a terminal action error.
- `RequeueAfterError` continues actions, requests the specified delayed
  requeue, and remains silent: it is adapted internally for scheduling but
  does not cause a failed condition or add advisory context.

For finalizer actions, `StopError` deliberately follows the new terminal-error
semantics rather than its previous deletion behavior:

- without a positive delay, it stops the remaining finalizer actions, retains
  the finalizer, and returns an error for normal controller-runtime backoff;
- with a positive delay, it stops the remaining finalizer actions, retains the
  finalizer, and schedules the next reconciliation after that delay; and
- it is no longer swallowed during deletion and therefore no longer permits
  finalizer removal.

A cleanup action must return `nil` only after cleanup is complete and the
object's finalizer may be removed.

New actions should return `ActionError` when they need semantic behavior.
Use `ActionError.NonBlocking()` when an ordinary error should continue the
pipeline; use a plain Go error when it must stop and use normal rate-limited
retry.
