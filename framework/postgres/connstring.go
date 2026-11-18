package postgres

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ErrUnsupportedDriver is returned when ConnectionString is given an unknown Driver.
var ErrUnsupportedDriver = errors.New("unsupported driver")

// Driver selects the connection-string dialect returned by [Params.ConnectionString].
type Driver string

const (
	// DriverLibpq selects libpq keyword/value connection strings.
	DriverLibpq Driver = "libpq"
	// DriverJDBC selects JDBC URL connection strings.
	DriverJDBC Driver = "jdbc"
)

// ConnectionString composes a driver-specific connection string.
// For verify-ca, verify-full, or require with CA material, caFile must be a
// path from WriteCACert (or equivalent); an empty path is rejected so
// sslrootcert is not silently omitted. The returned string contains the
// password in plaintext; callers must not log or persist it. JDBC URLs also
// embed the password as a query param; pgJDBC may log that URL at FINE.
func (p Params) ConnectionString(driver Driver, caFile string) (string, error) {
	err := p.Validate()
	if err != nil {
		return "", err
	}

	if p.needsCAFile() && caFile == "" {
		return "", fmt.Errorf("%s %q requires a CA file path (use WriteCACert): %w",
			SecretKeySSLMode, p.effectiveSSLMode(), ErrCARequired)
	}

	switch driver {
	case DriverLibpq:
		return p.libpq(caFile), nil
	case DriverJDBC:
		return p.jdbc(caFile), nil
	default:
		return "", fmt.Errorf("%w %q", ErrUnsupportedDriver, driver)
	}
}

// Libpq is shorthand for ConnectionString(DriverLibpq, caFile).
// See ConnectionString for caFile rules. The returned string contains the
// password in plaintext; callers must not log or persist it.
func (p Params) Libpq(caFile string) (string, error) {
	return p.ConnectionString(DriverLibpq, caFile)
}

// JDBC is shorthand for ConnectionString(DriverJDBC, caFile).
// See ConnectionString for caFile rules. The URL includes password as a query
// param; avoid FINE logging of org.postgresql (pgJDBC logs the connect URL).
func (p Params) JDBC(caFile string) (string, error) {
	return p.ConnectionString(DriverJDBC, caFile)
}

func (p Params) libpq(caFile string) string {
	parts := []string{
		kw(SecretKeyHost, p.Host),
		kw(SecretKeyPort, strconv.Itoa(p.Port)),
		kw(SecretKeyUser, p.User),
		kw(SecretKeyPassword, p.Password),
		kw(SecretKeyDatabase, p.DBName),
	}

	mode := p.effectiveSSLMode()
	if mode != "" {
		parts = append(parts, kw(SecretKeySSLMode, mode))
		// Keep TLS + claim CA in force: with GSSAPI available, libpq may skip
		// certificate checks even when sslmode asks for verify-*. Omitted
		// sslmode is left unchanged (libpq ≥ 12 for gssencmode).
		if mode != SSLModeDisable {
			parts = append(parts, kw("gssencmode", "disable"))
		}
	}

	if caFile != "" && p.needsCAFile() {
		parts = append(parts, kw("sslrootcert", caFile))
	}

	if p.Schema != "" {
		// No space after -c so the options value stays a single libpq token.
		parts = append(parts, kw("options", "-csearch_path="+p.Schema))
	}

	return strings.Join(parts, " ")
}

func (p Params) jdbc(caFile string) string {
	// PathEscape leaves '+' intact; pgJDBC form-decodes '+' as space, so encode as %2B.
	dbPath := strings.ReplaceAll(url.PathEscape(p.DBName), "+", "%2B")
	base := fmt.Sprintf("jdbc:postgresql://%s/%s",
		net.JoinHostPort(p.Host, strconv.Itoa(p.Port)), dbPath)
	q := url.Values{}
	q.Set("user", p.User)
	q.Set("password", p.Password)

	mode := p.effectiveSSLMode()
	if mode != "" {
		q.Set(SecretKeySSLMode, mode)
	}

	if caFile != "" && p.needsCAFile() {
		q.Set("sslrootcert", caFile)
	}

	if p.Schema != "" {
		q.Set("currentSchema", p.Schema)
	}

	return base + "?" + q.Encode()
}

// effectiveSSLMode returns the sslmode emitted for drivers / TLSConfig.
// require with a CA present is promoted to verify-ca so Go, libpq, and JDBC
// share the same trust posture (encrypt + verify chain).
func (p Params) effectiveSSLMode() string {
	if p.SSLMode == SSLModeRequire && p.CACert != "" {
		return SSLModeVerifyCA
	}

	return p.SSLMode
}

// needsCAFile reports whether a CA file should be written / attached for the
// current Params. verify-ca and verify-full always need it; require needs it
// only when CA material is present (same promotion as effectiveSSLMode).
func (p Params) needsCAFile() bool {
	switch p.effectiveSSLMode() {
	case SSLModeVerifyCA, SSLModeVerifyFull:
		return true
	default:
		return false
	}
}

// kw formats a libpq keyword/value pair with single-quote escaping,
// matching the e2e hand-compose escape_conninfo behavior.
func kw(key, value string) string {
	return key + "='" + escapeLibpq(value) + "'"
}

func escapeLibpq(v string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v)
}
