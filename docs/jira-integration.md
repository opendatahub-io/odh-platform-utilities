# Jira Integration

The flakiness quarantine system automatically files a Jira Bug ticket when a
test is quarantined. This document explains how to configure the integration
and wire it into a consumer.

## Prerequisites

- A Jira Cloud account with permission to create issues in the target project
- An API token for that account (see [Generating an API Token](#generating-an-api-token))

## Generating an API Token

1. Go to <https://id.atlassian.com/manage-profile/security/api-tokens>
2. Click **Create API token**
3. Give it a descriptive label (e.g. `odh-quarantine-bot`)
4. Copy the token — it is shown only once

Note the expiry date. Set `Config.TokenExpiresAt` and configure the
[token expiry workflow](#token-expiry-monitoring) so you are warned before it
breaks CI.

## Configuration

### Via YAML config file (recommended)

The flakiness system reads configuration from a YAML file (see `flakiness.Config`).
Add a `jira:` section with the fields below, then call `jira.FromFlakinessConfig`
to build the client config — it resolves the API token from the env var named in
`token_env` automatically:

```yaml
# .flakiness.yaml
jira:
  api_url: https://redhat.atlassian.net
  user_email: bot@redhat.com
  project: RHOAIENG
  issue_type: Bug            # optional, defaults to "Bug"
  component: Test Reliability # optional
  labels:
    - flaky-test
    - quarantine
  token_env: QUARANTINE_JIRA_API_TOKEN  # name of the env var that holds the token
```

```go
cfg, err := flakiness.LoadConfig(".flakiness.yaml")
if err != nil {
    log.Fatalf("load config: %v", err)
}

clientCfg, err := jira.FromFlakinessConfig(cfg.Jira)
if err != nil {
    log.Fatalf("invalid jira config: %v", err)
}

client, err := jira.NewClient(clientCfg, nil)
```

Environment variables override any YAML field at runtime:

| Env var | YAML field |
|---------|-----------|
| `FLAKINESS_JIRA_API_URL` | `jira.api_url` |
| `FLAKINESS_JIRA_USER_EMAIL` | `jira.user_email` |
| `FLAKINESS_JIRA_PROJECT` | `jira.project` |
| `FLAKINESS_JIRA_ISSUE_TYPE` | `jira.issue_type` |
| `FLAKINESS_JIRA_COMPONENT` | `jira.component` |
| `FLAKINESS_JIRA_TOKEN_ENV` | `jira.token_env` |

### Via Go code (advanced / library consumers)

Construct `jira.Config` directly when not using the YAML config loader:

```go
import (
    "os"
    "time"

    "github.com/opendatahub-io/odh-platform-utilities/flakiness/jira"
)

cfg := jira.Config{
    APIURL:             "https://redhat.atlassian.net",
    UserEmail:          os.Getenv("JIRA_USER_EMAIL"),
    APIToken:           os.Getenv("QUARANTINE_JIRA_API_TOKEN"),
    ProjectKey:         "RHOAIENG",
    IssueType:          "Bug",
    Component:          "Test Reliability",            // optional
    Labels:             []string{"flaky-test", "quarantine"}, // optional
    QuarantineDuration: 30 * 24 * time.Hour,
    TokenExpiresAt:     time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), // optional
}

client, err := jira.NewClient(cfg, nil)
```

### Required fields

| Field | YAML key | Description |
|-------|----------|-------------|
| `APIURL` | `api_url` | Base URL of your Jira instance |
| `UserEmail` | `user_email` | Email of the account that owns the API token |
| `APIToken` | resolved from `token_env` | API token for that account |
| `ProjectKey` | `project` | Jira project key where bugs are created |
| `IssueType` | `issue_type` | Issue type name; defaults to `"Bug"` |

### Optional fields

| Field | Default | Description |
|-------|---------|-------------|
| `Component` | — | Component name to set on created issues |
| `Labels` | — | Labels to apply to created issues |
| `QuarantineDuration` | 0 | Used to compute the expected re-enable date in the bug description |
| `TokenExpiresAt` | zero | Enables expiry warning via `TokenExpirySoon` |
| `TokenExpiryWarningDays` | 14 | Days before expiry to start warning |

## Storing Secrets

Never hardcode the API token or email in source. `token_env` in the YAML config
specifies the **name** of the environment variable that holds the token — the
actual token is injected at runtime.

**GitHub Actions:**

Add two repository secrets under Settings → Secrets and variables → Actions:

| Secret | Value |
|--------|-------|
| `QUARANTINE_JIRA_API_TOKEN` | The API token |
| `JIRA_USER_EMAIL` | The account email (or set `user_email` in YAML) |

Reference them in your workflow:

```yaml
env:
  QUARANTINE_JIRA_API_TOKEN: ${{ secrets.QUARANTINE_JIRA_API_TOKEN }}
  JIRA_USER_EMAIL: ${{ secrets.JIRA_USER_EMAIL }}
```

**OpenShift CI (Prow):**

Secrets are managed via the `openshift/release` repository. Add the token as a
Prow secret and mount it as an environment variable in your `ci-operator` config.
Contact the CI team for access.

## Token Expiry Monitoring

The workflow at `.github/workflows/check-jira-token-expiry.yaml` runs every
Monday at 09:00 UTC. It validates the token against the Jira API and opens a
GitHub issue labeled `jira-token-rotation` if the token is expired or missing.
The issue is created only once — duplicates are suppressed until the existing
issue is closed.

To test the workflow manually, go to Actions → **Check Jira Token Expiry** →
**Run workflow**.

To check expiry programmatically in your pipeline:

```go
if cfg.TokenExpirySoon(time.Now()) {
    log.Warn("Jira API token expires soon — rotate before it breaks CI")
}
```

## Local Testing

Verify your credentials before wiring into CI:

```bash
export JIRA_USER_EMAIL=your@redhat.com
export QUARANTINE_JIRA_API_TOKEN=your-token
bash .github/scripts/validate-jira-token.sh
# status=valid → credentials are correct
```

To create a real test ticket, set `JIRA_TEST_PROJECT_KEY` to a sandbox project
(not the production project) and run the integration tests:

```bash
JIRA_TEST_PROJECT_KEY=SANDBOX \
go test ./flakiness/jira/... -tags integration -v -run TestIntegration
```

The test prints the URL of the created ticket and deletes it on completion.
