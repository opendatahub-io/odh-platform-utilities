package postgres_test

import (
	"testing"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/postgres"
	"github.com/stretchr/testify/assert"
)

func TestSecretKeyConstantsMatchClaimContract(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "host", postgres.SecretKeyHost)
	assert.Equal(t, "port", postgres.SecretKeyPort)
	assert.Equal(t, "user", postgres.SecretKeyUser)
	assert.Equal(t, "password", postgres.SecretKeyPassword)
	assert.Equal(t, "dbname", postgres.SecretKeyDatabase)
	assert.Equal(t, "schema", postgres.SecretKeySchema)
	assert.Equal(t, "ca.crt", postgres.SecretKeyCA)
	assert.Equal(t, "sslmode", postgres.SecretKeySSLMode)
}

func TestSSLModeConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "disable", postgres.SSLModeDisable)
	assert.Equal(t, "require", postgres.SSLModeRequire)
	assert.Equal(t, "verify-ca", postgres.SSLModeVerifyCA)
	assert.Equal(t, "verify-full", postgres.SSLModeVerifyFull)
}

func TestDefaultPortAndConnectionAnnotation(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 5432, postgres.DefaultPort)
	assert.Equal(t, "postgres", postgres.DefaultAdminDatabase)
	assert.Equal(t, "opendatahub.io/connection-type-protocol", postgres.AnnotationConnectionTypeProtocol)
	assert.Equal(t, "postgres", postgres.ConnectionTypeProtocolPostgres)
}
