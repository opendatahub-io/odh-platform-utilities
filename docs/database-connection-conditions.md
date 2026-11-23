# Database Connection Conditions

A component that consumes a database through the shared database
infrastructure service
([ODH-ADR-Operator-0017](https://github.com/opendatahub-io/architecture-decision-records/blob/main/architecture-decision-records/operator/ODH-ADR-Operator-0017-shared-database-infrastructure-service.md))
can report its connectivity on its own custom resource with a standard
condition: `DatabaseConnected`. This library publishes the condition type,
its reason vocabulary, and the setters that make reporting it a one-line
affair — so every component uses the same signal instead of inventing
parallel condition names.

The condition is **optional and informational per module**: the orchestrator
does not require it and does not evaluate it as part of the mandatory
PlatformObject contract. What a component does with it (for example,
aggregating it into `Ready`) is the component's choice.

## The vocabulary

The condition type and reasons live in `api/common` and are the published
contract — their exact string values must not drift:

| Constant | Value |
|----------|-------|
| `common.ConditionTypeDatabaseConnected` | `DatabaseConnected` |
| `common.ReasonConnectionSucceeded` | `ConnectionSucceeded` |
| `common.ReasonConnectionFailed` | `ConnectionFailed` |
| `common.ReasonCredentialsUnavailable` | `CredentialsUnavailable` |
| `common.ReasonCredentialsRejected` | `CredentialsRejected` |
| `common.ReasonHostUnreachable` | `HostUnreachable` |
| `common.ReasonTLSVerificationFailed` | `TLSVerificationFailed` |

The helpers live in `pkg/database/conditions`, in the root module, so a
consumer does not need the controller framework to import them.

## Quick start

A component CR that embeds `common.Status` already satisfies the
`ConditionsAccessor` interface the helpers operate on. Where the controller
checks its database connection:

```go
import "github.com/opendatahub-io/odh-platform-utilities/pkg/database/conditions"

err := db.PingContext(ctx)
if err != nil {
    conditions.SetFailed(obj, err)
} else {
    conditions.SetConnected(obj)
}
```

Resulting status:

```yaml
status:
  conditions:
  - type: DatabaseConnected
    status: "False"
    reason: HostUnreachable
    message: "dial tcp 10.0.0.5:5432: connect: connection refused"
```

Both setters upsert the condition, leave sibling conditions untouched, and
return whether anything changed — use the return value to skip a status
write when nothing moved. `LastTransitionTime` only advances when the
status flips, so steady-state reconciles produce no churn.

## Persisting

The helpers mutate the object in memory only. Write the status subresource
as the controller already does, typically with `pkg/status`:

```go
gen := obj.GetGeneration()
uid := obj.GetUID()

connErr := db.PingContext(ctx)

err := status.Update(ctx, client, obj, func(o *MyModule) {
    if o.GetUID() != uid || o.GetGeneration() != gen {
        return // object replaced or spec changed mid-flight; requeue
    }
    if connErr != nil {
        conditions.SetFailed(o, connErr, kcond.WithObservedGeneration(gen))
    } else {
        conditions.SetConnected(o, kcond.WithObservedGeneration(gen))
    }
})
```

Capture the object's generation and UID when the check starts and compare
both inside the callback: `status.Update` re-runs the callback on conflict
retries against a freshly read object, so a result from an older spec must
not be written onto a newer one — and the generation alone is not enough,
because a deleted-and-recreated object restarts at generation 1. Stamping
`WithObservedGeneration` lets readers detect a stale condition
independently.

## Classifying failures

`SetFailed` derives the reason from the error with `ReasonForError`, which
uses standard library inspection only:

| Error | Reason |
|-------|--------|
| `x509` certificate / hostname errors, `tls.RecordHeaderError`, `tls.AlertError` | `TLSVerificationFailed` |
| Errors implementing `net.Error` (DNS, connection refused, network and dialer timeouts) | `HostUnreachable` |
| Caller cancellation (`context.Canceled`), even when surfaced through a network operation | `ConnectionFailed` |
| Caller deadline (`context.DeadlineExceeded`) not surfaced through a network operation | `ConnectionFailed` |
| Anything else, including `nil` | `ConnectionFailed` |

Two classifications cannot be derived and must be passed explicitly:

- **Authentication failures** (PostgreSQL SQLSTATE 28P01) are invisible to
  standard library inspection. When the caller knows the server rejected
  the credentials:

  ```go
  conditions.SetFailed(obj, err,
      kcond.WithReason(common.ReasonCredentialsRejected))
  ```

- **Missing credentials** — the claim Secret is absent or incomplete, so no
  connection was attempted — should be reported as
  `ReasonCredentialsUnavailable`, again via `WithReason`.

Cancellation is unambiguous — nothing in the network stack cancels a
context on its own — so it always maps to the caller's `ConnectionFailed`,
even when the error surfaces through a network operation. Deadlines are
classified by how they surface: a dialer's own timeout surfaces as a
`net.OpError` wrapping the same sentinel as `context.DeadlineExceeded` on
some Go versions — and the context sentinels themselves satisfy
`net.Error` on others — so neither the sentinel nor the interface alone
can tell the two apart. A deadline is treated as the caller's budget only
when no `net.OpError` appears in its error chain; anything that surfaced
through a network operation, including dialer timeouts, is reported as
`HostUnreachable`.

## Message redaction

The final message is always passed through `Redact`, including messages
supplied via options (`WithMessage`, `WithError`): driver errors frequently
embed the connection string together with its password, and the ADR requires
consumers to keep that material out of logs and status. Redaction covers:

- `password=...` and `sslpassword=...` parameters, including quoted values
  with escapes (`password='tw0 words'`, `password='tw0\'s words'`); the key
  name is preserved, and an unquoted value consumes the complete
  non-whitespace token honouring backslash escapes, so characters like `;`
  and `&` are treated as password material, not separators. An unterminated
  quote masks the remainder of the string — redaction fails closed;
- credentials embedded in URIs (`scheme://user:password@host` and password
  query parameters). URI spans are parsed structurally: userinfo splits at
  the last `@` (so a password containing `@`, `:`, `/` or `?` is fully
  masked), query keys are percent-decoded before comparison, DSN quoting
  rules never apply to query values, and non-secret query fields are
  preserved.

Redaction is idempotent and leaves clean messages unchanged — including
`pq: password authentication failed for user "x"`, which contains no
key/value pair.

`Redact` is exported for the same purpose anywhere else a connection string
might reach a log line.

## Options

Both setters accept the options from `pkg/controller/conditions`:

| Option | Effect |
|--------|--------|
| `WithReason(string)` | Override the computed reason (e.g. `CredentialsRejected`) |
| `WithMessage(format, args...)` | Replace the message (still redacted) |
| `WithSeverity(common.ConditionSeverityInfo)` | Report as informational — a `False` with `Info` severity does not affect `Ready` aggregation |
| `WithObservedGeneration(int64)` | Stamp the reconciled generation |
| `WithError(err)` | Sets the message and Error severity; the generic reason it sets is replaced by the computed classification. `WithSeverity(Info)` must come **after** `WithError` to keep informational severity |

Pass `WithObservedGeneration` on **every** call: the upsert replaces the
condition wholesale (matching `SetStatusCondition` and Manager semantics),
so a previously stamped value is cleared by a bare call.

## Rolling up into `Ready`

Components that manage conditions with the
`pkg/controller/conditions` `Manager` should register
`DatabaseConnected` as a dependent instead of calling the raw setters, so
aggregation happens:

```go
mgr := kcond.NewManager(obj,
    string(common.ConditionTypeReady),
    string(common.ConditionTypeDatabaseConnected))

mgr.MarkFalse(string(common.ConditionTypeDatabaseConnected),
    kcond.WithReason(common.ReasonHostUnreachable),
    kcond.WithMessage(conditions.Redact(err.Error())))
// Ready is now False with the reason propagated.
```

**The Manager path does not redact.** Redaction happens in the raw
setters only, so a dynamic message supplied to `WithMessage` — which the
Manager also copies into the propagated `Ready` condition — must be passed
through `Redact` first, or credentials embedded in a driver error would
land in the status.

A `False` dependent with default (Error) severity pulls `Ready` to `False`;
the same failure with `WithSeverity(ConditionSeverityInfo)` does not count
as an unhealthy dependent — `Ready` is still recomputed, so a sole
dependent moving to `False`/`Info` can take `Ready` from `Unknown` to
`True`. `Unknown` dependents count as unhappy, so a component that has
not checked its database yet does not report `Ready=True`. One rollout
caveat: registering the dependent on an object whose `Ready` is already
`True` seeds `DatabaseConnected=True` before any probe — if the first
check must gate readiness, mark the new dependent `Unknown`
(`mgr.MarkUnknown(...)`) after constructing the Manager, or probe before
the first status write.

**Do not mix the two paths on one object.** The raw setters upsert through
`SetStatusCondition` directly and do not trigger the Manager's happiness
recomputation. Pick one path per condition type.

## Known limitations

- Driver-specific classification (SQLSTATE and friends) requires a driver;
  this package deliberately imports none, keeping the root module
  dependency-light. Callers classify explicitly with `WithReason`.
- The condition reports the **last check**, point in time. Keeping it
  current is the component's reconcile loop's job; reacting to credential
  changes without polling is the watch-wiring deliverable of the same epic,
  not this package.

## Worked example

A runnable end-to-end demonstration — success, classified network failure,
explicit auth failure with redaction, and Manager aggregation — lives at
`examples/database-connection-condition`:

```sh
go run ./examples/database-connection-condition
```
