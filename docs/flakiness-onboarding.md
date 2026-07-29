# Flakiness System Onboarding

This guide walks module teams through setting up the flakiness/quarantine
system for their component.

## Prerequisites

- Your component's CI jobs produce JUnit XML artifacts in GCS
  (`test-platform-results` bucket — the public OpenShift CI artifact store)
- A Jira API token secret is available in your CI namespace (for auto-filing)

## Step 1: Add `.flakiness.yaml` to your repo

Create a `.flakiness.yaml` in your repository root:

```yaml
component: kserve

gcs:
  bucket: test-platform-results
  job_prefixes:
    - pr-logs/pull/opendatahub-io_kserve/pull-ci-kserve-main-e2e
    - logs/periodic-ci-opendatahub-io-kserve-main-e2e

analysis:
  threshold: 0.2        # flake rate above which test is quarantined
  window_days: 30       # rolling window for flake rate computation
  min_runs: 5           # minimum runs before a test can be classified

quarantine:
  config_path: hack/quarantine.json
  auto_quarantine: true
  exclude_patterns:
    - "TestSmoke/.*"     # tests that should never be quarantined

jira:
  api_url: https://redhat.atlassian.net
  user_email: bot@redhat.com       # email of the account that owns the token
  project: RHOAIENG
  issue_type: Bug                  # optional, defaults to "Bug"
  component: KServe
  labels: [flaky-test, kserve]
  token_env: QUARANTINE_JIRA_API_TOKEN  # name of the env var holding the token
```

Find your GCS job prefixes by browsing
`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/test-platform-results/`
and locating your component's periodic and presubmit job directories.

## Step 2: Add the Prow periodic job

Create a periodic job in your `openshift/release` configuration that runs
the flakiness analysis and pushes updated quarantine state back to your repo.

```yaml
periodics:
  - name: periodic-flake-analysis-<component>
    interval: 6h
    decorate: true
    extra_refs:
      - org: opendatahub-io
        repo: <your-repo>
        base_ref: main
    spec:
      containers:
        - image: quay.io/opendatahub/flakiness-tool:v0.1.0
          command:
            - flakiness-tool
          args:
            - run
            - --config=.flakiness.yaml
            - --push-back
          env:
            - name: QUARANTINE_JIRA_API_TOKEN
              valueFrom:
                secretKeyRef:
                  name: quarantine-jira-token
                  key: token
            - name: FLAKINESS_JIRA_USER_EMAIL
              valueFrom:
                secretKeyRef:
                  name: quarantine-jira-token
                  key: email
            - name: GITHUB_TOKEN
              valueFrom:
                secretKeyRef:
                  name: quarantine-github-token
                  key: token
```

The `--push-back` flag causes the tool to commit and push the updated
`quarantine.json` back to your repository after a successful run. It requires
the `GITHUB_TOKEN` env var (a GitHub App installation token or PAT with
`contents: write` scope).

If you don't need push-back (e.g. you prefer manual PR-based updates), omit
`--push-back` and the `GITHUB_TOKEN` secret:

```yaml
periodics:
  - name: periodic-flake-analysis-<component>
    interval: 6h
    decorate: true
    extra_refs:
      - org: opendatahub-io
        repo: <your-repo>
        base_ref: main
    spec:
      containers:
        - image: quay.io/opendatahub/flakiness-tool:v0.1.0
          command:
            - flakiness-tool
          args:
            - run
            - --config=.flakiness.yaml
          env:
            - name: QUARANTINE_JIRA_API_TOKEN
              valueFrom:
                secretKeyRef:
                  name: quarantine-jira-token
                  key: token
            - name: FLAKINESS_JIRA_USER_EMAIL
              valueFrom:
                secretKeyRef:
                  name: quarantine-jira-token
                  key: email
```

## Step 3: Commit an initial quarantine file

Create an empty quarantine file at the path specified in your config:

```bash
echo '[]' > hack/quarantine.json
git add hack/quarantine.json
```

## Step 4: Verify

Run locally to confirm your config is valid and GCS paths are correct:

```bash
go run github.com/opendatahub-io/odh-platform-utilities/flakiness/cmd/flakiness-tool \
  run --config .flakiness.yaml
```

## Configuration Reference

All optional fields have sensible defaults. The tool validates the config on
load and reports all errors at once.

### Environment Variable Overrides

Any scalar config field can be overridden via environment variables in CI:

| Variable | Overrides |
|----------|-----------|
| `FLAKINESS_COMPONENT` | `component` |
| `FLAKINESS_GCS_BUCKET` | `gcs.bucket` |
| `FLAKINESS_THRESHOLD` | `analysis.threshold` |
| `FLAKINESS_WINDOW_DAYS` | `analysis.window_days` |
| `FLAKINESS_MIN_RUNS` | `analysis.min_runs` |
| `FLAKINESS_QUARANTINE_CONFIG_PATH` | `quarantine.config_path` |
| `FLAKINESS_AUTO_QUARANTINE` | `quarantine.auto_quarantine` |
| `FLAKINESS_JIRA_API_URL` | `jira.api_url` |
| `FLAKINESS_JIRA_USER_EMAIL` | `jira.user_email` |
| `FLAKINESS_JIRA_PROJECT` | `jira.project` |
| `FLAKINESS_JIRA_ISSUE_TYPE` | `jira.issue_type` |
| `FLAKINESS_JIRA_COMPONENT` | `jira.component` |
| `FLAKINESS_JIRA_TOKEN_ENV` | `jira.token_env` |

Example: `FLAKINESS_THRESHOLD=0.3 flakiness-tool run --config .flakiness.yaml`

### Component Isolation

Each component's quarantine state is fully independent:

- Separate `quarantine.json` per repo (path set in `.flakiness.yaml`)
- Jira tickets filed with component-specific labels
- Flake rate data scoped to configured job prefixes only

## Integration Paths

### Path 1: Go Library (direct import)

```go
import "github.com/opendatahub-io/odh-platform-utilities/flakiness"

cfg, err := flakiness.LoadConfig(".flakiness.yaml")
if err != nil { ... }

result, err := flakiness.Run(ctx, cfg,
    flakiness.WithJiraClient(jiraClient),
    flakiness.WithPriorEntries(priorEntries),
)
// result.QuarantinedTests — tests to skip
// result.NewlyQuarantined — Jira tickets filed this run
// result.Unquarantined    — tests removed (Jira resolved)
```

### Path 2: CLI tool

```bash
flakiness-tool run --config .flakiness.yaml
```

### Path 3: Container image (Prow periodic job)

```bash
# One-shot run (no push-back):
docker run --rm -v "$PWD:/work" -w /work \
  quay.io/opendatahub/flakiness-tool:v0.1.0 \
  run --config .flakiness.yaml

# With push-back (commits result to repo):
docker run --rm -v "$PWD:/work" -w /work \
  -e GITHUB_TOKEN="$GITHUB_TOKEN" \
  quay.io/opendatahub/flakiness-tool:v0.1.0 \
  run --config .flakiness.yaml --push-back
```

## Test Runner Skip Integration

The quarantine JSON output (`hack/quarantine.json`) is a simple array of
entries with a `quarantined` boolean. Any test runner can consume it.

### Go (Ginkgo / testing)

```go
package e2e

import (
    "encoding/json"
    "os"
    "testing"
)

type QuarantineEntry struct {
    Name        string `json:"name"`
    Quarantined bool   `json:"quarantined"`
}

var quarantined map[string]bool

func TestMain(m *testing.M) {
    quarantined = loadQuarantine("hack/quarantine.json")
    os.Exit(m.Run())
}

func loadQuarantine(path string) map[string]bool {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil
    }
    var entries []QuarantineEntry
    if err := json.Unmarshal(data, &entries); err != nil {
        return nil
    }
    m := make(map[string]bool)
    for _, e := range entries {
        if e.Quarantined {
            m[e.Name] = true
        }
    }
    return m
}

func TestExample(t *testing.T) {
    if quarantined[t.Name()] {
        t.Skip("quarantined: flaky test under investigation")
    }
    // ... test body ...
}
```

### Ginkgo

```go
BeforeEach(func() {
    if quarantined[CurrentSpecReport().FullText()] {
        Skip("quarantined: flaky test under investigation")
    }
})
```

### pytest (Python)

```python
import json, pytest

def pytest_collection_modifyitems(config, items):
    try:
        with open("hack/quarantine.json") as f:
            entries = json.load(f)
    except (OSError, json.JSONDecodeError):
        return
    skip_names = {e["name"] for e in entries if e.get("quarantined")}
    for item in items:
        if item.name in skip_names:
            item.add_marker(pytest.mark.skip(reason="quarantined"))
```
