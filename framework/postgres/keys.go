package postgres

// Secret key names for connection parameters. Prefer bare (undotted) keys over
// PostgreSQL PGXXX env names for envFrom portability: older Kubernetes
// releases dropped keys containing a period; Kubernetes 1.35+ may copy them.
// SecretKeyCA remains "ca.crt" to match the claim-Secret convention.
const (
	SecretKeyHost     = "host"
	SecretKeyPort     = "port"
	SecretKeyUser     = "user"
	SecretKeyPassword = "password"
	SecretKeyDatabase = "dbname"
	SecretKeySchema   = "schema"
	// SecretKeyCA holds PEM-encoded CA data used to verify the server
	// certificate when sslmode is verify-ca or verify-full.
	SecretKeyCA = "ca.crt"
	// SecretKeySSLMode holds the libpq sslmode value (e.g. "disable", "require",
	// "verify-full"). Optional for parse/Libpq/JDBC; callers default it before
	// TLSConfig (Internal: disable, External: require).
	SecretKeySSLMode = "sslmode"

	// DefaultPort is the standard PostgreSQL port, used when port is absent.
	DefaultPort = 5432
	// DefaultAdminDatabase is the default maintenance database name.
	DefaultAdminDatabase = "postgres"

	// SSLModeDisable disables TLS.
	SSLModeDisable = "disable"
	SSLModeAllow   = "allow"
	SSLModePrefer  = "prefer"
	// SSLModeRequire encrypts only (libpq). With ca.crt, promoted to verify-ca.
	SSLModeRequire = "require"
	// SSLModeVerifyCA verifies the server certificate chain but not the hostname.
	SSLModeVerifyCA = "verify-ca"
	// SSLModeVerifyFull verifies the server certificate chain and hostname.
	SSLModeVerifyFull = "verify-full"

	// AnnotationConnectionTypeProtocol is the ODH Connection API annotation
	// that marks a Secret as a postgres connection artifact.
	AnnotationConnectionTypeProtocol = "opendatahub.io/connection-type-protocol"
	// ConnectionTypeProtocolPostgres is the annotation value for PostgreSQL.
	ConnectionTypeProtocolPostgres = "postgres"
)
