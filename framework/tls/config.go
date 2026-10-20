package tls

import (
	"crypto/tls"

	configv1 "github.com/openshift/api/config/v1"
	commonTLS "github.com/openshift/controller-runtime-common/pkg/tls"
	ocpcrypto "github.com/openshift/library-go/pkg/crypto"
)

// ConfigFromProfile returns a function that configures a tls.Config from the
// given OpenShift TLSProfileSpec, along with cipher and group names the Go
// crypto/tls stack cannot use for the configured minimum version. The returned
// function is intended for controller-runtime TLSOpts.
//
// CipherSuites are only set when MinVersion is below TLS 1.3, because Go's
// TLS 1.3 implementation does not allow configuring cipher suites.
// See: https://github.com/golang/go/issues/29349
//
// Known Groups are mapped onto CurvePreferences; unknown group names are skipped.
func ConfigFromProfile(profile configv1.TLSProfileSpec) (func(*tls.Config), []string) {
	_, err := ocpcrypto.TLSVersion(string(profile.MinTLSVersion))
	if err != nil {
		profile.MinTLSVersion = configv1.VersionTLS12
	}

	var unsupportedTLS13 []string
	if profile.MinTLSVersion != configv1.VersionTLS13 {
		profile.Ciphers, unsupportedTLS13 = filterTLS13CipherSuites(profile.Ciphers)
	}

	// Ciphers no OpenShift profile can name (RC4 suites, per RFC 7465) are
	// filtered before delegating to controller-runtime-common, whose pinned
	// version still maps and applies RC4 names.
	var unsupportedUnnameable []string
	profile.Ciphers, unsupportedUnnameable = filterUnnameableCipherSuites(profile.Ciphers)

	tlsConfig, unsupported := commonTLS.NewTLSConfigFromProfile(profile)
	return tlsConfig, append(append(unsupported, unsupportedTLS13...), unsupportedUnnameable...)
}

// ConfigFromProfileStrict validates a TLS profile before converting it to
// tls.Config options. Unlike ConfigFromProfile, malformed versions and
// unusable profiles return an error instead of being normalized or applied.
func ConfigFromProfileStrict(profile configv1.TLSProfileSpec) (func(*tls.Config), []string, error) {
	if err := ValidateStrictTLSProfile(profile); err != nil {
		return nil, nil, err
	}
	tlsConfig, unsupported := ConfigFromProfile(profile)
	return tlsConfig, unsupported, nil
}

// unnameableInsecureCipherIDs is the set of cipher suite IDs from Go's
// insecure table that no OpenShift TLS profile can name. RC4 suites are
// negotiable by the Go runtime (they appear in tls.InsecureCipherSuites)
// but RFC 7465 prohibits them and no profile contains one, so profiles
// naming such a suite are treated as unsupported rather than honored.
var unnameableInsecureCipherIDs = func() map[uint16]bool {
	profileNames := make(map[string]bool)
	for _, spec := range configv1.TLSProfiles {
		for _, name := range ocpcrypto.OpenSSLToIANACipherSuites(spec.Ciphers) {
			profileNames[name] = true
		}
	}
	ids := make(map[uint16]bool)
	for _, suite := range tls.InsecureCipherSuites() {
		if !profileNames[suite.Name] {
			ids[suite.ID] = true
		}
	}
	return ids
}()

func cipherCode(cipher string) uint16 {
	code, ok := resolveCipherCode(cipher)
	if !ok {
		return 0
	}
	return maskUnnameableInsecure(code)
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

// maskUnnameableInsecure maps IDs of insecure suites no OpenShift profile
// can name to 0, so those names are handled like unsupported ciphers
// everywhere cipherCode is used.
func maskUnnameableInsecure(code uint16) uint16 {
	if unnameableInsecureCipherIDs[code] {
		return 0
	}
	return code
}

// filterUnnameableCipherSuites removes suites no OpenShift profile can name
// by their resolved IDs, in IANA or OpenSSL name form. It resolves names
// directly instead of via cipherCode, which masks those IDs to 0.
func filterUnnameableCipherSuites(ciphers []string) (filtered, removed []string) {
	for _, cipher := range ciphers {
		if isUnnameableInsecureCipher(cipher) {
			removed = append(removed, cipher)
			continue
		}
		filtered = append(filtered, cipher)
	}

	return filtered, removed
}

func isUnnameableInsecureCipher(cipher string) bool {
	code, ok := resolveCipherCode(cipher)
	if !ok {
		return false
	}

	return unnameableInsecureCipherIDs[code]
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
