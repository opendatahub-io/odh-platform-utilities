package postgres_test

import (
	"testing"

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
