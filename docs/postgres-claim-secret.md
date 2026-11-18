# PostgreSQL claim Secrets

`framework/postgres` is the shared consumer of the claim-Secret contract.
A DatabaseClaim or SchemaClaim Secret stores discrete fields (`host`, `port`,
`user`, `password`, `dbname`, `sslmode`, optional `schema` and `ca.crt`). There
is no pre-composed URI: this package validates those fields and either opens a
pgx connection or composes a driver-correct string.

```go
import "github.com/opendatahub-io/odh-platform-utilities/framework/postgres"
```

`ParseClaimSecret` reads a `*corev1.Secret`. If the
`opendatahub.io/connection-type-protocol` annotation is present, it must be
`postgres`; an absent annotation is allowed for administrator-authored
Connection API Secrets. Use `ParseSecretData` when you already hold the data
map. Errors name Secret keys and never include the password.

Choose one path after parse:

| Caller | Path |
|--------|------|
| This process talks to Postgres (controller, job) | `Connect` |
| Another process needs a string (`psql`, pgJDBC, sidecar) | `WriteCACert` then `Libpq` or `JDBC` |

## sslmode

Parse allows empty `sslmode` so administrator-authored Secrets still load.
`Connect` and `TLSConfig` reject empty `sslmode` so pgx cannot fall back to
`prefer` (TLS, then plaintext). Set an explicit default before those calls.

| Value | Meaning |
|-------|---------|
| `SSLModeDisable` | No TLS. Typical default for in-cluster traffic. |
| `SSLModeRequire` | Encrypt only. Typical default for external traffic. With `ca.crt`, promoted to `verify-ca`. |
| `SSLModeVerifyCA` | Encrypt and verify the server chain. Requires `ca.crt`. Does not check hostname. |
| `SSLModeVerifyFull` | Encrypt, verify the chain, and check hostname. Requires `ca.crt`. |

Promotion of `require` plus CA keeps Go, libpq, and JDBC on the same trust
posture.

## ParseClaimSecret → Connect

`Connect` applies TLS from `ca.crt` in memory (no temp file) and sets
`search_path` when `schema` is present. Close the connection when finished.

```go
params, err := postgres.ParseClaimSecret(secret)
if err != nil {
    return err
}
if params.SSLMode == "" {
    params.SSLMode = postgres.SSLModeRequire // or SSLModeDisable in-cluster
}
conn, err := postgres.Connect(ctx, params)
if err != nil {
    return err
}
defer conn.Close(ctx)
```

## WriteCACert → Libpq / JDBC

Path-based drivers cannot use PEM from the Secret; they need a CA file.
`WriteCACert` writes one under `dir` (or the system temp dir when `dir` is
empty) when the sslmode requires it. Otherwise the path is empty and cleanup
is a no-op. For a sidecar (or any other container) that opens the connection,
pass a shared volume path both containers can read—not the caller’s temp dir.
Call cleanup only after drivers using that string have finished connecting
(`sslrootcert` is read at connect time), so the temp file is not left on disk.

Pass that path into `Libpq` or `JDBC` even when it is empty. An empty path is
rejected for `verify-ca`, `verify-full`, and `require` with CA so
`sslrootcert` is not silently omitted.

```go
params, err := postgres.ParseClaimSecret(secret)
if err != nil {
    return err
}
if params.SSLMode == "" {
    params.SSLMode = postgres.SSLModeRequire // or SSLModeDisable in-cluster
}

caFile, cleanup, err := params.WriteCACert("")
if err != nil {
    return err
}
defer cleanup()

libpq, err := params.Libpq(caFile)
if err != nil {
    return err
}

jdbc, err := params.JDBC(caFile)
if err != nil {
    return err
}
```

Schema is applied as libpq `options=-csearch_path=…` or JDBC `currentSchema`.
The returned strings contain the password in plaintext; do not log or persist
them. pgJDBC may log the JDBC URL at FINE.
