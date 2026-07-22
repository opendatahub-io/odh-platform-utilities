package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

// Client creates and manages Jira issues for quarantined tests.
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient returns a Client after validating cfg. Pass a non-nil
// httpClient to override the default (useful in tests with httptest).
func NewClient(cfg Config, httpClient *http.Client) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{cfg: cfg, http: httpClient}, nil
}

// CreateBug files a Jira Bug for a quarantined test and returns the
// new ticket key (e.g. "RHOAIENG-1234").
func (c *Client) CreateBug(ctx context.Context, entry flakiness.QuarantineEntry) (string, error) {
	now := time.Now().UTC()
	reenableAt := now.Add(c.cfg.QuarantineDuration)

	summary := fmt.Sprintf("[Flaky Test] %s (%.1f%% flake rate)", entry.Name, entry.FlakeRate*100)

	fields := map[string]any{
		"project":     map[string]string{"key": c.cfg.ProjectKey},
		"summary":     summary,
		"issuetype":   map[string]string{"name": c.cfg.IssueType},
		"description": descriptionADF(entry, now, reenableAt),
	}

	if c.cfg.Component != "" {
		fields["components"] = []map[string]string{{"name": c.cfg.Component}}
	}

	if len(c.cfg.Labels) > 0 {
		fields["labels"] = c.cfg.Labels
	}

	var result struct {
		Key string `json:"key"`
	}

	if err := c.do(ctx, http.MethodPost, "/rest/api/3/issue", map[string]any{"fields": fields}, &result); err != nil {
		return "", fmt.Errorf("creating jira bug for %q: %w", entry.Name, err)
	}

	return result.Key, nil
}

// GetStatus returns the status name of a Jira issue (e.g. "Done").
func (c *Client) GetStatus(ctx context.Context, key string) (string, error) {
	var result struct {
		Fields struct {
			Status struct {
				Name string `json:"name"`
			} `json:"status"`
		} `json:"fields"`
	}

	if err := c.do(ctx, http.MethodGet, "/rest/api/3/issue/"+url.PathEscape(key)+"?fields=status", nil, &result); err != nil {
		return "", fmt.Errorf("getting status for %q: %w", key, err)
	}

	return result.Fields.Status.Name, nil
}

// AddComment posts a plain-text comment to an existing Jira issue.
func (c *Client) AddComment(ctx context.Context, key, text string) error {
	if err := c.do(ctx, http.MethodPost, "/rest/api/3/issue/"+url.PathEscape(key)+"/comment", map[string]any{"body": adfDoc(text)}, nil); err != nil {
		return fmt.Errorf("adding comment to %q: %w", key, err)
	}

	return nil
}

// do executes a JSON request against the Jira REST API.
func (c *Client) do(ctx context.Context, method, path string, reqBody, respBody any) error {
	var bodyReader io.Reader

	if reqBody != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(reqBody); err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}

		bodyReader = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, c.cfg.APIURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	creds := base64.StdEncoding.EncodeToString([]byte(c.cfg.UserEmail + ":" + c.cfg.APIToken))
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}

	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}

	if respBody != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(respBody); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}

// adfDoc wraps plain text in a single ADF paragraph — used for comments.
func adfDoc(text string) map[string]any {
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []map[string]any{
			{
				"type": "paragraph",
				"content": []map[string]any{
					{"type": "text", "text": text},
				},
			},
		},
	}
}

// descriptionADF builds an ADF document for a bug description. Each field
// gets its own paragraph with a bold label and a plain value.
func descriptionADF(entry flakiness.QuarantineEntry, quarantinedAt, reenableAt time.Time) map[string]any {
	type field struct{ label, value string }

	fields := []field{
		{"Test", entry.Name},
	}

	if entry.Suite != "" {
		fields = append(fields, field{"Suite", entry.Suite})
	}

	if entry.Job != "" {
		fields = append(fields, field{"Job", entry.Job})
	}

	fields = append(fields, field{
		"Flake rate",
		fmt.Sprintf("%.1f%% (%d/%d runs failed)", entry.FlakeRate*100, entry.FailedRuns, entry.TotalRuns),
	})

	if !entry.LastFailed.IsZero() {
		fields = append(fields, field{"Last failed", entry.LastFailed.UTC().Format(time.DateOnly)})
	}

	fields = append(fields, field{"Quarantined", quarantinedAt.Format(time.DateOnly)})

	if !reenableAt.Equal(quarantinedAt) {
		fields = append(fields, field{"Expected re-enable", reenableAt.Format(time.DateOnly)})
	}

	strong := []map[string]any{{"type": "strong"}}

	paragraphs := make([]map[string]any, len(fields))
	for i, f := range fields {
		paragraphs[i] = map[string]any{
			"type": "paragraph",
			"content": []map[string]any{
				{"type": "text", "text": f.label + ": ", "marks": strong},
				{"type": "text", "text": f.value},
			},
		}
	}

	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": paragraphs,
	}
}
