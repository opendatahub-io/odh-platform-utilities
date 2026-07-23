package jira

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/flakiness"
)

const defaultTokenExpiryWarningDays = 14

// Config holds all settings for the Jira client.
// The consumer is responsible for loading values (flags, env vars, etc.).
type Config struct {
	APIURL                 string
	UserEmail              string // Jira Cloud: email paired with APIToken for Basic auth
	ProjectKey             string
	IssueType              string
	Component              string
	Labels                 []string
	APIToken               string
	TokenExpiresAt         time.Time     // zero means "not tracked"
	TokenExpiryWarningDays int           // days before expiry to warn; 0 → 14
	QuarantineDuration     time.Duration // used to compute ReenableAt in bug descriptions
}

// Validate returns an error if required fields are missing.
func (c Config) Validate() error {
	switch {
	case strings.TrimSpace(c.APIURL) == "":
		return errors.New("jira: APIURL is required")
	case strings.TrimSpace(c.UserEmail) == "":
		return errors.New("jira: UserEmail is required")
	case strings.TrimSpace(c.ProjectKey) == "":
		return errors.New("jira: ProjectKey is required")
	case strings.TrimSpace(c.IssueType) == "":
		return errors.New("jira: IssueType is required")
	case strings.TrimSpace(c.APIToken) == "":
		return errors.New("jira: APIToken is required")
	}

	return nil
}

// FromFlakinessConfig builds a Config from a [flakiness.JiraConfig] loaded
// from a YAML file. The API token is resolved by reading the environment
// variable named in fc.TokenEnv.
func FromFlakinessConfig(fc flakiness.JiraConfig) (Config, error) {
	token := ""
	if fc.TokenEnv != "" {
		token = os.Getenv(fc.TokenEnv)
	}

	cfg := Config{
		APIURL:             fc.APIURL,
		UserEmail:          fc.UserEmail,
		ProjectKey:         fc.Project,
		IssueType:          fc.IssueType,
		Component:          fc.Component,
		Labels:             fc.Labels,
		APIToken:           token,
		QuarantineDuration: fc.QuarantineDuration,
	}

	return cfg, cfg.Validate()
}

// TokenExpirySoon reports whether the token expires within the warning window.
func (c Config) TokenExpirySoon(now time.Time) bool {
	if c.TokenExpiresAt.IsZero() {
		return false
	}

	days := c.TokenExpiryWarningDays
	if days <= 0 {
		days = defaultTokenExpiryWarningDays
	}

	return c.TokenExpiresAt.Before(now.Add(time.Duration(days) * 24 * time.Hour))
}
