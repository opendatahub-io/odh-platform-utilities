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
err := status.Update(ctx, client, obj, func(o *MyModule) {
    if connErr != nil {
        conditions.SetFailed(o, connErr)
    } else {
        conditions.SetConnected(o)
    }
})
```

## Classifying failures

`SetFailed` derives the reason from the error with `ReasonForError`, which
uses standard library inspection only:

| Error | Reason |
|-------|--------|
| `x509` certificate / hostname errors, `tls.RecordHeaderError` | `TLSVerificationFailed` |
| Errors implementing `net.Error` (DNS, connection refused, network and dialer timeouts) | `HostUnreachable` |
| Caller context errors (`context.DeadlineExceeded`, `context.Canceled`) not surfaced through a network operation | `ConnectionFailed` |
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

Deadlines are classified by how they surface: a dialer's own timeout
surfaces as a `net.OpError` wrapping the same sentinel as
`context.DeadlineExceeded` on some Go versions — and the context sentinels
themselves satisfy `net.Error` on others — so neither the sentinel nor the
interface alone can tell the two apart. A context error is treated as the
caller's budget only when no `net.OpError` appears in its error chain;
anything that surfaced through a network operation, including dialer
timeouts, is reported as `HostUnreachable`.

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
- passwords embedded in URIs (`scheme://user:password@host`), including an
  empty user and passwords containing a slash.

Redaction is idempotent and leaves clean messages unchanged — including
`pq: password authentication failed for user "x"`, which contains no
key/value pair. Known limitation: a password containing an unencoded `@`
inside a URI cannot be fully masked, because the URI itself is ambiguous
at that point.

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
| `WithError(err)` | Message only; the generic reason it sets is replaced by the computed classification |

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
    kcond.WithMessage("dial tcp: connection refused"))
// Ready is now False with the reason propagated.
```

A `False` dependent with default (Error) severity pulls `Ready` to `False`;
the same failure with `WithSeverity(ConditionSeverityInfo)` leaves `Ready`
untouched. `Unknown` dependents count as unhappy, so a component that has
not checked its database yet does not report `Ready=True`.

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
