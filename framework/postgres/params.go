package postgres

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

var (
	// ErrNilSecretData is returned when ParseSecretData is given a nil data map.
	ErrNilSecretData = errors.New("secret data is nil")
	// ErrNilSecret is returned when ParseClaimSecret is given a nil Secret.
	ErrNilSecret = errors.New("secret is nil")
	// ErrMissingKey is returned when a required Secret key is absent or empty.
	ErrMissingKey = errors.New("missing or empty key in Secret")
	// ErrInvalidPort is returned when port is not in 1-65535.
	ErrInvalidPort = errors.New("invalid port")
	// ErrUnsupportedSSLMode is returned when sslmode is not a supported libpq value.
	ErrUnsupportedSSLMode = errors.New("unsupported sslmode")
	// ErrCARequired is returned when verify-ca or verify-full is set without ca.crt.
	ErrCARequired = errors.New("sslmode requires ca.crt")
	// ErrSSLModeRequired is returned by TLSConfig (and Connect) when
	// sslmode is empty. Parse/Libpq/JDBC still allow empty sslmode so callers
	// can apply their own default; opening a live connection must not inherit
	// pgx's prefer behavior.
	ErrSSLModeRequired = errors.New("sslmode is required")
	// ErrInvalidConnectionType is returned when the connection-type annotation is present
	// but is not postgres.
	ErrInvalidConnectionType = errors.New("invalid connection-type annotation")
	// ErrInvalidSchema is returned when schema is not a single unquoted identifier.
	ErrInvalidSchema = errors.New("invalid schema")
	// ErrInvalidHost is returned when host is not a single host name or address
	// (e.g. multi-host lists, host:port, or URL delimiters that would break
	// JDBC authority / JoinHostPort).
	ErrInvalidHost = errors.New("invalid host")
	// ErrInvalidValue is returned when a required field contains a NUL byte that
	// would truncate a libpq connection string.
	ErrInvalidValue = errors.New("invalid value")

	// schemaPattern restricts Schema to one PostgreSQL-style unquoted identifier
	// so it cannot inject libpq options (-c…) or widen search_path via commas.
	schemaPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]{0,62}$`)
)

// Params holds the discrete claim-Secret connection fields.
type Params struct {
	Host     string
	User     string
	Password string
	DBName   string
	// Schema is optional; set for SchemaClaim Secrets.
	Schema string
	// SSLMode is optional on administrator-authored Secrets; claim Secrets
	// written by the db-operator always include it.
	SSLMode string
	// CACert is optional PEM content from ca.crt.
	CACert string
	// Port is the database TCP port. ParseSecretData defaults this to DefaultPort
	// when the Secret omits the port key.
	Port int
}

// ParseSecretData decodes a claim Secret's data map into Params and validates
// required fields. Port defaults to DefaultPort when absent.
func ParseSecretData(data map[string][]byte) (Params, error) {
	p := Params{Port: DefaultPort}
	if data == nil {
		return p, ErrNilSecretData
	}

	p.Host = string(data[SecretKeyHost])
	p.User = string(data[SecretKeyUser])
	p.Password = string(data[SecretKeyPassword])
	p.DBName = string(data[SecretKeyDatabase])
	p.Schema = string(data[SecretKeySchema])
	p.SSLMode = string(data[SecretKeySSLMode])
	p.CACert = string(data[SecretKeyCA])

	if raw, ok := data[SecretKeyPort]; ok && len(raw) > 0 {
		port, err := strconv.Atoi(string(raw))
		if err != nil {
			return p, fmt.Errorf("invalid %s: %w: %w", SecretKeyPort, err, ErrInvalidPort)
		}

		p.Port = port
	}

	err := p.Validate()
	if err != nil {
		return p, err
	}

	return p, nil
}

// ParseClaimSecret decodes a Kubernetes Secret into Params. When the
// connection-type annotation is present, it must equal
// ConnectionTypeProtocolPostgres. Absent annotation is allowed so
// administrator-authored Connection API Secrets still parse. The
// operator-internal credential-stamp annotation is ignored.
func ParseClaimSecret(secret *corev1.Secret) (Params, error) {
	if secret == nil {
		return Params{}, ErrNilSecret
	}

	if v, ok := secret.Annotations[AnnotationConnectionTypeProtocol]; ok && v != ConnectionTypeProtocolPostgres {
		return Params{}, fmt.Errorf("annotation %q must be %q, got %q: %w",
			AnnotationConnectionTypeProtocol, ConnectionTypeProtocolPostgres, v, ErrInvalidConnectionType)
	}

	return ParseSecretData(secret.Data)
}

// Validate checks required fields, port range, sslmode, schema identifier shape,
// and TLS trust readiness for verify modes. Empty sslmode is allowed here so
// administrator-authored Secrets can omit it; callers must set an explicit
// default before TLSConfig or Connect. Empty schema is allowed.
// Errors name Secret keys and never include the password value.
func (p Params) Validate() error {
	for _, field := range [...]struct {
		key string
		val string
	}{
		{SecretKeyHost, p.Host},
		{SecretKeyUser, p.User},
		{SecretKeyPassword, p.Password},
		{SecretKeyDatabase, p.DBName},
	} {
		if field.val == "" {
			return fmt.Errorf("missing or empty key %q in Secret: %w", field.key, ErrMissingKey)
		}
		// Libpq treats values as C strings; a NUL truncates before the closing quote.
		if strings.ContainsRune(field.val, 0) {
			return fmt.Errorf("invalid %q in Secret: contains a NUL byte: %w", field.key, ErrInvalidValue)
		}
	}

	err := validateHost(p)
	if err != nil {
		return err
	}

	if p.Port <= 0 || p.Port > 65535 {
		return fmt.Errorf("invalid %s: must be a port number 1-65535, got %d: %w", SecretKeyPort, p.Port, ErrInvalidPort)
	}

	err = validateSSLMode(p)
	if err != nil {
		return err
	}

	return validateSchema(p)
}

func validateHost(p Params) error {
	// Claim Secrets are single-host; reject multi-host lists, brackets, and URL
	// delimiters so Connect cannot fall back to a host without claim-CA
	// TLSConfig, JDBC cannot treat path/query fragments as params, and pgx
	// cannot strip [hostname] brackets and dial the inner name. Bare IPv6
	// (e.g. ::1) stays unbracketed; JoinHostPort adds brackets when needed.
	if strings.ContainsAny(p.Host, ",/?#@&[]") {
		return fmt.Errorf("invalid %s: must be a single host without URL delimiters: %w",
			SecretKeyHost, ErrInvalidHost)
	}

	// Port is a separate Secret key. Reject host:port so JoinHostPort cannot
	// emit [host:port]:Port. Bare IPv6 literals (e.g. ::1) remain valid.
	if strings.Contains(p.Host, ":") && net.ParseIP(p.Host) == nil {
		return fmt.Errorf("invalid %s: must be a single host without an embedded port: %w",
			SecretKeyHost, ErrInvalidHost)
	}

	return nil
}

func validateSSLMode(p Params) error {
	switch p.SSLMode {
	case "", SSLModeDisable, SSLModeRequire, SSLModeVerifyCA, SSLModeVerifyFull:
	default:
		return fmt.Errorf("unsupported %s value %q: %w", SecretKeySSLMode, p.SSLMode, ErrUnsupportedSSLMode)
	}

	if (p.SSLMode == SSLModeVerifyCA || p.SSLMode == SSLModeVerifyFull) && p.CACert == "" {
		return fmt.Errorf("%s %q requires key %q: %w", SecretKeySSLMode, p.SSLMode, SecretKeyCA, ErrCARequired)
	}

	return nil
}

func validateSchema(p Params) error {
	if p.Schema != "" && !schemaPattern.MatchString(p.Schema) {
		return fmt.Errorf("invalid %s: must be a single identifier: %w", SecretKeySchema, ErrInvalidSchema)
	}

	return nil
}

// TLSEnabled reports whether sslmode requests TLS.
func (p Params) TLSEnabled() bool {
	return p.SSLMode != "" && p.SSLMode != SSLModeDisable
}

// TLSReady reports whether the config includes the trust material required by
// the selected sslmode (same rules as db-operator postgres.Config.TLSReady).
func (p Params) TLSReady() bool {
	switch p.SSLMode {
	case SSLModeVerifyCA, SSLModeVerifyFull:
		return p.CACert != ""
	case "", SSLModeDisable:
		return false
	default:
		return true
	}
}

// requireSSLMode fails closed when sslmode is absent. Used by TLSConfig so
// callers never inherit libpq prefer (TLS-then-plaintext).
func (p Params) requireSSLMode() error {
	if p.SSLMode == "" {
		return fmt.Errorf("missing or empty key %q in Secret: %w", SecretKeySSLMode, ErrSSLModeRequired)
	}

	return nil
}
