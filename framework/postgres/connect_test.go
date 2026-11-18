package postgres_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/opendatahub-io/odh-platform-utilities/framework/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnect_ValidateBeforeDial(t *testing.T) {
	t.Parallel()

	p := postgres.Params{
		Port:     postgres.DefaultPort,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
		SSLMode:  postgres.SSLModeDisable,
	}
	_, err := postgres.Connect(t.Context(), p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), postgres.SecretKeyHost)
}

func TestConnect_EmptySSLModeRejected(t *testing.T) {
	t.Parallel()

	p := postgres.Params{
		Host:     "db.example.test",
		Port:     postgres.DefaultPort,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
	}
	_, err := postgres.Connect(t.Context(), p)
	require.Error(t, err)
	require.ErrorIs(t, err, postgres.ErrSSLModeRequired)
}

func TestConnect_InvalidCACertRejectedBeforeDial(t *testing.T) {
	t.Parallel()

	p := postgres.Params{
		Host:     "db.example.test",
		Port:     postgres.DefaultPort,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
		SSLMode:  postgres.SSLModeVerifyFull,
		CACert:   "not-a-pem",
	}
	_, err := postgres.Connect(t.Context(), p)
	require.Error(t, err)
	require.ErrorIs(t, err, postgres.ErrInvalidCACert)
}

func TestConnect_IgnoresMissingPGSSLROOTCERT(t *testing.T) {
	// Not parallel: t.Setenv affects process env for this test.
	t.Setenv("PGSSLROOTCERT", "/nonexistent/odh-claim-ca.crt")

	p := postgres.Params{
		// Loopback + closed port: prove a dial attempt without waiting on DNS.
		Host:     "127.0.0.1",
		Port:     1,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
		SSLMode:  postgres.SSLModeVerifyFull,
		CACert:   mustConnectTestCA(t),
	}
	_, err := postgres.Connect(t.Context(), p)
	require.Error(t, err)
	// ParseConfig must not fail on the missing ambient file; claim CA is used instead.
	assert.NotContains(t, err.Error(), "unable to read CA file")
	// Substring "connect" also matches "parse connection config"; require a
	// connection-stage error so parse failures cannot pass this assertion.
	var connectErr *pgconn.ConnectError
	require.ErrorAs(t, err, &connectErr)
}

func TestConnect_InvalidClaimCAStillRejectedWithPGSSLROOTCERT(t *testing.T) {
	t.Setenv("PGSSLROOTCERT", "/nonexistent/odh-claim-ca.crt")

	p := postgres.Params{
		Host:     "db.example.test",
		Port:     postgres.DefaultPort,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
		SSLMode:  postgres.SSLModeVerifyFull,
		CACert:   "not-a-pem",
	}
	_, err := postgres.Connect(t.Context(), p)
	require.Error(t, err)
	require.ErrorIs(t, err, postgres.ErrInvalidCACert)
	assert.NotContains(t, err.Error(), "unable to read CA file")
}

func TestConnect_IgnoresMissingPGSSLCERT(t *testing.T) {
	// Not parallel: t.Setenv affects process env for this test.
	t.Setenv("PGSSLCERT", "/nonexistent/odh-client.crt")
	t.Setenv("PGSSLKEY", "/nonexistent/odh-client.key")

	p := postgres.Params{
		Host:     "127.0.0.1",
		Port:     1,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
		SSLMode:  postgres.SSLModeVerifyFull,
		CACert:   mustConnectTestCA(t),
	}
	_, err := postgres.Connect(t.Context(), p)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "unable to read")
	var connectErr *pgconn.ConnectError
	require.ErrorAs(t, err, &connectErr)
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
