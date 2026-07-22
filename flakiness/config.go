package flakiness

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultThreshold     = 0.2
	DefaultWindowDays    = 30
	DefaultMinRunsConfig = 5
)

// Config holds per-component flakiness system configuration, typically
// loaded from a .flakiness.yaml file in the module repository root.
type Config struct {
	Component  string           `yaml:"component"`
	GCS        GCSConfig        `yaml:"gcs"`
	Analysis   AnalysisConfig   `yaml:"analysis"`
	Quarantine QuarantineConfig `yaml:"quarantine"`
	Jira       JiraConfig       `yaml:"jira"`
}

// GCSConfig specifies the GCS bucket and job prefixes to scrape.
type GCSConfig struct {
	Bucket      string   `yaml:"bucket"`
	JobPrefixes []string `yaml:"job_prefixes"` //nolint:tagliatelle // snake_case is the YAML config convention
}

// AnalysisConfig controls the flake detection algorithm parameters.
type AnalysisConfig struct {
	Threshold  float64 `yaml:"threshold"`
	WindowDays int     `yaml:"window_days"` //nolint:tagliatelle // snake_case is the YAML config convention
	MinRuns    int     `yaml:"min_runs"`    //nolint:tagliatelle // snake_case is the YAML config convention
}

// QuarantineConfig controls quarantine output and behavior.
type QuarantineConfig struct {
	ConfigPath      string   `yaml:"config_path"`      //nolint:tagliatelle // snake_case is the YAML config convention
	AutoQuarantine  bool     `yaml:"auto_quarantine"`  //nolint:tagliatelle // snake_case is the YAML config convention
	ExcludePatterns []string `yaml:"exclude_patterns"` //nolint:tagliatelle // snake_case is the YAML config convention
}

// JiraConfig specifies how quarantine Jira tickets are filed.
type JiraConfig struct {
	APIURL             string        `yaml:"api_url"`    //nolint:tagliatelle // snake_case is the YAML config convention
	UserEmail          string        `yaml:"user_email"` //nolint:tagliatelle // snake_case is the YAML config convention
	Project            string        `yaml:"project"`
	IssueType          string        `yaml:"issue_type"` //nolint:tagliatelle // snake_case is the YAML config convention
	Component          string        `yaml:"component"`
	Labels             []string      `yaml:"labels"`
	TokenEnv           string        `yaml:"token_env"`           //nolint:tagliatelle // snake_case is the YAML config convention
	QuarantineDuration time.Duration `yaml:"quarantine_duration"` //nolint:tagliatelle // snake_case is the YAML config convention
}

// LoadConfig reads a YAML configuration file and returns a validated
// Config with defaults applied. Environment variables override the
// corresponding config fields (see [applyEnvOverrides]).
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is caller-provided config file location
	if err != nil {
		return Config{}, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cfg Config

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	cfg.applyDefaults()

	if err := cfg.applyEnvOverrides(); err != nil {
		return Config{}, fmt.Errorf("applying env overrides: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validating config from %s: %w", path, err)
	}

	return cfg, nil
}

// Validate checks the configuration for required fields and value
// constraints. Returns a combined error describing all violations.
func (c *Config) Validate() error {
	var errs []error

	if c.Component == "" {
		errs = append(errs, errors.New("component is required"))
	}

	if c.GCS.Bucket == "" {
		errs = append(errs, errors.New("gcs.bucket is required"))
	}

	if len(c.GCS.JobPrefixes) == 0 {
		errs = append(errs, errors.New("gcs.job_prefixes must contain at least one entry"))
	}

	if c.Analysis.Threshold <= 0 || c.Analysis.Threshold > 1 {
		errs = append(errs, fmt.Errorf("analysis.threshold must be in (0, 1], got %g", c.Analysis.Threshold))
	}

	if c.Analysis.WindowDays <= 0 {
		errs = append(errs, fmt.Errorf("analysis.window_days must be positive, got %d", c.Analysis.WindowDays))
	}

	if c.Analysis.MinRuns <= 0 {
		errs = append(errs, fmt.Errorf("analysis.min_runs must be positive, got %d", c.Analysis.MinRuns))
	}

	for i, pattern := range c.Quarantine.ExcludePatterns {
		if _, err := regexp.Compile(pattern); err != nil {
			errs = append(errs, fmt.Errorf("quarantine.exclude_patterns[%d] %q: invalid regex: %w", i, pattern, err))
		}
	}

	return errors.Join(errs...)
}

func (c *Config) applyDefaults() {
	if c.Analysis.Threshold == 0 {
		c.Analysis.Threshold = DefaultThreshold
	}

	if c.Analysis.WindowDays == 0 {
		c.Analysis.WindowDays = DefaultWindowDays
	}

	if c.Analysis.MinRuns == 0 {
		c.Analysis.MinRuns = DefaultMinRunsConfig
	}

	if c.Jira.IssueType == "" {
		c.Jira.IssueType = "Bug"
	}
}

// applyEnvOverrides overrides scalar config fields from FLAKINESS_*
// environment variables. Returns an error if a numeric variable is set
// but cannot be parsed.
func (c *Config) applyEnvOverrides() error {
	var errs []error

	if v := os.Getenv("FLAKINESS_COMPONENT"); v != "" {
		c.Component = v
	}

	if v := os.Getenv("FLAKINESS_GCS_BUCKET"); v != "" {
		c.GCS.Bucket = v
	}

	if v := os.Getenv("FLAKINESS_THRESHOLD"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("FLAKINESS_THRESHOLD=%q: %w", v, err))
		} else {
			c.Analysis.Threshold = f
		}
	}

	if v := os.Getenv("FLAKINESS_WINDOW_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("FLAKINESS_WINDOW_DAYS=%q: %w", v, err))
		} else {
			c.Analysis.WindowDays = n
		}
	}

	if v := os.Getenv("FLAKINESS_MIN_RUNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("FLAKINESS_MIN_RUNS=%q: %w", v, err))
		} else {
			c.Analysis.MinRuns = n
		}
	}

	if v := os.Getenv("FLAKINESS_QUARANTINE_CONFIG_PATH"); v != "" {
		c.Quarantine.ConfigPath = v
	}

	if v := os.Getenv("FLAKINESS_AUTO_QUARANTINE"); v != "" {
		c.Quarantine.AutoQuarantine = strings.EqualFold(v, "true")
	}

	if v := os.Getenv("FLAKINESS_JIRA_API_URL"); v != "" {
		c.Jira.APIURL = v
	}

	if v := os.Getenv("FLAKINESS_JIRA_USER_EMAIL"); v != "" {
		c.Jira.UserEmail = v
	}

	if v := os.Getenv("FLAKINESS_JIRA_PROJECT"); v != "" {
		c.Jira.Project = v
	}

	if v := os.Getenv("FLAKINESS_JIRA_ISSUE_TYPE"); v != "" {
		c.Jira.IssueType = v
	}

	if v := os.Getenv("FLAKINESS_JIRA_COMPONENT"); v != "" {
		c.Jira.Component = v
	}

	if v := os.Getenv("FLAKINESS_JIRA_TOKEN_ENV"); v != "" {
		c.Jira.TokenEnv = v
	}

	return errors.Join(errs...)
}

// FilterExcluded removes quarantine entries whose test name matches
// any of the configured exclude patterns. Entries that match are
// returned with Quarantined set to false.
func (c *Config) FilterExcluded(entries []QuarantineEntry) []QuarantineEntry {
	if len(c.Quarantine.ExcludePatterns) == 0 {
		return entries
	}

	patterns := make([]*regexp.Regexp, 0, len(c.Quarantine.ExcludePatterns))
	for _, p := range c.Quarantine.ExcludePatterns {
		patterns = append(patterns, regexp.MustCompile(p))
	}

	result := make([]QuarantineEntry, 0, len(entries))

	for _, e := range entries {
		if matchesAny(patterns, e.Name) {
			e.Quarantined = false
		}

		result = append(result, e)
	}

	return result
}

func matchesAny(patterns []*regexp.Regexp, s string) bool {
	for _, p := range patterns {
		if p.MatchString(s) {
			return true
		}
	}

	return false
}
