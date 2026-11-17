package postgres_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustGenerateTestCA(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
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

func TestWriteCACert_NoopWhenDisable(t *testing.T) {
	t.Parallel()

	p := baseParams()
	path, cleanup, err := p.WriteCACert(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(cleanup)
	assert.Empty(t, path)
}

func TestWriteCACert_WritesPEM(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeVerifyFull
	p.CACert = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"

	path, cleanup, err := p.WriteCACert(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NotEmpty(t, path)

	got, err := os.ReadFile(path) //nolint:gosec // G304: path is a temp file created by WriteCACert.
	require.NoError(t, err)
	assert.Equal(t, p.CACert, string(got))
}

func TestWriteCACert_VerifyWithoutCAErrors(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeVerifyCA
	p.CACert = ""
	_, _, err := p.WriteCACert(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), postgres.SecretKeyCA)
}

func TestTLSConfig_DisableReturnsNil(t *testing.T) {
	t.Parallel()

	cfg, err := baseParams().TLSConfig()
	require.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestTLSConfig_VerifyFull(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeVerifyFull
	p.CACert = mustGenerateTestCA(t)

	cfg, err := p.TLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.False(t, cfg.InsecureSkipVerify)
	assert.Equal(t, p.Host, cfg.ServerName)
	assert.NotNil(t, cfg.RootCAs)
	assert.Nil(t, cfg.VerifyPeerCertificate)
}

func TestTLSConfig_VerifyCA(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeVerifyCA
	p.CACert = mustGenerateTestCA(t)

	cfg, err := p.TLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.InsecureSkipVerify)
	assert.NotNil(t, cfg.VerifyPeerCertificate)
	assert.NotNil(t, cfg.RootCAs)
}

func TestTLSConfig_InvalidPEM(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeVerifyFull
	p.CACert = "not-a-pem"
	_, err := p.TLSConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to add CA to cert pool")
}

func TestTLSConfig_RequireWithoutCA(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeRequire
	cfg, err := p.TLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.InsecureSkipVerify)
	assert.Nil(t, cfg.VerifyPeerCertificate)
}

func TestTLSConfig_RequireWithCAPromotesToVerifyCA(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeRequire
	p.CACert = mustGenerateTestCA(t)

	cfg, err := p.TLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.InsecureSkipVerify)
	assert.NotNil(t, cfg.VerifyPeerCertificate)
	assert.NotNil(t, cfg.RootCAs)
}

func TestTLSConfig_EmptySSLModeRejected(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = ""
	_, err := p.TLSConfig()
	require.Error(t, err)
	require.ErrorIs(t, err, postgres.ErrSSLModeRequired)
	assert.Contains(t, err.Error(), postgres.SecretKeySSLMode)
}
