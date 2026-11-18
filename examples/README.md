# Examples

This directory contains runnable example programs demonstrating how to use
the utilities provided by `odh-platform-utilities`.

Each subdirectory is a self-contained Go module. Run with:

```bash
cd examples/<example-name>
go run .
```

| Example | Description |
|---------|-------------|
| `flakiness-scraper` | Scrapes JUnit XML artifacts from a public OpenShift CI GCS bucket and queries the ingested metrics. |
| `runtime-budget` | Scrapes JUnit artifacts then runs runtime analysis: top-N slowest tests, suite totals, and timeout budget utilisation. |

For the optional DatabaseClaim and SchemaClaim startup pattern, see
[Waiting for a database or schema claim](../docs/claim-readiness.md).
