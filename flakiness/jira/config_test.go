package jira_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
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
			name:    "HTTP URL rejected",
			mutate:  func(c *jira.Config) { c.APIURL = "http://issues.example.com" },
			wantErr: "must be an absolute HTTPS URL",
		},
		{
			name:    "missing host rejected",
			mutate:  func(c *jira.Config) { c.APIURL = "https://" },
			wantErr: "must be an absolute HTTPS URL",
		},
		{
			name:    "no scheme rejected",
			mutate:  func(c *jira.Config) { c.APIURL = "issues.example.com" },
			wantErr: "must be an absolute HTTPS URL",
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

func TestFromFlakinessConfig(t *testing.T) {
	// subtests set env vars so they run sequentially (no t.Parallel in subtests)

	base := flakiness.JiraConfig{
		APIURL:             "https://redhat.atlassian.net",
		UserEmail:          "bot@example.com",
		Project:            "RHOAIENG",
		IssueType:          "Bug",
		Component:          "KServe",
		Labels:             []string{"flaky-test"},
		TokenEnv:           "TEST_JIRA_TOKEN",
		QuarantineDuration: 30 * 24 * time.Hour,
	}

	t.Run("happy path maps all fields", func(t *testing.T) {
		t.Setenv("TEST_JIRA_TOKEN", "my-secret-token")

		cfg, err := jira.FromFlakinessConfig(base)
		require.NoError(t, err)

		assert.Equal(t, base.APIURL, cfg.APIURL)
		assert.Equal(t, base.UserEmail, cfg.UserEmail)
		assert.Equal(t, base.Project, cfg.ProjectKey)
		assert.Equal(t, base.IssueType, cfg.IssueType)
		assert.Equal(t, base.Component, cfg.Component)
		assert.Equal(t, base.Labels, cfg.Labels)
		assert.Equal(t, base.QuarantineDuration, cfg.QuarantineDuration)
		assert.Equal(t, "my-secret-token", cfg.APIToken)
	})

	t.Run("token resolved from env var named by TokenEnv", func(t *testing.T) {
		t.Setenv("TEST_JIRA_TOKEN", "injected-token")

		cfg, err := jira.FromFlakinessConfig(base)
		require.NoError(t, err)
		assert.Equal(t, "injected-token", cfg.APIToken)
	})

	t.Run("empty TokenEnv leaves APIToken empty and fails validation", func(t *testing.T) {
		fc := base
		fc.TokenEnv = ""
		_, err := jira.FromFlakinessConfig(fc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "APIToken is required")
	})

	t.Run("missing required field returns validation error", func(t *testing.T) {
		t.Setenv("TEST_JIRA_TOKEN", "token")

		fc := base
		fc.APIURL = ""
		_, err := jira.FromFlakinessConfig(fc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "APIURL is required")
	})
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
