package tls

import (
	"crypto/tls"
	"fmt"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	commonTLS "github.com/openshift/controller-runtime-common/pkg/tls"
	ocpcrypto "github.com/openshift/library-go/pkg/crypto"
)

// ConfigFromProfile converts a TLS profile to tls.Config options, along with
// cipher and group names the Go crypto/tls stack cannot use for the configured
// minimum version. The returned function is intended for controller-runtime
// TLSOpts.
//
// It is the legacy compatibility entrypoint: it does not validate the profile
// (invalid minimum versions are floored to TLS 1.2; unusable ciphers and groups
// are filtered and reported in the second return value) and it cannot report
// build failures. New callers should use ConfigFromProfileWithError, which
// returns an error when no configurer can be built at all; callers must treat
// that as fail-closed rather than silently downgrading to another profile.
//
// CipherSuites are only set when MinVersion is below TLS 1.3, because Go's
// TLS 1.3 implementation does not allow configuring cipher suites.
// See: https://github.com/golang/go/issues/29349
//
// Known Groups are mapped onto CurvePreferences; unknown group names are skipped.
func ConfigFromProfile(profile configv1.TLSProfileSpec) (func(*tls.Config), []string) {
	tlsConfig, unsupported, _ := ConfigFromProfileWithError(profile)
	return tlsConfig, unsupported
}

// ConfigFromProfileWithError converts a TLS profile to tls.Config options,
// along with cipher and group names the Go crypto/tls stack cannot use for
// the configured minimum version, and an error when no configurer can be
// built at all.
//
// Like the legacy ConfigFromProfile it does not validate the profile: invalid
// minimum versions are floored to TLS 1.2, and unusable ciphers and groups are
// filtered and reported in the second return value. The error cannot result
// from user-supplied profile values; callers must treat it as fail-closed.
func ConfigFromProfileWithError(profile configv1.TLSProfileSpec) (func(*tls.Config), []string, error) {
	_, err := ocpcrypto.TLSVersion(string(profile.MinTLSVersion))
	if err != nil {
		profile.MinTLSVersion = configv1.VersionTLS12
	}

	var unsupportedTLS13 []string
	if profile.MinTLSVersion != configv1.VersionTLS13 {
		profile.Ciphers, unsupportedTLS13 = filterTLS13CipherSuites(profile.Ciphers)
	}

	// RC4 is filtered before delegating to controller-runtime-common, whose
	// pinned version still maps and applies RC4 names.
	var unsupportedRC4 []string
	profile.Ciphers, unsupportedRC4 = filterRC4CipherSuites(profile.Ciphers)

	tlsConfig, unsupported, err := commonTLS.NewTLSConfigFromProfile(profile)
	if err != nil {
		return nil, nil, fmt.Errorf("building TLS config from profile: %w", err)
	}
	return tlsConfig, append(append(unsupported, unsupportedTLS13...), unsupportedRC4...), nil
}

// ConfigFromProfileStrict validates a TLS profile before converting it to
// tls.Config options. Unlike ConfigFromProfile, malformed versions and
// unusable profiles return an error instead of being normalized or applied.
func ConfigFromProfileStrict(profile configv1.TLSProfileSpec) (func(*tls.Config), []string, error) {
	if err := ValidateStrictTLSProfile(profile); err != nil {
		return nil, nil, err
	}
	return ConfigFromProfileWithError(profile)
}

func cipherCode(cipher string) uint16 {
	code, ok := resolveCipherCode(cipher)
	if !ok {
		return 0
	}
	return maskRC4(code)
}

func resolveCipherCode(cipher string) (uint16, bool) {
	if code, err := ocpcrypto.CipherSuite(cipher); err == nil {
		return code, true
	}

	ianaCiphers := ocpcrypto.OpenSSLToIANACipherSuites([]string{cipher})
	if len(ianaCiphers) != 1 {
		return 0, false
	}

	code, err := ocpcrypto.CipherSuite(ianaCiphers[0])
	if err != nil {
		return 0, false
	}
	return code, true
}

func maskRC4(code uint16) uint16 {
	if isRC4CipherID(code) {
		return 0
	}
	return code
}

func filterRC4CipherSuites(ciphers []string) (filtered, removed []string) {
	for _, cipher := range ciphers {
		if isRC4CipherName(cipher) {
			removed = append(removed, cipher)
			continue
		}
		filtered = append(filtered, cipher)
	}

	return filtered, removed
}

func isRC4CipherName(cipher string) bool {
	code, ok := resolveCipherCode(cipher)
	if !ok {
		return false
	}

	return isRC4CipherID(code)
}

func isRC4CipherID(code uint16) bool {
	return strings.Contains(tls.CipherSuiteName(code), "RC4")
}

func filterTLS13CipherSuites(ciphers []string) (filtered, removed []string) {
	for _, cipher := range ciphers {
		code := cipherCode(cipher)
		if isTLS13CipherSuiteID(code) {
			removed = append(removed, cipher)
			continue
		}
		filtered = append(filtered, cipher)
	}

	return filtered, removed
}

// TLS 1.3 cipher suite IDs occupy the 0x1300-0x13ff range.
func isTLS13CipherSuiteID(code uint16) bool {
	return code&0xff00 == 0x1300
}
