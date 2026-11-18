package postgres_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	fwkpostgres "github.com/opendatahub-io/odh-platform-utilities/framework/postgres"
	pkgpostgres "github.com/opendatahub-io/odh-platform-utilities/pkg/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnect_ValidateBeforeDial(t *testing.T) {
	t.Parallel()

	p := pkgpostgres.Params{
		Port:     pkgpostgres.DefaultPort,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
		SSLMode:  pkgpostgres.SSLModeDisable,
	}
	_, err := fwkpostgres.Connect(t.Context(), p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), pkgpostgres.SecretKeyHost)
}

func TestConnect_EmptySSLModeRejected(t *testing.T) {
	t.Parallel()

	p := pkgpostgres.Params{
		Host:     "db.example.test",
		Port:     pkgpostgres.DefaultPort,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
	}
	_, err := fwkpostgres.Connect(t.Context(), p)
	require.Error(t, err)
	require.ErrorIs(t, err, pkgpostgres.ErrSSLModeRequired)
}

func TestConnect_InvalidCACertRejectedBeforeDial(t *testing.T) {
	t.Parallel()

	p := pkgpostgres.Params{
		Host:     "db.example.test",
		Port:     pkgpostgres.DefaultPort,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
		SSLMode:  pkgpostgres.SSLModeVerifyFull,
		CACert:   "not-a-pem",
	}
	_, err := fwkpostgres.Connect(t.Context(), p)
	require.Error(t, err)
	require.ErrorIs(t, err, pkgpostgres.ErrInvalidCACert)
}

func TestConnect_IgnoresMissingPGSSLROOTCERT(t *testing.T) {
	// Not parallel: t.Setenv affects process env for this test.
	t.Setenv("PGSSLROOTCERT", "/nonexistent/odh-claim-ca.crt")

	p := pkgpostgres.Params{
		Host:     "db.example.test",
		Port:     pkgpostgres.DefaultPort,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
		SSLMode:  pkgpostgres.SSLModeVerifyFull,
		CACert:   mustConnectTestCA(t),
	}
	_, err := fwkpostgres.Connect(t.Context(), p)
	require.Error(t, err)
	// ParseConfig must not fail on the missing ambient file; claim CA is used instead.
	assert.NotContains(t, err.Error(), "unable to read CA file")
	assert.True(t, strings.Contains(err.Error(), "connect") || strings.Contains(err.Error(), "dial"),
		"expected dial/connect failure, got: %v", err)
}

func TestConnect_InvalidClaimCAStillRejectedWithPGSSLROOTCERT(t *testing.T) {
	t.Setenv("PGSSLROOTCERT", "/nonexistent/odh-claim-ca.crt")

	p := pkgpostgres.Params{
		Host:     "db.example.test",
		Port:     pkgpostgres.DefaultPort,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
		SSLMode:  pkgpostgres.SSLModeVerifyFull,
		CACert:   "not-a-pem",
	}
	_, err := fwkpostgres.Connect(t.Context(), p)
	require.Error(t, err)
	require.ErrorIs(t, err, pkgpostgres.ErrInvalidCACert)
	assert.NotContains(t, err.Error(), "unable to read CA file")
}

func mustConnectTestCA(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "connect-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
