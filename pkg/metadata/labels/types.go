// Package labels provides well-known label key constants and helpers for the
// ODH platform. Labels are divided into two categories:
//
//   - Contract labels: required by the orchestrator. Wrong values will break
//     orchestrator discovery and lifecycle management.
//   - Recommended standard labels: the blessed convention for consistent
//     resource tracking, event routing, and garbage collection across modules.
//     Not required by the orchestrator, but strongly encouraged.
package labels

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	labelValueRegexp = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]*[a-z0-9])?$`)

	// ErrLabelValueTooLong is returned when a label value exceeds the
	// Kubernetes 63-character limit.
	ErrLabelValueTooLong = errors.New("label value exceeds 63-character limit")

	// ErrLabelValueInvalid is returned when a label value contains characters
	// not permitted by Kubernetes (must match [a-z0-9._-] and start/end with
	// an alphanumeric character).
	ErrLabelValueInvalid = errors.New("label value contains invalid characters")
)

// --- Label prefixes ---

const (
	// ODHAppPrefix is the label key prefix for application-level ODH labels.
	ODHAppPrefix = "app.opendatahub.io"

	// ODHPlatformPrefix is the label key prefix for platform-level ODH labels.
	ODHPlatformPrefix = "platform.opendatahub.io"

	// ODHInfrastructurePrefix is the label key prefix for infrastructure-level
	// ODH labels.
	ODHInfrastructurePrefix = "infrastructure.opendatahub.io"
)

// --- Contract labels (orchestrator requires these) ---

const (
	// ManagedBy is the label key the orchestrator uses to discover module
	// bootstrap resources. The orchestrator watches all resources carrying this
	// label and uses it to prune resources when a module is removed.
	//
	// Contract: the orchestrator breaks if this label is missing or incorrect.
	ManagedBy = "components." + ODHPlatformPrefix + "/managed-by"
)

// --- Recommended standard labels ---

const (
	// PlatformPartOf identifies which controller owns a deployed resource.
	// The reconciler builder uses it for watch filtering, and the GC action
	// uses it as the label selector. The standard value is the lowercase Kind
	// name of the controller CR, normalized via NormalizePartOfValue.
	//
	// Recommended standard: not required by the orchestrator, but the blessed
	// convention for modules using the shared deploy/GC framework.
	PlatformPartOf = ODHPlatformPrefix + "/part-of"

	// PlatformDependency marks dependency relationships between platform
	// resources.
	//
	// Recommended standard.
	PlatformDependency = ODHPlatformPrefix + "/dependency"

	// InfrastructurePartOf is the infrastructure-layer equivalent of
	// PlatformPartOf. Used for CloudManager resources.
	//
	// Recommended standard.
	InfrastructurePartOf = ODHInfrastructurePrefix + "/part-of"

	// Platform is the value used with PlatformPartOf for platform-level
	// resources such as CRDs.
	//
	// Recommended standard.
	Platform = "platform"
)

// NormalizePartOfValue lowercases and trims whitespace from v so that cache
// selectors and deploy actions produce identical part-of label values.
// Both sides must normalize identically for GC label selection to work.
//
// The result is validated against Kubernetes label-value rules (max 63
// characters, alphanumeric with '-', '_', '.' allowed). An empty string
// is valid. An error is returned if the normalized value is not a legal
// Kubernetes label value.
func NormalizePartOfValue(v string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(v))
	if normalized == "" {
		return normalized, nil
	}

	if len(normalized) > 63 {
		return "", fmt.Errorf("%w: %q (%d characters)", ErrLabelValueTooLong, normalized, len(normalized))
	}

	if !labelValueRegexp.MatchString(normalized) {
		return "", fmt.Errorf("%w: %q (must match [a-z0-9._-])", ErrLabelValueInvalid, normalized)
	}

	return normalized, nil
}
