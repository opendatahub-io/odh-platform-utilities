package postgres

// Secret key names for connection parameters. Bare (undotted) keys rather than
// PostgreSQL's PGXXX env-var names, so the Secret can be safely projected with
// Kubernetes envFrom (EnvFromSource silently drops keys containing a period).
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
	// "verify-full"). Optional in the Secret for parse/Libpq/JDBC; when absent,
	// callers apply their own default ("disable" for Internal, "require" for
	// External) before TLSConfig or framework Connect, which require non-empty sslmode.
	SecretKeySSLMode = "sslmode"

	// DefaultPort is the standard PostgreSQL port, used when port is absent.
	DefaultPort = 5432
	// DefaultAdminDatabase is the default maintenance database name.
	DefaultAdminDatabase = "postgres"

	// SSLModeDisable disables TLS.
	SSLModeDisable = "disable"
	// SSLModeRequire encrypts without verifying the server certificate.
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
