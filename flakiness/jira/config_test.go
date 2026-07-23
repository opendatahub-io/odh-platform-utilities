package jira_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness/jira"
)

func validConfig() jira.Config {
	return jira.Config{
		APIURL:     "https://issues.example.com",
		UserEmail:  "bot@example.com",
		ProjectKey: "TEST",
		IssueType:  "Bug",
		APIToken:   "secret",
	}
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*jira.Config)
		wantErr string
	}{
		{
			name:   "valid config",
			mutate: func(_ *jira.Config) {},
		},
		{
			name:    "missing APIURL",
			mutate:  func(c *jira.Config) { c.APIURL = "" },
			wantErr: "APIURL is required",
		},
		{
			name:    "missing ProjectKey",
			mutate:  func(c *jira.Config) { c.ProjectKey = "" },
			wantErr: "ProjectKey is required",
		},
		{
			name:    "missing IssueType",
			mutate:  func(c *jira.Config) { c.IssueType = "" },
			wantErr: "IssueType is required",
		},
		{
			name:    "missing UserEmail",
			mutate:  func(c *jira.Config) { c.UserEmail = "" },
			wantErr: "UserEmail is required",
		},
		{
			name:    "missing APIToken",
			mutate:  func(c *jira.Config) { c.APIToken = "" },
			wantErr: "APIToken is required",
		},
		{
			name:    "whitespace-only APIURL",
			mutate:  func(c *jira.Config) { c.APIURL = "   " },
			wantErr: "APIURL is required",
		},
		{
			name:    "whitespace-only APIToken",
			mutate:  func(c *jira.Config) { c.APIToken = "\t" },
			wantErr: "APIToken is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validConfig()
			tc.mutate(&cfg)

			err := cfg.Validate()

			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestConfigTokenExpirySoon(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		tokenExpiresAt time.Time
		warningDays    int
		want           bool
	}{
		{
			name:           "zero expiry is never soon",
			tokenExpiresAt: time.Time{},
			want:           false,
		},
		{
			name:           "expires within default 14-day window",
			tokenExpiresAt: now.Add(10 * 24 * time.Hour),
			want:           true,
		},
		{
			name:           "expires exactly at 14-day boundary",
			tokenExpiresAt: now.Add(14 * 24 * time.Hour),
			want:           false,
		},
		{
			name:           "expires well outside default window",
			tokenExpiresAt: now.Add(30 * 24 * time.Hour),
			want:           false,
		},
		{
			name:           "already expired",
			tokenExpiresAt: now.Add(-1 * 24 * time.Hour),
			want:           true,
		},
		{
			name:           "custom warning days respected",
			tokenExpiresAt: now.Add(20 * 24 * time.Hour),
			warningDays:    30,
			want:           true,
		},
		{
			name:           "zero warning days falls back to default",
			tokenExpiresAt: now.Add(10 * 24 * time.Hour),
			warningDays:    0,
			want:           true,
		},
		{
			name:           "negative warning days falls back to default",
			tokenExpiresAt: now.Add(10 * 24 * time.Hour),
			warningDays:    -5,
			want:           true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := jira.Config{
				TokenExpiresAt:         tc.tokenExpiresAt,
				TokenExpiryWarningDays: tc.warningDays,
			}

			assert.Equal(t, tc.want, cfg.TokenExpirySoon(now))
		})
	}
}
