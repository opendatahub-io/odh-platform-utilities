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

	"github.com/opendatahub-io/odh-platform-utilities/framework/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  string
}

func mustGenerateTestCA(t *testing.T) string {
	t.Helper()
	return mustGenerateCA(t).pem
}

func mustGenerateCA(t *testing.T) testCA {
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

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	return testCA{
		cert: cert,
		key:  key,
		pem:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
	}
}

func mustIssueLeaf(t *testing.T, ca testCA, dnsNames []string, notBefore, notAfter time.Time) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "leaf"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	require.NoError(t, err)

	return der
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

func TestTLSConfig_VerifyCA_VerifyPeerCertificate(t *testing.T) {
	t.Parallel()

	ca := mustGenerateCA(t)
	otherCA := mustGenerateCA(t)

	p := baseParams()
	p.SSLMode = postgres.SSLModeVerifyCA
	p.CACert = ca.pem

	cfg, err := p.TLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.VerifyPeerCertificate)

	now := time.Now()
	validLeaf := mustIssueLeaf(t, ca, []string{p.Host}, now.Add(-time.Hour), now.Add(24*time.Hour))
	foreignLeaf := mustIssueLeaf(t, otherCA, []string{p.Host}, now.Add(-time.Hour), now.Add(24*time.Hour))
	expiredLeaf := mustIssueLeaf(t, ca, []string{p.Host}, now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	mismatchedHostLeaf := mustIssueLeaf(t, ca, []string{"other.example.test"}, now.Add(-time.Hour), now.Add(24*time.Hour))

	t.Run("trusted leaf", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, cfg.VerifyPeerCertificate([][]byte{validLeaf}, nil))
	})
	t.Run("foreign CA", func(t *testing.T) {
		t.Parallel()
		require.Error(t, cfg.VerifyPeerCertificate([][]byte{foreignLeaf}, nil))
	})
	t.Run("expired leaf", func(t *testing.T) {
		t.Parallel()
		require.Error(t, cfg.VerifyPeerCertificate([][]byte{expiredLeaf}, nil))
	})
	t.Run("empty chain", func(t *testing.T) {
		t.Parallel()

		err := cfg.VerifyPeerCertificate(nil, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, postgres.ErrNoServerCertificates)
	})
	t.Run("hostname mismatch allowed", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, cfg.VerifyPeerCertificate([][]byte{mismatchedHostLeaf}, nil))
	})
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

func TestTLSConfig_PreferAllowRejected(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{postgres.SSLModePrefer, postgres.SSLModeAllow} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			p := baseParams()
			p.SSLMode = mode
			cfg, err := p.TLSConfig()
			require.Error(t, err)
			require.ErrorIs(t, err, postgres.ErrUnsupportedSSLMode)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), mode)
		})
	}
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
