package tls_test

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgtls "github.com/opendatahub-io/odh-platform-utilities/pkg/tls"
)

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
		{name: "TLS 1.0 floors to 1.2 short", version: configv1.VersionTLS10, format: pkgtls.FormatShort, expected: "TLS1.2"},
		{name: "TLS 1.0 floors to 1.2 Go", version: configv1.VersionTLS10, format: pkgtls.FormatGo, expected: "VersionTLS12"},
		{name: "TLS 1.1 floors to 1.2 short", version: configv1.VersionTLS11, format: pkgtls.FormatShort, expected: "TLS1.2"},
		{name: "TLS 1.1 floors to 1.2 Go", version: configv1.VersionTLS11, format: pkgtls.FormatGo, expected: "VersionTLS12"},
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
				Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
				MinTLSVersion: configv1.VersionTLS12,
			},
		},
	}, pkgtls.FormatShort)
	assert.Equal(t, "TLS1.2", minVersion)
	assert.Equal(t, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", cipherSuites)
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
	customCiphers := []string{"ECDHE-RSA-AES128-GCM-SHA256"}
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
			Ciphers:       []string{"DHE-RSA-AES128-GCM-SHA256", "DHE-RSA-AES256-GCM-SHA384"},
			MinTLSVersion: configv1.VersionTLS12,
		}
		assert.Equal(t, intermediateIANACiphers, pkgtls.CipherSuitesFromSpec(ctx, spec))
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
			Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "DHE-RSA-AES128-GCM-SHA256"},
			MinTLSVersion: configv1.VersionTLS12,
		}
		assert.Equal(t, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", pkgtls.CipherSuitesFromSpec(ctx, spec))
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

// intermediateIANACiphers is the expected comma-joined IANA cipher string for
// the Intermediate TLS profile. Derived from openshift/api
// TLSProfileIntermediateType through library-go OpenSSL→IANA mapping, not from
// CipherSuitesFromSpec, so it is an independent oracle.
const intermediateIANACiphers = "TLS_AES_128_GCM_SHA256," +
	"TLS_AES_256_GCM_SHA384," +
	"TLS_CHACHA20_POLY1305_SHA256," +
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256," +
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256," +
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384," +
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384," +
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256," +
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256"

const oldIANACiphers = "TLS_AES_128_GCM_SHA256," +
	"TLS_AES_256_GCM_SHA384," +
	"TLS_CHACHA20_POLY1305_SHA256," +
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256," +
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
