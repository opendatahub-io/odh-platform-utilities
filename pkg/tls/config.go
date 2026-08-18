package tls

import (
	"crypto/tls"

	configv1 "github.com/openshift/api/config/v1"
	ocpcrypto "github.com/openshift/library-go/pkg/crypto"
)

// ConfigFromProfile returns a function that configures a tls.Config from the
// given TLSProfileSpec, along with cipher names the Go crypto/tls stack does
// not support. The returned function is intended for controller-runtime TLSOpts.
//
// CipherSuites are only set when MinVersion is below TLS 1.3, because Go's
// TLS 1.3 implementation does not allow configuring cipher suites.
// See: https://github.com/golang/go/issues/29349
//
// Known Groups are mapped onto CurvePreferences; unknown group names are skipped.
func ConfigFromProfile(profile configv1.TLSProfileSpec) (func(*tls.Config), []string) {
	minVersion, err := ocpcrypto.TLSVersion(string(profile.MinTLSVersion))
	if err != nil {
		minVersion = tls.VersionTLS12
	}

	cipherSuites, unsupportedCiphers := cipherCodes(profile.Ciphers)
	curves := curveIDs(profile.Groups)

	return func(tlsConf *tls.Config) {
		tlsConf.MinVersion = minVersion
		if len(curves) > 0 {
			tlsConf.CurvePreferences = curves
		}

		if minVersion != tls.VersionTLS13 {
			tlsConf.CipherSuites = cipherSuites
		}
	}, unsupportedCiphers
}

func cipherCode(cipher string) uint16 {
	code, err := ocpcrypto.CipherSuite(cipher)
	if err == nil {
		return code
	}

	ianaCiphers := ocpcrypto.OpenSSLToIANACipherSuites([]string{cipher})
	if len(ianaCiphers) != 1 {
		return 0
	}

	code, err = ocpcrypto.CipherSuite(ianaCiphers[0])
	if err == nil {
		return code
	}

	return 0
}

func cipherCodes(ciphers []string) (codes []uint16, unsupportedCiphers []string) {
	for _, cipher := range ciphers {
		code := cipherCode(cipher)
		if code == 0 {
			unsupportedCiphers = append(unsupportedCiphers, cipher)

			continue
		}

		codes = append(codes, code)
	}

	return codes, unsupportedCiphers
}

func curveIDs(groups []configv1.TLSGroup) []tls.CurveID {
	if len(groups) == 0 {
		return nil
	}

	curves := make([]tls.CurveID, 0, len(groups))
	for _, group := range groups {
		id, ok := curveID(group)
		if !ok {
			continue
		}

		curves = append(curves, id)
	}

	return curves
}

func curveID(group configv1.TLSGroup) (tls.CurveID, bool) {
	switch group {
	case configv1.TLSGroupX25519:
		return tls.X25519, true
	case configv1.TLSGroupSecP256r1:
		return tls.CurveP256, true
	case configv1.TLSGroupSecP384r1:
		return tls.CurveP384, true
	case configv1.TLSGroupSecP521r1:
		return tls.CurveP521, true
	case configv1.TLSGroupX25519MLKEM768:
		return tls.X25519MLKEM768, true
	case configv1.TLSGroupSecP256r1MLKEM768, configv1.TLSGroupSecP384r1MLKEM1024:
		// Not in Go 1.25 crypto/tls. Skip rather than fail.
		return 0, false
	default:
		return 0, false
	}
}
