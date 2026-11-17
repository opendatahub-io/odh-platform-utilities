package postgres_test

import (
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseParams() postgres.Params {
	return postgres.Params{
		Host:     "db.example.test",
		Port:     5432,
		User:     "app",
		Password: "s3cret",
		DBName:   "appdb",
		SSLMode:  postgres.SSLModeDisable,
	}
}

func TestLibpq_IncludesSchemaSearchPath(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.Schema = "claim_abc"
	s, err := p.Libpq("")
	require.NoError(t, err)
	assert.Contains(t, s, "options='-csearch_path=claim_abc'")
	assert.Contains(t, s, "host='db.example.test'")
	assert.Contains(t, s, "dbname='appdb'")
	assert.Contains(t, s, "sslmode='disable'")
}

func TestLibpq_EscapesQuotesAndBackslashes(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.Password = `p'ass\word`
	s, err := p.Libpq("")
	require.NoError(t, err)
	assert.Contains(t, s, `password='p\'ass\\word'`)
}

func TestLibpq_SSLRootCertForVerifyModes(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeVerifyFull
	p.CACert = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"

	s, err := p.Libpq("/tmp/ca.crt")
	require.NoError(t, err)
	assert.Contains(t, s, "sslmode='verify-full'")
	assert.Contains(t, s, "sslrootcert='/tmp/ca.crt'")

	sDisable, err := baseParams().Libpq("/tmp/ca.crt")
	require.NoError(t, err)
	assert.NotContains(t, sDisable, "sslrootcert=")
}

func TestLibpq_OmitsSSLRootCertWhenPathEmpty(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeVerifyCA
	p.CACert = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	s, err := p.Libpq("")
	require.NoError(t, err)
	assert.NotContains(t, s, "sslrootcert=")
}

func TestJDBC_IncludesCurrentSchema(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.Schema = "claim_abc"
	s, err := p.JDBC("")
	require.NoError(t, err)
	assert.Contains(t, s, "jdbc:postgresql://db.example.test:5432/appdb?")
	assert.Contains(t, s, "currentSchema=claim_abc")
	assert.Contains(t, s, "user=app")
	assert.Contains(t, s, "sslmode=disable")
}

func TestJDBC_SSLRootCertForVerifyModes(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeVerifyCA
	p.CACert = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	s, err := p.JDBC("/etc/db-trust/ca.crt")
	require.NoError(t, err)
	assert.Contains(t, s, "sslrootcert=")
	assert.Contains(t, s, "%2Fetc%2Fdb-trust%2Fca.crt") // url.Values encodes the path
}

func TestRequireWithCA_PromotesAcrossAPIs(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeRequire
	p.CACert = mustGenerateTestCA(t)

	libpq, err := p.Libpq("/tmp/ca.crt")
	require.NoError(t, err)
	assert.Contains(t, libpq, "sslmode='verify-ca'")
	assert.Contains(t, libpq, "sslrootcert='/tmp/ca.crt'")

	jdbc, err := p.JDBC("/tmp/ca.crt")
	require.NoError(t, err)
	assert.Contains(t, jdbc, "sslmode=verify-ca")
	assert.Contains(t, jdbc, "sslrootcert=")

	path, cleanup, err := p.WriteCACert(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(cleanup)
	assert.NotEmpty(t, path)

	cfg, err := p.TLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.True(t, cfg.InsecureSkipVerify)
	assert.NotNil(t, cfg.VerifyPeerCertificate)
	assert.NotNil(t, cfg.RootCAs)
}

func TestRequireWithoutCA_DoesNotAttachRootCert(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.SSLMode = postgres.SSLModeRequire

	libpq, err := p.Libpq("/tmp/ca.crt")
	require.NoError(t, err)
	assert.Contains(t, libpq, "sslmode='require'")
	assert.NotContains(t, libpq, "sslrootcert=")

	path, cleanup, err := p.WriteCACert(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(cleanup)
	assert.Empty(t, path)
}

func TestConnectionString_UnsupportedDriver(t *testing.T) {
	t.Parallel()

	_, err := baseParams().ConnectionString("odbc", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported driver")
}

func TestConnectionString_ValidatesParams(t *testing.T) {
	t.Parallel()

	p := baseParams()
	p.Host = ""
	_, err := p.Libpq("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), postgres.SecretKeyHost)
}
