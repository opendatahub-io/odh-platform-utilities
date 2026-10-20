// Package tls resolves OpenShift TLS security profiles for framework users.
package tls

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	commonTLS "github.com/openshift/controller-runtime-common/pkg/tls"
	ocpcrypto "github.com/openshift/library-go/pkg/crypto"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	// ErrCustomTLSProfileNil is returned when a strict Custom profile has no spec.
	ErrCustomTLSProfileNil = errors.New("custom TLS profile has no custom specification")
	// ErrUnsupportedTLSProfileType is returned for an unknown strict profile type.
	ErrUnsupportedTLSProfileType = errors.New("unsupported TLS profile type")
	// ErrUnsupportedTLSProfileVersion is returned when the version is not one
	// OpenShift defines (TLS 1.0 through 1.3).
	ErrUnsupportedTLSProfileVersion = errors.New("TLS profile minimum version is unsupported")
	// ErrTLSProfileHasNoCipherSuites is returned for TLS 1.2 profiles with no ciphers.
	ErrTLSProfileHasNoCipherSuites = errors.New("TLS profile contains no cipher suites")
	// ErrTLSProfileHasNoSupportedCipherSuites is returned when Go supports none of the ciphers.
	ErrTLSProfileHasNoSupportedCipherSuites = errors.New("TLS profile contains no cipher suites supported by Go")
	// ErrTLSProfileHasNoSupportedGroups is returned when Go supports none of the groups.
	ErrTLSProfileHasNoSupportedGroups = errors.New("TLS profile contains no TLS groups supported by Go")
)

// VersionFormat controls the output format of TLS version strings.
type VersionFormat int

const (
	// FormatShort outputs "TLS1.0".."TLS1.3" (used by kube-auth-proxy).
	FormatShort VersionFormat = iota
	// FormatGo outputs "VersionTLS10".."VersionTLS13" (used by upstream
	// kube-rbac-proxy, which parses all four version names).
	FormatGo
)

const (
	tls12ShortVersion = "TLS1.2"
	tls12GoVersion    = "VersionTLS12"
)

func intermediateSpec() configv1.TLSProfileSpec {
	return *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
}

// ProfileSpecFromSecurityProfile resolves a TLSSecurityProfile to a concrete TLSProfileSpec.
// Returns the Intermediate profile for nil input, unknown types, or a Custom type with a nil spec.
func ProfileSpecFromSecurityProfile(profile *configv1.TLSSecurityProfile) *configv1.TLSProfileSpec {
	spec, err := commonTLS.GetTLSProfileSpec(profile)
	if err != nil {
		return configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	}

	return &spec
}

func minVersionToShort(v configv1.TLSProtocolVersion) string {
	switch v {
	case configv1.VersionTLS10:
		return "TLS1.0"
	case configv1.VersionTLS11:
		return "TLS1.1"
	case configv1.VersionTLS12:
		return tls12ShortVersion
	case configv1.VersionTLS13:
		return "TLS1.3"
	default:
		return ""
	}
}

func minVersionToGo(v configv1.TLSProtocolVersion) string {
	switch v {
	case configv1.VersionTLS10:
		return "VersionTLS10"
	case configv1.VersionTLS11:
		return "VersionTLS11"
	case configv1.VersionTLS12:
		return tls12GoVersion
	case configv1.VersionTLS13:
		return "VersionTLS13"
	default:
		return ""
	}
}

// MinVersionFromSpec returns the TLS minimum version string for the given profile
// spec. All four versions map in both formats: kube-auth-proxy (short format) and
// kube-rbac-proxy (Go format) both parse TLS 1.0 through TLS 1.3 names.
func MinVersionFromSpec(ctx context.Context, spec *configv1.TLSProfileSpec, format VersionFormat) string {
	l := logf.FromContext(ctx).WithName("MinVersionFromSpec")
	minVersion := configv1.TLSProfiles[configv1.TLSProfileIntermediateType].MinTLSVersion

	if spec != nil && spec.MinTLSVersion != "" {
		minVersion = spec.MinTLSVersion
	}

	var name string

	switch format {
	case FormatGo:
		name = minVersionToGo(minVersion)
	default:
		name = minVersionToShort(minVersion)
	}

	if name == "" {
		l.V(1).Info("unsupported MinTLSVersion, using TLS 1.2 as floor", "minVersion", minVersion)

		if format == FormatGo {
			return tls12GoVersion
		}

		return tls12ShortVersion
	}

	return name
}

// CipherSuitesFromSpec returns a comma-separated list of IANA cipher suite names
// for the given profile spec. Unmappable or TLS 1.3-only suites under TLS 1.2
// are logged and dropped.
func CipherSuitesFromSpec(ctx context.Context, spec *configv1.TLSProfileSpec) string {
	l := logf.FromContext(ctx).WithName("CipherSuitesFromSpec")

	if spec == nil {
		spec = configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	}

	ianaCiphers, dropped := cipherSuitesForMinVersion(spec.Ciphers, spec.MinTLSVersion)

	if len(dropped) > 0 {
		l.V(1).Info("cipher suites unusable for the configured minimum version were dropped",
			"dropped", dropped, "droppedCount", len(dropped), "totalRequested", len(spec.Ciphers))
	}

	if len(ianaCiphers) == 0 {
		l.V(1).Info("no mappable cipher suites in profile, falling back to Intermediate profile ciphers")

		intermediate := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		ianaCiphers, _ = cipherSuitesForMinVersion(intermediate.Ciphers, intermediate.MinTLSVersion)
	}

	return strings.Join(ianaCiphers, ",")
}

func cipherSuitesForMinVersion(
	ciphers []string,
	minVersion configv1.TLSProtocolVersion,
) (ianaCiphers, dropped []string) {
	for _, cipher := range ciphers {
		mapped := ocpcrypto.OpenSSLToIANACipherSuites([]string{cipher})
		if len(mapped) == 0 {
			dropped = append(dropped, cipher)
			continue
		}

		for _, ianaCipher := range mapped {
			code := cipherCode(ianaCipher)
			if code == 0 || minVersion != configv1.VersionTLS13 && isTLS13CipherSuiteID(code) {
				dropped = append(dropped, cipher)
				continue
			}
			ianaCiphers = append(ianaCiphers, ianaCipher)
		}
	}

	return ianaCiphers, dropped
}

// IsVersionSupported reports whether the legacy (non-strict) resolution path
// honors the version without flooring it to the Intermediate profile. TLS
// 1.0/1.1 return false there by design: Legacy adherence keeps hardened
// defaults. The strict path (FromProfileStrict) honors all four versions in
// both formats.
func IsVersionSupported(v configv1.TLSProtocolVersion) bool {
	switch v {
	case configv1.VersionTLS12, configv1.VersionTLS13:
		return true
	default:
		return false
	}
}

// isKnownTLSVersion reports whether the version is one of the four versions
// OpenShift defines (TLS 1.0 through 1.3). All four map to Go crypto/tls
// minimum versions: honoring a configured TLS 1.0/1.1 minimum is the contract
// under strict adherence, so only versions outside that set are rejected.
func isKnownTLSVersion(v configv1.TLSProtocolVersion) bool {
	switch v {
	case configv1.VersionTLS10, configv1.VersionTLS11, configv1.VersionTLS12, configv1.VersionTLS13:
		return true
	default:
		return false
	}
}

// FromProfile resolves a TLSSecurityProfile to version and cipher strings.
// If the profile's MinTLSVersion is unsupported (TLS 1.0/1.1), both version
// and ciphers are floored to the Intermediate profile.
func FromProfile(ctx context.Context, profile *configv1.TLSSecurityProfile, format VersionFormat) (string, string) {
	minVersion, cipherSuites, _, err := FromProfileWithCurvePreferences(ctx, profile, format)
	if err == nil {
		return minVersion, cipherSuites
	}

	l := logf.FromContext(ctx).WithName("FromProfile")
	l.V(1).Info("unsupported curve preferences; retaining TLS version and cipher settings", "error", err)

	spec := ProfileSpecFromSecurityProfile(profile)
	if !IsVersionSupported(spec.MinTLSVersion) {
		l.V(1).Info("unsupported MinTLSVersion; flooring version and ciphers to Intermediate profile",
			"requestedMinVersion", spec.MinTLSVersion)
		spec = configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	}

	return MinVersionFromSpec(ctx, spec, format), CipherSuitesFromSpec(ctx, spec)
}

// FromProfileWithCurvePreferences resolves a profile using the legacy fallback
// behavior and returns version, cipher, and curve preference strings.
func FromProfileWithCurvePreferences(
	ctx context.Context,
	profile *configv1.TLSSecurityProfile,
	format VersionFormat,
) (string, string, string, error) {
	l := logf.FromContext(ctx).WithName("FromProfile")

	spec := ProfileSpecFromSecurityProfile(profile)
	if !IsVersionSupported(spec.MinTLSVersion) {
		l.V(1).Info("unsupported MinTLSVersion; flooring version and ciphers to Intermediate profile",
			"requestedMinVersion", spec.MinTLSVersion)
		spec = configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	}

	curvePreferences, err := CurvePreferencesFromSpec(ctx, spec)
	if err != nil {
		return "", "", "", err
	}

	return MinVersionFromSpec(ctx, spec, format), CipherSuitesFromSpec(ctx, spec), curvePreferences, nil
}

// FromProfileStrict resolves a profile without silently flooring an unsupported
// minimum TLS version, replacing an unusable TLS 1.2 cipher list, or ignoring
// unsupported groups.
func FromProfileStrict(
	ctx context.Context,
	profile *configv1.TLSSecurityProfile,
	format VersionFormat,
) (string, string, error) {
	spec, err := strictProfileSpec(profile)
	if err != nil {
		return "", "", err
	}

	return fromProfileStrictSpec(ctx, spec, format)
}

// FromProfileStrictWithCurvePreferences resolves a strict profile and includes
// its supported TLS group preferences. A non-empty group list with no supported
// groups is rejected instead of being silently discarded.
func FromProfileStrictWithCurvePreferences(
	ctx context.Context,
	profile *configv1.TLSSecurityProfile,
	format VersionFormat,
) (string, string, string, error) {
	spec, err := strictProfileSpec(profile)
	if err != nil {
		return "", "", "", err
	}

	minVersion, cipherSuites, err := fromProfileStrictSpec(ctx, spec, format)
	if err != nil {
		return "", "", "", err
	}

	curvePreferences, err := CurvePreferencesFromSpec(ctx, spec)
	if err != nil {
		return "", "", "", err
	}

	return minVersion, cipherSuites, curvePreferences, nil
}

func strictProfileSpec(profile *configv1.TLSSecurityProfile) (*configv1.TLSProfileSpec, error) {
	if profile == nil || profile.Type == "" {
		return configv1.TLSProfiles[configv1.TLSProfileIntermediateType], nil
	}

	if profile.Type == configv1.TLSProfileCustomType {
		if profile.Custom == nil {
			return nil, ErrCustomTLSProfileNil
		}

		return &profile.Custom.TLSProfileSpec, nil
	}

	spec, ok := configv1.TLSProfiles[profile.Type]
	if !ok || spec == nil {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedTLSProfileType, profile.Type)
	}

	return spec, nil
}

// GetStrictTLSProfileSpec resolves a security profile without silently
// substituting Intermediate for an explicitly malformed profile type.
// Empty and nil profiles still resolve to the OpenShift Intermediate default.
func GetStrictTLSProfileSpec(profile *configv1.TLSSecurityProfile) (configv1.TLSProfileSpec, error) {
	spec, err := strictProfileSpec(profile)
	if err != nil {
		return configv1.TLSProfileSpec{}, err
	}
	return *spec, nil
}

func fromProfileStrictSpec(
	ctx context.Context,
	spec *configv1.TLSProfileSpec,
	format VersionFormat,
) (string, string, error) {
	if err := ValidateStrictTLSProfile(*spec); err != nil {
		return "", "", err
	}

	ianaCiphers, _ := cipherSuitesForMinVersion(spec.Ciphers, spec.MinTLSVersion)
	return MinVersionFromSpec(ctx, spec, format), strings.Join(ianaCiphers, ","), nil
}

// CurvePreferencesFromSpec returns comma-separated Go crypto/tls CurveID values
// for the profile's supported groups. Unknown groups are logged and dropped.
func CurvePreferencesFromSpec(ctx context.Context, spec *configv1.TLSProfileSpec) (string, error) {
	if spec == nil || len(spec.Groups) == 0 {
		return "", nil
	}

	l := logf.FromContext(ctx).WithName("CurvePreferencesFromSpec")

	curves, unsupportedGroups := ocpcrypto.TLSGroupsToCurveIDs(spec.Groups)
	for _, group := range unsupportedGroups {
		l.V(1).Info("TLS group is unsupported by Go crypto/tls and was dropped", "group", group)
	}
	if len(curves) == 0 {
		return "", ErrTLSProfileHasNoSupportedGroups
	}

	curveStrings := make([]string, 0, len(curves))
	for _, curve := range curves {
		curveStrings = append(curveStrings, strconv.FormatInt(int64(curve), 10))
	}

	return strings.Join(curveStrings, ","), nil
}

// ValidateStrictTLSProfile checks that Go's TLS implementation can honor the
// strict profile. TLS 1.0/1.1 minimums are honored: they map to Go crypto/tls
// minimum versions (Go 1.22+ defaults the server minimum to TLS 1.2 only when
// tls.Config.MinVersion is unset; an explicit minimum needs no GODEBUG).
// TLS 1.3 cipher lists are not configurable in Go and are not validated;
// TLS 1.2 requires at least one usable cipher suite.
func ValidateStrictTLSProfile(spec configv1.TLSProfileSpec) error {
	if !isKnownTLSVersion(spec.MinTLSVersion) {
		return fmt.Errorf("%w: %q", ErrUnsupportedTLSProfileVersion, spec.MinTLSVersion)
	}

	if spec.MinTLSVersion != configv1.VersionTLS13 {
		if len(spec.Ciphers) == 0 {
			return ErrTLSProfileHasNoCipherSuites
		}

		ianaCiphers, _ := cipherSuitesForMinVersion(spec.Ciphers, spec.MinTLSVersion)
		if len(ianaCiphers) == 0 {
			return ErrTLSProfileHasNoSupportedCipherSuites
		}
	}

	if len(spec.Groups) > 0 {
		curves, _ := ocpcrypto.TLSGroupsToCurveIDs(spec.Groups)
		if len(curves) == 0 {
			return ErrTLSProfileHasNoSupportedGroups
		}
	}

	return nil
}

// ShouldHonorClusterTLSProfile reports whether the APIServer adherence policy
// requires components to use the cluster TLS profile. The empty value is
// OpenShift's NoOpinion policy; other unknown values fail closed as Strict.
func ShouldHonorClusterTLSProfile(adherence configv1.TLSAdherencePolicy) bool {
	switch adherence {
	case configv1.TLSAdherencePolicyNoOpinion, configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly:
		return false
	case configv1.TLSAdherencePolicyStrictAllComponents:
		return true
	default:
		logf.Log.WithName("tls").Info(
			"unknown TLS adherence policy, treating it as StrictAllComponents",
			"adherence", adherence,
		)

		return true
	}
}
