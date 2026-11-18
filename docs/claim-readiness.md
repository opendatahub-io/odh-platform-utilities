# Waiting for a database or schema claim

Platform runlevel ordering is best-effort and primarily guides upgrades, as it
does for other component dependencies. It does not guarantee that a consumer
starts only after a database connection is usable. The database service reports
`Provisioned=True` on a `DatabaseClaim` or `SchemaClaim` after provisioning its
credentials. Consumers that need a stronger startup gate can opt in to
`claim.WaitForDatabaseClaimProvisioned` or
`claim.WaitForSchemaClaimProvisioned`.

The helper reads a named claim in its namespace until its `Provisioned`
condition is `True`. It retries while the claim is missing or not provisioned,
stops after the caller's timeout, and returns an error naming the claim on
expiry. It propagates API errors and cancellation.
Waiting for this condition does not test whether the application can establish
a database connection; applications should still handle connection failures.

## DatabaseClaim startup example

This component waits for its `DatabaseClaim` before starting its HTTP server.
Set `POD_NAMESPACE` from the Pod's namespace and `DATABASE_CLAIM_NAME` to the
claim name. The component's ServiceAccount needs `get` permission on that
`DatabaseClaim` in the same namespace. With that permission, it can gate
startup as follows:

```go
package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "time"

    "github.com/opendatahub-io/odh-platform-utilities/pkg/claim"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
    kubeClient, err := client.New(config.GetConfigOrDie(), client.Options{})
    if err != nil {
        log.Fatal(err)
    }

    err = claim.WaitForDatabaseClaimProvisioned(
        context.Background(), kubeClient,
        os.Getenv("POD_NAMESPACE"), os.Getenv("DATABASE_CLAIM_NAME"),
        5*time.Minute,
    )
    if err != nil {
        log.Fatal(err)
    }

    log.Fatal(http.ListenAndServe(":8080", http.DefaultServeMux))
}
```

For the default `DatabaseClaim` configuration, the credentials Secret has the
same name as the claim. Mount or read that Secret separately and configure the
database driver from its discrete fields. The wait helper does not read
credentials or replace the application's normal connection retries.

## SchemaClaim consumers

A component using a `SchemaClaim` waits for that claim instead. Give its
ServiceAccount `get` permission on that `SchemaClaim` in the same namespace,
then call:

```go
err = claim.WaitForSchemaClaimProvisioned(
    context.Background(), kubeClient,
    os.Getenv("POD_NAMESPACE"), os.Getenv("SCHEMA_CLAIM_NAME"),
    5*time.Minute,
)
if err != nil {
    log.Fatal(err)
}
```

The default credentials Secret for a `SchemaClaim` is named `sc-` followed by
the claim name. If a component uses both claim kinds, wait for both before
starting the part of the component that needs them.
