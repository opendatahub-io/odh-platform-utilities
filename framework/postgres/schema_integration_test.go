//go:build integration

// Schema placement integration test. Requires Docker or Podman.
//
//	go test ./postgres/... -tags integration -race -count=1 -run TestSchemaPlacement
package postgres_test

import (
	"context"
	"net"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	fwkpostgres "github.com/opendatahub-io/odh-platform-utilities/framework/postgres"
	pkgpostgres "github.com/opendatahub-io/odh-platform-utilities/pkg/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSchemaPlacement_ViaAccessorConnect(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	host, port := startPostgres(t)

	adminDSN := "postgres://app:s3cret@" + net.JoinHostPort(host, strconv.Itoa(port)) + "/appdb?sslmode=disable"
	admin, err := pgx.Connect(ctx, adminDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	const claimSchema = "claim_test"

	_, err = admin.Exec(ctx, `CREATE SCHEMA `+claimSchema)
	require.NoError(t, err)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				pkgpostgres.AnnotationConnectionTypeProtocol: pkgpostgres.ConnectionTypeProtocolPostgres,
			},
		},
		Data: map[string][]byte{
			pkgpostgres.SecretKeyHost:     []byte(host),
			pkgpostgres.SecretKeyPort:     []byte(strconv.Itoa(port)),
			pkgpostgres.SecretKeyUser:     []byte("app"),
			pkgpostgres.SecretKeyPassword: []byte("s3cret"),
			pkgpostgres.SecretKeyDatabase: []byte("appdb"),
			pkgpostgres.SecretKeySchema:   []byte(claimSchema),
			pkgpostgres.SecretKeySSLMode:  []byte(pkgpostgres.SSLModeDisable),
		},
	}
	params, err := pkgpostgres.ParseClaimSecret(secret)
	require.NoError(t, err)

	conn, err := fwkpostgres.Connect(ctx, params)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	_, err = conn.Exec(ctx, `CREATE TABLE schema_probe (id int)`)
	require.NoError(t, err)

	var inClaim, inPublic bool
	err = admin.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = 'schema_probe'
		)`, claimSchema).Scan(&inClaim)
	require.NoError(t, err)
	err = admin.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'schema_probe'
		)`).Scan(&inPublic)
	require.NoError(t, err)

	assert.True(t, inClaim, "table must exist in claimed schema %q", claimSchema)
	assert.False(t, inPublic, "table must not exist in public")
}

func startPostgres(t *testing.T) (host string, port int) {
	t.Helper()

	ctx := t.Context()
	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("appdb"),
		tcpostgres.WithUsername("app"),
		tcpostgres.WithPassword("s3cret"),
		tcpostgres.BasicWaitStrategies(), // log readiness + published 5432 listening
	)
	// Register before asserting so a readiness timeout still tears the container down.
	testcontainers.CleanupContainer(t, container)
	require.NoError(t, err)

	host, err = container.Host(ctx)
	require.NoError(t, err)
	mappedPort, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)
	port, err = strconv.Atoi(mappedPort.Port())
	require.NoError(t, err)

	return host, port
}
