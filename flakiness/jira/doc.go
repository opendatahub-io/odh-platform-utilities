// Package jira provides a Jira REST API client for the flaky test quarantine
// system. When a test is quarantined, this package creates a Bug ticket so the
// responsible team has an actionable item to investigate and fix the root cause.
//
// See docs/jira-integration.md for a full setup and configuration guide.
//
// # Authentication
//
// Jira Cloud uses HTTP Basic auth. The [Config] requires both a user email and
// an API token — together they are base64-encoded and sent as the Authorization
// header on every request:
//
//	Authorization: Basic base64(<UserEmail>:<APIToken>)
//
// Generate an API token at https://id.atlassian.com/manage-profile/security/api-tokens.
// Store the token in a secret (GitHub Actions secret, Prow secret, or similar)
// and pass it to [Config] at runtime — never hardcode it in source.
//
// # Configuration
//
// Construct a [Config] and pass it to [NewClient]. Required fields:
//
//   - APIURL — base URL of your Jira instance, e.g. "https://redhat.atlassian.net"
//   - UserEmail — email of the account that owns the API token
//   - ProjectKey — Jira project where Bug tickets are created, e.g. "RHOAIENG"
//   - IssueType — issue type name, typically "Bug"
//   - APIToken — API token for the account
//
// Optional fields:
//
//   - Component — component name to set on created issues
//   - Labels — list of labels to apply, e.g. []string{"flaky-test", "quarantine"}
//   - QuarantineDuration — how long a test stays quarantined; used to compute
//     the expected re-enable date in the bug description
//   - TokenExpiresAt — expiry time of the API token; enables [Config.TokenExpirySoon]
//   - TokenExpiryWarningDays — days before expiry to warn; defaults to 14
//
// [Config.Validate] rejects missing or whitespace-only required fields and
// should be called before storing or passing the config anywhere:
//
//	cfg := jira.Config{
//	    APIURL:      "https://redhat.atlassian.net",
//	    UserEmail:   os.Getenv("JIRA_USER_EMAIL"),
//	    APIToken:    os.Getenv("QUARANTINE_JIRA_API_TOKEN"),
//	    ProjectKey:  "RHOAIENG",
//	    IssueType:   "Bug",
//	    Labels:      []string{"flaky-test", "quarantine"},
//	    QuarantineDuration: 30 * 24 * time.Hour,
//	}
//	if err := cfg.Validate(); err != nil {
//	    // handle invalid config early
//	}
//
// # Creating a Client
//
// When using the flakiness YAML config loader, use [FromFlakinessConfig] to
// build a Config from the loaded [flakiness.JiraConfig]. It resolves the API
// token from the environment variable named in TokenEnv automatically:
//
//	fCfg, err := flakiness.LoadConfig(".flakiness.yaml")
//	if err != nil {
//	    // handle error
//	}
//	clientCfg, err := jira.FromFlakinessConfig(fCfg.Jira)
//	if err != nil {
//	    // handle error
//	}
//	client, err := jira.NewClient(clientCfg, nil)
//	if err != nil {
//	    // handle error
//	}
//
// When constructing Config directly, pass a validated [Config] to [NewClient].
// The second argument is an optional *http.Client — pass nil to use the default
// (30 s timeout). In tests, pass httptest.Server.Client() to point the client
// at a local test server:
//
//	client, err := jira.NewClient(cfg, nil)
//
// # Usage
//
// [Client.CreateBug] files a Bug ticket for a quarantined test and returns the
// new ticket key. Store the key in [flakiness.QuarantineEntry.JiraKey] so future
// runs can reference it:
//
//	key, err := client.CreateBug(ctx, entry)
//	if err != nil {
//	    // handle error
//	}
//	entry.JiraKey = key
//
// [Client.GetStatus] returns the current status name of a ticket (e.g. "Done").
// The composition layer uses this to auto-remove quarantine entries whose tickets
// have been resolved:
//
//	status, err := client.GetStatus(ctx, entry.JiraKey)
//	if status == "Done" {
//	    // remove entry from quarantine list
//	}
//
// [Client.AddComment] posts a plain-text comment to an existing ticket. Use this
// to note re-quarantine events or test deletions without opening a new ticket:
//
//	err := client.AddComment(ctx, entry.JiraKey, "re-quarantining: still flaky after expiry")
//
// # Token Expiry
//
// Track token expiry with [Config.TokenExpiresAt] and check it with
// [Config.TokenExpirySoon] before each pipeline run:
//
//	if cfg.TokenExpirySoon(time.Now()) {
//	    log.Warn("Jira API token expires soon — rotate before it breaks CI")
//	}
//
// The .github/workflows/check-jira-token-expiry.yaml workflow automates this
// check weekly: it validates the token against the Jira API and opens a GitHub
// issue if the token is expired or missing.
package jira
