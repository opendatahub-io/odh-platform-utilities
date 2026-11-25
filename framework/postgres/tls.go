package postgres

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

var (
	// ErrInvalidCACert is returned when CA PEM cannot be added to a cert pool.
	ErrInvalidCACert = errors.New("unable to add CA to cert pool")
	// ErrNoServerCertificates is returned when the server sends an empty certificate chain.
	ErrNoServerCertificates = errors.New("server sent no certificates")
)

// WriteCACert writes PEM CA material to a file under dir (or the system temp
// directory when dir is empty) and returns the path plus a cleanup function.
// When no CA file is required for the current sslmode, path is empty and
// cleanup is a no-op.
func (p Params) WriteCACert(dir string) (path string, cleanup func(), err error) {
	noop := func() {}
	if !p.needsCAFile() {
		return "", noop, nil
	}

	if p.CACert == "" {
		return "", noop, fmt.Errorf("%s %q requires key %q: %w",
			SecretKeySSLMode, p.effectiveSSLMode(), SecretKeyCA, ErrCARequired)
	}

	if dir == "" {
		dir = os.TempDir()
	}

	f, err := os.CreateTemp(dir, "odh-pg-ca-*.crt")
	if err != nil {
		return "", noop, fmt.Errorf("create CA temp file: %w", err)
	}

	path = f.Name()

	_, err = f.WriteString(p.CACert)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(path)

		return "", noop, fmt.Errorf("write CA temp file: %w", err)
	}

	err = f.Close()
	if err != nil {
		_ = os.Remove(path)

		return "", noop, fmt.Errorf("close CA temp file: %w", err)
	}

	cleanup = func() { _ = os.Remove(path) }

	return path, cleanup, nil
}

// TLSConfig builds a *tls.Config matching db-operator/libpq: require alone
// encrypts without peer auth; require+CA promotes to verify-ca; verify-ca checks
// chain only; verify-full also checks hostname. Empty sslmode is rejected.
// Returns (nil, nil) for disable.
func (p Params) TLSConfig() (*tls.Config, error) {
	err := p.Validate()
	if err != nil {
		return nil, err
	}

	err = p.requireSSLMode()
	if err != nil {
		return nil, err
	}

	if !p.TLSEnabled() {
		return nil, nil //nolint:nilnil // sslmode=disable is not an error.
	}

	cfg := &tls.Config{
		MinVersion:             tls.VersionTLS12,
		ServerName:             p.Host,
		SessionTicketsDisabled: true,
	}

	if p.CACert != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(p.CACert)) {
			return nil, ErrInvalidCACert
		}

		cfg.RootCAs = pool
		cfg.ClientCAs = pool
	}

	applySSLMode(cfg, p.effectiveSSLMode())

	return cfg, nil
}

func applySSLMode(cfg *tls.Config, mode string) {
	switch mode {
	case SSLModeVerifyCA:
		// Policy exception: libpq verify-ca / db-operator client_tls.go parity.
		// Go has no chain-only mode; InsecureSkipVerify + VerifyPeerCertificate
		// validates the CA chain and intentionally skips hostname .
		// nosemgrep: go-tls-bypass -- verify-ca skips hostname check; verifyCertificateChain does CA validation.
		cfg.InsecureSkipVerify = true
		// Disable session tickets so resumed handshakes cannot skip VerifyPeerCertificate.
		cfg.SessionTicketsDisabled = true
		cfg.VerifyPeerCertificate = verifyCertificateChain(cfg)
	case SSLModeVerifyFull:
		cfg.InsecureSkipVerify = false
		cfg.VerifyPeerCertificate = nil
	case SSLModeRequire, SSLModePrefer, SSLModeAllow:
		// nosemgrep: go-tls-bypass -- intentional libpq require; no peer verify without CA.
		cfg.InsecureSkipVerify = true
		cfg.VerifyPeerCertificate = nil
	}
}

func verifyCertificateChain(tlsConfig *tls.Config) func([][]byte, [][]*x509.Certificate) error {
	return func(certificates [][]byte, _ [][]*x509.Certificate) error {
		if len(certificates) == 0 {
			return ErrNoServerCertificates
		}

		certs := make([]*x509.Certificate, len(certificates))
		for i, asn1Data := range certificates {
			cert, err := x509.ParseCertificate(asn1Data)
			if err != nil {
				return fmt.Errorf("failed to parse certificate from server: %w", err)
			}

			certs[i] = cert
		}

		opts := x509.VerifyOptions{
			Roots:         tlsConfig.RootCAs,
			Intermediates: x509.NewCertPool(),
		}

		for _, cert := range certs[1:] {
			opts.Intermediates.AddCert(cert)
		}

		_, err := certs[0].Verify(opts)
		if err != nil {
			return fmt.Errorf("verify server certificate: %w", err)
		}

		return nil
	}
}
