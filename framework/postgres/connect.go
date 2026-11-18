// Package postgres parses claim Secrets and opens pgx connections.
package postgres

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// Connect opens a pgx connection using claim-Secret Params. TLS trust is applied
// via Params.TLSConfig and schema via search_path. SSLMode must be set
// explicitly; empty sslmode is rejected so pgx cannot fall back to prefer.
func Connect(ctx context.Context, p Params) (*pgx.Conn, error) {
	err := p.Validate()
	if err != nil {
		return nil, err
	}

	if p.SSLMode == "" {
		return nil, fmt.Errorf("missing or empty key %q in Secret: %w",
			SecretKeySSLMode, ErrSSLModeRequired)
	}

	cfg, err := pgx.ParseConfig(pgxDSN(p))
	if err != nil {
		return nil, fmt.Errorf("parse connection config: %w", err)
	}

	cfg.Password = p.Password

	if p.TLSEnabled() {
		tlsConfig, tlsErr := p.TLSConfig()
		if tlsErr != nil {
			return nil, tlsErr
		}

		// Keep ALPN from ParseConfig (e.g. sslnegotiation=direct needs "postgresql").
		if cfg.TLSConfig != nil && len(cfg.TLSConfig.NextProtos) > 0 {
			tlsConfig.NextProtos = append([]string(nil), cfg.TLSConfig.NextProtos...)
		}

		cfg.TLSConfig = tlsConfig
	}

	if p.Schema != "" {
		if cfg.RuntimeParams == nil {
			cfg.RuntimeParams = map[string]string{}
		}

		cfg.RuntimeParams["search_path"] = p.Schema
	}

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	return conn, nil
}

func pgxDSN(p Params) string {
	q := url.Values{}
	q.Set(SecretKeySSLMode, p.SSLMode)
	// Override PGSSLROOTCERT so a missing ambient file cannot fail ParseConfig
	// before Params.TLSConfig applies the claim Secret CA.
	q.Set("sslrootcert", "")
	// Claim Secrets do not supply a client identity; ignore ambient PGSSLCERT /
	// PGSSLKEY so a missing client cert cannot block ParseConfig.
	q.Set("sslcertmode", "disable")

	u := url.URL{
		Scheme: "postgres",
		User:   url.User(p.User),
		Host:   net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
		// Leading slash keeps the path separator distinct from DBName (e.g. "/reports").
		Path:     "/" + p.DBName,
		RawQuery: q.Encode(),
	}

	return u.String()
}
