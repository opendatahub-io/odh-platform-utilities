# Flakiness System Onboarding

This guide walks module teams through setting up the flakiness/quarantine
system for their component.

## Prerequisites

- Your component's CI jobs produce JUnit XML artifacts in GCS
  (`origin-ci-test` bucket or similar)
- A Jira API token secret is available in your CI namespace (for auto-filing)

## Step 1: Add `.flakiness.yaml` to your repo

Create a `.flakiness.yaml` in your repository root:

```yaml
component: kserve

gcs:
  bucket: origin-ci-test
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
`https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/`
and locating your component's periodic and presubmit job directories.

## Step 2: Add the Prow post step

In your `openshift/release` job configuration, add the flake analysis step:

```yaml
post:
  - name: flake-analysis
    image: quay.io/opendatahub/flakiness-tool:latest
    commands: |
      flakiness-tool run --config .flakiness.yaml
    env:
      - name: QUARANTINE_JIRA_API_TOKEN
        valueFrom:
          secretKeyRef:
            name: quarantine-jira-token
            key: token
      - name: JIRA_USER_EMAIL
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
