package tls_test

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgtls "github.com/opendatahub-io/odh-platform-utilities/framework/tls"
)

// Cipher suite names used across the TLS tests, spelled as OpenShift profile
// data carries them: OpenSSL names for TLS <= 1.2 suites and IANA names for
// TLS 1.3 suites and library-go's mapping output. No library exports these
// strings as constants (openshift/api holds them as TLSProfiles data and
// library-go as map keys), so they are defined once here.
const (
	// OpenSSL-form cipher names, as they appear in
	// APIServer tlsSecurityProfile custom cipher lists.
	openSSLECDHERSAAES128GCM    = "ECDHE-RSA-AES128-GCM-SHA256"
	openSSLECDHERSAAES256SHA384 = "ECDHE-RSA-AES256-SHA384"
	openSSLECDHEECDSAAES256SHA  = "ECDHE-ECDSA-AES256-SHA384"
	openSSLECDHERSAAES256GCM    = "ECDHE-RSA-AES256-GCM-SHA384"
	openSSLDHERSAAES128GCM      = "DHE-RSA-AES128-GCM-SHA256"
	openSSLAES256SHA256         = "AES256-SHA256"

	// IANA-form cipher names: the TLS 1.3 suite names and library-go's
	// OpenSSLToIANACipherSuites output.
	ianaTLS13AES128GCM    = "TLS_AES_128_GCM_SHA256"
	ianaTLS13AES256GCM    = "TLS_AES_256_GCM_SHA384"
	ianaTLS13CHACHA20     = "TLS_CHACHA20_POLY1305_SHA256"
	ianaECDHERSAAES128GCM = "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
	ianaECDHERSACHACHA20  = "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256"
	ianaRSA3DES           = "TLS_RSA_WITH_3DES_EDE_CBC_SHA"
)

// intermediateIANACiphers is the expected comma-joined IANA cipher string for
// the Intermediate TLS profile. Derived from openshift/api
// TLSProfileIntermediateType through library-go OpenSSL→IANA mapping, not from
// CipherSuitesFromSpec, so it is an independent oracle.
const intermediateIANACiphers = "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256," +
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256," +
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384," +
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384," +
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256," +
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256"

const tls13IANACiphers = "TLS_AES_128_GCM_SHA256," +
	"TLS_AES_256_GCM_SHA384," +
	"TLS_CHACHA20_POLY1305_SHA256"

const oldIANACiphers = "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256," +
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256," +
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384," +
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384," +
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256," +
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256," +
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256," +
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256," +
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA," +
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA," +
	"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA," +
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA," +
	"TLS_RSA_WITH_AES_128_GCM_SHA256," +
	"TLS_RSA_WITH_AES_256_GCM_SHA384," +
	"TLS_RSA_WITH_AES_128_CBC_SHA256," +
	"TLS_RSA_WITH_AES_128_CBC_SHA," +
	"TLS_RSA_WITH_AES_256_CBC_SHA," +
	"TLS_RSA_WITH_3DES_EDE_CBC_SHA"

func TestMinVersionFromSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected string
		version  configv1.TLSProtocolVersion
		format   pkgtls.VersionFormat
	}{
		{name: "TLS 1.2 short", version: configv1.VersionTLS12, format: pkgtls.FormatShort, expected: "TLS1.2"},
		{name: "TLS 1.3 short", version: configv1.VersionTLS13, format: pkgtls.FormatShort, expected: "TLS1.3"},
		{name: "TLS 1.2 Go", version: configv1.VersionTLS12, format: pkgtls.FormatGo, expected: "VersionTLS12"},
		{name: "TLS 1.3 Go", version: configv1.VersionTLS13, format: pkgtls.FormatGo, expected: "VersionTLS13"},
		{name: "TLS 1.0 short", version: configv1.VersionTLS10, format: pkgtls.FormatShort, expected: "TLS1.0"},
		{name: "TLS 1.0 Go", version: configv1.VersionTLS10, format: pkgtls.FormatGo, expected: "VersionTLS10"},
		{name: "TLS 1.1 short", version: configv1.VersionTLS11, format: pkgtls.FormatShort, expected: "TLS1.1"},
		{name: "TLS 1.1 Go", version: configv1.VersionTLS11, format: pkgtls.FormatGo, expected: "VersionTLS11"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := &configv1.TLSProfileSpec{MinTLSVersion: tt.version}
			assert.Equal(t, tt.expected, pkgtls.MinVersionFromSpec(context.Background(), spec, tt.format))
		})
	}
}

func TestMinVersionFromSpec_NilSpec(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "TLS1.2", pkgtls.MinVersionFromSpec(context.Background(), nil, pkgtls.FormatShort))
	assert.Equal(t, "VersionTLS12", pkgtls.MinVersionFromSpec(context.Background(), nil, pkgtls.FormatGo))
}

func TestFromProfile_Nil(t *testing.T) {
	t.Parallel()

	minVersion, cipherSuites := pkgtls.FromProfile(context.Background(), nil, pkgtls.FormatShort)
	assert.Equal(t, "TLS1.2", minVersion)
	assert.Equal(t, intermediateIANACiphers, cipherSuites)

	minVersion, cipherSuites = pkgtls.FromProfile(context.Background(), nil, pkgtls.FormatGo)
	assert.Equal(t, "VersionTLS12", minVersion)
	assert.Equal(t, intermediateIANACiphers, cipherSuites)
}

func TestFromProfile_Old(t *testing.T) {
	t.Parallel()

	minVersion, cipherSuites := pkgtls.FromProfile(
		context.Background(),
		&configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType},
		pkgtls.FormatShort,
	)
	assert.Equal(t, "TLS1.2", minVersion)
	assert.Equal(t, intermediateIANACiphers, cipherSuites)
}

func TestFromProfile_Modern(t *testing.T) {
	t.Parallel()

	minVersion, _ := pkgtls.FromProfile(
		context.Background(),
		&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		pkgtls.FormatShort,
	)
	assert.Equal(t, "TLS1.3", minVersion)

	minVersion, _ = pkgtls.FromProfile(
		context.Background(),
		&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		pkgtls.FormatGo,
	)
	assert.Equal(t, "VersionTLS13", minVersion)
}

func TestFromProfile_Custom(t *testing.T) {
	t.Parallel()

	minVersion, cipherSuites := pkgtls.FromProfile(context.Background(), &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{
			TLSProfileSpec: configv1.TLSProfileSpec{
				Ciphers:       []string{openSSLECDHERSAAES128GCM},
				MinTLSVersion: configv1.VersionTLS12,
			},
		},
	}, pkgtls.FormatShort)
	assert.Equal(t, "TLS1.2", minVersion)
	assert.Equal(t, ianaECDHERSAAES128GCM, cipherSuites)
}

func TestIsVersionSupported(t *testing.T) {
	t.Parallel()

	assert.True(t, pkgtls.IsVersionSupported(configv1.VersionTLS12))
	assert.True(t, pkgtls.IsVersionSupported(configv1.VersionTLS13))
	assert.False(t, pkgtls.IsVersionSupported(configv1.VersionTLS10))
	assert.False(t, pkgtls.IsVersionSupported(configv1.VersionTLS11))
}

func TestProfileSpecFromSecurityProfile(t *testing.T) { //nolint:funlen // Table-driven test with many cases.
	t.Parallel()

	intermediateSpec := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	oldSpec := configv1.TLSProfiles[configv1.TLSProfileOldType]
	modernSpec := configv1.TLSProfiles[configv1.TLSProfileModernType]
	customCiphers := []string{openSSLECDHERSAAES128GCM}
	customSpec := &configv1.TLSProfileSpec{
		Ciphers:       customCiphers,
		MinTLSVersion: configv1.VersionTLS11,
	}

	tests := []struct { //nolint:govet // fieldalignment of table cases is not worth a named type
		name     string
		profile  *configv1.TLSSecurityProfile
		expected *configv1.TLSProfileSpec
	}{
		{name: "nil profile defaults to intermediate", profile: nil, expected: intermediateSpec},
		{
			name:     "intermediate type",
			profile:  &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType},
			expected: intermediateSpec,
		},
		{
			name:     "old type",
			profile:  &configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType},
			expected: oldSpec,
		},
		{
			name:     "modern type",
			profile:  &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
			expected: modernSpec,
		},
		{
			name: "custom type with spec",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: *customSpec,
				},
			},
			expected: customSpec,
		},
		{
			name:     "custom type without spec falls back to intermediate",
			profile:  &configv1.TLSSecurityProfile{Type: configv1.TLSProfileCustomType},
			expected: intermediateSpec,
		},
		{
			name:     "unknown type falls back to intermediate",
			profile:  &configv1.TLSSecurityProfile{Type: "Unknown"},
			expected: intermediateSpec,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := pkgtls.ProfileSpecFromSecurityProfile(tt.profile)
			require.NotNil(t, got)
			assert.Equal(t, tt.expected.MinTLSVersion, got.MinTLSVersion)
			assert.Equal(t, tt.expected.Ciphers, got.Ciphers)
		})
	}
}

func TestGetStrictTLSProfileSpecRejectsUnknownType(t *testing.T) {
	t.Parallel()

	_, err := pkgtls.GetStrictTLSProfileSpec(&configv1.TLSSecurityProfile{Type: "Unknown"})
	assert.ErrorIs(t, err, pkgtls.ErrUnsupportedTLSProfileType)
}

func TestCipherSuitesFromSpec(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("nil spec falls back to intermediate", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, intermediateIANACiphers, pkgtls.CipherSuitesFromSpec(ctx, nil))
	})

	t.Run("profile with only unmappable DHE ciphers falls back to intermediate", func(t *testing.T) {
		t.Parallel()

		spec := &configv1.TLSProfileSpec{
			Ciphers:       []string{openSSLDHERSAAES128GCM, "DHE-RSA-AES256-GCM-SHA384"},
			MinTLSVersion: configv1.VersionTLS12,
		}
		assert.Equal(t, intermediateIANACiphers, pkgtls.CipherSuitesFromSpec(ctx, spec))
	})

	t.Run("TLS 1.3-only ciphers do not satisfy a TLS 1.2 profile", func(t *testing.T) {
		t.Parallel()

		spec := &configv1.TLSProfileSpec{
			MinTLSVersion: configv1.VersionTLS12,
			Ciphers:       []string{ianaTLS13AES128GCM},
		}
		assert.Equal(t, intermediateIANACiphers, pkgtls.CipherSuitesFromSpec(ctx, spec))
	})

	t.Run("TLS 1.3 profile retains TLS 1.3 cipher suites", func(t *testing.T) {
		t.Parallel()

		spec := configv1.TLSProfiles[configv1.TLSProfileModernType]
		assert.Equal(t, tls13IANACiphers, pkgtls.CipherSuitesFromSpec(ctx, spec))
	})

	t.Run("empty ciphers slice falls back to intermediate", func(t *testing.T) {
		t.Parallel()

		spec := &configv1.TLSProfileSpec{
			Ciphers:       []string{},
			MinTLSVersion: configv1.VersionTLS12,
		}
		assert.Equal(t, intermediateIANACiphers, pkgtls.CipherSuitesFromSpec(ctx, spec))
	})

	t.Run("profile with mixed ciphers retains only mappable ones", func(t *testing.T) {
		t.Parallel()

		spec := &configv1.TLSProfileSpec{
			Ciphers:       []string{openSSLECDHERSAAES128GCM, openSSLDHERSAAES128GCM},
			MinTLSVersion: configv1.VersionTLS12,
		}
		assert.Equal(t, ianaECDHERSAAES128GCM, pkgtls.CipherSuitesFromSpec(ctx, spec))
	})

	t.Run("intermediate profile produces expected IANA ciphers", func(t *testing.T) {
		t.Parallel()

		got := pkgtls.CipherSuitesFromSpec(ctx, configv1.TLSProfiles[configv1.TLSProfileIntermediateType])
		assert.Equal(t, intermediateIANACiphers, got)
	})

	t.Run("old profile produces expected IANA ciphers", func(t *testing.T) {
		t.Parallel()

		got := pkgtls.CipherSuitesFromSpec(ctx, configv1.TLSProfiles[configv1.TLSProfileOldType])
		assert.Equal(t, oldIANACiphers, got)
	})
}
