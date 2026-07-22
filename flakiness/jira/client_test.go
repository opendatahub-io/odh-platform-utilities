package jira_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
	"github.com/opendatahub-io/odh-platform-utilities/flakiness/jira"
)

func newTestClient(t *testing.T, srv *httptest.Server) *jira.Client {
	t.Helper()

	cfg := jira.Config{
		APIURL:             srv.URL,
		UserEmail:          "bot@example.com",
		ProjectKey:         "TEST",
		IssueType:          "Bug",
		APIToken:           "test-token",
		QuarantineDuration: 30 * 24 * time.Hour,
	}

	client, err := jira.NewClient(cfg, srv.Client())
	require.NoError(t, err)

	return client
}

func TestClientCreateBug(t *testing.T) {
	t.Parallel()

	entry := flakiness.QuarantineEntry{
		Name:       "TestFoo",
		Suite:      "e2e",
		Job:        "periodic-ci",
		FlakeRate:  0.42,
		TotalRuns:  50,
		FailedRuns: 21,
		LastFailed: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
	}

	t.Run("creates bug and returns key", func(t *testing.T) {
		t.Parallel()

		var captured map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/rest/api/3/issue", r.URL.Path)
			assert.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("bot@example.com:test-token")), r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			_ = json.NewDecoder(r.Body).Decode(&captured)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"10001","key":"TEST-123"}`))
		}))
		defer srv.Close()

		key, err := newTestClient(t, srv).CreateBug(context.Background(), entry)
		require.NoError(t, err)
		assert.Equal(t, "TEST-123", key)

		fields, ok := captured["fields"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, fields["summary"], "TestFoo")
		assert.Contains(t, fields["summary"], "42.0%")
	})

	t.Run("includes component and labels when configured", func(t *testing.T) {
		t.Parallel()

		var captured map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&captured)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"key":"TEST-124"}`))
		}))
		defer srv.Close()

		cfg := jira.Config{
			APIURL:     srv.URL,
			UserEmail:  "bot@example.com",
			ProjectKey: "TEST",
			IssueType:  "Bug",
			APIToken:   "token",
			Component:  "testing",
			Labels:     []string{"flaky-test", "quarantine"},
		}
		client, err := jira.NewClient(cfg, srv.Client())
		require.NoError(t, err)

		_, err = client.CreateBug(context.Background(), entry)
		require.NoError(t, err)

		fields, ok := captured["fields"].(map[string]any)
		require.True(t, ok)
		components, ok := fields["components"].([]any)
		require.True(t, ok)
		assert.Len(t, components, 1)
		component, ok := components[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "testing", component["name"])
		labels, ok := fields["labels"].([]any)
		require.True(t, ok)
		assert.Contains(t, labels, "flaky-test")
	})

	t.Run("description contains flakiness details", func(t *testing.T) {
		t.Parallel()

		var captured map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&captured)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"key":"TEST-123"}`))
		}))
		defer srv.Close()

		_, err := newTestClient(t, srv).CreateBug(context.Background(), entry)
		require.NoError(t, err)

		fields, ok := captured["fields"].(map[string]any)
		require.True(t, ok)
		adf, ok := fields["description"].(map[string]any)
		require.True(t, ok)
		paragraphs, ok := adf["content"].([]any)
		require.True(t, ok)

		// collect all text across all paragraphs (bold label + plain value nodes)
		var combined strings.Builder
		for _, para := range paragraphs {
			nodes, ok := para.(map[string]any)["content"].([]any)
			require.True(t, ok)
			for _, node := range nodes {
				text, ok := node.(map[string]any)["text"].(string)
				require.True(t, ok)
				combined.WriteString(text)
			}
			combined.WriteString("\n")
		}
		text := combined.String()

		assert.Contains(t, text, "TestFoo")
		assert.Contains(t, text, "e2e")         // suite
		assert.Contains(t, text, "periodic-ci") // job
		assert.Contains(t, text, "42.0%")       // flake rate
		assert.Contains(t, text, "21/50")       // failed/total runs
		assert.Contains(t, text, "2026-07-15")  // last failed
		assert.Contains(t, text, "Quarantined:")
		assert.Contains(t, text, "Expected re-enable:")
		assert.Greater(t, len(paragraphs), 1) // each field is its own paragraph
	})

	t.Run("returns error on non-2xx response", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errorMessages":["unauthorized"]}`))
		}))
		defer srv.Close()

		_, err := newTestClient(t, srv).CreateBug(context.Background(), entry)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "401")
	})
}

func TestClientGetStatus(t *testing.T) {
	t.Parallel()

	t.Run("returns status name", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "/rest/api/3/issue/TEST-123", r.URL.Path)
			assert.Equal(t, "status", r.URL.Query().Get("fields"))

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"fields":{"status":{"name":"Done"}}}`))
		}))
		defer srv.Close()

		status, err := newTestClient(t, srv).GetStatus(context.Background(), "TEST-123")
		require.NoError(t, err)
		assert.Equal(t, "Done", status)
	})

	t.Run("returns error on non-2xx response", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errorMessages":["issue not found"]}`))
		}))
		defer srv.Close()

		_, err := newTestClient(t, srv).GetStatus(context.Background(), "TEST-999")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
		assert.Contains(t, err.Error(), "TEST-999")
	})
}

func TestClientAddComment(t *testing.T) {
	t.Parallel()

	t.Run("posts comment body", func(t *testing.T) {
		t.Parallel()

		var captured map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/rest/api/3/issue/TEST-123/comment", r.URL.Path)
			_ = json.NewDecoder(r.Body).Decode(&captured)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"1"}`))
		}))
		defer srv.Close()

		err := newTestClient(t, srv).AddComment(context.Background(), "TEST-123", "re-quarantining after expiry")
		require.NoError(t, err)

		adf, ok := captured["body"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "doc", adf["type"])
		content, ok := adf["content"].([]any)
		require.True(t, ok)
		paragraph, ok := content[0].(map[string]any)["content"].([]any)
		require.True(t, ok)
		text, ok := paragraph[0].(map[string]any)["text"].(string)
		require.True(t, ok)
		assert.Equal(t, "re-quarantining after expiry", text)
	})

	t.Run("returns error on non-2xx response", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer srv.Close()

		err := newTestClient(t, srv).AddComment(context.Background(), "TEST-123", "hello")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "403")
	})
}

func TestNewClientInvalidConfig(t *testing.T) {
	t.Parallel()

	_, err := jira.NewClient(jira.Config{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APIURL is required")
}
