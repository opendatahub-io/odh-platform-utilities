//go:build integration

// Integration tests for the Jira client. These tests make real HTTP calls
// to a Jira instance and require the following env vars:
//
//	JIRA_USER_EMAIL            — email of the Jira account
//	QUARANTINE_JIRA_API_TOKEN  — API token for that account
//	JIRA_TEST_PROJECT_KEY      — project key to create test tickets in
//
// Run with: go test ./flakiness/jira/... -tags integration -v -run TestIntegration
package jira_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
	"github.com/opendatahub-io/odh-platform-utilities/flakiness/jira"
)

func integrationConfig(t *testing.T) jira.Config {
	t.Helper()

	email := os.Getenv("JIRA_USER_EMAIL")
	token := os.Getenv("QUARANTINE_JIRA_API_TOKEN")

	if email == "" || token == "" {
		t.Skip("JIRA_USER_EMAIL and QUARANTINE_JIRA_API_TOKEN must be set for integration tests")
	}

	projectKey := os.Getenv("JIRA_TEST_PROJECT_KEY")
	if projectKey == "" {
		t.Fatal("JIRA_TEST_PROJECT_KEY must be set to avoid creating tickets in the wrong project")
	}

	return jira.Config{
		APIURL:             "https://redhat.atlassian.net",
		UserEmail:          email,
		ProjectKey:         projectKey,
		IssueType:          "Bug",
		APIToken:           token,
		QuarantineDuration: 30 * 24 * time.Hour,
		Labels:             []string{"flaky-test", "quarantine", "integration-test"},
	}
}

// cleanupIssue transitions a Jira issue to Done — used for test cleanup.
func cleanupIssue(t *testing.T, cfg jira.Config, key string) {
	t.Helper()

	creds := base64.StdEncoding.EncodeToString([]byte(cfg.UserEmail + ":" + cfg.APIToken))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// fetch available transitions
	req, err := http.NewRequestWithContext(ctx,
		http.MethodGet, cfg.APIURL+"/rest/api/3/issue/"+key+"/transitions", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	require.Truef(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
		"GET transitions returned HTTP %d for %s", resp.StatusCode, key)

	var result struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"transitions"`
	}
	require.NoError(t, json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result))

	for _, tr := range result.Transitions {
		if tr.Name == "Done" || tr.Name == "Closed" || tr.Name == "Resolved" {
			body, _ := json.Marshal(map[string]any{"transition": map[string]string{"id": tr.ID}})
			doReq, err := http.NewRequestWithContext(ctx,
				http.MethodPost, cfg.APIURL+"/rest/api/3/issue/"+key+"/transitions",
				bytes.NewReader(body))
			require.NoError(t, err)
			doReq.Header.Set("Authorization", "Basic "+creds)
			doReq.Header.Set("Content-Type", "application/json")
			doResp, err := http.DefaultClient.Do(doReq)
			require.NoError(t, err)
			_ = doResp.Body.Close()
			require.Truef(t, doResp.StatusCode >= 200 && doResp.StatusCode < 300,
				"POST transition returned HTTP %d for %s", doResp.StatusCode, key)
			t.Logf("cleanup: transitioned %s to %q", key, tr.Name)
			return
		}
	}

	require.Failf(t, "cleanup failed", "no Done/Closed/Resolved transition found for %s — delete it manually", key)
}

func TestIntegrationCreateBug(t *testing.T) {
	cfg := integrationConfig(t)

	client, err := jira.NewClient(cfg, nil)
	require.NoError(t, err)

	entry := flakiness.QuarantineEntry{
		Name:       fmt.Sprintf("TestIntegration/smoke-%d", time.Now().UnixMilli()),
		Suite:      "integration",
		FlakeRate:  0.42,
		TotalRuns:  50,
		FailedRuns: 21,
		LastFailed: time.Now().Add(-24 * time.Hour).UTC(),
	}

	key, err := client.CreateBug(context.Background(), entry)
	require.NoError(t, err)
	require.NotEmpty(t, key)
	t.Logf("created issue: %s/browse/%s", cfg.APIURL, key)

	t.Cleanup(func() { cleanupIssue(t, cfg, key) })

	t.Run("GetStatus returns a non-empty status", func(t *testing.T) {
		status, err := client.GetStatus(context.Background(), key)
		require.NoError(t, err)
		assert.NotEmpty(t, status)
		t.Logf("status: %s", status)
	})

	t.Run("AddComment succeeds", func(t *testing.T) {
		err := client.AddComment(context.Background(), key, "integration test comment — safe to ignore")
		require.NoError(t, err)
	})
}

func TestIntegrationGetStatusNotFound(t *testing.T) {
	cfg := integrationConfig(t)

	client, err := jira.NewClient(cfg, nil)
	require.NoError(t, err)

	_, err = client.GetStatus(context.Background(), "DOESNOTEXIST-99999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}
