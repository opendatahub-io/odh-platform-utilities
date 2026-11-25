package postgres_test

import (
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/framework/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validSecretData() map[string][]byte {
	return map[string][]byte{
		postgres.SecretKeyHost:     []byte("db.example.test"),
		postgres.SecretKeyPort:     []byte("5433"),
		postgres.SecretKeyUser:     []byte("app"),
		postgres.SecretKeyPassword: []byte("s3cret"),
		postgres.SecretKeyDatabase: []byte("appdb"),
		postgres.SecretKeySSLMode:  []byte(postgres.SSLModeDisable),
		postgres.SecretKeySchema:   []byte("claim_abc"),
	}
}

func TestParseSecretData_Success(t *testing.T) {
	t.Parallel()

	p, err := postgres.ParseSecretData(validSecretData())
	require.NoError(t, err)
	assert.Equal(t, "db.example.test", p.Host)
	assert.Equal(t, 5433, p.Port)
	assert.Equal(t, "app", p.User)
	assert.Equal(t, "s3cret", p.Password)
	assert.Equal(t, "appdb", p.DBName)
	assert.Equal(t, "claim_abc", p.Schema)
	assert.Equal(t, postgres.SSLModeDisable, p.SSLMode)
}

func TestParseSecretData_DefaultPort(t *testing.T) {
	t.Parallel()

	data := validSecretData()
	delete(data, postgres.SecretKeyPort)
	p, err := postgres.ParseSecretData(data)
	require.NoError(t, err)
	assert.Equal(t, postgres.DefaultPort, p.Port)
}

func TestParseSecretData_NilData(t *testing.T) {
	t.Parallel()

	_, err := postgres.ParseSecretData(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestParseSecretData_MissingKeys(t *testing.T) {
	t.Parallel()

	for _, key := range []string{
		postgres.SecretKeyHost,
		postgres.SecretKeyUser,
		postgres.SecretKeyPassword,
		postgres.SecretKeyDatabase,
	} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			data := validSecretData()
			delete(data, key)
			_, err := postgres.ParseSecretData(data)
			require.Error(t, err)
			assert.Contains(t, err.Error(), key)
			assert.NotContains(t, err.Error(), "s3cret")
		})
	}
}

func TestParseSecretData_NULRejected(t *testing.T) {
	t.Parallel()

	for _, key := range []string{
		postgres.SecretKeyHost,
		postgres.SecretKeyUser,
		postgres.SecretKeyPassword,
		postgres.SecretKeyDatabase,
	} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			data := validSecretData()
			data[key] = []byte("ok\x00more")
			_, err := postgres.ParseSecretData(data)
			require.Error(t, err)
			require.ErrorIs(t, err, postgres.ErrInvalidValue)
			assert.Contains(t, err.Error(), key)
			assert.NotContains(t, err.Error(), "s3cret")
			assert.NotContains(t, err.Error(), "\x00")
		})
	}
}

func TestParseSecretData_QuotesAndBackslashesAllowed(t *testing.T) {
	t.Parallel()

	data := validSecretData()
	data[postgres.SecretKeyPassword] = []byte(`p'ass\word`)
	data[postgres.SecretKeyUser] = []byte(`u'ser`)
	p, err := postgres.ParseSecretData(data)
	require.NoError(t, err)
	assert.Equal(t, `p'ass\word`, p.Password)
	assert.Equal(t, `u'ser`, p.User)
}

func TestParseSecretData_InvalidPort(t *testing.T) {
	t.Parallel()

	data := validSecretData()
	data[postgres.SecretKeyPort] = []byte("notanumber")
	_, err := postgres.ParseSecretData(data)
	require.Error(t, err)
	require.ErrorIs(t, err, postgres.ErrInvalidPort)
	assert.Contains(t, err.Error(), postgres.SecretKeyPort)
	assert.NotContains(t, err.Error(), "s3cret")
}

func TestParseSecretData_PortOverflow(t *testing.T) {
	t.Parallel()

	data := validSecretData()
	data[postgres.SecretKeyPort] = []byte("99999999999999999999")
	_, err := postgres.ParseSecretData(data)
	require.Error(t, err)
	require.ErrorIs(t, err, postgres.ErrInvalidPort)
	assert.Contains(t, err.Error(), postgres.SecretKeyPort)
}

func TestParseSecretData_PortOutOfRange(t *testing.T) {
	t.Parallel()

	data := validSecretData()
	data[postgres.SecretKeyPort] = []byte("0")
	_, err := postgres.ParseSecretData(data)
	require.Error(t, err)
	require.ErrorIs(t, err, postgres.ErrInvalidPort)
	assert.Contains(t, err.Error(), postgres.SecretKeyPort)
}

func TestParseSecretData_VerifyRequiresCA(t *testing.T) {
	t.Parallel()

	data := validSecretData()
	data[postgres.SecretKeySSLMode] = []byte(postgres.SSLModeVerifyFull)
	delete(data, postgres.SecretKeyCA)
	_, err := postgres.ParseSecretData(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), postgres.SecretKeyCA)
	assert.NotContains(t, err.Error(), "s3cret")
}

func TestParseSecretData_VerifyWithCA(t *testing.T) {
	t.Parallel()

	data := validSecretData()
	data[postgres.SecretKeySSLMode] = []byte(postgres.SSLModeVerifyFull)
	data[postgres.SecretKeyCA] = []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n")
	p, err := postgres.ParseSecretData(data)
	require.NoError(t, err)
	assert.True(t, p.TLSEnabled())
	assert.True(t, p.TLSReady())
}

func TestParseSecretData_UnsupportedSSLMode(t *testing.T) {
	t.Parallel()

	data := validSecretData()
	data[postgres.SecretKeySSLMode] = []byte("not-a-mode")
	_, err := postgres.ParseSecretData(data)
	require.Error(t, err)
	require.ErrorIs(t, err, postgres.ErrUnsupportedSSLMode)
	assert.Contains(t, err.Error(), postgres.SecretKeySSLMode)
}

func TestParseSecretData_PreferAndAllowAccepted(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{postgres.SSLModePrefer, postgres.SSLModeAllow} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			data := validSecretData()
			data[postgres.SecretKeySSLMode] = []byte(mode)
			p, err := postgres.ParseSecretData(data)
			require.NoError(t, err)
			assert.Equal(t, mode, p.SSLMode)
			assert.True(t, p.TLSEnabled())
			assert.True(t, p.TLSReady())
		})
	}
}

func TestParseSecretData_InvalidSchemaRejected(t *testing.T) {
	t.Parallel()

	for _, schema := range []string{
		"claim_a -crole=admin",
		"claim_a,public",
		"claim-abc",
	} {
		t.Run(schema, func(t *testing.T) {
			t.Parallel()

			data := validSecretData()
			data[postgres.SecretKeySchema] = []byte(schema)
			_, err := postgres.ParseSecretData(data)
			require.Error(t, err)
			require.ErrorIs(t, err, postgres.ErrInvalidSchema)
			assert.Contains(t, err.Error(), postgres.SecretKeySchema)
			assert.NotContains(t, err.Error(), "s3cret")
		})
	}
}

func TestParseSecretData_EmptySchemaAllowed(t *testing.T) {
	t.Parallel()

	data := validSecretData()
	delete(data, postgres.SecretKeySchema)
	p, err := postgres.ParseSecretData(data)
	require.NoError(t, err)
	assert.Empty(t, p.Schema)
}

func TestParseSecretData_InvalidHostRejected(t *testing.T) {
	t.Parallel()

	for _, host := range []string{
		"a.example.test,b.example.test",
		"db.example.test/x?foo=bar&",
		"db.example.test/extra",
		"db.example.test?sslmode=disable",
		"user@db.example.test",
		"db.example.test:5433",
		"127.0.0.1:5432",
		"[::1]:5432",
		"[::1]",
		"[db.example.test]",
	} {
		t.Run(host, func(t *testing.T) {
			t.Parallel()

			data := validSecretData()
			data[postgres.SecretKeyHost] = []byte(host)
			_, err := postgres.ParseSecretData(data)
			require.Error(t, err)
			require.ErrorIs(t, err, postgres.ErrInvalidHost)
			assert.Contains(t, err.Error(), postgres.SecretKeyHost)
			assert.NotContains(t, err.Error(), "s3cret")
		})
	}
}

func TestParseSecretData_IPv6HostAccepted(t *testing.T) {
	t.Parallel()

	data := validSecretData()
	data[postgres.SecretKeyHost] = []byte("::1")
	p, err := postgres.ParseSecretData(data)
	require.NoError(t, err)
	assert.Equal(t, "::1", p.Host)
}

func TestParseClaimSecret_Annotation(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				postgres.AnnotationConnectionTypeProtocol: postgres.ConnectionTypeProtocolPostgres,
			},
		},
		Data: validSecretData(),
	}
	_, err := postgres.ParseClaimSecret(secret)
	require.NoError(t, err)

	secret.Annotations[postgres.AnnotationConnectionTypeProtocol] = "mysql"
	_, err = postgres.ParseClaimSecret(secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), postgres.AnnotationConnectionTypeProtocol)
}

func TestParseClaimSecret_AbsentAnnotationAllowed(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{Data: validSecretData()}
	_, err := postgres.ParseClaimSecret(secret)
	require.NoError(t, err)
}

func TestParseClaimSecret_IgnoresCredentialStamp(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				postgres.AnnotationConnectionTypeProtocol:           postgres.ConnectionTypeProtocolPostgres,
				"db.infrastructure.opendatahub.io/credential-stamp": "abc",
			},
		},
		Data: validSecretData(),
	}
	_, err := postgres.ParseClaimSecret(secret)
	require.NoError(t, err)
}

func TestParseClaimSecret_Nil(t *testing.T) {
	t.Parallel()

	_, err := postgres.ParseClaimSecret(nil)
	require.Error(t, err)
}
